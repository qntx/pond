package pond_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qntx/pond"
)

func TestPoolSubmit(t *testing.T) {
	pool := pond.NewPool(100)

	taskCount := 1000
	var executedCount atomic.Int64

	for range taskCount {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	pool.Stop().Wait()

	if executedCount.Load() != int64(taskCount) {
		t.Errorf("executedCount = %d, want %d", executedCount.Load(), taskCount)
	}
}

func TestPoolSubmitAndWait(t *testing.T) {
	pool := pond.NewPool(100)

	done := make(chan int, 1)
	task := pool.Submit(func() {
		done <- 10
	})

	err := task.Wait()
	if err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
	if v := <-done; v != 10 {
		t.Errorf("done = %d, want 10", v)
	}
}

func TestPoolSubmitWithPanic(t *testing.T) {
	pool := pond.NewPool(100)
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

func TestPoolSubmitWithErr(t *testing.T) {
	pool := pond.NewPool(100)

	task := pool.SubmitErr(func() error {
		return errors.New("sample error")
	})

	err := task.Wait()
	if err == nil || err.Error() != "sample error" {
		t.Errorf("Wait() = %v, want 'sample error'", err)
	}
}

func TestPoolWithContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool := pond.NewPool(10, pond.WithContext(ctx))

	taskCount := 10000
	var executedCount atomic.Int64

	for range taskCount {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	time.Sleep(5 * time.Millisecond)
	cancel()

	pool.Stop().Wait()

	if executedCount.Load() >= int64(taskCount) {
		t.Errorf("executedCount = %d, should be less than %d", executedCount.Load(), taskCount)
	}
	if pool.RunningWorkers() != 0 {
		t.Errorf("RunningWorkers() = %d, want 0", pool.RunningWorkers())
	}
}

func TestPoolMetrics(t *testing.T) {
	pool := pond.NewPool(100)

	if pool.RunningWorkers() != 0 {
		t.Errorf("RunningWorkers() = %d, want 0", pool.RunningWorkers())
	}
	if pool.SubmittedTasks() != 0 {
		t.Errorf("SubmittedTasks() = %d, want 0", pool.SubmittedTasks())
	}

	taskCount := 10000
	var executedCount atomic.Int64

	for i := range taskCount {
		n := i
		pool.SubmitErr(func() error {
			executedCount.Add(1)
			if n%2 == 0 {
				return nil
			}
			return errors.New("sample error")
		})
	}

	pool.Stop().Wait()

	if executedCount.Load() != int64(taskCount) {
		t.Errorf("executedCount = %d, want %d", executedCount.Load(), taskCount)
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

func TestPoolSubmitOnStoppedPool(t *testing.T) {
	pool := pond.NewPool(100)
	pool.Submit(func() {})
	pool.StopAndWait()

	err := pool.Submit(func() {}).Wait()
	if err != pond.ErrPoolStopped {
		t.Errorf("Submit().Wait() = %v, want ErrPoolStopped", err)
	}

	err = pool.Go(func() {})
	if err != pond.ErrPoolStopped {
		t.Errorf("Go() = %v, want ErrPoolStopped", err)
	}

	if !pool.Stopped() {
		t.Error("Stopped() = false, want true")
	}
}

func TestNewPoolWithInvalidMaxConcurrency(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewPool(-1) did not panic")
		}
	}()
	pond.NewPool(-1)
}

func TestPoolStoppedAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool := pond.NewPool(10, pond.WithContext(ctx))

	err := pool.Submit(func() {
		cancel()
	}).Wait()

	if err != context.Canceled {
		t.Errorf("Wait() = %v, want context.Canceled", err)
	}

	err = pool.Submit(func() {}).Wait()
	if err != pond.ErrPoolStopped {
		t.Errorf("Submit().Wait() = %v, want ErrPoolStopped", err)
	}

	if !pool.Stopped() {
		t.Error("Stopped() = false, want true")
	}
}

func TestPoolWithQueueSize(t *testing.T) {
	pool := pond.NewPool(1, pond.WithQueueSize(10))

	if pool.QueueSize() != 10 {
		t.Errorf("QueueSize() = %d, want 10", pool.QueueSize())
	}

	taskCount := 50
	for range taskCount {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
		})
	}

	pool.Stop().Wait()

	if pool.SubmittedTasks() != uint64(taskCount) {
		t.Errorf("SubmittedTasks() = %d, want %d", pool.SubmittedTasks(), taskCount)
	}
}

