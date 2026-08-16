package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkstationRequestExecutorConfig supplies the immutable Runtime facts needed
// to resolve a direct Worker Session request before handing it to the shared
// Workers Execute boundary. It is deliberately separate from the Runtime's
// active attempt lifecycle: direct Worker Sessions own their own supervision.
type WorkstationRequestExecutorConfig struct {
	Service                    workers.Service
	RuntimeDefinitions         interfaces.RuntimeDefinitionLookup
	InvocationInterpolation    interfaces.InvocationInterpolationService
	InvocationFileReader       interfaces.FileReader
	WorkflowContext            *workers.Context
	FactorySessionID           string
	RuntimeID                  string
	RecordingID                string
	EventHistory               recordings.RuntimeLedger
	NewID                      factory.IDGenerator
	PromptRenderer             runtimePromptRenderer
	TemplateFieldResolver      runtimeTemplateFieldResolver
	PromptSourceReader         interfaces.FileReader
	MockWorkers                *workers.MockWorkersConfig
	ProgressPublisher          workers.ProgressPublisher
	Net                        *state.Net
	ExpectedArtifactFileSystem any
}

type attemptProcessObserver struct {
	lifecycle *attemptLifecycle
	attempt   *activeAttempt
}

func (observer attemptProcessObserver) ProcessStarted(_ platformprocess.ProcessInfo) {}

func (observer attemptProcessObserver) ProcessExited(_ platformprocess.ProcessInfo) {
	observer.lifecycle.reconcileProcessGone(observer.attempt)
}

func processGoneAttemptResult(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	result.Correlation = request.Correlation
	result.Outcome = workers.ExecutionOutcomeFailed
	result.Output = workers.ProposedOutput{}
	result.StructuredResult = nil
	result.StructuredResultPresent = false
	result.Continuation = nil
	result.Failure = &workers.ExecutionFailure{
		Type:    workers.WorkFailureTypeUnknown,
		Family:  workers.WorkFailureFamilyRetryable,
		Message: workers.ErrWorkstationDispatchProcessGone.Error(),
	}
	return result
}

func processGoneDispatchResult(
	result *workers.WorkResult,
	terminal *workers.WorkstationDispatchTerminalOutcome,
	executeErr error,
) workers.WorkstationDispatchReconciliationReason {
	if result == nil || terminal == nil || !errors.Is(executeErr, workers.ErrWorkstationDispatchProcessGone) {
		return ""
	}
	*terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	result.Outcome = workers.OutcomeFailed
	result.Error = workers.ErrWorkstationDispatchProcessGone.Error()
	result.FailureMetadata = &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyRetryable,
		Type:   workers.WorkFailureTypeUnknown,
	}
	result.Diagnostics = &workers.WorkDiagnostics{Metadata: map[string]string{
		workers.ProviderResponseMetadataFailureOperation:      "worker_session_reconciliation",
		workers.ProviderResponseMetadataFailureClassification: "process_gone",
		workers.ProviderResponseMetadataFailureStage:          "process",
	}}
	return workers.WorkstationDispatchReconciliationReasonProcessGone
}

type PromptRenderer = runtimePromptRenderer
type TemplateFieldResolver = runtimeTemplateFieldResolver

// SetReplayEvents binds the immutable canonical history loaded for a legacy
// replay. Runtime execution may regenerate derived events, while observation
// streams continue to expose the persisted event positions.
func (f *factoryImpl) SetReplayEvents(events []interfaces.FactoryEvent) {
	if f == nil || f.cfg == nil {
		return
	}
	f.cfg.replayEvents = cloneAndSortFactoryEvents(events)
}

