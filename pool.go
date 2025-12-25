package pond

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/qntx/pond/internal/buffer"
	"github.com/qntx/pond/internal/future"
)

const (
	Unbounded               = math.MaxInt // Unbounded queue size
	DefaultQueueSize        = Unbounded
	DefaultNonBlocking      = false
	LinkedBufferInitialSize = 1024
	LinkedBufferMaxCapacity = 100 * 1024
)

var (
	ErrQueueFull             = errors.New("queue is full")
	ErrQueueEmpty            = errors.New("queue is empty")
	ErrPoolStopped           = errors.New("pool stopped")
	ErrMaxConcurrencyReached = errors.New("max concurrency reached")

	poolStoppedFuture = func() Task {
		future, resolve := future.NewFuture(context.Background())
		resolve(ErrPoolStopped)
		return future
	}()
)

// basePool is the base interface for all pool types.
type basePool interface {
	RunningWorkers() int64   // Number of active workers
	SubmittedTasks() uint64  // Total tasks submitted
	WaitingTasks() uint64    // Tasks waiting in queue
	FailedTasks() uint64     // Tasks completed with error
	SuccessfulTasks() uint64 // Tasks completed successfully
	CompletedTasks() uint64  // Total completed tasks
	DroppedTasks() uint64    // Tasks dropped (queue full)
	MaxConcurrency() int     // Maximum concurrent workers
	QueueSize() int          // Task queue size
	NonBlocking() bool       // True if pool doesn't block on full queue
	Context() context.Context
	Stop() Task    // Stop and return future for completion
	StopAndWait()  // Stop and wait for all tasks
	Stopped() bool // True if stopped or context cancelled
	Resize(maxConcurrency int)
}

// Pool represents a pool of goroutines that can execute tasks concurrently.
type Pool interface {
	basePool
	// Go submits a task without waiting. Returns ErrPoolStopped if stopped.
	Go(task func()) error
	// Submit submits a task and returns a future.
	Submit(task func()) Task
	// SubmitErr submits a task that returns an error.
	SubmitErr(task func() error) Task
	// TrySubmit attempts non-blocking submit. Returns false if queue full.
	TrySubmit(task func()) (Task, bool)
	// TrySubmitErr attempts non-blocking submit for error-returning task.
	TrySubmitErr(task func() error) (Task, bool)
	// NewSubpool creates a child pool with specified concurrency.
	NewSubpool(maxConcurrency int, options ...Option) Pool
	// NewGroup creates a task group.
	NewGroup() TaskGroup
	// NewGroupContext creates a task group with specified context.
	NewGroupContext(ctx context.Context) TaskGroup
}

// Option configures a pool.
type Option func(*pool)

// WithContext sets the context for the pool.
func WithContext(ctx context.Context) Option {
	return func(p *pool) { p.ctx = ctx }
}

// WithQueueSize sets the max queue size.
func WithQueueSize(size int) Option {
	return func(p *pool) { p.queueSize = size }
}

// WithNonBlocking makes the pool non-blocking when queue is full.
func WithNonBlocking(nonBlocking bool) Option {
	return func(p *pool) { p.nonBlocking = nonBlocking }
}

// WithoutPanicRecovery disables panic recovery in workers.
func WithoutPanicRecovery() Option {
	return func(p *pool) { p.panicRecovery = false }
}

type pool struct {
	mutex               sync.Mutex
	parent              *pool
	ctx                 context.Context
	cancel              context.CancelCauseFunc
	nonBlocking         bool
	panicRecovery       bool
	maxConcurrency      int
	closed              atomic.Bool
	workerCount         atomic.Int64
	workerWaitGroup     sync.WaitGroup
	submitWaiters       chan struct{}
	queueSize           int
	tasks               *buffer.LinkedBuffer[any]
	submittedTaskCount  atomic.Uint64
	successfulTaskCount atomic.Uint64
	failedTaskCount     atomic.Uint64
	droppedTaskCount    atomic.Uint64
}

