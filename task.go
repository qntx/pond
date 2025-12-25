package pond

import (
	"errors"
	"fmt"
	"runtime/debug"
)

// ErrPanic is returned when a task panics and panic recovery is enabled.
var ErrPanic = errors.New("task panicked")

// wrappedTask wraps a task with a callback to be invoked on completion.
type wrappedTask[R any, C func(error) | func(R, error)] struct {
	task        any
	callback    C
	catchPanics bool
}

func (w *wrappedTask[R, C]) Run() error {
	r, err := invokeTask[R](w.task, w.catchPanics)
	switch cb := any(w.callback).(type) {
	case func(error):
		cb(err)
	case func(R, error):
		cb(r, err)
	default:
		panic(fmt.Sprintf("unsupported callback type: %T", w.callback))
	}
	return err
}

func wrapTask[R any, C func(error) | func(R, error)](task any, cb C, catchPanics bool) func() error {
	return (&wrappedTask[R, C]{task: task, callback: cb, catchPanics: catchPanics}).Run
}

func invokeTask[R any](task any, catchPanics bool) (out R, err error) {
	if catchPanics {
		defer func() {
			if p := recover(); p != nil {
				if e, ok := p.(error); ok {
					err = fmt.Errorf("%w: %w\n%s", ErrPanic, e, debug.Stack())
				} else {
					err = fmt.Errorf("%w: %v\n%s", ErrPanic, p, debug.Stack())
				}
			}
		}()
	}

	switch fn := task.(type) {
	case func():
		fn()
	case func() error:
		err = fn()
	case func() R:
		out = fn()
	case func() (R, error):
		out, err = fn()
	default:
		panic(fmt.Sprintf("unsupported task type: %T", task))
	}
	return
}
