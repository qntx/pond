package pond_test

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qntx/pond"
)

func TestResultPoolSubmitAndWait(t *testing.T) {
	pool := pond.NewResultPool[int](1000)
	defer pool.StopAndWait()

	task := pool.Submit(func() int { return 5 })

	output, err := task.Wait()
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if output != 5 {
		t.Errorf("Wait() = %d, want 5", output)
	}
}

func TestResultPoolSubmitTaskWithPanic(t *testing.T) {
	pool := pond.NewResultPool[int](1000)

	task := pool.Submit(func() int {
		panic("dummy panic")
	})

	output, err := task.Wait()
	if !errors.Is(err, pond.ErrPanic) {
		t.Errorf("Wait() err = %v, want ErrPanic", err)
	}
	if !strings.HasPrefix(err.Error(), "task panicked: dummy panic") {
		t.Errorf("err.Error() = %q, want prefix 'task panicked: dummy panic'", err.Error())
	}
	if output != 0 {
		t.Errorf("Wait() = %d, want 0", output)
	}
}

func TestResultPoolMetrics(t *testing.T) {
	pool := pond.NewResultPool[int](1000)

	if pool.RunningWorkers() != 0 {
		t.Errorf("RunningWorkers() = %d, want 0", pool.RunningWorkers())
	}
	if pool.SubmittedTasks() != 0 {
		t.Errorf("SubmittedTasks() = %d, want 0", pool.SubmittedTasks())
	}

	taskCount := 10000
	var executedCount atomic.Int64

	for i := range taskCount {
		i := i
		pool.SubmitErr(func() (int, error) {
			executedCount.Add(1)
			if i%2 == 0 {
				return i, nil
			}
			return 0, errors.New("sample error")
		})
	}

	pool.Stop().Wait()

	if executedCount.Load() != int64(taskCount) {
		t.Errorf("executedCount = %d, want %d", executedCount.Load(), taskCount)
	}
	if pool.RunningWorkers() != 0 {
		t.Errorf("RunningWorkers() = %d, want 0", pool.RunningWorkers())
	}
	if pool.SubmittedTasks() != uint64(taskCount) {
		t.Errorf("SubmittedTasks() = %d, want %d", pool.SubmittedTasks(), taskCount)
	}
	if pool.CompletedTasks() != uint64(taskCount) {
		t.Errorf("CompletedTasks() = %d, want %d", pool.CompletedTasks(), taskCount)
	}
	if pool.FailedTasks() != uint64(taskCount/2) {
		t.Errorf("FailedTasks() = %d, want %d", pool.FailedTasks(), taskCount/2)
	}
	if pool.SuccessfulTasks() != uint64(taskCount/2) {
		t.Errorf("SuccessfulTasks() = %d, want %d", pool.SuccessfulTasks(), taskCount/2)
	}
}

func TestResultPoolSubpool(t *testing.T) {
	pool := pond.NewResultPool[int](1000)
	subpool := pool.NewSubpool(10)

	var executedCount atomic.Int64

	for i := range 100 {
		i := i
		subpool.SubmitErr(func() (int, error) {
			executedCount.Add(1)
			return i, nil
		})
	}

	subpool.StopAndWait()

	if executedCount.Load() != 100 {
		t.Errorf("executedCount = %d, want 100", executedCount.Load())
	}
}

func TestResultSubpoolMaxConcurrency(t *testing.T) {
	pool := pond.NewResultPool[int](10)

	defer func() {
		if r := recover(); r == nil {
			t.Error("NewSubpool(-1) did not panic")
		}
	}()
	pool.NewSubpool(-1)
}

func TestResultSubpoolMaxConcurrencyExceedsParent(t *testing.T) {
	pool := pond.NewResultPool[int](10)

	defer func() {
		if r := recover(); r == nil {
			t.Error("NewSubpool(11) did not panic")
		}
	}()
	pool.NewSubpool(11)
}

func TestResultSubpoolMaxConcurrencyZero(t *testing.T) {
	pool := pond.NewResultPool[int](10)
	subpool := pool.NewSubpool(0)

	if subpool.MaxConcurrency() != 10 {
		t.Errorf("MaxConcurrency() = %d, want 10", subpool.MaxConcurrency())
	}
}

