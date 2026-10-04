package process_time_test

import (
	"testing"
	"time"
)

func TestJourneySchedulerSeparatesReadinessAndSourceWaits(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	scheduler := newJourneyScheduler(base)
	deadline := scheduler.NewTimer(time.Hour)
	defer deadline.Stop()
	readiness := scheduler.NewTimer(10 * time.Millisecond)
	select {
	case <-readiness.C():
	default:
		t.Fatal("readiness poll was not driven")
	}
	if !scheduler.Now().Equal(base) {
		t.Fatal("readiness advanced selected scheduling time")
	}
	channel := scheduler.After(10 * time.Second)
	wait := scheduler.await(t, 10*time.Second)
	if wait.channel != channel {
		t.Fatal("registered wait lost channel identity")
	}
	scheduler.SetTick(9)
	assertSourceWaitHeld(t, wait)
	scheduler.SetTick(10)
	select {
	case <-channel:
	default:
		t.Fatal("source deadline did not fire")
	}
	select {
	case <-deadline.C():
		t.Fatal("source advancement fired unrelated deadline")
	default:
	}
}
