package events

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestFactoryEventHistory_EventRecorderReadsDuringOrderedDelivery(t *testing.T) {
	for _, operation := range []string{"replay", "type replay", "ready", "append", "validation"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			history := newTestFactoryEventHistory(nil, time.Now)
			history.RecordRunRequest()
			entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			gate := func() { once.Do(func() { close(entered); <-release }) }
			go func() {
				switch operation {
				case "replay":
					history.AddEventRecorder(func(interfaces.FactoryEvent) { gate() })
				case "type replay":
					history.AddEventTypeRecorder(func(interfaces.FactoryEventType) { gate() })
				case "ready":
					history.AddEventTypeRecorderWithReady(func(interfaces.FactoryEventType) {}, gate)
				case "append":
					history.AddEventRecorder(func(event interfaces.FactoryEvent) {
						if event.Context.Sequence == 1 {
							gate()
						}
					})
					history.RecordInitialStructure()
				case "validation":
					_, _ = history.AppendRecordedEventWithValidation(interfaces.FactoryEvent{Id: "validation", Type: interfaces.FactoryEventTypeRunResponse, Payload: []byte(`{}`)}, func(interfaces.FactoryEvent) error { gate(); return fmt.Errorf("rejected") })
				}
				close(finished)
			}()
			defer func() { close(release); <-finished }()
			awaitHistorySignal(t, entered)
			readDone := make(chan struct{})
			go func() {
				defer close(readDone)
				if history.StreamGenerationID() == "" {
					t.Error("lost generation")
				}
				if _, err := history.CurrentSessionProjectionFacts(); err != nil {
					t.Error(err)
				}
				prefix := history.CanonicalEvents()
				stream, err := history.Subscribe(t.Context(), nil, interfaces.FactoryEventReconnectScope{})
				if err != nil {
					t.Error(err)
					return
				}
				if len(prefix) != len(stream.History) {
					t.Error("incoherent prefix")
				}
				prefix[0].Payload[0] = 'X'
				stream.History[0].Payload[0] = 'Y'
				if history.CanonicalEvents()[0].Payload[0] != '{' {
					t.Error("read mutated canonical payload")
				}
			}()
			awaitHistorySignal(t, readDone)
		})
	}
}

func TestFactoryEventHistory_EventRecorderReplayHandoffAndConcurrentProvenance(t *testing.T) {
	history := newTestFactoryEventHistory(nil, time.Now)
	secret := []recordings.RecordingSecret{{JSONPointer: "/private", Provenance: recordings.RecordingSecretProvenanceDeclared}}
	appendSecret := func(id string) {
		history.appendEventWithProvenance(interfaces.FactoryEvent{Id: id, Type: interfaces.FactoryEventTypeFactoryStateResponse, Payload: []byte(`{}`)}, secret)
	}
	appendSecret("prefix")
	entered, release, registered := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var observed []string
	go func() {
		history.AddEventRecorder(func(event interfaces.FactoryEvent) {
			if event.Id == "prefix" {
				close(entered)
				<-release
			}
			provenance := history.SecretProvenanceDuringAppend(event)
			if !reflect.DeepEqual(provenance, secret) {
				t.Errorf("provenance for %s = %#v", event.Id, provenance)
			}
			provenance[0].JSONPointer = "/mutation"
			if !reflect.DeepEqual(history.SecretProvenanceForEvent(event), secret) {
				t.Error("provenance mutation reached canonical state")
			}
			observed = append(observed, event.Id)
		})
		close(registered)
	}()
	awaitHistorySignal(t, entered)
	ctx, cancel := context.WithCancel(t.Context())
	stream, err := history.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	appended := make(chan struct{})
	go func() { appendSecret("live"); close(appended) }()
	// These reads overlap both the held replay and an attempted append.
	if !reflect.DeepEqual(history.SecretProvenanceDuringAppend(stream.History[0]), secret) {
		t.Error("prefix provenance unavailable during replay")
	}
	close(release)
	awaitHistorySignal(t, registered)
	awaitHistorySignal(t, appended)
	select {
	case event := <-stream.Events:
		if event.Id != "live" || event.Context.Sequence != 1 {
			t.Errorf("live event = %#v", event)
		}
	case <-time.After(5 * time.Second): //nolint:testsleep // Failure ceiling for an event handoff; not synchronization.
		t.Fatal("missing live handoff")
	}
	if !reflect.DeepEqual(observed, []string{"prefix", "live"}) {
		t.Fatalf("recorder order = %v", observed)
	}
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		for range 100 {
			for _, event := range history.CanonicalEvents() {
				_ = history.SecretProvenanceDuringAppend(event)
				_, _ = history.CurrentSessionProjectionFacts()
			}
		}
	}()
	for index := range 20 {
		appendSecret(fmt.Sprintf("concurrent-%d", index))
	}
	readers.Wait()
}

func awaitHistorySignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second): //nolint:testsleep // Failure ceiling for progress while the recorder gate remains held.
		t.Fatal("history operation did not complete before releasing recorder gate")
	}
}
