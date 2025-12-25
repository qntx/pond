package pond

import (
	"context"

	"github.com/qntx/pond/internal/future"
)

// ResultTask represents a task that yields a result. If the task fails, the error can be retrieved.
type ResultTask[R any] interface {
	// Done returns a channel that is closed when the task is complete or has failed.
	Done() <-chan struct{}
	// Wait waits for the task to complete and returns the result and any error that occurred.
	Wait() (R, error)
}

// ResultPool is a pool for tasks that return a result.
type ResultPool[R any] interface {
	basePool
	// Submit submits a task and returns a future for the result.
	Submit(task func() R) ResultTask[R]
	// SubmitErr submits a task that can return an error.
	SubmitErr(task func() (R, error)) ResultTask[R]
	// TrySubmit attempts to submit without blocking. Returns false if queue is full.
	TrySubmit(task func() R) (ResultTask[R], bool)
	// TrySubmitErr attempts to submit without blocking. Returns false if queue is full.
	TrySubmitErr(task func() (R, error)) (ResultTask[R], bool)
	// NewSubpool creates a child pool with the specified concurrency.
	NewSubpool(maxConcurrency int, options ...Option) ResultPool[R]
	// NewGroup creates a new task group.
	NewGroup() ResultTaskGroup[R]
	// NewGroupContext creates a new task group with the specified context.
	NewGroupContext(ctx context.Context) ResultTaskGroup[R]
}

type resultPool[R any] struct{ *pool }

// NewResultPool creates a pool for tasks that return a result.
// maxConcurrency of 0 means unlimited.
func NewResultPool[R any](maxConcurrency int, options ...Option) ResultPool[R] {
	return newResultPool[R](maxConcurrency, nil, options...)
}

func (p *resultPool[R]) NewSubpool(maxConcurrency int, options ...Option) ResultPool[R] {
	return newResultPool[R](maxConcurrency, p.pool, options...)
}

func (p *resultPool[R]) NewGroup() ResultTaskGroup[R] {
	return newResultTaskGroup[R](p.pool, p.Context())
}

func (p *resultPool[R]) NewGroupContext(ctx context.Context) ResultTaskGroup[R] {
	return newResultTaskGroup[R](p.pool, ctx)
}

func (p *resultPool[R]) Submit(task func() R) ResultTask[R] {
	f, _ := p.submit(task, p.nonBlocking)
	return f
}

func (p *resultPool[R]) SubmitErr(task func() (R, error)) ResultTask[R] {
	f, _ := p.submit(task, p.nonBlocking)
	return f
}

func (p *resultPool[R]) TrySubmit(task func() R) (ResultTask[R], bool) {
	return p.submit(task, true)
}

func (p *resultPool[R]) TrySubmitErr(task func() (R, error)) (ResultTask[R], bool) {
	return p.submit(task, true)
}

func (p *resultPool[R]) submit(task any, nonBlocking bool) (ResultTask[R], bool) {
	f, resolve := future.NewValueFuture[R](p.Context())

	if p.Stopped() {
		var zero R
		resolve(zero, ErrPoolStopped)
		return f, false
	}

	wrapped := wrapTask[R, func(R, error)](task, resolve, p.pool.panicRecovery)
	if err := p.pool.submit(wrapped, nonBlocking); err != nil {
		var zero R
		resolve(zero, err)
		return f, false
	}
	return f, true
}

func newResultPool[R any](maxConcurrency int, parent *pool, options ...Option) *resultPool[R] {
	return &resultPool[R]{newPool(maxConcurrency, parent, options...)}
}
