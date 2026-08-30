package runtime

import (
	"errors"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontext "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestRuntimeCompletionDurablyClosesSourceForSuccessorMetrics(t *testing.T) {
	history := newCompletionTestHistory(t)
	completionPublished := 0
	history.AddDeferredSessionCompletionRecorder(func() { completionPublished++ })

	var flushed [][]recordings.FactoryEvent
	factory := completionTestFactory(history, func() error {
		flushed = append(flushed, history.CanonicalEvents())
		return nil
	})

	if err := recordSessionLifecycleCompletionFromFactory(
		factory, 2, testCompletionFactoryState(), "", completionTestTime(),
	); err != nil {
		t.Fatalf("record terminal source lifecycle: %v", err)
	}
	if completionPublished != 0 {
		t.Fatalf("completion callbacks before durability publication = %d, want 0", completionPublished)
	}
	if len(flushed) != 2 {
		t.Fatalf("durability flushes = %d, want result and completion flushes", len(flushed))
	}
	if countCompletionEvents(flushed[0]) != 0 {
		t.Fatalf("pre-completion durable snapshot contains SESSION_COMPLETED: %#v", flushed[0])
	}
	if countCompletionEvents(flushed[1]) != 1 {
		t.Fatalf("post-completion durable snapshot has %d SESSION_COMPLETED events, want 1", countCompletionEvents(flushed[1]))
	}

	publishDeferredSessionCompletion(history)
	publishDeferredSessionCompletion(history)
	if completionPublished != 1 {
		t.Fatalf("completion callbacks = %d, want exactly one", completionPublished)
	}
	if countCompletionEvents(history.CanonicalEvents()) != 1 {
		t.Fatalf("in-memory SESSION_COMPLETED count = %d, want exactly one", countCompletionEvents(history.CanonicalEvents()))
	}
}

func TestRuntimeCompletionFlushFailureLeavesSourceIncompleteAndRetryable(t *testing.T) {
	history := newCompletionTestHistory(t)
	completionPublished := 0
	history.AddDeferredSessionCompletionRecorder(func() { completionPublished++ })
	flushErr := errors.New("durable source flush failed")
	flushCalls := 0
	factory := completionTestFactory(history, func() error {
		flushCalls++
		if flushCalls == 1 {
			return flushErr
		}
		return nil
	})

	err := recordSessionLifecycleCompletionFromFactory(
		factory, 2, testCompletionFactoryState(), "", completionTestTime(),
	)
	if !errors.Is(err, flushErr) {
		t.Fatalf("first completion error = %v, want flush cause", err)
	}
	if countCompletionEvents(history.CanonicalEvents()) != 0 || completionPublished != 0 {
		t.Fatalf("failed completion advertised close: events=%d callbacks=%d", countCompletionEvents(history.CanonicalEvents()), completionPublished)
	}

	if err := recordSessionLifecycleCompletionFromFactory(
		factory, 2, testCompletionFactoryState(), "", completionTestTime(),
	); err != nil {
		t.Fatalf("retry terminal source lifecycle: %v", err)
	}
	publishDeferredSessionCompletion(history)
	if countCompletionEvents(history.CanonicalEvents()) != 1 || completionPublished != 1 {
		t.Fatalf("retry completion = events:%d callbacks:%d, want one durable close and callback", countCompletionEvents(history.CanonicalEvents()), completionPublished)
	}
}

func TestRuntimeCompletionConcurrentCallbacksRemainExactlyOnce(t *testing.T) {
	history := newCompletionTestHistory(t)
	factory := completionTestFactory(history, nil)
	const callbacks = 24
	var group sync.WaitGroup
	group.Add(callbacks)
	for index := 0; index < callbacks; index++ {
		go func() {
			defer group.Done()
			if err := recordSessionLifecycleCompletionFromFactory(
				factory, 2, testCompletionFactoryState(), "", completionTestTime(),
			); err != nil {
				t.Errorf("concurrent completion callback: %v", err)
			}
		}()
	}
	group.Wait()

	if got := countCompletionEvents(history.CanonicalEvents()); got != 1 {
		t.Fatalf("concurrent SESSION_COMPLETED count = %d, want exactly one", got)
	}
}

type completionTestLedger struct {
	recordings.RuntimeLedger
	mu             sync.Mutex
	events         []recordings.FactoryEvent
	completionJobs []func()
	pending        bool
	published      bool
}

func (ledger *completionTestLedger) CanonicalEvents() []recordings.FactoryEvent {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return append([]recordings.FactoryEvent(nil), ledger.events...)
}

func (ledger *completionTestLedger) RecordSessionLifecycleResultUpdated(
	string, *interfaces.FactoryConfig, int, interfaces.FactoryState, string, time.Time,
) {
	ledger.mu.Lock()
	ledger.events = append(ledger.events, recordings.FactoryEvent{
		Type: interfaces.FactoryEventTypeSessionResultUpdated,
	})
	ledger.mu.Unlock()
}

func (ledger *completionTestLedger) RecordSessionLifecycleCompleted(
	string, *interfaces.FactoryConfig, int, interfaces.FactoryState, string, time.Time,
) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for _, event := range ledger.events {
		if event.Type == interfaces.FactoryEventTypeSessionCompleted {
			return
		}
	}
	ledger.events = append(ledger.events, recordings.FactoryEvent{
		Type: interfaces.FactoryEventTypeSessionCompleted,
	})
	ledger.pending = true
}

func (ledger *completionTestLedger) AddDeferredSessionCompletionRecorder(recorder func()) {
	if recorder == nil {
		return
	}
	ledger.mu.Lock()
	ledger.completionJobs = append(ledger.completionJobs, recorder)
	if countCompletionEvents(ledger.events) > 0 {
		ledger.pending = true
	}
	ledger.mu.Unlock()
}

func (ledger *completionTestLedger) PublishDeferredSessionCompletion() {
	ledger.mu.Lock()
	if !ledger.pending || ledger.published {
		ledger.mu.Unlock()
		return
	}
	ledger.published = true
	ledger.pending = false
	jobs := append([]func(){}, ledger.completionJobs...)
	ledger.completionJobs = nil
	ledger.mu.Unlock()
	for _, job := range jobs {
		job()
	}
}

func newCompletionTestHistory(t *testing.T) *completionTestLedger {
	t.Helper()
	return &completionTestLedger{}
}

func completionTestFactory(history recordings.RuntimeLedger, flush func() error) *factoryImpl {
	return &factoryImpl{
		cfg: &runtimeConfig{
			workflowContext: &factorycontext.FactoryContext{SessionID: "source-session"},
		},
		eventHistory:             history,
		completionDurabilityGate: flush,
	}
}

func testCompletionFactoryState() interfaces.FactoryState {
	return interfaces.FactoryStateCompleted
}

func completionTestTime() time.Time {
	return time.Date(2026, 8, 29, 2, 0, 0, 0, time.UTC)
}

func countCompletionEvents(events []recordings.FactoryEvent) int {
	count := 0
	for _, event := range events {
		if event.Type == interfaces.FactoryEventTypeSessionCompleted {
			count++
		}
	}
	return count
}