// NewWorkstationRequestExecutor creates the Runtime-owned compatibility
// adapter used by top-level Worker Session routes. It resolves minimal legacy
// direct requests from the immutable Factory definition and then invokes the
// process-scoped Workers service with a complete detached ExecuteRequest.
func NewWorkstationRequestExecutor(
	config WorkstationRequestExecutorConfig,
) workers.WorkstationRequestExecutor {
	if config.Service == nil {
		return nil
	}
	return &workstationRequestExecutor{
		service: config.Service,
		cfg: &runtimeConfig{
			executeService:             config.Service,
			runtimeConfig:              config.RuntimeDefinitions,
			invocationInterpolation:    config.InvocationInterpolation,
			invocationFileReader:       config.InvocationFileReader,
			workflowContext:            config.WorkflowContext.Clone(),
			recordingID:                strings.TrimSpace(config.RecordingID),
			runtimeID:                  strings.TrimSpace(config.RuntimeID),
			eventHistory:               config.EventHistory,
			newID:                      config.NewID,
			promptRenderer:             config.PromptRenderer,
			templateFieldResolver:      config.TemplateFieldResolver,
			promptSourceReader:         config.PromptSourceReader,
			mockWorkersConfig:          config.MockWorkers.Clone(),
			progressPublisher:          config.ProgressPublisher,
			net:                        config.Net,
			expectedArtifactFileSystem: expectedArtifactFileSystemFrom(config.ExpectedArtifactFileSystem),
		},
		sessionID: strings.TrimSpace(config.FactorySessionID),
	}
}

type workstationRequestExecutor struct {
	service   executeCapability
	cfg       *runtimeConfig
	sessionID string
}

// SetRuntimeLogger binds the opened Runtime's log sink to this compatibility
// adapter. The adapter itself remains detached; only the per-attempt logger
// capability crosses into the process-scoped Workers request.
func (executor *workstationRequestExecutor) SetRuntimeLogger(logger factory.Logger) {
	if executor == nil || executor.cfg == nil {
		return
	}
	executor.cfg.logger = logger
}

func isRuntimePromptRenderError(err error) bool {
	var promptErr *runtimePromptRenderError
	return errors.As(err, &promptErr)
}

type runtimePromptRenderError struct {
	cause error
}

func (err *runtimePromptRenderError) Error() string {
	if err == nil || err.cause == nil {
		return "runtime prompt rendering failed"
	}
	return err.cause.Error()
}

func (err *runtimePromptRenderError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func wrapRuntimePromptRenderError(err error) error {
	if err == nil {
		return nil
	}
	var promptErr *runtimePromptRenderError
	if errors.As(err, &promptErr) {
		return err
	}
	return &runtimePromptRenderError{cause: err}
}

func (executor workstationRequestExecutor) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	if executor.service == nil || executor.cfg == nil {
		return workers.WorkResult{}, workers.ErrExecuteUnavailable
	}
	request = workers.CloneWorkstationExecutionRequest(request)
	if strings.TrimSpace(request.FactorySessionID) == "" {
		request.FactorySessionID = executor.sessionID
	}
	request.WorkerType = firstRuntimeValue(request.WorkerType, request.Dispatch.WorkerType)
	if strings.TrimSpace(request.WorkstationType) == "" {
		request.WorkstationType = firstRuntimeValue(request.WorkstationType, request.Dispatch.WorkstationName)
	}
	request = runtimeRecordingExecutionRequest(executor.cfg, request)
	workstationRequest := workers.WorkstationDispatchRequest{
		WorkstationName: firstRuntimeValue(request.Dispatch.WorkstationName, request.WorkstationType),
		Execution:       request,
	}
	executeRequest, err := executeRequestFromWorkstationRequest(executor.cfg, workstationRequest)
	if err != nil {
		result := workers.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workers.OutcomeFailed,
			Error:        err.Error(),
		}
		if isRuntimePromptRenderError(err) {
			result.Error = "prompt render failed: " + err.Error()
			return result, nil
		}
		return result, err
	}
	result, executeErr := executor.service.Execute(ctx, executeRequest)
	result = normalizeDetachedExecutionResult(executor.cfg, executeRequest, result)
	dispatchResult, dispatchErr := workstationDispatchResultFromExecute(
		workstationDispatchRequestForResult(workstationRequest, executeRequest),
		result,
		executeErr,
	)
	return dispatchResult.Result, dispatchErr
}

func runtimeRecordingExecutionRequest(
	cfg *runtimeConfig,
	request workers.WorkstationExecutionRequest,
) workers.WorkstationExecutionRequest {
	if cfg == nil {
		return request
	}
	if strings.TrimSpace(request.RuntimeID) == "" {
		request.RuntimeID = strings.TrimSpace(cfg.runtimeID)
	}
	if strings.TrimSpace(request.RecordingID) == "" {
		request.RecordingID = strings.TrimSpace(cfg.recordingID)
	}
	if strings.TrimSpace(request.GenerationID) == "" && cfg.eventHistory != nil {
		request.GenerationID = strings.TrimSpace(cfg.eventHistory.StreamGenerationID())
	}
	return request
}

