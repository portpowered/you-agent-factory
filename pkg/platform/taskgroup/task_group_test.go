package taskgroup

import (
	"errors"
	"testing"
	"time"
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

// TestGroupFailedClosesAsSoonAsOneTaskFailsWhileAnotherIsStillRunning proves
// Failed() reports a failure before every tracked task has returned, not
// only once Wait() itself would unblock -- the exact capability
// serveConnection needs to react to a dispatched "session/prompt" write
// failure while its own read loop may still be blocked in an unrelated
// read.
func TestGroupFailedClosesAsSoonAsOneTaskFailsWhileAnotherIsStillRunning(t *testing.T) {
	var g Group
	boom := errors.New("boom")
	release := make(chan struct{})
	stillRunning := make(chan struct{})

	g.Go(func() error {
		<-release
		close(stillRunning)
		return nil
	})
	g.Go(func() error { return boom })

	select {
	case <-g.Failed():
	case <-time.After(5 * time.Second):
		t.Fatal("Failed() never closed after a task returned an error")
	}
	if err := g.Err(); !errors.Is(err, boom) {
		t.Fatalf("Err() = %v, want the failing task's own error", err)
	}

	select {
	case <-stillRunning:
		t.Fatal("the other task completed before Failed() was observed, want it still in flight")
	default:
	}
	close(release)

	if err := g.Wait(); !errors.Is(err, boom) {
		t.Fatalf("Wait() error = %v, want the same failing task's error", err)
	}
}

// TestGroupFailedNeverClosesWhenEveryTaskSucceeds proves Failed() stays open
// for a group whose tasks all succeed, so a caller selecting on it never
// mistakes ordinary completion for a failure.
func TestGroupFailedNeverClosesWhenEveryTaskSucceeds(t *testing.T) {
	var g Group
	g.Go(func() error { return nil })
	g.Go(func() error { return nil })
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}

	select {
	case <-g.Failed():
		t.Fatal("Failed() closed, want it to stay open when no task ever failed")
	default:
	}
	if err := g.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil when no task ever failed", err)
	}
}

// TestGroupFailedIsSafeBeforeAnyGoCall proves Failed() can be called on a
// zero-value Group with no tasks tracked yet, and reports no failure.
func TestGroupFailedIsSafeBeforeAnyGoCall(t *testing.T) {
	var g Group
	select {
	case <-g.Failed():
		t.Fatal("Failed() closed for a group with no tracked tasks")
	default:
	}
}

// TestGroupDoneClosesAfterEveryTaskReturnsRegardlessOfOutcome proves Done()
// closes once every Go call made before it has returned, whether or not any
// of them failed, and does so without Wait() itself ever being called
// first -- the non-blocking completion signal serveConnection selects on
// alongside another Group's Failed().
func TestGroupDoneClosesAfterEveryTaskReturnsRegardlessOfOutcome(t *testing.T) {
	var g Group
	release := make(chan struct{})
	boom := errors.New("boom")
	g.Go(func() error { <-release; return nil })
	g.Go(func() error { <-release; return boom })

	done := g.Done()
	select {
	case <-done:
		t.Fatal("Done() closed before its tracked tasks returned")
	default:
	}

	close(release)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Done() never closed after every tracked task returned")
	}
	if err := g.Wait(); !errors.Is(err, boom) {
		t.Fatalf("Wait() error = %v, want the recorded failure", err)
	}
}

// TestGroupDoneClosesImmediatelyForAGroupWithNoTasks proves Done() closes
// right away for a zero-value Group that never ran a task, matching Wait's
// own immediate-success behavior for the same case.
func TestGroupDoneClosesImmediatelyForAGroupWithNoTasks(t *testing.T) {
	var g Group
	select {
	case <-g.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done() never closed for a group with no tracked tasks")
	}
}