func (p *pool) Context() context.Context { return p.ctx }
func (p *pool) Stopped() bool            { return p.closed.Load() || p.ctx.Err() != nil }
func (p *pool) QueueSize() int           { return p.queueSize }
func (p *pool) NonBlocking() bool        { return p.nonBlocking }
func (p *pool) RunningWorkers() int64    { return p.workerCount.Load() }
func (p *pool) SubmittedTasks() uint64   { return p.submittedTaskCount.Load() }
func (p *pool) WaitingTasks() uint64     { return p.tasks.Len() }
func (p *pool) FailedTasks() uint64      { return p.failedTaskCount.Load() }
func (p *pool) SuccessfulTasks() uint64  { return p.successfulTaskCount.Load() }
func (p *pool) CompletedTasks() uint64 {
	return p.successfulTaskCount.Load() + p.failedTaskCount.Load()
}
func (p *pool) DroppedTasks() uint64 { return p.droppedTaskCount.Load() }

func (p *pool) MaxConcurrency() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.maxConcurrency
}

func (p *pool) Resize(maxConcurrency int) {
	if maxConcurrency == 0 {
		maxConcurrency = math.MaxInt
	}
	if maxConcurrency < 0 {
		panic(errors.New("maxConcurrency must be greater than or equal to 0"))
	}

	p.mutex.Lock()
	newWorkers := min(maxConcurrency-p.maxConcurrency, int(p.tasks.Len()))
	p.maxConcurrency = maxConcurrency
	if newWorkers > 0 {
		p.workerCount.Add(int64(newWorkers))
		p.workerWaitGroup.Add(newWorkers)
	}
	p.mutex.Unlock()

	for range newWorkers {
		p.launchWorker(nil)
	}
}

func (p *pool) worker(task any) {
	for {
		if task != nil {
			_, err := invokeTask[any](task, p.panicRecovery)
			p.updateMetrics(err)
		}
		var err error
		if task, err = p.readTask(); err != nil {
			return
		}
	}
}

func (p *pool) subpoolWorker(task any) func() (any, error) {
	return func() (any, error) {
		var out any
		var err error
		if task != nil {
			out, err = invokeTask[any](task, p.panicRecovery)
			p.updateMetrics(err)
		}
		if next, readErr := p.readTask(); readErr == nil {
			p.parent.submit(p.subpoolWorker(next), p.nonBlocking)
		}
		return out, err
	}
}

func (p *pool) Go(task func()) error    { return p.submit(task, p.nonBlocking) }
func (p *pool) Submit(task func()) Task { f, _ := p.wrapAndSubmit(task, p.nonBlocking); return f }
func (p *pool) SubmitErr(task func() error) Task {
	f, _ := p.wrapAndSubmit(task, p.nonBlocking)
	return f
}
func (p *pool) TrySubmit(task func()) (Task, bool)          { return p.wrapAndSubmit(task, true) }
func (p *pool) TrySubmitErr(task func() error) (Task, bool) { return p.wrapAndSubmit(task, true) }

func (p *pool) wrapAndSubmit(task any, nonBlocking bool) (Task, bool) {
	if p.Stopped() {
		return poolStoppedFuture, false
	}
	f, resolve := future.NewFuture(p.ctx)
	wrapped := wrapTask[struct{}, func(error)](task, resolve, p.panicRecovery)
	if err := p.submit(wrapped, nonBlocking); err != nil {
		resolve(err)
		return f, false
	}
	return f, true
}

func (p *pool) submit(task any, nonBlocking bool) error {
	p.submittedTaskCount.Add(1)
	var err error
	if nonBlocking {
		err = p.trySubmit(task)
	} else {
		err = p.blockingTrySubmit(task)
	}
	if err != nil {
		p.droppedTaskCount.Add(1)
	}
	return err
}

func (p *pool) blockingTrySubmit(task any) error {
	for {
		if err := p.trySubmit(task); err != ErrQueueFull {
			return err
		}
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		case <-p.submitWaiters:
			if p.ctx.Err() != nil {
				return p.ctx.Err()
			}
		}
	}
}

