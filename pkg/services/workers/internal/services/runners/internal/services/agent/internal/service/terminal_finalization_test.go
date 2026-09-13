package service

import (
	"context"
	"errors"
	"strings"
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

func TestExecutePartialNativeResultOnTimeoutKeepsTypedFailure(t *testing.T) {
	t.Parallel()

	provider := &resultErrorProvidersFake{
		result: providers.ExecuteResult{Content: "partial output before timeout"},
		err: providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindTimeout,
			Message: "provider invocation timed out",
		},
	}
	recorder := &progressFragmentRecorder{}
	runner, err := New(provider, recorder.publish)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := baseAgentRequest()
	request.StopToken = "COMPLETE"
	result, err := runner.Execute(t.Context(), request)
	if err == nil {
		t.Fatal("Execute() error = nil, want timeout failure for partial native output")
	}
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Type != workers.WorkFailureTypeTimeout {
		t.Fatalf("Execute() error = %v, want typed timeout ProviderError", err)
	}
	if result.Content != "partial output before timeout" {
		t.Fatalf("result.Content = %q, want retained partial output", result.Content)
	}

	fragments := recorder.snapshot()
	if countFragmentsByKind(fragments, workers.CompletedFragmentKind) != 0 {
		t.Fatalf("completed terminal fragments = %#v, want none", fragments)
	}
	if countFragmentsByKind(fragments, workers.FailedFragmentKind) != 1 {
		t.Fatalf("failed terminal fragments = %#v, want exactly one", fragments)
	}
}

func TestExecuteEmptyResultPublishesNoUsableFailure(t *testing.T) {
	t.Parallel()

	recorder := &progressFragmentRecorder{}
	runner, err := New(&resultErrorProvidersFake{}, recorder.publish)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.Execute(t.Context(), baseAgentRequest())
	var providerErr *workers.ProviderError
	if err == nil || !errors.As(err, &providerErr) ||
		providerErr.Type != workers.WorkFailureTypeInternalServerError ||
		strings.Contains(err.Error(), noUsableAgentResultMessage) {
		t.Fatalf("Execute() error = %v, want bounded no-usable-result failure", err)
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

func TestTerminalPublicationNilIsNoop(t *testing.T) {
	var publication *terminalPublication
	fragment := workers.ProgressFragment{Kind: workers.ProgressFragmentKind}

	publication.progress(fragment)
	publication.terminal(fragment)
	publication.close()
}

func TestExecuteNativeLifecycleDoesNotSynthesizeCompletion(t *testing.T) {
	t.Parallel()

	recorder := &progressFragmentRecorder{}
	provider := &resultProvidersFake{result: providers.ExecuteResult{
		Content: "native completed output",
		Diagnostics: &providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{{
			Phase:  "turn.completed",
			Detail: "native terminal lifecycle",
		}}},
	}}
	runner, err := New(provider, recorder.publish)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := runner.Execute(t.Context(), baseAgentRequest()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	fragments := recorder.snapshot()
	if countFragmentsByKind(fragments, workers.CompletedFragmentKind) != 0 {
		t.Fatalf("completed terminal fragments = %#v, want native lifecycle only", fragments)
	}
	if len(fragments) != 2 || fragments[0].Type != "turn.completed" ||
		fragments[1].Type != "message.completed" {
		t.Fatalf("native lifecycle fragments = %#v, want native lifecycle and ordered message progress", fragments)
	}
}

func TestMergeDecisionEnvelopeDiagnosticsCopiesOwnerLayers(t *testing.T) {
	t.Parallel()

	envelope := &workers.WorkDiagnostics{
		Provider: &workers.ProviderDiagnostic{
			Provider: "definitions",
			ResponseMetadata: map[string]string{
				"classification": "accepted",
			},
		},
		Metadata: map[string]string{"decision": "owner"},
	}
	if merged := mergeDecisionEnvelopeDiagnostics(nil, envelope); merged == nil ||
		merged.Provider == nil || merged.Provider.ResponseMetadata["classification"] != "accepted" {
		t.Fatalf("nil-base diagnostics = %#v, want cloned owner diagnostics", merged)
	}

	baseWithoutProvider := &workers.WorkDiagnostics{}
	merged := mergeDecisionEnvelopeDiagnostics(baseWithoutProvider, envelope)
	if merged.Provider == nil || merged.Provider.Provider != "definitions" {
		t.Fatalf("provider overlay = %#v, want owner provider", merged.Provider)
	}

	baseWithoutMetadata := &workers.WorkDiagnostics{
		Provider: &workers.ProviderDiagnostic{Provider: "codex"},
	}
	merged = mergeDecisionEnvelopeDiagnostics(baseWithoutMetadata, envelope)
	if merged.Provider.ResponseMetadata["classification"] != "accepted" ||
		merged.Metadata["decision"] != "owner" {
		t.Fatalf("layered diagnostics = %#v, want provider and top-level metadata", merged)
	}
}

func TestRunnerResultCopiesDetachedProviderDiagnostics(t *testing.T) {
	t.Parallel()

	response := runnerResult(providers.ExecuteResult{
		Content: "diagnosed output",
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "diagnosed-session",
		},
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 7,
			Command: &providers.ExecuteCommandDiagnostics{
				Command: "codex", Args: []string{"exec"}, Env: map[string]string{"SAFE": "1"},
				Stdin: "prompt", Stdout: "out", Stderr: "err", ExitCode: 3,
				TimedOut: true, DurationMS: 4, WorkingDir: "work",
			},
			Panic: &providers.ExecutePanicDiagnostics{Message: "panic", Stack: "stack"},
		},
	}, providers.IDCodex)
	if response.Diagnostics == nil || response.Diagnostics.Provider == nil ||
		response.Diagnostics.Provider.ResponseMetadata[workers.ProviderResponseMetadataDurationMS] != "7" {
		t.Fatalf("provider diagnostics = %#v, want duration metadata", response.Diagnostics)
	}
	if response.Diagnostics.Command == nil || !response.Diagnostics.Command.TimedOut ||
		response.Diagnostics.Panic == nil || response.Diagnostics.Panic.Message != "panic" {
		t.Fatalf("detached command/panic diagnostics = %#v, want copied facts", response.Diagnostics)
	}
}

func TestLiveProviderSessionSnapshotCopiesAuthoredReference(t *testing.T) {
	t.Parallel()

	live := &liveProviderSession{}
	live.set(providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "live-session",
	})
	snapshot := live.snapshot()
	if snapshot == nil || snapshot.ProviderSessionID != "live-session" {
		t.Fatalf("live session snapshot = %#v, want authored continuation", snapshot)
	}
}

func TestPreserveContinuationUsesLegacySessionID(t *testing.T) {
	t.Parallel()

	request := baseAgentRequest()
	request.SessionID = "legacy-session"
	response := preserveContinuation(
		workers.RunnerExecutionResult{},
		request,
		nil,
	)
	if response.Continuation == nil || response.Continuation.ProviderSessionID != "legacy-session" {
		t.Fatalf("preserved continuation = %#v, want legacy session", response.Continuation)
	}
}