func TestPoolWithQueueSizeAndNonBlocking(t *testing.T) {
	pool := pond.NewPool(10, pond.WithQueueSize(10), pond.WithNonBlocking(true))

	if !pool.NonBlocking() {
		t.Error("NonBlocking() = false, want true")
	}

	taskStarted := make(chan struct{}, 10)
	taskWait := make(chan struct{})

	for range 10 {
		pool.Submit(func() {
			taskStarted <- struct{}{}
			<-taskWait
		})
	}

	for range 10 {
		<-taskStarted
	}

	if pool.RunningWorkers() != 10 {
		t.Errorf("RunningWorkers() = %d, want 10", pool.RunningWorkers())
	}

	for range 10 {
		pool.Submit(func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	task := pool.Submit(func() {})
	close(taskWait)

	if task.Wait() != pond.ErrQueueFull {
		t.Errorf("Wait() = %v, want ErrQueueFull", task.Wait())
	}
	if pool.DroppedTasks() != 1 {
		t.Errorf("DroppedTasks() = %d, want 1", pool.DroppedTasks())
	}

	pool.Stop().Wait()
}

func TestPoolResize(t *testing.T) {
	pool := pond.NewPool(1, pond.WithQueueSize(10))

	if pool.MaxConcurrency() != 1 {
		t.Errorf("MaxConcurrency() = %d, want 1", pool.MaxConcurrency())
	}

	taskWait := make(chan struct{})

	for range 10 {
		pool.Submit(func() {
			<-taskWait
		})
	}

	time.Sleep(10 * time.Millisecond)
	if pool.WaitingTasks() != 9 {
		t.Errorf("WaitingTasks() = %d, want 9", pool.WaitingTasks())
	}
	if pool.RunningWorkers() != 1 {
		t.Errorf("RunningWorkers() = %d, want 1", pool.RunningWorkers())
	}

	pool.Resize(3)
	if pool.MaxConcurrency() != 3 {
		t.Errorf("MaxConcurrency() = %d, want 3", pool.MaxConcurrency())
	}

	time.Sleep(10 * time.Millisecond)
	if pool.RunningWorkers() != 3 {
		t.Errorf("RunningWorkers() = %d, want 3", pool.RunningWorkers())
	}

	close(taskWait)
	pool.Stop().Wait()
}

func TestPoolResizeWithZeroMaxConcurrency(t *testing.T) {
	pool := pond.NewPool(10)
	pool.Resize(0)

	if pool.MaxConcurrency() != math.MaxInt {
		t.Errorf("MaxConcurrency() = %d, want MaxInt", pool.MaxConcurrency())
	}
}

func TestPoolResizeWithNegativeMaxConcurrency(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Resize(-1) did not panic")
		}
	}()
	pond.NewPool(10).Resize(-1)
}

func TestPoolSubmitWhileStopping(t *testing.T) {
	pool := pond.NewPool(10)

	task := pool.Submit(func() {
		for !pool.Stopped() {
			time.Sleep(1 * time.Millisecond)
		}

		err := pool.Submit(func() {}).Wait()
		if err != pond.ErrPoolStopped {
			panic("expected ErrPoolStopped")
		}
	})

	pool.Stop().Wait()

	if task.Wait() != nil {
		t.Errorf("task.Wait() = %v, want nil", task.Wait())
	}
}

func TestPoolSubmitWhileStoppingHasNoRace(t *testing.T) {
	pool := pond.NewPool(0)

	var wg sync.WaitGroup

	wg.Go(func() {
		time.Sleep(500 * time.Microsecond)
		pool.StopAndWait()
	})

	for range 10000 {
		wg.Go(func() {
			pool.Submit(func() {
				time.Sleep(10 * time.Millisecond)
			})
		})
	}

	wg.Wait()
}

func TestPoolTrySubmit(t *testing.T) {
	pool := pond.NewPool(1, pond.WithQueueSize(1))

	completeFirstTask := make(chan struct{})

	task, ok := pool.TrySubmit(func() {
		completeFirstTask <- struct{}{}
	})
	if !ok {
		t.Error("TrySubmit() = false, want true")
	}

	task2, ok := pool.TrySubmit(func() {})
	if !ok {
		t.Error("TrySubmit() to queue = false, want true")
	}

	task3, ok := pool.TrySubmit(func() {})
	if ok {
		t.Error("TrySubmit() when full = true, want false")
	}

	<-completeFirstTask

	pool.StopAndWait()
	task4, ok := pool.TrySubmit(func() {})
	if ok {
		t.Error("TrySubmit() to stopped pool = true, want false")
	}
	if task4.Wait() != pond.ErrPoolStopped {
		t.Errorf("task4.Wait() = %v, want ErrPoolStopped", task4.Wait())
	}

	if task.Wait() != nil {
		t.Errorf("task.Wait() = %v, want nil", task.Wait())
	}
	if task2.Wait() != nil {
		t.Errorf("task2.Wait() = %v, want nil", task2.Wait())
	}
	if task3.Wait() != pond.ErrQueueFull {
		t.Errorf("task3.Wait() = %v, want ErrQueueFull", task3.Wait())
	}

	if pool.SubmittedTasks() != 3 {
		t.Errorf("SubmittedTasks() = %d, want 3", pool.SubmittedTasks())
	}
	if pool.DroppedTasks() != 1 {
		t.Errorf("DroppedTasks() = %d, want 1", pool.DroppedTasks())
	}
}

func TestPoolTrySubmitErr(t *testing.T) {
	pool := pond.NewPool(1, pond.WithQueueSize(1))

	completeFirstTask := make(chan struct{})

	task, ok := pool.TrySubmitErr(func() error {
		completeFirstTask <- struct{}{}
		return nil
	})
	if !ok {
		t.Error("TrySubmitErr() = false, want true")
	}

	task2, ok := pool.TrySubmitErr(func() error {
		return errors.New("sample error")
	})
	if !ok {
		t.Error("TrySubmitErr() to queue = false, want true")
	}

	task3, ok := pool.TrySubmitErr(func() error { return nil })
	if ok {
		t.Error("TrySubmitErr() when full = true, want false")
	}

	<-completeFirstTask

	pool.StopAndWait()
	task4, ok := pool.TrySubmitErr(func() error { return nil })
	if ok {
		t.Error("TrySubmitErr() to stopped pool = true, want false")
	}
	if task4.Wait() != pond.ErrPoolStopped {
		t.Errorf("task4.Wait() = %v, want ErrPoolStopped", task4.Wait())
	}

	if task.Wait() != nil {
		t.Errorf("task.Wait() = %v, want nil", task.Wait())
	}
	if task2.Wait() == nil || task2.Wait().Error() != "sample error" {
		t.Errorf("task2.Wait() = %v, want 'sample error'", task2.Wait())
	}
	if task3.Wait() != pond.ErrQueueFull {
		t.Errorf("task3.Wait() = %v, want ErrQueueFull", task3.Wait())
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
