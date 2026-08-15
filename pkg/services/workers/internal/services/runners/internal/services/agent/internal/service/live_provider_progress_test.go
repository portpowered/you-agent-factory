package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// streamingProvidersFake reports its progress facts live through the attempt's
// ProgressObserver and then returns those same facts flagged as already
// observed, exactly as the ACP provider does for a session/prompt turn.
type streamingProvidersFake struct {
	providers.Service
	facts []providers.ExecuteProgress
	// sampleMidAttempt, when set, is called after the facts have been
	// observed but before Execute returns, so a test can prove publication
	// already happened while the attempt was still running.
	sampleMidAttempt     func()
	publishedBeforeCount int
}

func (fake *streamingProvidersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	for _, fact := range fake.facts {
		request.ObserveProgress(fact)
	}
	if fake.sampleMidAttempt != nil {
		fake.sampleMidAttempt()
	}
	return providers.ExecuteResult{
		Content: "final answer",
		Diagnostics: &providers.ExecuteDiagnostics{
			Progress:                fake.facts,
			ProgressAlreadyObserved: true,
		},
	}, nil
}

// bufferingProvidersFake returns its facts only in diagnostics, as the native
// subprocess adapters do.
type bufferingProvidersFake struct {
	providers.Service
	facts []providers.ExecuteProgress
}

func (fake *bufferingProvidersFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{
		Content:     "final answer",
		Diagnostics: &providers.ExecuteDiagnostics{Progress: fake.facts},
	}, nil
}

func liveProgressFixture() []providers.ExecuteProgress {
	return []providers.ExecuteProgress{
		{Phase: "started", Metadata: map[string]string{"kind": "run"}},
		{Phase: "delta", Detail: "one", Metadata: map[string]string{"kind": "message", "item_id": "m1"}},
		{Phase: "delta", Detail: "two", Metadata: map[string]string{"kind": "message", "item_id": "m1"}},
		{Phase: "completed", Metadata: map[string]string{"kind": "run"}},
	}
}