func validateRuntimeExecutionSelection(selection runtimeExecutionSelection) error {
	if strings.EqualFold(strings.TrimSpace(selection.runnerID), "script") &&
		strings.TrimSpace(selection.command) == "" {
		return fmt.Errorf("construct script worker: misconfigured: script command is required")
	}
	return nil
}

func finalizeRuntimeWorkspaceSelection(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	fileSystem expectedArtifactFileSystem,
) {
	baseDirectory := strings.TrimSpace(selection.factoryDirectory)
	if cfg != nil {
		if runtimeLookup, ok := cfg.runtimeConfig.(interfaces.RuntimeConfigLookup); ok && runtimeLookup != nil {
			baseDirectory = firstRuntimeValue(
				strings.TrimSpace(runtimeLookup.RuntimeBaseDir()),
				baseDirectory,
			)
		}
	}
	if selection.workingDirectory == "" {
		// Detached execution must carry the same default workspace that the
		// legacy workstation executor derived from RuntimeConfig.
		selection.workingDirectory = baseDirectory
	}
	selection.workingDirectory = resolveRuntimePath(baseDirectory, selection.workingDirectory, fileSystem)
}

func runtimeWorkflowContext(cfg *runtimeConfig, sessionID string, supplied *workers.Context) *workers.Context {
	if context := supplied.Clone(); context != nil {
		if strings.TrimSpace(context.SessionID) == "" {
			context.SessionID = strings.TrimSpace(sessionID)
		}
		return context
	}
	if cfg != nil {
		if context := cfg.workflowContext.Clone(); context != nil {
			if strings.TrimSpace(context.SessionID) == "" {
				context.SessionID = strings.TrimSpace(sessionID)
			}
			return context
		}
	}
	return &workers.Context{SessionID: strings.TrimSpace(sessionID)}
}

const classifierFailureRawOutputLimit = 160

func normalizeScriptClassifierResult(
	target workers.ExecutionTarget,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	// A script classifier is a classifier; either signal selects this policy.
	if !target.Output.Classifier && !target.Output.ScriptClassifier {
		return result
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		return result
	}
	raw := primaryOutputText(result.Output.Primary)
	output := strings.TrimSpace(raw)
	// A script classifier writes its label on the final stdout line. Every other
	// classifier worker is expected to produce the bare label as its response.
	if target.Output.ScriptClassifier && strings.TrimSpace(target.RunnerID) == "script" {
		if output == "" {
			return result
		}
		lines := strings.Split(output, "\n")
		output = strings.TrimSpace(lines[len(lines)-1])
	}
	label, err := normalizeClassifierLabel(output)
	if err != nil {
		message := classifierOutputErrorDetail(raw, err)
		result.Outcome = workers.ExecutionOutcomeFailed
		result.Output.Classification = ""
		result.Failure = &workers.ExecutionFailure{
			Family:  workers.WorkFailureFamilyTerminal,
			Type:    workers.WorkFailureTypeStructuredOutputSchemaViolation,
			Message: message,
			Detail: &workers.FailureDetail{
				Reason:  workers.WorkFailureTypeStructuredOutputSchemaViolation,
				Message: message,
			},
		}
		return result
	}
	result.Output.Primary = []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: label,
	}}
	result.Output.Classification = label
	return result
}

// normalizeClassifierLabel mirrors the classifier contract enforced by the
// Workers workstation executor: a classifier must emit a bare routing label,
// never a structured payload.
func normalizeClassifierLabel(output string) (string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", errors.New("empty label")
	}
	if json.Valid([]byte(trimmed)) {
		return "", errors.New("expected plain string label")
	}
	return trimmed, nil
}

func classifierOutputErrorDetail(rawOutput string, err error) string {
	detail := "classifier output invalid: " + err.Error()
	trimmed := strings.TrimSpace(rawOutput)
	if trimmed == "" {
		return detail
	}
	if len(trimmed) > classifierFailureRawOutputLimit {
		trimmed = trimmed[:classifierFailureRawOutputLimit] + "..."
	}
	return detail + " (raw output: " + strconv.Quote(trimmed) + ")"
}

func normalizeDetachedExecutionResult(
	cfg *runtimeConfig,
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	result = normalizeScriptClassifierResult(request.Target, result)
	return verifyExpectedArtifactsForDispatch(cfg, request, result)
}

