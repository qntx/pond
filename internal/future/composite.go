package future

import (
	"context"
	"sync"
)

// CompositeFutureResolver resolves one future in a composite group by index.
// Call with nil error for success, non-nil for failure (cancels entire group).
type CompositeFutureResolver[V any] func(index int, value V, err error)

// compositeResolution holds a successful result.
type compositeResolution[V any] struct {
	index int
	value V
}

// compositeErrorResolution wraps an error to distinguish from external cancellation.
type compositeErrorResolution struct{ err error }

func (e *compositeErrorResolution) Error() string { return e.err.Error() }

// waitListener waits for a specific count of resolutions.
type waitListener struct {
	count int
	ch    chan struct{}
}

// CompositeFuture aggregates multiple async results with ordered output by index.
// Completes when all expected results resolve or any error occurs.
type CompositeFuture[V any] struct {
	ctx         context.Context
	cancel      context.CancelCauseFunc
	mu          sync.Mutex
	resolutions []compositeResolution[V]
	listeners   []waitListener
}

// NewCompositeFuture creates a CompositeFuture bound to the parent context.
// The resolver must be called exactly once per index (0 to n-1).
func NewCompositeFuture[V any](parent context.Context) (*CompositeFuture[V], CompositeFutureResolver[V]) {
	ctx, cancel := context.WithCancelCause(parent)
	f := &CompositeFuture[V]{
		ctx:    ctx,
		cancel: cancel,
	}
	return f, f.resolve
}

// Context returns the context canceled on first error or external cancel.
func (f *CompositeFuture[V]) Context() context.Context {
	return f.ctx
}

// Done returns a channel closed when count resolutions are available or on error/cancel.
func (f *CompositeFuture[V]) Done(count int) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan struct{})
	if len(f.resolutions) >= count || context.Cause(f.ctx) != nil {
		close(ch)
		return ch
	}
	f.listeners = append(f.listeners, waitListener{count: count, ch: ch})
	return ch
}

// Wait blocks until count resolutions are available or error/cancel.
// Returns values sorted by index and any error.
func (f *CompositeFuture[V]) Wait(count int) ([]V, error) {
	f.mu.Lock()
	if vals, err := f.resultLocked(count); vals != nil || err != nil {
		f.mu.Unlock()
		return vals, err
	}

	ch := make(chan struct{})
	f.listeners = append(f.listeners, waitListener{count: count, ch: ch})
	f.mu.Unlock()

	select {
	case <-ch:
	case <-f.ctx.Done():
	}

	f.mu.Lock()
	vals, err := f.resultLocked(count)
	f.mu.Unlock()
	return vals, err
}

// Cancel propagates cancellation with cause.
func (f *CompositeFuture[V]) Cancel(cause error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancel(cause)
	f.notifyLocked()
}

func (f *CompositeFuture[V]) resolve(index int, value V, err error) {
	if index < 0 {
		panic("composite future: negative index")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if context.Cause(f.ctx) != nil {
		return // already canceled
	}

	if err != nil {
		f.cancel(&compositeErrorResolution{err: err})
	} else {
		f.resolutions = append(f.resolutions, compositeResolution[V]{index: index, value: value})
	}
	f.notifyLocked()
}

// resultLocked returns results if ready. Caller must hold mu.
func (f *CompositeFuture[V]) resultLocked(count int) ([]V, error) {
	cause := context.Cause(f.ctx)
	hasEnough := len(f.resolutions) >= count

	if !hasEnough && cause == nil {
		return nil, nil // not ready
	}

	// Build result slice
	vals := make([]V, count)
	for _, r := range f.resolutions {
		if r.index < count {
			vals[r.index] = r.value
		}
	}

	if cause == nil {
		return vals, nil
	}
	if e, ok := cause.(*compositeErrorResolution); ok {
		return vals, e.err
	}
	if hasEnough {
		return vals, nil // external cancel after completion
	}
	return vals, cause
}

// notifyLocked wakes waiting listeners. Caller must hold mu.
func (f *CompositeFuture[V]) notifyLocked() {
	cause := context.Cause(f.ctx)
	n := 0
	for _, l := range f.listeners {
		if cause != nil || l.count <= len(f.resolutions) {
			close(l.ch)
		} else {
			f.listeners[n] = l
			n++
		}
	}
	f.listeners = f.listeners[:n]
}
