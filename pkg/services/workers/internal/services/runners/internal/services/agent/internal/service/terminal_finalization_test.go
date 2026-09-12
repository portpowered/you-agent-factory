package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestExecuteResultAndErrorSelectsCompletedTerminal(t *testing.T) {
	t.Parallel()

	failure := providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: "stream flush failed after the final answer",
		Diagnostics: &providers.ExecuteDiagnostics{
			Metadata: map[string]string{
				workers.ProviderResponseMetadataFailureStage: "flush",
			},
		},
	}
	envelopes := &recordingDecisionEnvelopeService{
		result: workers.WorkResult{
			Outcome: workers.OutcomeAccepted,
			Output:  "accepted output",
		},
	}
	provider := &resultErrorProvidersFake{
		result: providers.ExecuteResult{Content: `{"decision":"ACCEPTED"}`},
		err:    failure,
	}
	recorder := &progressFragmentRecorder{}
	runner, err := New(provider, recorder.publish, envelopes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := baseAgentRequest()
	request.DecisionEnvelope = true
	result, err := runner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v, want the usable result to win", err)
	}
	if result.Content != "accepted output" || result.Outcome != workers.OutcomeAccepted {
		t.Fatalf("result = %#v, want the accepted normalized result", result)
	}
	if calls := len(envelopes.snapshot()); calls != 1 {
		t.Fatalf("decision-envelope normalization calls = %d, want exactly one", calls)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[suppressedTerminalErrorKindMetadata] != string(providers.ExecuteFailureKindDependency) {
		t.Fatalf("result diagnostics = %#v, want bounded suppressed dependency kind", result.Diagnostics)
	}
	if result.Diagnostics.Metadata[workers.ProviderResponseMetadataFailureStage] != "flush" {
		t.Fatalf("result diagnostics = %#v, want bounded flush stage", result.Diagnostics)
	}

	fragments := recorder.snapshot()
	if countFragmentsByKind(fragments, workers.CompletedFragmentKind) != 1 {
		t.Fatalf("completed terminal fragments = %#v, want exactly one", fragments)
	}
	if countFragmentsByKind(fragments, workers.FailedFragmentKind) != 0 {
		t.Fatalf("failed terminal fragments = %#v, want none", fragments)
	}
	completed := fragmentByKind(fragments, workers.CompletedFragmentKind)
	if completed.Metadata[suppressedTerminalErrorKindMetadata] != string(providers.ExecuteFailureKindDependency) ||
		completed.Metadata[workers.ProviderResponseMetadataFailureStage] != "flush" {
		t.Fatalf("completed metadata = %#v, want bounded suppressed dependency/flush facts", completed.Metadata)
	}
}

func TestExecuteUnusableResultKeepsTypedFailureTerminal(t *testing.T) {
	t.Parallel()

	provider := &resultErrorProvidersFake{
		err: providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "provider failed without a result",
		},
	}
	recorder := &progressFragmentRecorder{}
	runner, err := New(provider, recorder.publish)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.Execute(t.Context(), baseAgentRequest())
	if err == nil {
		t.Fatal("Execute() error = nil, want the typed provider failure")
	}
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypeInternalServerError {
		t.Fatalf("ProviderError.Type = %q, want internal_server_error", providerErr.Type)
	}
	if result.Outcome != workers.OutcomeFailed {
		t.Fatalf("result.Outcome = %q, want FAILED", result.Outcome)
	}

	fragments := recorder.snapshot()
	if countFragmentsByKind(fragments, workers.CompletedFragmentKind) != 0 {
		t.Fatalf("completed terminal fragments = %#v, want none", fragments)
	}
	if countFragmentsByKind(fragments, workers.FailedFragmentKind) != 1 {
		t.Fatalf("failed terminal fragments = %#v, want exactly one", fragments)
	}
}

