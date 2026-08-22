package agentrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
)

// detachedRunnerStub is the provider edge one detached agent loop turns on. It
// returns a complete runner result -- not just text -- because ExecuteDetached
// must preserve the whole last turn, including the fields the agent runner's
// decision-envelope normalization writes.
type detachedRunnerStub struct {
	mu       sync.Mutex
	results  []workerexecution.RunnerExecutionResult
	err      error
	calls    int
	requests []workerexecution.RunnerExecutionRequest
}

func (runner *detachedRunnerStub) Execute(
	_ context.Context,
	request workerexecution.RunnerExecutionRequest,
) (workerexecution.RunnerExecutionResult, error) {
	runner.mu.Lock()
	index := runner.calls
	runner.calls++
	runner.requests = append(runner.requests, request)
	runner.mu.Unlock()
	if runner.err != nil {
		return workerexecution.RunnerExecutionResult{}, runner.err
	}
	if len(runner.results) == 0 {
		return workerexecution.RunnerExecutionResult{}, nil
	}
	if index >= len(runner.results) {
		index = len(runner.results) - 1
	}
	return runner.results[index], nil
}

func (runner *detachedRunnerStub) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *detachedRunnerStub) requestAt(index int) workerexecution.RunnerExecutionRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if index >= len(runner.requests) {
		return workerexecution.RunnerExecutionRequest{}
	}
	return runner.requests[index]
}

// inferencingHarnessStub drives the inferencer the way the real agent loop
// does, so the runner behind ExecuteDetached actually executes.
type inferencingHarnessStub struct {
	turns     int
	result    HarnessResult
	err       error
	lastInput HarnessInput
	inferErrs []error
}

func (harness *inferencingHarnessStub) Execute(
	ctx context.Context,
	input HarnessInput,
) (HarnessResult, error) {
	harness.lastInput = input
	turns := harness.turns
	if turns == 0 {
		turns = 1
	}
	for turn := 0; turn < turns; turn++ {
		_, err := input.Inferencer.Infer(ctx, messages.InferenceRequest{
			Messages: []messages.Message{
				messages.NewTextMessage(messages.RoleUser, "continue"),
			},
		})
		harness.inferErrs = append(harness.inferErrs, err)
	}
	if harness.err != nil {
		return harness.result, harness.err
	}
	return harness.result, nil
}

// silentHarnessStub never reaches the inferencer, which is the only case where
// the harness text is the answer.
type silentHarnessStub struct {
	result HarnessResult
	err    error
}

func (harness *silentHarnessStub) Execute(context.Context, HarnessInput) (HarnessResult, error) {
	return harness.result, harness.err
}

func detachedAttempt() workerexecution.RunnerExecutionRequest {
	return workerexecution.RunnerExecutionRequest{
		SystemPrompt:     "You are an agent.",
		UserMessage:      "Review the change.",
		WorkingDirectory: "/factory/work",
		DecisionEnvelope: true,
	}
}

