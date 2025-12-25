package future_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qntx/pond/internal/future"
)

func TestValueFutureWait(t *testing.T) {
	f, resolve := future.NewValueFuture[int](context.Background())
	resolve(5, nil)

	out, err := f.Wait()
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if out != 5 {
		t.Errorf("Wait() = %d, want 5", out)
	}
}

func TestValueFutureWaitWithError(t *testing.T) {
	f, resolve := future.NewValueFuture[int](context.Background())
	sampleErr := errors.New("sample error")
	resolve(0, sampleErr)

	out, err := f.Wait()
	if err != sampleErr {
		t.Errorf("Wait() err = %v, want %v", err, sampleErr)
	}
	if out != 0 {
		t.Errorf("Wait() = %d, want 0", out)
	}
}

func TestValueFutureWaitWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f, resolve := future.NewValueFuture[int](ctx)

	cancel() // cancel before resolve
	resolve(0, nil)

	out, err := f.Wait()
	if err != context.Canceled {
		t.Errorf("Wait() err = %v, want context.Canceled", err)
	}
	if out != 0 {
		t.Errorf("Wait() = %d, want 0", out)
	}
}

func TestValueFutureDone(t *testing.T) {
	f, resolve := future.NewValueFuture[int](context.Background())

	go func() {
		time.Sleep(1 * time.Millisecond)
		resolve(10, nil)
	}()

	<-f.Done()

	value, err := f.Result()
	if err != nil {
		t.Errorf("Result() err = %v, want nil", err)
	}
	if value != 10 {
		t.Errorf("Result() = %d, want 10", value)
	}
}

func TestValueFutureResolved(t *testing.T) {
	f, resolve := future.NewValueFuture[int](context.Background())

	if f.Resolved() {
		t.Error("Resolved() = true before resolve, want false")
	}

	resolve(42, nil)

	if !f.Resolved() {
		t.Error("Resolved() = false after resolve, want true")
	}
}

func TestValueFutureResolvedWithExternalCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f, _ := future.NewValueFuture[int](ctx)

	cancel()
	<-f.Done()

	if f.Resolved() {
		t.Error("Resolved() = true after external cancel, want false")
	}
}