func (p *pool) trySubmit(task any) error {
	p.mutex.Lock()
	if p.Stopped() {
		p.mutex.Unlock()
		return ErrPoolStopped
	}

	queueEnabled := p.queueSize > 0
	tasksLen := int(p.tasks.Len())

	if queueEnabled && tasksLen >= p.queueSize {
		p.mutex.Unlock()
		return ErrQueueFull
	}

	if int(p.workerCount.Load()) >= p.maxConcurrency {
		if !queueEnabled {
			p.mutex.Unlock()
			return ErrQueueFull
		}
		p.tasks.Write(task)
		p.mutex.Unlock()
		return nil
	}

	p.workerCount.Add(1)
	p.workerWaitGroup.Add(1)

	if queueEnabled && tasksLen > 0 {
		p.tasks.Write(task)
		task, _ = p.tasks.Read()
	}
	p.mutex.Unlock()

	p.launchWorker(task)
	p.notifySubmitWaiter()
	return nil
}

func (p *pool) launchWorker(task any) {
	if p.parent == nil {
		go p.worker(task)
	} else {
		p.parent.submit(p.subpoolWorker(task), p.nonBlocking)
	}
}

func (p *pool) readTask() (any, error) {
	p.mutex.Lock()

	select {
	case <-p.ctx.Done():
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
		p.mutex.Unlock()
		return nil, p.ctx.Err()
	default:
	}

	if p.tasks.Len() == 0 {
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
		p.mutex.Unlock()
		p.notifySubmitWaiter()
		return nil, ErrQueueEmpty
	}

	if p.maxConcurrency > 0 && int(p.workerCount.Load()) > p.maxConcurrency {
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
		p.mutex.Unlock()
		return nil, ErrMaxConcurrencyReached
	}

	task, _ := p.tasks.Read()
	p.mutex.Unlock()
	p.notifySubmitWaiter()
	return task, nil
}

func (p *pool) notifySubmitWaiter() {
	select {
	case p.submitWaiters <- struct{}{}:
	default:
	}
}

func (p *pool) updateMetrics(err error) {
	if err != nil {
		p.failedTaskCount.Add(1)
	} else {
		p.successfulTaskCount.Add(1)
	}
}

func (p *pool) Stop() Task {
	return Submit(func() {
		p.mutex.Lock()
		p.closed.Store(true)
		p.mutex.Unlock()
		p.workerWaitGroup.Wait()
		p.cancel(ErrPoolStopped)
	})
}

func (p *pool) StopAndWait() { p.Stop().Wait() }

func (p *pool) NewSubpool(maxConcurrency int, options ...Option) Pool {
	return newPool(maxConcurrency, p, options...)
}
func (p *pool) NewGroup() TaskGroup                           { return newTaskGroup(p, p.ctx) }
func (p *pool) NewGroupContext(ctx context.Context) TaskGroup { return newTaskGroup(p, ctx) }

func newPool(maxConcurrency int, parent *pool, options ...Option) *pool {
	if parent != nil {
		if maxConcurrency > parent.MaxConcurrency() {
			panic(fmt.Errorf("maxConcurrency cannot be greater than the parent pool's maxConcurrency (%d)", parent.MaxConcurrency()))
		}
		if maxConcurrency == 0 {
			maxConcurrency = parent.MaxConcurrency()
		}
	}
	if maxConcurrency == 0 {
		maxConcurrency = math.MaxInt
	}
	if maxConcurrency < 0 {
		panic(errors.New("maxConcurrency must be greater than or equal to 0"))
	}

	p := &pool{
		ctx:            context.Background(),
		nonBlocking:    DefaultNonBlocking,
		panicRecovery:  true,
		maxConcurrency: maxConcurrency,
		queueSize:      DefaultQueueSize,
		submitWaiters:  make(chan struct{}, 1), // buffer 1 to prevent deadlock
	}

	if parent != nil {
		p.parent = parent
		p.ctx = parent.ctx
		p.queueSize = parent.queueSize
		p.nonBlocking = parent.nonBlocking
		p.panicRecovery = parent.panicRecovery
	}

	for _, opt := range options {
		opt(p)
	}

	p.ctx, p.cancel = context.WithCancelCause(p.ctx)
	p.tasks = buffer.NewLinkedBuffer[any](LinkedBufferInitialSize, LinkedBufferMaxCapacity)
	return p
}

// NewPool creates a pool with the given max concurrency (0 = unlimited).
func NewPool(maxConcurrency int, options ...Option) Pool {
	return newPool(maxConcurrency, nil, options...)
}
