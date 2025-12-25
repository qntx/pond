package future_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qntx/pond/internal/future"
)

func TestFutureWait(t *testing.T) {
	f, resolve := future.NewFuture(context.Background())
	resolve(nil)

	err := f.Wait()
	if err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
}

func TestFutureWaitWithError(t *testing.T) {
	f, resolve := future.NewFuture(context.Background())
	sampleErr := errors.New("sample error")
	resolve(sampleErr)

	err := f.Wait()
	if err != sampleErr {
		t.Errorf("Wait() = %v, want %v", err, sampleErr)
	}
	if err.Error() != "sample error" {
		t.Errorf("err.Error() = %q, want %q", err.Error(), "sample error")
	}
	if f.Err() != sampleErr {
		t.Errorf("Err() = %v, want %v", f.Err(), sampleErr)
	}
}

func TestFutureWaitWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f, resolve := future.NewFuture(ctx)

	cancel()
	resolve(errors.New("sample error"))

	err := f.Wait()
	if err != context.Canceled {
		t.Errorf("Wait() = %v, want context.Canceled", err)
	}
}

func TestFutureDone(t *testing.T) {
	f, resolve := future.NewFuture(context.Background())
	sampleErr := errors.New("sample error")

	go func() {
		time.Sleep(1 * time.Millisecond)
		resolve(sampleErr)
	}()

	<-f.Done()

	if f.Err() != sampleErr {
		t.Errorf("Err() = %v, want %v", f.Err(), sampleErr)
	}
}

func TestFutureResolved(t *testing.T) {
	f, resolve := future.NewFuture(context.Background())

	if f.Resolved() {
		t.Error("Resolved() = true before resolve, want false")
	}

	resolve(nil)

	if !f.Resolved() {
		t.Error("Resolved() = false after resolve, want true")
	}
}

func TestFutureResolvedWithExternalCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f, _ := future.NewFuture(ctx)

	cancel()
	<-f.Done()

	if f.Resolved() {
		t.Error("Resolved() = true after external cancel, want false")
	}
}
