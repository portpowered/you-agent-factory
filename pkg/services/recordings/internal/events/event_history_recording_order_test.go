package events

import (
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// TestFactoryEventHistory_EventRecorderFollowsCanonicalAppendOrder protects
// the ordering contract consumed by the portable recording writer. The
// callback for the first committed event is held while a second append is
// attempted; the second append must not reach the recorder until the first
// callback has returned.
func TestFactoryEventHistory_EventRecorderFollowsCanonicalAppendOrder(t *testing.T) {
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
	)
	firstCallbackStarted := make(chan struct{})
	releaseFirstCallback := make(chan struct{})
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	var recordedMu sync.Mutex
	var recorded []int
	history.AddEventRecorder(func(event interfaces.FactoryEvent) {
		if event.Context.Sequence == 0 {
			close(firstCallbackStarted)
			<-releaseFirstCallback
		}
		recordedMu.Lock()
		recorded = append(recorded, event.Context.Sequence)
		recordedMu.Unlock()
	})

	go func() {
		history.RecordFactoryStateChange(
			1, interfaces.FactoryStateIdle, interfaces.FactoryStateRunning,
			"first", time.Unix(1, 0).UTC(),
		)
		close(firstDone)
	}()
	select {
	case <-firstCallbackStarted:
	case <-time.After(time.Second):
		t.Fatal("first event recorder callback did not start")
	}

	go func() {
		history.RecordFactoryStateChange(
			2, interfaces.FactoryStateIdle, interfaces.FactoryStateRunning,
			"second", time.Unix(2, 0).UTC(),
		)
		close(secondDone)
	}()
	select {
	case <-secondDone:
		close(releaseFirstCallback)
		<-firstDone
		t.Fatal("second append reached the recorder before the first callback returned")
	case <-time.After(time.Second):
	}

	close(releaseFirstCallback)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first append did not finish after releasing its recorder callback")
	}
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second append did not finish after releasing the first callback")
	}

	recordedMu.Lock()
	defer recordedMu.Unlock()
	if len(recorded) != 2 || recorded[0] != 0 || recorded[1] != 1 {
		t.Fatalf("recorded callback sequence = %#v, want [0 1]", recorded)
	}
}