// runtimeRecordingRequest carries process-owned recording, runtime, and
// generation identities into the detached Runtime path. Callers may provide
// explicit identities, while ordinary Factory dispatches inherit the values
// opened for this runtime.
func runtimeRecordingRequest(cfg *runtimeConfig, request workers.WorkstationDispatchRequest) workers.WorkstationDispatchRequest {
	if cfg == nil {
		return request
	}
	if strings.TrimSpace(request.Execution.RecordingID) == "" {
		request.Execution.RecordingID = strings.TrimSpace(cfg.recordingID)
	}
	if strings.TrimSpace(request.Execution.RuntimeID) == "" {
		request.Execution.RuntimeID = strings.TrimSpace(cfg.runtimeID)
	}
	if strings.TrimSpace(request.Execution.GenerationID) == "" && cfg.eventHistory != nil {
		request.Execution.GenerationID = strings.TrimSpace(cfg.eventHistory.StreamGenerationID())
	}
	return request
}

// capturedWorkerSessionControlTargets is the immutable set one committed
// Factory-turn control owns. The Factory Session identity is represented by
// the session-scoped canonical ledger captured below; associations from a
// different Factory Session must therefore be read from a different ledger.
//
// The later fan-out stories retain this value with the committed control. They
// must not call captureAssociatedWorkerSessionTargets again on a retry, or a
// retry could include a child associated after the control linearized.
type capturedWorkerSessionControlTargets struct {
	turnID           string
	workerSessionIDs []string
}

// workerSessionIDsSnapshot returns a detached deterministic target order.
func (c capturedWorkerSessionControlTargets) workerSessionIDsSnapshot() []string {
	return append([]string(nil), c.workerSessionIDs...)
}

// captureAssociatedWorkerSessionTargets selects exactly the Worker Sessions
// canonically associated with turnID at one ledger snapshot. It deliberately
// consumes only the W4 association event rather than tracking dispatches or
// retaining a second association registry. RequestID on that event is the
// Factory invocation's immutable turn correlation supplied by ACP.
func captureAssociatedWorkerSessionTargets(
	ledger recordings.RuntimeLedger,
	turnID string,
) capturedWorkerSessionControlTargets {
	turnID = strings.TrimSpace(turnID)
	if ledger == nil || turnID == "" {
		return capturedWorkerSessionControlTargets{turnID: turnID}
	}
	return selectAssociatedWorkerSessionTargets(ledger.CanonicalEvents(), turnID)
}

func selectAssociatedWorkerSessionTargets(
	events []interfaces.FactoryEvent,
	turnID string,
) capturedWorkerSessionControlTargets {
	turnID = strings.TrimSpace(turnID)
	captured := capturedWorkerSessionControlTargets{turnID: turnID}
	if turnID == "" {
		return captured
	}

	type candidate struct {
		sequence        int
		workerSessionID string
		eventID         string
		index           int
	}
	candidates := make([]candidate, 0)
	for index, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc ||
			event.Context.DispatchID == nil || strings.TrimSpace(*event.Context.DispatchID) == "" ||
			event.Context.RequestID == nil || strings.TrimSpace(*event.Context.RequestID) != turnID {
			continue
		}
		var association interfaces.DispatchWorkerSessionAssociationEventPayload
		if event.DecodePayload(&association) != nil {
			continue
		}
		workerSessionID := strings.TrimSpace(association.WorkerSessionID)
		if workerSessionID == "" {
			continue
		}
		candidates = append(candidates, candidate{
			sequence:        event.Context.Sequence,
			workerSessionID: workerSessionID,
			eventID:         event.Id,
			index:           index,
		})
	}

	// Canonical sequence is the normal order. The remaining keys make a
	// partially reconstructed or test fixture event stream deterministic too.
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].sequence != candidates[right].sequence {
			return candidates[left].sequence < candidates[right].sequence
		}
		if candidates[left].eventID != candidates[right].eventID {
			return candidates[left].eventID < candidates[right].eventID
		}
		if candidates[left].workerSessionID != candidates[right].workerSessionID {
			return candidates[left].workerSessionID < candidates[right].workerSessionID
		}
		return candidates[left].index < candidates[right].index
	})

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, duplicate := seen[candidate.workerSessionID]; duplicate {
			continue
		}
		seen[candidate.workerSessionID] = struct{}{}
		captured.workerSessionIDs = append(captured.workerSessionIDs, candidate.workerSessionID)
	}
	return captured
}
