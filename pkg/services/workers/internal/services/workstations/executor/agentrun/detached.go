package agentrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
)

// DetachedRequest is the complete input for one agent-run attempt started from
// the request-scoped Workers Execute path. The caller has already resolved the
// workstation and worker taxonomy, prompt, provider target, and tool policy, so
// this entry point reads no Factory definition and keeps no workstation state.
type DetachedRequest struct {
	// Correlation is the immutable admitted execution identity, including the
	// physical attempt. Final observations must retain it alongside the draft.
	Correlation workerexecution.ExecutionCorrelation
	// Attempt is the resolved provider request that every harness turn runs.
	Attempt workerexecution.RunnerExecutionRequest
	// ProgressPublisher receives the request-scoped canonical final-message
	// observation after a successful harness run.
	ProgressPublisher workerexecution.ProgressPublisher
	// ToolPolicy is the authored agent tool policy applied to the loop. An
	// unset policy disables tool execution.
	ToolPolicy string
	// WorkingDirectory bounds the filesystem tools the policy allows.
	WorkingDirectory string
}

// ExecuteDetached runs one agent loop over runner and reduces it to the runner
// result the caller's own normalization already understands. Outcome
// classification, stop tokens, decision envelopes, and observation delivery
// remain with the caller, exactly as they are for a single provider attempt.
func ExecuteDetached(
	ctx context.Context,
	harness HarnessAdapter,
	runner workerexecution.Runner,
	request DetachedRequest,
) (workerexecution.RunnerExecutionResult, error) {
	if harness == nil {
		return workerexecution.RunnerExecutionResult{}, fmt.Errorf("agent-run harness is required")
	}
	if runner == nil {
		return workerexecution.RunnerExecutionResult{}, fmt.Errorf("agent-run runner is required")
	}
	attempt := request.Attempt
	// The harness owns tool execution for every turn it drives, so the provider
	// request it replays always declares tools required.
	attempt.ToolExecutionMode = workerexecution.RunnerToolExecutionModeRequired
	recorder := NewToolDiagnosticRecorder()
	observed := &lastRunnerResult{runner: runner, publish: request.ProgressPublisher}
	harnessResult, err := harness.Execute(ctx, HarnessInput{
		SystemPrompt: attempt.SystemPrompt,
		UserMessage:  attempt.UserMessage,
		Inferencer:   newRunnerInferencer(observed, attempt),
		ToolPolicy:   request.ToolPolicy,
		WorkingDir:   request.WorkingDirectory,
		ToolRecorder: recorder,
	})
	// The loop's final message is the last turn the runner produced, so that
	// turn's result is already the answer -- including the Provider Session,
	// provider diagnostics, and the output-policy classification the runner
	// applied. Only a loop that ended without reaching the runner falls back to
	// the harness text.
	result, executed := observed.snapshot()
	finalContent := harnessResult.FinalText
	if !executed {
		result.Content = harnessResult.FinalText
	} else {
		finalContent = result.Content
	}
	result.Diagnostics = mergeAgentRunDiagnostics(
		agentRunDiagnostics(toolDiagnosticsMetadata(request.ToolPolicy, recorder)),
		result.Diagnostics,
	)
	if err != nil {
		// The go-agent-loop engine flattens a failing turn into
		// errors.New(message) before it reaches the harness result, which
		// destroys the typed provider error's retryable/terminal
		// classification. The last runner turn observed that typed error
		// first-hand, so re-attach it -- unless the loop ended for a
		// caller-owned cancellation or deadline, which must keep precedence.
		if runnerErr := observed.lastError(); runnerErr != nil &&
			!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			err = runnerErr
		}
		return result, err
	}
	publishAgentFinalMessage(
		request.ProgressPublisher,
		request.Attempt.Dispatch.DispatchID,
		request.Correlation,
		capturedFinalContent(finalContent, request.Attempt),
		observed.messageIdentity(),
	)
	return result, nil
}

// lastRunnerResult keeps the most recent provider turn of one agent loop. It is
// request-scoped: a new value is created for every ExecuteDetached call.
type lastRunnerResult struct {
	runner   workerexecution.Runner
	mu       sync.Mutex
	result   workerexecution.RunnerExecutionResult
	lastErr  error
	executed bool
	publish  workerexecution.ProgressPublisher
	message  *workerexecution.Draft
	turn     uint64
}