func TestExecuteMalformedResultPublishesFailureTerminal(t *testing.T) {
	t.Parallel()

	envelopes := &recordingDecisionEnvelopeService{
		result: workers.WorkResult{
			Outcome: workers.OutcomeFailed,
			Error:   "decision envelope is malformed",
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal,
				Type:   workers.WorkFailureTypeUnknown,
			},
		},
	}
	recorder := &progressFragmentRecorder{}
	runner, err := New(&providersFake{content: "not-json"}, recorder.publish, envelopes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.Execute(t.Context(), func() workers.RunnerExecutionRequest {
		request := baseAgentRequest()
		request.DecisionEnvelope = true
		return request
	}())
	if err == nil {
		t.Fatal("Execute() error = nil, want malformed decision-envelope failure")
	}
	if calls := len(envelopes.snapshot()); calls != 1 {
		t.Fatalf("decision-envelope normalization calls = %d, want exactly one", calls)
	}
	fragments := recorder.snapshot()
	if countFragmentsByKind(fragments, workers.CompletedFragmentKind) != 0 {
		t.Fatalf("completed terminal fragments = %#v, want none", fragments)
	}
	if countFragmentsByKind(fragments, workers.FailedFragmentKind) != 1 {
		t.Fatalf("failed terminal fragments = %#v, want exactly one", fragments)
	}
}

func TestTerminalFinalizationSuppressesConcurrentDuplicates(t *testing.T) {
	recorder := &progressFragmentRecorder{}
	publication := newTerminalPublication(recorder.publish)
	fragment := workers.ProgressFragment{
		Kind:              workers.CompletedFragmentKind,
		Type:              "COMPLETED",
		ExternalEventType: "STREAM_COMPLETED",
	}

	var waitGroup sync.WaitGroup
	for index := 0; index < 32; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			publication.terminal(fragment)
		}()
	}
	waitGroup.Wait()
	publication.progress(workers.ProgressFragment{Type: "late.progress"})

	fragments := recorder.snapshot()
	if len(fragments) != 1 || fragments[0].Kind != workers.CompletedFragmentKind {
		t.Fatalf("terminal publications = %#v, want one canonical completion and no late progress", fragments)
	}
}

func TestExecuteFinalizationIgnoresLateProviderProgress(t *testing.T) {
	t.Parallel()

	provider := &lateProgressProvidersFake{}
	recorder := &progressFragmentRecorder{}
	runner, err := New(provider, recorder.publish)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := runner.Execute(t.Context(), baseAgentRequest()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	observer := provider.progressObserver()
	if observer == nil {
		t.Fatal("provider progress observer = nil, want the runner callback seam")
	}
	observer(providers.ExecuteProgress{Phase: "late.progress", Detail: "must be ignored"})

	fragments := recorder.snapshot()
	for _, fragment := range fragments {
		if fragment.Type == "late.progress" {
			t.Fatalf("late provider progress was published: %#v", fragments)
		}
	}
	if countFragmentsByKind(fragments, workers.CompletedFragmentKind) != 1 {
		t.Fatalf("completed terminal fragments = %#v, want exactly one", fragments)
	}
}

type resultErrorProvidersFake struct {
	providers.Service
	result providers.ExecuteResult
	err    error
}

func (fake *resultErrorProvidersFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return fake.result.Clone(), fake.err
}

type lateProgressProvidersFake struct {
	providers.Service
	mu       sync.Mutex
	observer providers.ProgressObserver
}

func (fake *lateProgressProvidersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.mu.Lock()
	fake.observer = request.ProgressObserver
	fake.mu.Unlock()
	return providers.ExecuteResult{Content: "accepted output"}, nil
}

func (fake *lateProgressProvidersFake) progressObserver() providers.ProgressObserver {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.observer
}

type progressFragmentRecorder struct {
	mu        sync.Mutex
	fragments []workers.ProgressFragment
}

func (recorder *progressFragmentRecorder) publish(fragment workers.ProgressFragment) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	fragment.Metadata = cloneMetadata(fragment.Metadata)
	recorder.fragments = append(recorder.fragments, fragment)
}

func (recorder *progressFragmentRecorder) snapshot() []workers.ProgressFragment {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	fragments := append([]workers.ProgressFragment(nil), recorder.fragments...)
	for index := range fragments {
		fragments[index].Metadata = cloneMetadata(fragments[index].Metadata)
	}
	return fragments
}

func countFragmentsByKind(
	fragments []workers.ProgressFragment,
	kind string,
) int {
	count := 0
	for _, fragment := range fragments {
		if fragment.Kind == kind {
			count++
		}
	}
	return count
}

func fragmentByKind(
	fragments []workers.ProgressFragment,
	kind string,
) workers.ProgressFragment {
	for _, fragment := range fragments {
		if fragment.Kind == kind {
			return fragment
		}
	}
	return workers.ProgressFragment{}
}
