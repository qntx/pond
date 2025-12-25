package pond_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qntx/pond"
)

func TestResultTaskGroupWait(t *testing.T) {
	pool := pond.NewResultPool[int](10)
	group := pool.NewGroup()

	for i := 0; i < 5; i++ {
		i := i
		group.Submit(func() int { return i })
	}

	outputs, err := group.Wait()
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if len(outputs) != 5 {
		t.Fatalf("len(outputs) = %d, want 5", len(outputs))
	}
	for i := range 5 {
		if outputs[i] != i {
			t.Errorf("outputs[%d] = %d, want %d", i, outputs[i], i)
		}
	}
}

func TestResultTaskGroupWaitWithError(t *testing.T) {
	pool := pond.NewResultPool[int](1)
	group := pool.NewGroup()
	sampleErr := errors.New("sample error")

	for i := range 5 {
		i := i
		if i == 3 {
			group.SubmitErr(func() (int, error) { return 0, sampleErr })
		} else {
			group.SubmitErr(func() (int, error) { return i, nil })
		}
	}

	outputs, err := group.Wait()
	if err != sampleErr {
		t.Errorf("Wait() err = %v, want %v", err, sampleErr)
	}
	if len(outputs) != 5 {
		t.Fatalf("len(outputs) = %d, want 5", len(outputs))
	}
}

func TestResultTaskGroupWaitWithErrorInLastTask(t *testing.T) {
	group := pond.NewResultPool[int](10).NewGroup()
	sampleErr := errors.New("sample error")

	group.SubmitErr(func() (int, error) { return 1, nil })
	time.Sleep(10 * time.Millisecond)
	group.SubmitErr(func() (int, error) { return 0, sampleErr })

	outputs, err := group.Wait()
	if err != sampleErr {
		t.Errorf("Wait() err = %v, want %v", err, sampleErr)
	}
	if len(outputs) != 2 {
		t.Fatalf("len(outputs) = %d, want 2", len(outputs))
	}
	if outputs[0] != 1 {
		t.Errorf("outputs[0] = %d, want 1", outputs[0])
	}
}

func TestResultTaskGroupWaitWithMultipleErrors(t *testing.T) {
	pool := pond.NewResultPool[int](10)
	group := pool.NewGroup()
	sampleErr := errors.New("sample error")

	var wg sync.WaitGroup
	wg.Add(5)

	for i := range 5 {
		i := i
		group.SubmitErr(func() (int, error) {
			wg.Done()
			wg.Wait()
			if i%2 == 0 {
				time.Sleep(10 * time.Millisecond)
				return 0, sampleErr
			}
			return i, nil
		})
	}

	outputs, err := group.Wait()
	if err != sampleErr {
		t.Errorf("Wait() err = %v, want %v", err, sampleErr)
	}
	if len(outputs) != 5 {
		t.Fatalf("len(outputs) = %d, want 5", len(outputs))
	}
}

