package future_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qntx/pond/internal/future"
)

func TestCompositeFutureWait(t *testing.T) {
	f, resolve := future.NewCompositeFuture[string](context.Background())

	resolve(0, "output1", nil)
	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := f.Wait(3)
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if len(outputs) != 3 {
		t.Fatalf("len(outputs) = %d, want 3", len(outputs))
	}
	if outputs[0] != "output1" || outputs[1] != "output2" || outputs[2] != "output3" {
		t.Errorf("outputs = %v, want [output1 output2 output3]", outputs)
	}
}

func TestCompositeFutureWaitWithError(t *testing.T) {
	f, resolve := future.NewCompositeFuture[string](context.Background())
	sampleErr := errors.New("sample error")

	resolve(0, "output1", nil)
	resolve(1, "output2", nil)
	resolve(2, "output3", sampleErr)

	outputs, err := f.Wait(3)
	if err != sampleErr {
		t.Errorf("Wait() err = %v, want %v", err, sampleErr)
	}
	if len(outputs) != 3 {
		t.Fatalf("len(outputs) = %d, want 3", len(outputs))
	}
	if outputs[0] != "output1" || outputs[1] != "output2" || outputs[2] != "" {
		t.Errorf("outputs = %v, want [output1 output2 ]", outputs)
	}
}

func TestCompositeFutureWaitWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f, resolve := future.NewCompositeFuture[string](ctx)

	cancel()
	resolve(0, "output1", nil)

	_, err := f.Wait(1)
	if err != context.Canceled {
		t.Errorf("Wait() err = %v, want context.Canceled", err)
	}
}

func TestCompositeFutureResolveWithNegativeIndex(t *testing.T) {
	_, resolve := future.NewCompositeFuture[string](context.Background())

	defer func() {
		if r := recover(); r == nil {
			t.Error("resolve(-1) did not panic")
		}
	}()
	resolve(-1, "output1", nil)
}

func TestCompositeFutureWithMultipleWait(t *testing.T) {
	f, resolve := future.NewCompositeFuture[string](context.Background())

	resolve(0, "output1", nil)

	outputs1, err := f.Wait(1)
	if err != nil {
		t.Errorf("Wait(1) err = %v, want nil", err)
	}
	if len(outputs1) != 1 || outputs1[0] != "output1" {
		t.Errorf("outputs1 = %v, want [output1]", outputs1)
	}

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := f.Wait(3)
	if err != nil {
		t.Errorf("Wait(3) err = %v, want nil", err)
	}
	if len(outputs) != 3 {
		t.Fatalf("len(outputs) = %d, want 3", len(outputs))
	}
	if outputs[0] != "output1" || outputs[1] != "output2" || outputs[2] != "output3" {
		t.Errorf("outputs = %v, want [output1 output2 output3]", outputs)
	}
}

func TestCompositeFutureWithErrorsAndMultipleWait(t *testing.T) {
	f, resolve := future.NewCompositeFuture[string](context.Background())
	sampleErr := errors.New("sample error")

	resolve(0, "output1", sampleErr)

	outputs1, err := f.Wait(1)
	if err != sampleErr {
		t.Errorf("Wait(1) err = %v, want %v", err, sampleErr)
	}
	if len(outputs1) != 1 || outputs1[0] != "" {
		t.Errorf("outputs1 = %v, want []", outputs1)
	}

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := f.Wait(3)
	if err != sampleErr {
		t.Errorf("Wait(3) err = %v, want %v", err, sampleErr)
	}
	if len(outputs) != 3 {
		t.Fatalf("len(outputs) = %d, want 3", len(outputs))
	}
}

func TestCompositeFutureWaitBeforeResolution(t *testing.T) {
	f, resolve := future.NewCompositeFuture[string](context.Background())

	go func() {
		time.Sleep(10 * time.Millisecond)
		resolve(0, "output1", nil)
	}()

	outputs, err := f.Wait(1)
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if len(outputs) != 1 || outputs[0] != "output1" {
		t.Errorf("outputs = %v, want [output1]", outputs)
	}
}

func TestCompositeFutureWaitBeforeContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f, _ := future.NewCompositeFuture[string](ctx)

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	outputs, err := f.Wait(1)
	if err != context.Canceled {
		t.Errorf("Wait() err = %v, want context.Canceled", err)
	}
	if len(outputs) != 1 || outputs[0] != "" {
		t.Errorf("outputs = %v, want []", outputs)
	}
}

func TestCompositeFutureWaitWithContextCanceledAfterResolution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f, resolve := future.NewCompositeFuture[string](ctx)

	resolve(0, "output1", nil)
	cancel()

	outputs, err := f.Wait(1)
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if len(outputs) != 1 || outputs[0] != "output1" {
		t.Errorf("outputs = %v, want [output1]", outputs)
	}
}

func TestCompositeFutureWithMultipleDone(t *testing.T) {
	f, resolve := future.NewCompositeFuture[string](context.Background())

	resolve(0, "output1", nil)
	<-f.Done(1)

	outputs1, err := f.Wait(1)
	if err != nil {
		t.Errorf("Wait(1) err = %v, want nil", err)
	}
	if len(outputs1) != 1 || outputs1[0] != "output1" {
		t.Errorf("outputs1 = %v, want [output1]", outputs1)
	}

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)
	<-f.Done(3)

	outputs, err := f.Wait(3)
	if err != nil {
		t.Errorf("Wait(3) err = %v, want nil", err)
	}
	if len(outputs) != 3 {
		t.Fatalf("len(outputs) = %d, want 3", len(outputs))
	}
}

func TestCompositeFutureWithErrorsAndMultipleDone(t *testing.T) {
	f, resolve := future.NewCompositeFuture[string](context.Background())
	sampleErr := errors.New("sample error")

	resolve(0, "output1", sampleErr)
	<-f.Done(1)

	outputs1, err := f.Wait(1)
	if err != sampleErr {
		t.Errorf("Wait(1) err = %v, want %v", err, sampleErr)
	}
	if len(outputs1) != 1 {
		t.Fatalf("len(outputs1) = %d, want 1", len(outputs1))
	}

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)
	<-f.Done(3)

	outputs, err := f.Wait(3)
	if err != sampleErr {
		t.Errorf("Wait(3) err = %v, want %v", err, sampleErr)
	}
	if len(outputs) != 3 {
		t.Fatalf("len(outputs) = %d, want 3", len(outputs))
	}
}

func TestCompositeFutureCancel(t *testing.T) {
	f, resolve := future.NewCompositeFuture[string](context.Background())

	resolve(0, "output1", nil)
	f.Cancel(errors.New("sample error"))

	outputs, err := f.Wait(1)
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("len(outputs) = %d, want 1", len(outputs))
	}
}