func runOneAttempt(t *testing.T, service providers.Service) []workers.ProgressFragment {
	t.Helper()
	var published []workers.ProgressFragment
	runner, err := New(service, func(fragment workers.ProgressFragment) {
		published = append(published, fragment)
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-progress-1",
			WorkerType:      "goal-executor",
			WorkstationName: "execute-goal",
		},
		RunnerID:     string(providers.IDCodex),
		WorkerType:   "goal-executor",
		SystemPrompt: "system",
		UserMessage:  "user",
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return published
}

func progressPayloads(fragments []workers.ProgressFragment) []string {
	var payloads []string
	for _, fragment := range fragments {
		if fragment.Kind == workers.ProgressFragmentKind {
			payloads = append(payloads, fragment.Type+"|"+fragment.Payload)
		}
	}
	return payloads
}

// TestExecutePublishesStreamedProviderProgressExactlyOnce is the
// anti-duplication proof at the runner boundary. A provider that already
// delivered its facts live must not have ExecuteDiagnostics.Progress replayed,
// or a customer sees the Worker's whole trace twice.
func TestExecutePublishesStreamedProviderProgressExactlyOnce(t *testing.T) {
	t.Parallel()

	facts := liveProgressFixture()
	published := runOneAttempt(t, &streamingProvidersFake{facts: facts})

	got := progressPayloads(published)
	// The streamed facts appear once each, followed by the authoritative
	// terminal message synthesized from the attempt's assembled content. That
	// terminal fragment is not a republication: the streamed facts carry
	// message deltas, never a "message.completed" phase.
	want := []string{
		"started|",
		"delta|one",
		"delta|two",
		"completed|",
		"message.completed|final answer",
	}
	if len(got) != len(want) {
		t.Fatalf("published progress = %v, want exactly the %d streamed facts once each", got, len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("published progress[%d] = %q, want %q (full: %v)", index, got[index], want[index], got)
		}
	}
}

// TestExecuteStillPublishesBufferedProviderProgress proves the guard is scoped
// to providers that actually streamed: an adapter that only returns
// diagnostics keeps its existing publication path.
func TestExecuteStillPublishesBufferedProviderProgress(t *testing.T) {
	t.Parallel()

	published := runOneAttempt(t, &bufferingProvidersFake{facts: liveProgressFixture()})

	got := progressPayloads(published)
	if len(got) < len(liveProgressFixture()) {
		t.Fatalf("published progress = %v, want every buffered fact published", got)
	}
}

// TestExecuteWithoutProviderSessionReferenceStillCarriesProviderIdentity
// proves a provider can leave the resumable session reference absent without
// making its normalized output provenance disappear with it.
func TestExecuteWithoutProviderSessionReferenceStillCarriesProviderIdentity(t *testing.T) {
	t.Parallel()

	published := runOneAttempt(t, &bufferingProvidersFake{facts: liveProgressFixture()})
	if len(published) == 0 {
		t.Fatal("published progress = empty, want provider output and terminal observation")
	}
	for index, fragment := range published {
		if fragment.Provider != string(providers.IDCodex) {
			t.Fatalf("published[%d].Provider = %q, want codex", index, fragment.Provider)
		}
		if fragment.Continuation != nil {
			t.Fatalf("published[%d] = %#v, want no synthesized continuation", index, fragment)
		}
	}
}

// TestExecutePublishesProviderProgressBeforeTheAttemptReturns proves the facts
// reach the publisher while the provider attempt is still running, which is
// the behavior a live Worker trace depends on. A buffered provider cannot pass
// this: at the moment Execute is still inside the provider call it has
// published nothing.
func TestExecutePublishesProviderProgressBeforeTheAttemptReturns(t *testing.T) {
	t.Parallel()

	var published []workers.ProgressFragment
	fake := &streamingProvidersFake{facts: liveProgressFixture()}
	fake.sampleMidAttempt = func() { fake.publishedBeforeCount = len(published) }

	runner, err := New(fake, func(fragment workers.ProgressFragment) {
		published = append(published, fragment)
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-progress-2",
			WorkerType:      "goal-executor",
			WorkstationName: "execute-goal",
		},
		RunnerID:     string(providers.IDCodex),
		WorkerType:   "goal-executor",
		SystemPrompt: "system",
		UserMessage:  "user",
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if fake.publishedBeforeCount != len(fake.facts) {
		t.Fatalf("published %d fragments while the attempt was still running, want all %d streamed facts",
			fake.publishedBeforeCount, len(fake.facts))
	}
}

// TestExecutePublishesNothingMidAttemptForABufferedProvider is the negative
// control: it pins that the assertion above genuinely distinguishes streamed
// from buffered delivery rather than passing for any provider.
func TestExecutePublishesNothingMidAttemptForABufferedProvider(t *testing.T) {
	t.Parallel()

	var published []workers.ProgressFragment
	var midAttempt int
	fake := &midAttemptBufferingFake{
		facts:  liveProgressFixture(),
		sample: func() { midAttempt = len(published) },
	}
	runner, err := New(fake, func(fragment workers.ProgressFragment) {
		published = append(published, fragment)
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-progress-3",
			WorkerType:      "goal-executor",
			WorkstationName: "execute-goal",
		},
		RunnerID:     string(providers.IDCodex),
		WorkerType:   "goal-executor",
		SystemPrompt: "system",
		UserMessage:  "user",
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if midAttempt != 0 {
		t.Fatalf("buffered provider published %d fragments mid-attempt, want 0", midAttempt)
	}
	if len(published) == 0 {
		t.Fatal("buffered provider published nothing at all")
	}
}

type midAttemptBufferingFake struct {
	providers.Service
	facts  []providers.ExecuteProgress
	sample func()
}

func (fake *midAttemptBufferingFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if fake.sample != nil {
		fake.sample()
	}
	return providers.ExecuteResult{
		Content:     "final answer",
		Diagnostics: &providers.ExecuteDiagnostics{Progress: fake.facts},
	}, nil
}
