package pond_test

import (
	"errors"
	"testing"

	"github.com/qntx/pond"
)

func TestTaskPanicRecovery(t *testing.T) {
	pool := pond.NewPool(1)
	defer pool.StopAndWait()

	sampleErr := errors.New("sample error")

	task := pool.Submit(func() {
		panic(sampleErr)
	})

	err := task.Wait()
	if !errors.Is(err, pond.ErrPanic) {
		t.Errorf("Wait() err = %v, want ErrPanic", err)
	}
	if !errors.Is(err, sampleErr) {
		t.Errorf("Wait() err does not wrap sampleErr")
	}
}

func TestTaskPanicRecoveryWithString(t *testing.T) {
	pool := pond.NewPool(1)
	defer pool.StopAndWait()

	task := pool.Submit(func() {
		panic("boom")
	})

	err := task.Wait()
	if !errors.Is(err, pond.ErrPanic) {
		t.Errorf("Wait() err = %v, want ErrPanic", err)
	}
}

func TestResultTaskPanicRecovery(t *testing.T) {
	pool := pond.NewResultPool[int](1)
	defer pool.StopAndWait()

	sampleErr := errors.New("sample error")

	task := pool.Submit(func() int {
		panic(sampleErr)
	})

	result, err := task.Wait()
	if !errors.Is(err, pond.ErrPanic) {
		t.Errorf("Wait() err = %v, want ErrPanic", err)
	}
	if !errors.Is(err, sampleErr) {
		t.Errorf("Wait() err does not wrap sampleErr")
	}
	if result != 0 {
		t.Errorf("Wait() result = %d, want 0", result)
	}
}