func TestExecuteDetachedRequiresHarness(t *testing.T) {
	t.Parallel()

	_, err := ExecuteDetached(
		context.Background(),
		nil,
		&detachedRunnerStub{},
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if err == nil || !strings.Contains(err.Error(), "agent-run harness is required") {
		t.Fatalf("ExecuteDetached() error = %v, want harness requirement", err)
	}
}

func TestExecuteDetachedRequiresRunner(t *testing.T) {
	t.Parallel()

	_, err := ExecuteDetached(
		context.Background(),
		&inferencingHarnessStub{},
		nil,
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if err == nil || !strings.Contains(err.Error(), "agent-run runner is required") {
		t.Fatalf("ExecuteDetached() error = %v, want runner requirement", err)
	}
}

// TestExecuteDetachedKeepsLastRunnerTurnAuthoritative pins the invariant that
// has already been broken once: the agent runner's decision-envelope
// normalization writes Outcome, Feedback, and Content onto the runner result,
// so returning the harness FinalText instead silently discards the envelope and
// every decision-envelope workstation stops classifying.
func TestExecuteDetachedKeepsLastRunnerTurnAuthoritative(t *testing.T) {
	t.Parallel()

	runner := &detachedRunnerStub{results: []workerexecution.RunnerExecutionResult{
		{Content: "first turn", Outcome: workerexecution.WorkOutcome("INCOMPLETE")},
		{
			Content:  `{"outcome":"COMPLETE","feedback":"ship it"}`,
			Outcome:  workerexecution.WorkOutcome("COMPLETE"),
			Feedback: "ship it",
			Continuation: (&providers.SessionMetadata{
				Provider: "codex",
				ID:       "provider-session-9",
			}).ContinuationRef(),
		},
	}}
	harness := &inferencingHarnessStub{
		turns:  2,
		result: HarnessResult{FinalText: "I have finished the review."},
	}

	result, err := ExecuteDetached(
		context.Background(),
		harness,
		runner,
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if err != nil {
		t.Fatalf("ExecuteDetached() error = %v, want nil", err)
	}
	if result.Content != `{"outcome":"COMPLETE","feedback":"ship it"}` {
		t.Fatalf("ExecuteDetached() Content = %q, want the last runner turn rather than the harness FinalText", result.Content)
	}
	if result.Content == harness.result.FinalText {
		t.Fatal("ExecuteDetached() returned the harness FinalText, which discards the decision envelope")
	}
	if result.Outcome != workerexecution.WorkOutcome("COMPLETE") {
		t.Fatalf("ExecuteDetached() Outcome = %q, want the last runner turn outcome", result.Outcome)
	}
	if result.Feedback != "ship it" {
		t.Fatalf("ExecuteDetached() Feedback = %q, want the last runner turn feedback", result.Feedback)
	}
	if result.Continuation == nil || result.Continuation.ProviderSessionID != "provider-session-9" {
		t.Fatalf("ExecuteDetached() Continuation = %+v, want the last runner turn continuation", result.Continuation)
	}
	if runner.callCount() != 2 {
		t.Fatalf("runner executed %d times, want one execution per harness turn", runner.callCount())
	}
}

func TestExecuteDetachedFallsBackToHarnessTextWhenRunnerNeverExecuted(t *testing.T) {
	t.Parallel()

	runner := &detachedRunnerStub{results: []workerexecution.RunnerExecutionResult{{Content: "never used"}}}
	result, err := ExecuteDetached(
		context.Background(),
		&silentHarnessStub{result: HarnessResult{FinalText: "loop answered without a provider turn"}},
		runner,
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if err != nil {
		t.Fatalf("ExecuteDetached() error = %v, want nil", err)
	}
	if result.Content != "loop answered without a provider turn" {
		t.Fatalf("ExecuteDetached() Content = %q, want the harness FinalText fallback", result.Content)
	}
	if runner.callCount() != 0 {
		t.Fatalf("runner executed %d times, want 0", runner.callCount())
	}
}

func TestExecuteDetachedForcesRequiredToolExecutionOnEveryTurn(t *testing.T) {
	t.Parallel()

	runner := &detachedRunnerStub{results: []workerexecution.RunnerExecutionResult{{Content: "done"}}}
	attempt := detachedAttempt()
	attempt.ToolExecutionMode = workerexecution.RunnerToolExecutionModeDisabled

	if _, err := ExecuteDetached(
		context.Background(),
		&inferencingHarnessStub{turns: 2},
		runner,
		DetachedRequest{
			Attempt:          attempt,
			ToolPolicy:       "read_only",
			WorkingDirectory: "/factory/work",
		},
	); err != nil {
		t.Fatalf("ExecuteDetached() error = %v, want nil", err)
	}
	for turn := 0; turn < runner.callCount(); turn++ {
		if got := runner.requestAt(turn).ToolExecutionMode; got != workerexecution.RunnerToolExecutionModeRequired {
			t.Fatalf("turn %d ToolExecutionMode = %q, want %q", turn, got, workerexecution.RunnerToolExecutionModeRequired)
		}
	}
	if attempt.ToolExecutionMode != workerexecution.RunnerToolExecutionModeDisabled {
		t.Fatal("ExecuteDetached() mutated the caller's attempt request")
	}
}

func TestExecuteDetachedPassesToolPolicyAndWorkingDirectoryToHarness(t *testing.T) {
	t.Parallel()

	harness := &inferencingHarnessStub{}
	if _, err := ExecuteDetached(
		context.Background(),
		harness,
		&detachedRunnerStub{results: []workerexecution.RunnerExecutionResult{{Content: "done"}}},
		DetachedRequest{
			Attempt:          detachedAttempt(),
			ToolPolicy:       "read_write",
			WorkingDirectory: "/factory/worktree",
		},
	); err != nil {
		t.Fatalf("ExecuteDetached() error = %v, want nil", err)
	}
	if harness.lastInput.ToolPolicy != "read_write" {
		t.Fatalf("harness ToolPolicy = %q, want read_write", harness.lastInput.ToolPolicy)
	}
	if harness.lastInput.WorkingDir != "/factory/worktree" {
		t.Fatalf("harness WorkingDir = %q, want /factory/worktree", harness.lastInput.WorkingDir)
	}
	if harness.lastInput.SystemPrompt != "You are an agent." {
		t.Fatalf("harness SystemPrompt = %q, want the attempt system prompt", harness.lastInput.SystemPrompt)
	}
	if harness.lastInput.UserMessage != "Review the change." {
		t.Fatalf("harness UserMessage = %q, want the attempt user message", harness.lastInput.UserMessage)
	}
	if harness.lastInput.ToolRecorder == nil {
		t.Fatal("harness ToolRecorder is nil, want a request-scoped recorder")
	}
	if harness.lastInput.Inferencer == nil {
		t.Fatal("harness Inferencer is nil, want the per-turn runner inferencer")
	}
}

func TestExecuteDetachedMergesAgentRunDiagnosticsOverRunnerDiagnostics(t *testing.T) {
	t.Parallel()

	runner := &detachedRunnerStub{results: []workerexecution.RunnerExecutionResult{{
		Content: "done",
		Diagnostics: &workerexecution.WorkDiagnostics{
			Metadata: map[string]string{"decision_envelope_status": "valid"},
		},
	}}}

	result, err := ExecuteDetached(
		context.Background(),
		&inferencingHarnessStub{},
		runner,
		DetachedRequest{Attempt: detachedAttempt(), ToolPolicy: "read_only"},
	)
	if err != nil {
		t.Fatalf("ExecuteDetached() error = %v, want nil", err)
	}
	if result.Diagnostics == nil {
		t.Fatal("ExecuteDetached() Diagnostics = nil, want merged agent-run diagnostics")
	}
	if got := result.Diagnostics.Metadata[DiagnosticExecutionBehavior]; got != ExecutionBehaviorAgentRun {
		t.Fatalf("Diagnostics %s = %q, want %q", DiagnosticExecutionBehavior, got, ExecutionBehaviorAgentRun)
	}
	if got := result.Diagnostics.Metadata["decision_envelope_status"]; got != "valid" {
		t.Fatalf("Diagnostics decision_envelope_status = %q, want the runner's own validation fact to survive", got)
	}
}

func TestExecuteDetachedReturnsLastRunnerTurnWithHarnessError(t *testing.T) {
	t.Parallel()

	harnessErr := errors.New("agent loop finished with status failed")
	runner := &detachedRunnerStub{results: []workerexecution.RunnerExecutionResult{{
		Content: "partial answer",
		Outcome: workerexecution.WorkOutcome("INCOMPLETE"),
	}}}

	result, err := ExecuteDetached(
		context.Background(),
		&inferencingHarnessStub{
			result: HarnessResult{FinalText: "harness text"},
			err:    harnessErr,
		},
		runner,
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if !errors.Is(err, harnessErr) {
		t.Fatalf("ExecuteDetached() error = %v, want the harness error", err)
	}
	if result.Content != "partial answer" {
		t.Fatalf("ExecuteDetached() Content = %q, want the last runner turn preserved alongside the error", result.Content)
	}
	if result.Outcome != workerexecution.WorkOutcome("INCOMPLETE") {
		t.Fatalf("ExecuteDetached() Outcome = %q, want the last runner turn outcome", result.Outcome)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.Metadata[DiagnosticExecutionBehavior] != ExecutionBehaviorAgentRun {
		t.Fatalf("ExecuteDetached() Diagnostics = %+v, want agent-run execution behavior on the failure result", result.Diagnostics)
	}
}

// TestExecuteDetachedTimeoutSurfacesAgentRunTimeoutClass characterizes the
// typed timeout a detached agent-run caller observes. failure_test.go already
// covers failureClassForError as a pure mapping, but nothing asserted that a
// deadline reached inside the agent loop travels out of ExecuteDetached still
// classifiable as an agent-run timeout with the last provider turn intact --
// which is exactly what the detached caller reports as its terminal failure.
func TestExecuteDetachedTimeoutSurfacesAgentRunTimeoutClass(t *testing.T) {
	t.Parallel()

	timeoutErr := fmt.Errorf("agent loop exceeded its budget: %w", context.DeadlineExceeded)
	runner := &detachedRunnerStub{results: []workerexecution.RunnerExecutionResult{{
		Content: "last turn before the deadline",
		Outcome: workerexecution.WorkOutcome("INCOMPLETE"),
	}}}

	result, err := ExecuteDetached(
		context.Background(),
		&inferencingHarnessStub{
			result: HarnessResult{FinalText: "harness text"},
			err:    timeoutErr,
		},
		runner,
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteDetached() error = %v, want a deadline-classifiable agent-run failure", err)
	}
	if result.Content != "last turn before the deadline" {
		t.Fatalf("ExecuteDetached() Content = %q, want the last provider turn preserved through the timeout", result.Content)
	}
	if got := failureClassForError(err); got != FailureClassTimeout {
		t.Fatalf("failureClassForError(ExecuteDetached error) = %q, want %q", got, FailureClassTimeout)
	}
	metadata := failureMetadataForError(err)
	if metadata == nil || metadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("failureMetadataForError(ExecuteDetached error) = %#v, want a timeout failure type", metadata)
	}
	if got := agentRunFailureDiagnostics(err)[DiagnosticFailureClass]; got != FailureClassTimeout {
		t.Fatalf("agent-run diagnostics failure class = %q, want %q", got, FailureClassTimeout)
	}
}

func TestExecuteDetachedPropagatesRunnerFailure(t *testing.T) {
	t.Parallel()

	runnerErr := errors.New("provider refused the request")
	harness := &inferencingHarnessStub{result: HarnessResult{FinalText: "unused"}}

	result, err := ExecuteDetached(
		context.Background(),
		harness,
		&detachedRunnerStub{err: runnerErr},
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if err != nil {
		t.Fatalf("ExecuteDetached() error = %v, want nil because the harness absorbed the turn failure", err)
	}
	if len(harness.inferErrs) == 0 || harness.inferErrs[0] == nil {
		t.Fatalf("inferencer errors = %v, want the runner failure surfaced to the loop", harness.inferErrs)
	}
	// The turn ran, so its (empty) result -- not the harness text -- is the answer.
	if result.Content != "" {
		t.Fatalf("ExecuteDetached() Content = %q, want the failed runner turn result", result.Content)
	}
}

// TestExecuteDetachedPreservesTypedProviderErrorAcrossHarnessSeam pins the
// second root-cause fix from the rsm-4 review-lane kill: the go-agent-loop
// engine rebuilds a failing inference turn as errors.New(message), which
// destroyed the typed *ProviderError -- worker-session ledgers recorded
// `type: unknown, family: terminal` for a retryable provider failure. The
// typed error the runner produced must survive ExecuteDetached, through the
// REAL library harness, after the inferencer's bounded retries exhaust.
func TestExecuteDetachedPreservesTypedProviderErrorAcrossHarnessSeam(t *testing.T) {
	t.Parallel()

	providerFailure := workerexecution.NewProviderError(
		workerexecution.WorkFailureTypeInternalServerError,
		`Codex rollout inspection reached record limit for provider session "01a02180"`,
		nil,
	)
	runner := &detachedRunnerStub{err: providerFailure}

	result, err := ExecuteDetached(
		context.Background(),
		NewLibraryHarnessAdapter(localToolFileSystem{}),
		runner,
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if err == nil {
		t.Fatal("ExecuteDetached() error = nil, want the exhausted provider failure")
	}
	var providerErr *workerexecution.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Type != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("ExecuteDetached() error = %v, want the typed provider error preserved across the harness seam", err)
	}
	if decision := workerexecution.WorkFailureDecisionFromProviderError(providerErr); !decision.Retryable {
		t.Fatalf("failure decision = %#v, want the retryable classification to survive", decision)
	}
	if metadata := failureMetadataForError(err); metadata == nil ||
		metadata.Type != workerexecution.WorkFailureTypeInternalServerError ||
		metadata.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("failureMetadataForError() = %#v, want typed retryable metadata, not unknown/terminal", metadata)
	}
	if got := failureClassForError(err); got != FailureClassProvider {
		t.Fatalf("failureClassForError() = %q, want %q", got, FailureClassProvider)
	}
	// The inferencer's own bounded retry budget still applies before the typed
	// error surfaces: one initial call plus agentRunProviderMaxRetries.
	if runner.callCount() != 1+agentRunProviderMaxRetries {
		t.Fatalf("runner executed %d times, want %d bounded attempts", runner.callCount(), 1+agentRunProviderMaxRetries)
	}
	if result.Content != "" {
		t.Fatalf("ExecuteDetached() Content = %q, want the failed runner turn result", result.Content)
	}
}

// TestExecuteDetachedKeepsCancellationOverTypedRunnerError pins the precedence
// guard on the re-attach: a caller-owned cancellation or deadline that ends the
// loop must stay observable even when the last runner turn also failed.
func TestExecuteDetachedKeepsCancellationOverTypedRunnerError(t *testing.T) {
	t.Parallel()

	providerFailure := workerexecution.NewProviderError(
		workerexecution.WorkFailureTypePermanentBadRequest,
		"invalid request",
		nil,
	)
	canceled := fmt.Errorf("agent loop interrupted: %w", context.Canceled)

	_, err := ExecuteDetached(
		context.Background(),
		&inferencingHarnessStub{err: canceled},
		&detachedRunnerStub{err: providerFailure},
		DetachedRequest{Attempt: detachedAttempt()},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteDetached() error = %v, want the cancellation to keep precedence", err)
	}
	var providerErr *workerexecution.ProviderError
	if errors.As(err, &providerErr) {
		t.Fatalf("ExecuteDetached() error = %v, want the runner error not to replace a cancellation", err)
	}
}

// TestLastRunnerResultSnapshotIsConcurrencySafe exercises the request-scoped
// observer under concurrent turns; the agent loop may drive parallel tool
// turns, and a torn snapshot would return a mixed result.
func TestLastRunnerResultSnapshotIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	observed := &lastRunnerResult{runner: &detachedRunnerStub{
		results: []workerexecution.RunnerExecutionResult{{Content: "turn"}},
	}}
	if _, executed := observed.snapshot(); executed {
		t.Fatal("snapshot() reported an execution before any turn ran")
	}

	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := observed.Execute(
				context.Background(),
				workerexecution.RunnerExecutionRequest{},
			); err != nil {
				t.Errorf("Execute() error = %v, want nil", err)
			}
			observed.snapshot()
		}()
	}
	wait.Wait()

	result, executed := observed.snapshot()
	if !executed {
		t.Fatal("snapshot() executed = false, want true after concurrent turns")
	}
	if result.Content != "turn" {
		t.Fatalf("snapshot() Content = %q, want turn", result.Content)
	}
}
