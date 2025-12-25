package pond

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/qntx/pond/internal/future"
)

// ErrGroupStopped is returned when a task group is stopped.
var ErrGroupStopped = errors.New("task group stopped")

// TaskGroup represents a group of tasks that can be executed concurrently.
// The group can be waited on to block until all tasks have completed.
// If any task returns an error, the group returns the first error encountered.
type TaskGroup interface {
	// Submit submits tasks to the group.
	Submit(tasks ...func()) TaskGroup
	// SubmitErr submits tasks that can return an error.
	SubmitErr(tasks ...func() error) TaskGroup
	// Wait blocks until all tasks complete. Returns first error if any.
	Wait() error
	// Done returns a channel closed when all tasks complete or on error/stop.
	Done() <-chan struct{}
	// Stop cancels remaining tasks. Running tasks are not interrupted.
	Stop()
	// Context returns the group's context, canceled on error or parent cancel.
	Context() context.Context
}

// ResultTaskGroup represents a group of tasks that yield results.
// The group can be waited on to block until all tasks have completed.
// If any task returns an error, the group returns the first error encountered.
type ResultTaskGroup[O any] interface {
	// Submit submits tasks to the group.
	Submit(tasks ...func() O) ResultTaskGroup[O]
	// SubmitErr submits tasks that can return an error.
	SubmitErr(tasks ...func() (O, error)) ResultTaskGroup[O]
	// Wait blocks until all tasks complete. Returns results in submission order.
	Wait() ([]O, error)
	// Done returns a channel closed when all tasks complete or on error/stop.
	Done() <-chan struct{}
	// Stop cancels remaining tasks. Running tasks are not interrupted.
	Stop()
}

// result holds a task's output and error.
type result[O any] struct {
	Output O
	Err    error
}

// abstractTaskGroup is the generic base for task groups.
type abstractTaskGroup[T func() | func() O, E func() error | func() (O, error), O any] struct {
	pool      *pool
	nextIndex atomic.Int64
	wg        sync.WaitGroup
	future    *future.CompositeFuture[*result[O]]
	resolve   future.CompositeFutureResolver[*result[O]]
}

func (g *abstractTaskGroup[T, E, O]) Done() <-chan struct{} {
	return g.future.Done(int(g.nextIndex.Load()))
}

func (g *abstractTaskGroup[T, E, O]) Stop() {
	g.future.Cancel(ErrGroupStopped)
}

func (g *abstractTaskGroup[T, E, O]) Context() context.Context {
	return g.future.Context()
}

func (g *abstractTaskGroup[T, E, O]) Submit(tasks ...T) *abstractTaskGroup[T, E, O] {
	for _, task := range tasks {
		g.submit(task)
	}
	return g
}

func (g *abstractTaskGroup[T, E, O]) SubmitErr(tasks ...E) *abstractTaskGroup[T, E, O] {
	for _, task := range tasks {
		g.submit(task)
	}
	return g
}

func (g *abstractTaskGroup[T, E, O]) submit(task any) {
	index := int(g.nextIndex.Add(1) - 1)
	g.wg.Add(1)

	err := g.pool.submit(func() error {
		defer g.wg.Done()

		// Skip if context already canceled
		if err := g.future.Context().Err(); err != nil {
			g.resolve(index, &result[O]{Err: err}, err)
			return err
		}

		output, err := invokeTask[O](task, g.pool.panicRecovery)
		g.resolve(index, &result[O]{Output: output, Err: err}, err)
		return err
	}, g.pool.nonBlocking)

	if err != nil {
		g.wg.Done()
		g.resolve(index, &result[O]{Err: err}, err)
	}
}

// taskGroup implements TaskGroup.
type taskGroup struct {
	abstractTaskGroup[func(), func() error, struct{}]
}

func (g *taskGroup) Submit(tasks ...func()) TaskGroup {
	g.abstractTaskGroup.Submit(tasks...)
	return g
}

func (g *taskGroup) SubmitErr(tasks ...func() error) TaskGroup {
	g.abstractTaskGroup.SubmitErr(tasks...)
	return g
}

func (g *taskGroup) Wait() error {
	_, err := g.future.Wait(int(g.nextIndex.Load()))
	g.wg.Wait() // ensure all tasks finish before returning
	return err
}

// resultTaskGroup implements ResultTaskGroup.
type resultTaskGroup[O any] struct {
	abstractTaskGroup[func() O, func() (O, error), O]
}

func (g *resultTaskGroup[O]) Submit(tasks ...func() O) ResultTaskGroup[O] {
	g.abstractTaskGroup.Submit(tasks...)
	return g
}

func (g *resultTaskGroup[O]) SubmitErr(tasks ...func() (O, error)) ResultTaskGroup[O] {
	g.abstractTaskGroup.SubmitErr(tasks...)
	return g
}

func (g *resultTaskGroup[O]) Wait() ([]O, error) {
	results, err := g.future.Wait(int(g.nextIndex.Load()))
	g.wg.Wait() // ensure all tasks finish before returning

	values := make([]O, len(results))
	for i, r := range results {
		if r != nil {
			values[i] = r.Output
		}
	}
	return values, err
}

func newTaskGroup(pool *pool, ctx context.Context) TaskGroup {
	f, resolve := future.NewCompositeFuture[*result[struct{}]](ctx)
	return &taskGroup{
		abstractTaskGroup: abstractTaskGroup[func(), func() error, struct{}]{
			pool:    pool,
			future:  f,
			resolve: resolve,
		},
	}
}

func newResultTaskGroup[O any](pool *pool, ctx context.Context) ResultTaskGroup[O] {
	f, resolve := future.NewCompositeFuture[*result[O]](ctx)
	return &resultTaskGroup[O]{
		abstractTaskGroup: abstractTaskGroup[func() O, func() (O, error), O]{
			pool:    pool,
			future:  f,
			resolve: resolve,
		},
	}
}
