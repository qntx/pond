package future

import "context"

// ValueFutureResolver completes a ValueFuture with a value and optional error.
// Must be called exactly once; subsequent calls are no-ops.
type ValueFutureResolver[V any] func(value V, err error)

// valueFutureResolution wraps the result to distinguish from parent context cancellation.
type valueFutureResolution[V any] struct {
	value V
	err   error
}

func (r *valueFutureResolution[V]) Error() string {
	if r.err != nil {
		return r.err.Error()
	}
	return "success"
}

// ValueFuture represents an asynchronous operation that resolves to a value or error.
// When the parent context is canceled before resolution, the Future is also canceled.
type ValueFuture[V any] struct {
	ctx context.Context
}

// NewValueFuture creates a ValueFuture bound to the parent context.
// Returns the Future and a resolver function that must be called exactly once.
func NewValueFuture[V any](parent context.Context) (*ValueFuture[V], ValueFutureResolver[V]) {
	ctx, cancel := context.WithCancelCause(parent)
	return &ValueFuture[V]{ctx: ctx}, func(value V, err error) {
		cancel(&valueFutureResolution[V]{value: value, err: err})
	}
}

// Done returns a channel closed when the Future resolves or the parent context cancels.
func (f *ValueFuture[V]) Done() <-chan struct{} {
	return f.ctx.Done()
}

// Result blocks until resolution and returns the value and error.
// On success: (value, nil). On failure: (value, err). On external cancel: (zero, cause).
func (f *ValueFuture[V]) Result() (V, error) {
	<-f.ctx.Done()
	cause := context.Cause(f.ctx)
	if r, ok := cause.(*valueFutureResolution[V]); ok {
		return r.value, r.err
	}
	var zero V
	return zero, cause
}

// Wait is an alias for Result.
func (f *ValueFuture[V]) Wait() (V, error) {
	return f.Result()
}

// Resolved reports whether the Future completed via its resolver (not external cancellation).
func (f *ValueFuture[V]) Resolved() bool {
	select {
	case <-f.ctx.Done():
		_, ok := context.Cause(f.ctx).(*valueFutureResolution[V])
		return ok
	default:
		return false
	}
}