func TestResultPoolTrySubmit(t *testing.T) {
	pool := pond.NewResultPool[int](1, pond.WithQueueSize(1))
	defer pool.StopAndWait()

	completeFirstTask := make(chan struct{})

	task, ok := pool.TrySubmit(func() int {
		completeFirstTask <- struct{}{}
		return 42
	})
	if !ok {
		t.Error("TrySubmit() = false, want true")
	}

	task2, ok := pool.TrySubmit(func() int { return 43 })
	if !ok {
		t.Error("TrySubmit() to queue = false, want true")
	}

	task3, ok := pool.TrySubmit(func() int { return 44 })
	if ok {
		t.Error("TrySubmit() when full = true, want false")
	}

	<-completeFirstTask

	pool.StopAndWait()
	_, ok = pool.TrySubmit(func() int { return 45 })
	if ok {
		t.Error("TrySubmit() to stopped pool = true, want false")
	}

	result, err := task.Wait()
	if err != nil || result != 42 {
		t.Errorf("task.Wait() = (%d, %v), want (42, nil)", result, err)
	}

	result, err = task2.Wait()
	if err != nil || result != 43 {
		t.Errorf("task2.Wait() = (%d, %v), want (43, nil)", result, err)
	}

	result, err = task3.Wait()
	if err != pond.ErrQueueFull || result != 0 {
		t.Errorf("task3.Wait() = (%d, %v), want (0, ErrQueueFull)", result, err)
	}

	if pool.SubmittedTasks() != 3 {
		t.Errorf("SubmittedTasks() = %d, want 3", pool.SubmittedTasks())
	}
	if pool.CompletedTasks() != 2 {
		t.Errorf("CompletedTasks() = %d, want 2", pool.CompletedTasks())
	}
	if pool.DroppedTasks() != 1 {
		t.Errorf("DroppedTasks() = %d, want 1", pool.DroppedTasks())
	}
}

func TestResultPoolTrySubmitErr(t *testing.T) {
	pool := pond.NewResultPool[int](1, pond.WithQueueSize(1))
	defer pool.StopAndWait()

	completeFirstTask := make(chan struct{})
	sampleErr := errors.New("sample error")

	task, ok := pool.TrySubmitErr(func() (int, error) {
		completeFirstTask <- struct{}{}
		return 42, nil
	})
	if !ok {
		t.Error("TrySubmitErr() = false, want true")
	}

	task2, ok := pool.TrySubmitErr(func() (int, error) { return 0, sampleErr })
	if !ok {
		t.Error("TrySubmitErr() to queue = false, want true")
	}

	task3, ok := pool.TrySubmitErr(func() (int, error) { return 44, nil })
	if ok {
		t.Error("TrySubmitErr() when full = true, want false")
	}

	<-completeFirstTask

	pool.StopAndWait()

	result, err := task.Wait()
	if err != nil || result != 42 {
		t.Errorf("task.Wait() = (%d, %v), want (42, nil)", result, err)
	}

	result, err = task2.Wait()
	if err != sampleErr || result != 0 {
		t.Errorf("task2.Wait() = (%d, %v), want (0, sampleErr)", result, err)
	}

	result, err = task3.Wait()
	if err != pond.ErrQueueFull || result != 0 {
		t.Errorf("task3.Wait() = (%d, %v), want (0, ErrQueueFull)", result, err)
	}

	if pool.SubmittedTasks() != 3 {
		t.Errorf("SubmittedTasks() = %d, want 3", pool.SubmittedTasks())
	}
	if pool.SuccessfulTasks() != 1 {
		t.Errorf("SuccessfulTasks() = %d, want 1", pool.SuccessfulTasks())
	}
	if pool.FailedTasks() != 1 {
		t.Errorf("FailedTasks() = %d, want 1", pool.FailedTasks())
	}
	if pool.DroppedTasks() != 1 {
		t.Errorf("DroppedTasks() = %d, want 1", pool.DroppedTasks())
	}
}
