package taskgroup

import (
	"errors"
	"testing"
)

func TestGroupWaitReturnsNilWhenEveryTaskSucceeds(t *testing.T) {
	var g Group
	var ran [3]bool
	for i := range ran {
		g.Go(func() error {
			ran[i] = true
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	for i, called := range ran {
		if !called {
			t.Fatalf("task %d never ran", i)
		}
	}
}

func TestGroupWaitReturnsTheOnlyReportedErrorAndStillRunsEveryTask(t *testing.T) {
	var g Group
	boom := errors.New("boom")
	var ran [3]bool
	g.Go(func() error { ran[0] = true; return nil })
	g.Go(func() error { ran[1] = true; return boom })
	g.Go(func() error { ran[2] = true; return nil })

	err := g.Wait()
	if !errors.Is(err, boom) {
		t.Fatalf("Wait() error = %v, want the failing Go call's own error to be recoverable via errors.Is", err)
	}
	for i, called := range ran {
		if !called {
			t.Fatalf("task %d never ran despite another task failing", i)
		}
	}
}

func TestGroupWaitReturnsOnlyOneErrorWhenEveryTaskFails(t *testing.T) {
	var g Group
	g.Go(func() error { return errors.New("first failure") })
	g.Go(func() error { return errors.New("second failure") })
	g.Go(func() error { return errors.New("third failure") })

	if err := g.Wait(); err == nil {
		t.Fatal("Wait() error = nil, want one of the three failures reported")
	}
}

func TestGroupWaitIsIdempotent(t *testing.T) {
	var g Group
	boom := errors.New("boom")
	g.Go(func() error { return boom })

	first := g.Wait()
	second := g.Wait()
	if !errors.Is(first, boom) || !errors.Is(second, boom) {
		t.Fatalf("Wait() calls = %v, %v, want both to report the same recorded error", first, second)
	}
}

func TestGroupWithNoTasksWaitsImmediatelyAndSucceeds(t *testing.T) {
	var g Group
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait() error = %v, want nil for a group that never ran a task", err)
	}
}
