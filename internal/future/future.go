package future

import "context"

// FutureResolver completes a Future. Call with nil for success, non-nil for failure.
// Must be called exactly once; subsequent calls are no-ops (context already canceled).
type FutureResolver func(err error)

// futureResolution wraps the result to distinguish from parent context cancellation.
type futureResolution struct{ err error }

func (r *futureResolution) Error() string {
	if r.err != nil {
		return r.err.Error()
	}
	return "success"
}

// Future represents an asynchronous operation that resolves to success or failure.
// When the parent context is canceled before resolution, the Future is also canceled.
type Future struct {
	ctx context.Context
}

// NewFuture creates a Future bound to the parent context.
// Returns the Future and a resolver function that must be called exactly once.
func NewFuture(parent context.Context) (*Future, FutureResolver) {
	ctx, cancel := context.WithCancelCause(parent)
	return &Future{ctx: ctx}, func(err error) {
		cancel(&futureResolution{err: err})
	}
}

// Done returns a channel closed when the Future resolves or the parent context cancels.
func (f *Future) Done() <-chan struct{} {
	return f.ctx.Done()
}

// Err blocks until resolution and returns the result.
// Returns nil on success, the task error on failure, or the parent's cause if canceled externally.
func (f *Future) Err() error {
	<-f.ctx.Done()
	cause := context.Cause(f.ctx)
	if r, ok := cause.(*futureResolution); ok {
		return r.err
	}
	return cause
}

// Wait is an alias for Err.
func (f *Future) Wait() error {
	return f.Err()
}

// Resolved reports whether the Future completed via its resolver (not external cancellation).
func (f *Future) Resolved() bool {
	select {
	case <-f.ctx.Done():
		_, ok := context.Cause(f.ctx).(*futureResolution)
		return ok
	default:
		return false
	}
}