func TestResultTaskGroupWaitWithContextCanceledAndOngoingTasks(t *testing.T) {
	pool := pond.NewResultPool[string](1)
	ctx, cancel := context.WithCancel(context.Background())
	group := pool.NewGroupContext(ctx)

	group.Submit(func() string {
		cancel()
		time.Sleep(10 * time.Millisecond)
		return "output1"
	})
	group.Submit(func() string {
		time.Sleep(10 * time.Millisecond)
		return "output2"
	})

	results, err := group.Wait()
	if err != context.Canceled {
		t.Errorf("Wait() err = %v, want context.Canceled", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
}

func TestTaskGroupWaitWithContextCanceledAndOngoingTasks(t *testing.T) {
	pool := pond.NewPool(1)
	var executedCount atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	group := pool.NewGroupContext(ctx)

	group.Submit(func() {
		cancel()
		time.Sleep(10 * time.Millisecond)
		executedCount.Add(1)
	})
	group.Submit(func() {
		time.Sleep(10 * time.Millisecond)
		executedCount.Add(1)
	})

	err := group.Wait()
	if err != context.Canceled {
		t.Errorf("Wait() err = %v, want context.Canceled", err)
	}
	if executedCount.Load() != 1 {
		t.Errorf("executedCount = %d, want 1", executedCount.Load())
	}
}

func TestTaskGroupWithStoppedPool(t *testing.T) {
	pool := pond.NewPool(100)
	pool.StopAndWait()

	err := pool.NewGroup().Submit(func() {}).Wait()
	if err != pond.ErrPoolStopped {
		t.Errorf("Wait() err = %v, want ErrPoolStopped", err)
	}
}

func TestTaskGroupWithContextCanceled(t *testing.T) {
	pool := pond.NewPool(100)
	group := pool.NewGroup()
	ctx, cancel := context.WithCancel(context.Background())
	taskStarted := make(chan struct{})

	task := group.SubmitErr(func() error {
		taskStarted <- struct{}{}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	})

	<-taskStarted
	cancel()

	err := task.Wait()
	if err != context.Canceled {
		t.Errorf("Wait() err = %v, want context.Canceled", err)
	}
}

func TestTaskGroupWithNoTasks(t *testing.T) {
	group := pond.NewResultPool[int](10).NewGroup()

	results, err := group.Submit().Wait()
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}

	results, err = group.SubmitErr().Wait()
	if err != nil {
		t.Errorf("Wait() err = %v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestTaskGroupCanceledShouldSkipRemainingTasks(t *testing.T) {
	pool := pond.NewPool(1)
	group := pool.NewGroup()
	var executedCount atomic.Int32
	sampleErr := errors.New("sample error")

	group.Submit(func() { executedCount.Add(1) })
	group.SubmitErr(func() error {
		time.Sleep(10 * time.Millisecond)
		return sampleErr
	})
	group.Submit(func() { executedCount.Add(1) })

	err := group.Wait()
	if err != sampleErr {
		t.Errorf("Wait() err = %v, want %v", err, sampleErr)
	}
	if executedCount.Load() != 1 {
		t.Errorf("executedCount = %d, want 1", executedCount.Load())
	}
}

func TestTaskGroupWithCustomContext(t *testing.T) {
	pool := pond.NewPool(1)
	ctx, cancel := context.WithCancel(context.Background())
	group := pool.NewGroupContext(ctx)
	var executedCount atomic.Int32

	group.Submit(func() { executedCount.Add(1) })
	group.Submit(func() {
		executedCount.Add(1)
		cancel()
	})
	group.Submit(func() { executedCount.Add(1) })

	err := group.Wait()
	if err != context.Canceled {
		t.Errorf("Wait() err = %v, want context.Canceled", err)
	}
	<-group.Done()
	if executedCount.Load() != 2 {
		t.Errorf("executedCount = %d, want 2", executedCount.Load())
	}
}

func TestTaskGroupStop(t *testing.T) {
	pool := pond.NewPool(1)
	group := pool.NewGroup()
	var executedCount atomic.Int32

	group.Submit(func() { executedCount.Add(1) })
	group.Submit(func() {
		executedCount.Add(1)
		group.Stop()
	})
	group.Submit(func() { executedCount.Add(1) })

	err := group.Wait()
	if err != pond.ErrGroupStopped {
		t.Errorf("Wait() err = %v, want ErrGroupStopped", err)
	}
	<-group.Done()
	if executedCount.Load() != 2 {
		t.Errorf("executedCount = %d, want 2", executedCount.Load())
	}
}

func TestTaskGroupDone(t *testing.T) {
	pool := pond.NewPool(10)
	group := pool.NewGroup()
	var executedCount atomic.Int32

	for range 5 {
		group.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	<-group.Done()
	if executedCount.Load() != 5 {
		t.Errorf("executedCount = %d, want 5", executedCount.Load())
	}
}

func TestTaskGroupMetrics(t *testing.T) {
	pool := pond.NewPool(1)
	group := pool.NewGroup()

	for i := 0; i < 9; i++ {
		group.Submit(func() { time.Sleep(1 * time.Millisecond) })
	}

	sampleErr := errors.New("sample error")
	group.SubmitErr(func() error {
		time.Sleep(1 * time.Millisecond)
		return sampleErr
	})

	err := group.Wait()
	time.Sleep(10 * time.Millisecond)

	if err != sampleErr {
		t.Errorf("Wait() err = %v, want %v", err, sampleErr)
	}
	if pool.SubmittedTasks() != 10 {
		t.Errorf("SubmittedTasks() = %d, want 10", pool.SubmittedTasks())
	}
	if pool.SuccessfulTasks() != 9 {
		t.Errorf("SuccessfulTasks() = %d, want 9", pool.SuccessfulTasks())
	}
	if pool.FailedTasks() != 1 {
		t.Errorf("FailedTasks() = %d, want 1", pool.FailedTasks())
	}
}

func TestTaskGroupMetricsWithCancelledContext(t *testing.T) {
	pool := pond.NewPool(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	group := pool.NewGroupContext(ctx)

	for i := range 10 {
		i := i
		group.Submit(func() {
			time.Sleep(20 * time.Millisecond)
			if i == 4 {
				cancel()
			}
		})
	}

	err := group.Wait()
	time.Sleep(10 * time.Millisecond)

	if err != context.Canceled {
		t.Errorf("Wait() err = %v, want context.Canceled", err)
	}
	if pool.SubmittedTasks() != 10 {
		t.Errorf("SubmittedTasks() = %d, want 10", pool.SubmittedTasks())
	}
	if pool.SuccessfulTasks() != 5 {
		t.Errorf("SuccessfulTasks() = %d, want 5", pool.SuccessfulTasks())
	}
	if pool.FailedTasks() != 5 {
		t.Errorf("FailedTasks() = %d, want 5", pool.FailedTasks())
	}
}

func TestTaskGroupWaitingTasks(t *testing.T) {
	pool := pond.NewPool(10)
	group := pool.NewGroup()

	start := make(chan struct{})
	end := make(chan struct{})

	for range 20 {
		group.Submit(func() {
			<-start
			<-end
		})
	}

	for i := 0; i < 10; i++ {
		start <- struct{}{}
	}

	if pool.RunningWorkers() != 10 {
		t.Errorf("RunningWorkers() = %d, want 10", pool.RunningWorkers())
	}
	if pool.WaitingTasks() != 10 {
		t.Errorf("WaitingTasks() = %d, want 10", pool.WaitingTasks())
	}

	close(start)
	close(end)
	group.Wait()
}

func TestTaskGroupContext(t *testing.T) {
	pool := pond.NewPool(2)
	group := pool.NewGroupContext(context.Background())

	var tasksRunning sync.WaitGroup
	tasksRunning.Add(2)

	group.SubmitErr(func() error {
		tasksRunning.Done()
		tasksRunning.Wait()
		return errors.New("first task error")
	})

	group.SubmitErr(func() error {
		tasksRunning.Done()
		tasksRunning.Wait()
		select {
		case <-group.Context().Done():
			return errors.New("second task canceled")
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	})

	group.Submit(func() {})

	err := group.Wait()
	pool.StopAndWait()

	if err == nil || err.Error() != "first task error" {
		t.Errorf("Wait() err = %v, want 'first task error'", err)
	}
	<-group.Done()
	<-group.Context().Done()
	if pool.SubmittedTasks() != 3 {
		t.Errorf("SubmittedTasks() = %d, want 3", pool.SubmittedTasks())
	}
	if pool.SuccessfulTasks() != 0 {
		t.Errorf("SuccessfulTasks() = %d, want 0", pool.SuccessfulTasks())
	}
	if pool.FailedTasks() != 3 {
		t.Errorf("FailedTasks() = %d, want 3", pool.FailedTasks())
	}
}