func (observed *lastRunnerResult) Execute(
	ctx context.Context,
	request workerexecution.RunnerExecutionRequest,
) (workerexecution.RunnerExecutionResult, error) {
	// Bind observations to this physical provider turn. A later turn without
	// an identity cannot borrow an earlier turn's message, even for equal text.
	publish := workerinternal.ProgressPublisherFromContext(ctx, observed.publish)
	observed.mu.Lock()
	observed.turn++
	turnID := "agent-turn-" + strconv.FormatUint(observed.turn, 10)
	observed.mu.Unlock()
	var mu sync.Mutex
	var message *workerexecution.Draft
	if publish != nil {
		ctx = workerinternal.WithProgressPublisher(ctx, func(fragment workerexecution.ProgressFragment) {
			// Native item IDs can be reused in a later provider turn. Preserve
			// the item and carry its request-local turn scope separately.
			fragment = scopeMessageTurn(fragment, turnID)
			if identity, isMessage := assistantMessageIdentity(fragment); isMessage {
				mu.Lock()
				message = identity
				mu.Unlock()
			}
			publish(fragment)
		})
	}
	result, err := observed.runner.Execute(ctx, request)
	mu.Lock()
	observed.mu.Lock()
	observed.result = result
	observed.lastErr = err
	observed.executed = true
	observed.message = message
	observed.mu.Unlock()
	mu.Unlock()
	return result, err
}

func (observed *lastRunnerResult) messageIdentity() *workerexecution.Draft {
	observed.mu.Lock()
	defer observed.mu.Unlock()
	return observed.message
}

func assistantMessageIdentity(fragment workerexecution.ProgressFragment) (*workerexecution.Draft, bool) {
	if draft, ok := fragment.CanonicalDraft.(workerexecution.Draft); ok {
		if draft.Kind != workerexecution.KindMessage {
			return nil, false
		}
		var payload workerexecution.MessagePayload
		if draft.Phase != workerexecution.PhaseDelta &&
			(json.Unmarshal(draft.Payload, &payload) != nil || payload.Role != "assistant") {
			return nil, true
		}
		if draft.ItemID == "" {
			return nil, true
		}
		return &draft, true
	}
	if fragment.Kind != workerexecution.ProgressFragmentKind ||
		!strings.HasPrefix(strings.ToLower(fragment.Type), "message.") {
		return nil, false
	}
	if strings.TrimSpace(fragment.Metadata["item_id"]) == "" {
		return nil, true
	}
	return &workerexecution.Draft{
		DispatchID: strings.TrimSpace(fragment.DispatchID), ItemID: strings.TrimSpace(fragment.Metadata["item_id"]),
		RunID: strings.TrimSpace(fragment.Metadata["run_id"]), TurnID: strings.TrimSpace(fragment.Metadata["turn_id"]),
	}, true
}

func scopeMessageTurn(fragment workerexecution.ProgressFragment, turnID string) workerexecution.ProgressFragment {
	if draft, ok := fragment.CanonicalDraft.(workerexecution.Draft); ok && draft.Kind == workerexecution.KindMessage {
		draft = workerexecution.CloneDraft(draft)
		if draft.TurnID == "" {
			draft.TurnID = turnID
		}
		fragment.CanonicalDraft = draft
	} else if fragment.Kind == workerexecution.ProgressFragmentKind && strings.HasPrefix(strings.ToLower(fragment.Type), "message.") {
		metadata := make(map[string]string, len(fragment.Metadata)+1)
		for key, value := range fragment.Metadata {
			metadata[key] = value
		}
		if metadata["turn_id"] == "" {
			metadata["turn_id"] = turnID
		}
		fragment.Metadata = metadata
	}
	return fragment
}

func (observed *lastRunnerResult) snapshot() (workerexecution.RunnerExecutionResult, bool) {
	observed.mu.Lock()
	defer observed.mu.Unlock()
	return observed.result, observed.executed
}

// lastError reports the typed error of the most recent runner turn, or nil when
// that turn succeeded or no turn ran. A turn error terminates the agent loop,
// so a non-nil value is always the root cause of a failed harness execution.
func (observed *lastRunnerResult) lastError() error {
	observed.mu.Lock()
	defer observed.mu.Unlock()
	return observed.lastErr
}
