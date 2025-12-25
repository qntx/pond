package pond_test

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qntx/pond"
)

func TestSubmit(t *testing.T) {
	done := make(chan int, 1)
	task := pond.Submit(func() {
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

func TestSubmitWithError(t *testing.T) {
	task := pond.SubmitErr(func() error {
		return errors.New("sample error")
	})

	err := task.Wait()
	if err == nil || err.Error() != "sample error" {
		t.Errorf("Wait() = %v, want 'sample error'", err)
	}
}

func TestSubmitWithPanic(t *testing.T) {
	task := pond.Submit(func() {
		panic("dummy panic")
	})

	err := task.Wait()
	if !errors.Is(err, pond.ErrPanic) {
		t.Errorf("Wait() = %v, want ErrPanic", err)
	}
	if !strings.HasPrefix(err.Error(), "task panicked: dummy panic") {
		t.Errorf("err.Error() = %q, want prefix 'task panicked: dummy panic'", err.Error())
	}
}

func TestNewGroup(t *testing.T) {
	group := pond.NewGroup()

	count := 10
	var done atomic.Int32

	for i := 0; i < count; i++ {
		group.SubmitErr(func() error {
			done.Add(1)
			return nil
		})
	}

	err := group.Wait()
	if err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
	if int(done.Load()) != count {
		t.Errorf("done = %d, want %d", done.Load(), count)
	}
}

func TestNewSubpool(t *testing.T) {
	pool := pond.NewSubpool(10)

	count := 10
	var done atomic.Int32

	for i := 0; i < count; i++ {
		pool.SubmitErr(func() error {
			done.Add(1)
			return nil
		})
	}

	pool.StopAndWait()

	if int(done.Load()) != count {
		t.Errorf("done = %d, want %d", done.Load(), count)
	}
}
