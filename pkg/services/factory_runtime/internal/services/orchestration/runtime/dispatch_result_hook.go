package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// dispatchPlanningResultHook is the Runtime-owned bridge between scheduler
// dispatches and the canonical Workers request boundary. The delegate retains
// the existing live/replay delivery mechanics; the planner owns acceptance,
// publication, correlation, and duplicate retirement.
type dispatchPlanningResultHook struct {
	planner           dispatchplanning.Service
	net               *state.Net
	resultBuffer      *buffers.TypedBuffer[workerexecution.WorkResult]
	completionPlanner factory.CompletionDeliveryPlanner
	workService       work.Service
	workRequestIDs    work.RequestIDGenerator
	factorySessionID  string
	waitCh            chan struct{}
	scheduled         []scheduledDispatchResult
	asyncErr          error
	onResult          func()
	acceptanceStates  sync.Map
	mu                sync.Mutex
}

func (h *dispatchPlanningResultHook) SetOnBufferedResult(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onResult = fn
}

type scheduledDispatchResult struct {
	deliveryTick int
	result       workerexecution.WorkResult
}

// dispatchAcceptanceState coordinates the one potentially blocking Work
// materialization per dispatch without holding a lock across planner or Work
// calls. The planner remains the durable terminal authority; this state only
// shares the detached canonical result that concurrent contenders submit.
type dispatchAcceptanceState struct {
	mu            sync.Mutex
	ready         chan struct{}
	initialized   bool
	materializing bool
	result        workerexecution.WorkResult
	outcome       dispatchplanning.TerminalResultOutcome
}

func newDispatchAcceptanceState() *dispatchAcceptanceState {
	return &dispatchAcceptanceState{ready: make(chan struct{})}
}

func (s *dispatchAcceptanceState) claimRoot(
	result workerexecution.WorkResult,
	outcome dispatchplanning.TerminalResultOutcome,
) (workerexecution.WorkResult, dispatchplanning.TerminalResultOutcome, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		s.initialized = true
		s.result = result
		s.outcome = outcome
		close(s.ready)
		return result, outcome, nil
	}
	if s.materializing {
		return workerexecution.WorkResult{}, "", s.ready
	}
	return s.result, s.outcome, nil
}

func (s *dispatchAcceptanceState) claimWorker() (
	workerexecution.WorkResult,
	dispatchplanning.TerminalResultOutcome,
	<-chan struct{},
	bool,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		s.initialized = true
		s.materializing = true
		return workerexecution.WorkResult{}, "", nil, true
	}
	if s.materializing {
		return workerexecution.WorkResult{}, "", s.ready, false
	}
	return s.result, s.outcome, nil, false
}

func (s *dispatchAcceptanceState) finishWorker(
	result workerexecution.WorkResult,
	outcome dispatchplanning.TerminalResultOutcome,
) {
	s.mu.Lock()
	s.result = result
	s.outcome = outcome
	s.materializing = false
	close(s.ready)
	s.mu.Unlock()
}

func (s *dispatchAcceptanceState) wait() (workerexecution.WorkResult, dispatchplanning.TerminalResultOutcome) {
	<-s.ready
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result, s.outcome
}

type dispatchHookSnapshot = interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

var _ factory.DispatchResultHook = (*dispatchPlanningResultHook)(nil)
var _ factory.DispatchResultHookWakeSignaler = (*dispatchPlanningResultHook)(nil)

func newCanonicalDispatchPlanningResultHook(
	planner dispatchplanning.Service,
	net *state.Net,
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult],
	completionPlanner factory.CompletionDeliveryPlanner,
	workService work.Service,
	workRequestIDs work.RequestIDGenerator,
	factorySessionID string,
) *dispatchPlanningResultHook {
	return &dispatchPlanningResultHook{
		planner: planner, net: net, resultBuffer: resultBuffer,
		completionPlanner: completionPlanner, workService: workService, workRequestIDs: workRequestIDs,
		factorySessionID: factorySessionID,
		waitCh:           make(chan struct{}, 1),
	}
}

func (h *dispatchPlanningResultHook) acceptanceState(dispatchID string) *dispatchAcceptanceState {
	value, _ := h.acceptanceStates.LoadOrStore(
		strings.TrimSpace(dispatchID),
		newDispatchAcceptanceState(),
	)
	return value.(*dispatchAcceptanceState)
}

func (h *dispatchPlanningResultHook) SubmitDispatch(
	ctx context.Context,
	dispatch work.WorkDispatch,
) error {
	decision := h.runnableDecision(dispatch)
	planned, err := h.planner.Plan(ctx, dispatchplanning.PlanRequest{
		Decisions: []dispatchplanning.RunnableDecision{decision},
	})
	if err != nil {
		return err
	}
	if len(planned.Actions) != 1 {
		return fmt.Errorf("dispatch planning produced %d actions for dispatch %q", len(planned.Actions), dispatch.DispatchID)
	}
	if _, err = h.planner.Publish(ctx, planned.Actions[0]); err != nil {
		return err
	}
	return nil
}

func (h *dispatchPlanningResultHook) OnTick(
	ctx context.Context,
	snapshot dispatchHookSnapshot,
) ([]workerexecution.WorkResult, error) {
	_ = ctx
	return h.takeCanonicalResults(snapshot.TickCount)
}

func (h *dispatchPlanningResultHook) takeCanonicalResults(
	currentTick int,
) ([]workerexecution.WorkResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.asyncErr != nil {
		err := h.asyncErr
		h.asyncErr = nil
		return nil, err
	}
	due := make([]workerexecution.WorkResult, 0, len(h.scheduled))
	pending := h.scheduled[:0]
	for _, scheduled := range h.scheduled {
		if scheduled.deliveryTick <= currentTick {
			due = append(due, scheduled.result)
		} else {
			pending = append(pending, scheduled)
		}
	}
	h.scheduled = pending
	if len(h.scheduled) > 0 {
		h.signalCanonicalLocked()
	}
	return due, nil
}

func (h *dispatchPlanningResultHook) WaitCh() <-chan struct{} {
	return h.waitCh
}

func (h *dispatchPlanningResultHook) HasPendingResults() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.asyncErr != nil || len(h.scheduled) > 0 ||
		(h.resultBuffer != nil && h.resultBuffer.HasData())
}

func (h *dispatchPlanningResultHook) HasBufferedResults() bool {
	return h.HasPendingResults()
}

func (h *dispatchPlanningResultHook) SignalBufferedResults() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.asyncErr != nil || len(h.scheduled) > 0 ||
		(h.resultBuffer != nil && h.resultBuffer.HasData()) {
		h.signalCanonicalLocked()
	}
}

func (h *dispatchPlanningResultHook) acceptWorkersResult(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) {
	dispatchID := firstRuntimeValue(
		request.Execution.Dispatch.DispatchID, result.DispatchID,
	)
	workResult := canonicalWorkResult(request, result, dispatchErr)
	planned, usedPlanned, err := h.plannedWorkersResult(request, workResult)
	if err != nil {
		h.recordCanonicalError(err)
		return
	}
	if usedPlanned {
		h.acceptPlannedWorkersResult(ctx, request, dispatchID, result, planned)
		return
	}
	h.acceptLiveWorkersResult(ctx, request, dispatchID, result, workResult)
}

func (h *dispatchPlanningResultHook) plannedWorkersResult(
	request workers.WorkstationDispatchRequest,
	workResult workerexecution.WorkResult,
) (workerexecution.WorkResult, bool, error) {
	provider, ok := h.completionPlanner.(plannedCompletionResultProvider)
	if !ok {
		return workResult, false, nil
	}
	planned, hasPlanned, err := provider.PlannedResultForDispatch(request.Execution.Dispatch)
	if err != nil || !hasPlanned ||
		(workResult.Outcome == workerexecution.OutcomeFailed &&
			planned.Outcome != workerexecution.OutcomeFailed) {
		return workResult, false, err
	}
	planned.DispatchID = request.Execution.Dispatch.DispatchID
	planned.TransitionID = request.Execution.Dispatch.TransitionID
	return planned, true, nil
}

func (h *dispatchPlanningResultHook) acceptPlannedWorkersResult(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	dispatchID string,
	result workers.WorkstationDispatchResult,
	workResult workerexecution.WorkResult,
) {
	outcome := workstationTerminalResultOutcome(result.TerminalOutcome, workResult.Outcome)
	state := h.acceptanceState(dispatchID)
	workResult, outcome, wait := state.claimRoot(workResult, outcome)
	if wait != nil {
		workResult, outcome = state.wait()
	}
	h.recordAcceptedResult(ctx, request, workResult, outcome)
}

func (h *dispatchPlanningResultHook) acceptLiveWorkersResult(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	dispatchID string,
	result workers.WorkstationDispatchResult,
	workResult workerexecution.WorkResult,
) {
	intent, ok := h.planner.Intent(workResult.DispatchID)
	if !ok || intent.Result != nil {
		outcome := workstationTerminalResultOutcome(result.TerminalOutcome, workResult.Outcome)
		h.recordAcceptedResult(ctx, request, workResult, outcome)
		return
	}
	state := h.acceptanceState(dispatchID)
	sharedResult, sharedOutcome, wait, materializer := state.claimWorker()
	if wait != nil {
		workResult, outcome := state.wait()
		h.recordAcceptedResult(ctx, request, workResult, outcome)
		return
	}
	if materializer {
		workResult = materializeWorkerOutputForDispatchWithProposal(
			ctx, h.workService, h.net, h.workRequestIDs, request,
			workResult, result.ProposedOutput,
		)
		state.finishWorker(
			workResult,
			workstationTerminalResultOutcome(result.TerminalOutcome, workResult.Outcome),
		)
	} else {
		workResult = sharedResult
		h.recordAcceptedResult(ctx, request, workResult, sharedOutcome)
		return
	}
	h.recordAcceptedResult(
		ctx, request, workResult,
		workstationTerminalResultOutcome(result.TerminalOutcome, workResult.Outcome),
	)
}

func (h *dispatchPlanningResultHook) recordAcceptedResult(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	result workerexecution.WorkResult,
	outcome dispatchplanning.TerminalResultOutcome,
) {
	if _, err := h.acceptCanonicalResult(ctx, request, result, "", outcome); err != nil {
		h.recordCanonicalError(err)
	}
}

func canonicalWorkResult(
	request workers.WorkstationDispatchRequest,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) workerexecution.WorkResult {
	workResult := result.Result
	if workResult.DispatchID == "" {
		workResult.DispatchID = request.Execution.Dispatch.DispatchID
	}
	if workResult.TransitionID == "" {
		workResult.TransitionID = request.Execution.Dispatch.TransitionID
	}
	if workResult.Cancellation == nil {
		workResult.Cancellation = result.Cancellation.Clone()
	}
	if result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled ||
		workResult.Cancellation != nil || workResult.Outcome == workerexecution.OutcomeCanceled {
		workResult.Outcome = workerexecution.OutcomeCanceled
		if workResult.Cancellation == nil {
			workResult.Cancellation = &workerexecution.DispatchCancellation{Reason: workerexecution.DispatchCancellationReasonCanceled}
		}
		if workResult.Error == "" {
			workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
		}
		workResult.FailureDetail = nil
		workResult.FailureMetadata = nil
		return workResult
	}
	if dispatchErr != nil || result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeFailed {
		workResult.Outcome = workerexecution.OutcomeFailed
		if workResult.Error == "" && dispatchErr != nil {
			workResult.Error = dispatchErr.Error()
		}
	}
	return workResult
}

func workstationTerminalResultOutcome(
	terminal workers.WorkstationDispatchTerminalOutcome,
	outcome workerexecution.WorkOutcome,
) dispatchplanning.TerminalResultOutcome {
	switch terminal {
	case workers.WorkstationDispatchTerminalOutcomeCanceled:
		return dispatchplanning.TerminalResultOutcomeCancelled
	case workers.WorkstationDispatchTerminalOutcomeFailed:
		return dispatchplanning.TerminalResultOutcomeFailure
	default:
		mapped, err := terminalResultOutcome(outcome)
		if err != nil {
			return dispatchplanning.TerminalResultOutcomeFailure
		}
		return mapped
	}
}

func (h *dispatchPlanningResultHook) acceptRootResult(
	ctx context.Context,
	req factory.AcceptDispatchResultRequest,
	outcome dispatchplanning.TerminalResultOutcome,
) (dispatchplanning.RetirementResult, error) {
	intent, ok := h.planner.Intent(req.DispatchID)
	if !ok {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch ID %q is not present",
			dispatchplanning.ErrUnknownDispatchCorrelation,
			req.DispatchID,
		)
	}
	workResult := workerexecution.WorkResult{
		DispatchID:   req.DispatchID,
		TransitionID: intent.Action.Request.Execution.Dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}
	switch outcome {
	case dispatchplanning.TerminalResultOutcomeFailure:
		workResult.Outcome = workerexecution.OutcomeFailed
		workResult.Error = "Workers dispatch failed"
	case dispatchplanning.TerminalResultOutcomeCancelled:
		workResult.Outcome = workerexecution.OutcomeCanceled
		workResult.Cancellation = &workerexecution.DispatchCancellation{Reason: workerexecution.DispatchCancellationReasonCanceled}
		workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
	}
	state := h.acceptanceState(req.DispatchID)
	workResult, outcome, wait := state.claimRoot(workResult, outcome)
	if wait != nil {
		workResult, outcome = state.wait()
	}
	return h.acceptCanonicalResult(ctx, intent.Action.Request, workResult, req.WorkID, outcome)
}

func (h *dispatchPlanningResultHook) acceptCanonicalResult(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	result workerexecution.WorkResult,
	workID string,
	outcome dispatchplanning.TerminalResultOutcome,
) (dispatchplanning.RetirementResult, error) {
	intent, ok := h.planner.Intent(result.DispatchID)
	if !ok {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch ID %q is not present",
			dispatchplanning.ErrUnknownDispatchCorrelation,
			result.DispatchID,
		)
	}
	workIDs := request.Execution.Dispatch.Execution.WorkIDs
	if len(workIDs) == 0 {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch %q has no Work lineage",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
			result.DispatchID,
		)
	}
	if workID == "" {
		workID = workIDs[0]
	}
	terminal := dispatchplanning.TerminalResult{
		DispatchID: result.DispatchID, CorrelationID: intent.Action.CorrelationID,
		WorkID: workID, Outcome: outcome,
	}
	if intent.Result != nil {
		return h.planner.Retire(ctx, terminal)
	}
	deliveryTick, scheduled := 0, false
	if h.completionPlanner != nil {
		var scheduleErr error
		deliveryTick, scheduled, scheduleErr = h.completionPlanner.DeliveryTickForDispatch(
			request.Execution.Dispatch,
		)
		if scheduleErr != nil {
			return dispatchplanning.RetirementResult{}, scheduleErr
		}
	}
	if !scheduled && h.resultBuffer == nil {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: Runtime result buffer is unavailable",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
		)
	}
	retired, err := h.planner.Retire(ctx, terminal)
	if err != nil || retired.Outcome != dispatchplanning.RetirementOutcomeRetired {
		return retired, err
	}
	if scheduled {
		h.mu.Lock()
		h.scheduled = append(h.scheduled, scheduledDispatchResult{
			deliveryTick: deliveryTick,
			result:       result,
		})
		h.notifyCanonicalResultLocked()
		h.signalCanonicalLocked()
		h.mu.Unlock()
		return retired, nil
	}
	wrote := h.resultBuffer.Write(ctx, result)
	if !wrote {
		h.mu.Lock()
		h.scheduled = append(h.scheduled, scheduledDispatchResult{result: result})
		h.notifyCanonicalResultLocked()
		h.signalCanonicalLocked()
		h.mu.Unlock()
		return retired, nil
	}
	h.mu.Lock()
	h.notifyCanonicalResultLocked()
	h.signalCanonicalLocked()
	h.mu.Unlock()
	return retired, nil
}

func (h *dispatchPlanningResultHook) recordCanonicalError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.asyncErr == nil {
		h.asyncErr = err
	}
	h.signalCanonicalLocked()
}

func (h *dispatchPlanningResultHook) signalCanonicalLocked() {
	select {
	case h.waitCh <- struct{}{}:
	default:
	}
}

func (h *dispatchPlanningResultHook) notifyCanonicalResultLocked() {
	if h.onResult != nil {
		h.onResult()
	}
}

func (h *dispatchPlanningResultHook) runnableDecision(
	dispatch work.WorkDispatch,
) dispatchplanning.RunnableDecision {
	workerType := dispatch.WorkerType
	if transition := h.net.Transitions[dispatch.TransitionID]; workerType == "" && transition != nil {
		workerType = transition.WorkerType
	}
	if workerType == "" {
		workerType = dispatch.WorkstationName
	}
	return dispatchplanning.RunnableDecision{
		CorrelationID: dispatch.DispatchID,
		Dispatch:      dispatch,
		Execution: dispatchplanning.ExecutionFacts{
			WorkerType:       workerType,
			ProjectID:        dispatch.ProjectID,
			InputPayload:     cloneDispatchInput(dispatch.InputTokens),
			FactorySessionID: h.factorySessionID,
		},
	}
}

func cloneDispatchInput(input []any) []any {
	cloned := make([]any, len(input))
	copy(cloned, input)
	return cloned
}

func terminalResultOutcome(outcome workerexecution.WorkOutcome) (dispatchplanning.TerminalResultOutcome, error) {
	switch outcome {
	case workerexecution.OutcomeAccepted,
		workerexecution.OutcomeContinue,
		workerexecution.OutcomeRejected:
		return dispatchplanning.TerminalResultOutcomeSuccess, nil
	case workerexecution.OutcomeFailed:
		return dispatchplanning.TerminalResultOutcomeFailure, nil
	case workerexecution.OutcomeCanceled:
		return dispatchplanning.TerminalResultOutcomeCancelled, nil
	default:
		return "", fmt.Errorf(
			"%w: Workers outcome %q is not terminal",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
			outcome,
		)
	}
}

type plannedCompletionResultProvider interface {
	PlannedResultForDispatch(dispatch work.WorkDispatch) (workerexecution.WorkResult, bool, error)
}

func recordedWorkExists(world interfaces.FactoryWorldState, events []interfaces.FactoryEvent, workID string) bool {
	if _, ok := world.WorkItemsByID[workID]; ok {
		return true
	}
	if _, ok := world.ActiveWorkItemsByID[workID]; ok {
		return true
	}
	if _, ok := world.TerminalWorkByID[workID]; ok {
		return true
	}
	if _, ok := world.FailedWorkItemsByID[workID]; ok {
		return true
	}
	for _, event := range events {
		if containsRecordedWorkID(pointerStringSlice(event.Context.WorkIDs), workID) {
			return true
		}
	}
	return false
}

func recordedObservationState(outcome string) workersessions.State {
	switch workers.WorkOutcome(outcome) {
	case workers.OutcomeAccepted, workers.OutcomeContinue:
		return workersessions.StateCompleted
	case workers.OutcomeFailed, workers.OutcomeRejected:
		return workersessions.StateFailed
	default:
		return workersessions.StateFailed
	}
}

func recordedFailure(outcome workers.WorkOutcome, detail *workers.FailureDetail, metadata *workers.WorkFailureMetadata, state workersessions.State) *workersessions.FailureCause {
	return recordedFailureWithDiagnostics(outcome, detail, metadata, state, nil)
}

func recordedFailureWithDiagnostics(
	outcome workers.WorkOutcome,
	detail *workers.FailureDetail,
	metadata *workers.WorkFailureMetadata,
	state workersessions.State,
	diagnostics *workers.SafeWorkDiagnostics,
) *workersessions.FailureCause {
	if !state.Terminal() || state == workersessions.StateCompleted {
		return nil
	}
	kind := workersessions.FailureCauseWorkersExecutionFailure
	if outcome == workers.OutcomeRejected {
		kind = workersessions.FailureCauseRejected
	} else if recordedIncompleteOutput(diagnostics) {
		kind = workersessions.FailureCauseIncompleteOutput
	}
	return &workersessions.FailureCause{Kind: kind, Detail: recordedFailureDetail(kind, detail, metadata), ProviderFailureKind: recordedProviderFailureKind(detail)}
}

func recordedIncompleteOutput(diagnostics *workers.SafeWorkDiagnostics) bool {
	if diagnostics == nil || diagnostics.Provider == nil {
		return false
	}
	metadata := diagnostics.Provider.ResponseMetadata
	if strings.ToLower(strings.TrimSpace(metadata[workerexecution.ProviderResponseMetadataFailureOperation])) != "completion_validation" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(metadata[workerexecution.ProviderResponseMetadataFailureClassification])) {
	case "contradictory_completion", "missing_completion_evidence", "missing_required_output":
		return true
	default:
		return false
	}
}

func recordedFailureDetail(kind workersessions.FailureCauseKind, detail *workers.FailureDetail, metadata *workers.WorkFailureMetadata) string {
	if kind == workersessions.FailureCauseRejected {
		return "the Workers result was rejected by the business review"
	}
	if kind == workersessions.FailureCauseIncompleteOutput {
		return "the Workers result did not include the required final output"
	}
	if metadata != nil {
		family, familyKnown := recordedFailureFamily(metadata.Family)
		typ, typeKnown := recordedFailureType(metadata.Type)
		if familyKnown && typeKnown && (family != "" || typ != "") {
			if family == "" {
				family = "unknown"
			}
			if typ == "" {
				typ = "unknown"
			}
			return "family=" + family + " type=" + typ
		}
	}
	if typ, ok := recordedFailureType(detailType(detail)); ok && typ != "" {
		return "type=" + typ
	}
	return "the Workers execution result was not successful"
}

func detailType(detail *workers.FailureDetail) workers.WorkFailureType {
	if detail == nil {
		return ""
	}
	return detail.Reason
}

func recordedFailureFamily(family workers.WorkFailureFamily) (string, bool) {
	switch family {
	case "":
		return "", true
	case workers.WorkFailureFamilyTerminal, workers.WorkFailureFamilyRetryable, workers.WorkFailureFamilyThrottle:
		return string(family), true
	default:
		return "", false
	}
}

func recordedFailureType(typ workers.WorkFailureType) (string, bool) {
	switch typ {
	case "":
		return "", true
	case workers.WorkFailureTypeAuthFailure, workers.WorkFailureTypePermanentBadRequest, workers.WorkFailureTypeThrottled,
		workers.WorkFailureTypeInternalServerError, workers.WorkFailureTypeTimeout, workers.WorkFailureTypeUnknown,
		workers.WorkFailureTypeMisconfigured, workers.WorkFailureTypeCommandLineTooLong, workers.WorkFailureTypeMissingExecutable,
		workers.WorkFailureTypeStructuredOutputSchemaViolation, workers.WorkFailureTypeExpectedArtifactsUnsatisfied:
		return string(typ), true
	default:
		return "", false
	}
}

func recordedProviderFailureKind(detail *workers.FailureDetail) providers.ExecuteFailureKind {
	if detail == nil {
		return ""
	}
	switch detail.Reason {
	case workers.WorkFailureTypeAuthFailure:
		return providers.ExecuteFailureKindAuthentication
	case workers.WorkFailureTypeTimeout:
		return providers.ExecuteFailureKindTimeout
	case workers.WorkFailureTypeThrottled:
		return providers.ExecuteFailureKindThrottled
	case workers.WorkFailureTypeMisconfigured:
		return providers.ExecuteFailureKindMisconfigured
	default:
		return ""
	}
}

func mergeRecordedObservations(recorded, live []workersessions.Observation) []workersessions.Observation {
	if len(recorded) == 0 && len(live) == 0 {
		return nil
	}

	// Recorded facts remain authoritative for an overlapping Worker Session,
	// while the live registry can contain a session whose association has not
	// reached the durable projection yet. Clone both sources so the read
	// decorator never mutates a service-owned observation while reconciling the
	// two views.
	merged := make([]workersessions.Observation, 0, len(recorded)+len(live))
	seen := make(map[string]struct{}, len(recorded)+len(live))
	liveBySession := make(map[string]workersessions.Observation, len(live))
	for _, observation := range live {
		liveBySession[observation.WorkerSessionID] = observation.Clone()
	}

	for _, recordedObservation := range recorded {
		if _, alreadyAdded := seen[recordedObservation.WorkerSessionID]; alreadyAdded {
			continue
		}
		seen[recordedObservation.WorkerSessionID] = struct{}{}
		mergedObservation := recordedObservation.Clone()
		if liveObservation, ok := liveBySession[recordedObservation.WorkerSessionID]; ok {
			mergeLiveObservation(&mergedObservation, liveObservation)
		}
		merged = append(merged, mergedObservation)
	}

	for _, liveObservation := range live {
		if _, alreadyAdded := seen[liveObservation.WorkerSessionID]; alreadyAdded {
			continue
		}
		seen[liveObservation.WorkerSessionID] = struct{}{}
		merged = append(merged, liveObservation.Clone())
	}
	sortObservationAttempts(merged)
	return merged
}

func mergeLiveObservation(recorded *workersessions.Observation, live workersessions.Observation) {
	if recorded == nil {
		return
	}
	if recorded.PredecessorWorkerSessionID == "" {
		recorded.PredecessorWorkerSessionID = live.PredecessorWorkerSessionID
	}
	if recorded.SuccessorWorkerSessionID == "" {
		recorded.SuccessorWorkerSessionID = live.SuccessorWorkerSessionID
	}
	if recorded.FactorySessionID == "" {
		recorded.FactorySessionID = live.FactorySessionID
	}
	recorded.Direct = live.Direct
	if len(recorded.WorkIDs) == 0 && len(live.WorkIDs) > 0 {
		recorded.WorkIDs = append([]string(nil), live.WorkIDs...)
	}
	if live.StartedAt != nil {
		started := *live.StartedAt
		recorded.StartedAt = &started
	}
	if recorded.EndedAt == nil && live.EndedAt != nil {
		ended := *live.EndedAt
		recorded.EndedAt = &ended
	}
	if recorded.Duration == nil && live.Duration != nil {
		duration := *live.Duration
		recorded.Duration = &duration
		if recorded.DurationBasis == workersessions.DurationBasisUnavailable {
			recorded.DurationBasis = live.DurationBasis
		}
	}
	if live.Model != nil && strings.TrimSpace(*live.Model) != "" {
		recorded.Model = cloneRecordedString(live.Model)
	}
	if live.ReasoningEffort != nil && strings.TrimSpace(*live.ReasoningEffort) != "" {
		recorded.ReasoningEffort = cloneRecordedString(live.ReasoningEffort)
	}
	if live.ProviderSessionAvailable {
		recorded.ProviderSession = live.ProviderSession.Clone()
		recorded.ProviderSessionAvailable = true
	}
	if live.TokenUsage != nil {
		clone := live.TokenUsage.Clone()
		recorded.TokenUsage = &clone
	}
	if live.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		recorded.Transcript = live.Transcript
		recorded.Parse = live.Parse.Clone()
	}
	if recorded.Failure == nil && live.Failure != nil {
		failure := *live.Failure
		recorded.Failure = &failure
	}
}

func sortObservationAttempts(observations []workersessions.Observation) {
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		switch {
		case left.StartedAt != nil && right.StartedAt != nil && !left.StartedAt.Equal(*right.StartedAt):
			return left.StartedAt.Before(*right.StartedAt)
		case left.StartedAt != nil && right.StartedAt == nil:
			return true
		case left.StartedAt == nil && right.StartedAt != nil:
			return false
		case left.AttemptID != right.AttemptID:
			return left.AttemptID < right.AttemptID
		default:
			return left.WorkerSessionID < right.WorkerSessionID
		}
	})
}

func cloneAndSortFactoryEvents(events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
	ordered := make([]interfaces.FactoryEvent, len(events))
	for index, event := range events {
		ordered[index] = event.Clone()
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.Context.Tick != right.Context.Tick {
			return left.Context.Tick < right.Context.Tick
		}
		if left.Context.Sequence != right.Context.Sequence {
			return left.Context.Sequence < right.Context.Sequence
		}
		if !left.Context.EventTime.Equal(right.Context.EventTime) {
			return left.Context.EventTime.Before(right.Context.EventTime)
		}
		return left.Id < right.Id
	})
	return ordered
}

func closeRuntimeEventSubscriptions(ledger recordings.RuntimeLedger) {
	if ledger == nil {
		return
	}
	closer, ok := ledger.(interface{ CloseLiveSubscriptions() })
	if !ok {
		return
	}
	closer.CloseLiveSubscriptions()
}

func eventTimeForDispatch(events []interfaces.FactoryEvent, dispatchID string) time.Time {
	for index := len(events) - 1; index >= 0; index-- {
		if stringPointerValue(events[index].Context.DispatchID) == dispatchID && events[index].Type == interfaces.FactoryEventTypeDispatchResponse {
			return events[index].Context.EventTime.UTC()
		}
	}
	return time.Time{}
}

func firstRecordedTime(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary.UTC()
	}
	return fallback.UTC()
}

func firstRecordedWorkIDs(primary, fallback []string) []string {
	if len(primary) > 0 {
		return append([]string(nil), primary...)
	}
	return append([]string(nil), fallback...)
}

func firstRecordedFailure(primary, fallback *workersessions.FailureCause) *workersessions.FailureCause {
	if primary != nil {
		return primary
	}
	return fallback
}

func containsRecordedWorkID(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendUniqueRecordedString(values []string, value string) []string {
	if value == "" || containsRecordedWorkID(values, value) {
		return values
	}
	return append(values, value)
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func pointerStringSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return append([]string(nil), (*value)...)
}

func providerSessionRef(metadata providers.SessionMetadata) providers.SessionRef {
	return providers.SessionRef{Provider: providers.ID(metadata.Provider), Kind: metadata.Kind, ID: metadata.ID}
}

func cloneProviderMetadata(metadata *providers.SessionMetadata) *providers.SessionMetadata {
	if metadata == nil {
		return nil
	}
	clone := *metadata
	return &clone
}

func isObservationNotFound(err error) bool {
	return err == workersessions.ErrObservationWorkNotFound
}

func isObservationProjectionUnavailable(err error) bool {
	return err == workersessions.ErrObservationProjectionUnavailable
}

func observationContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return workersessions.ErrObservationCanceled
	}
	return nil
}

func (f *factoryImpl) currentWorldState(tick int) *interfaces.FactoryWorldState {
	if f.eventHistory == nil || f.cfg == nil || f.cfg.worldStateProjector == nil {
		return nil
	}
	if recorder, ok := f.eventHistory.(interface {
		RecordCanonicalHistoryReduction()
	}); ok && recorder != nil {
		recorder.RecordCanonicalHistoryReduction()
	}
	state, err := f.cfg.worldStateProjector(f.eventHistory.CanonicalEvents(), tick)
	if err != nil {
		f.logger.Warn("factory world-state reconstruction failed; falling back to runtime snapshot", "error", err)
		return nil
	}
	return &state
}

func (f *factoryImpl) deriveRuntimeStatus(currentState interfaces.FactoryState, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) interfaces.RuntimeStatus {
	if currentState == interfaces.FactoryStateCompleted || currentState == interfaces.FactoryStateFailed {
		return interfaces.RuntimeStatusFinished
	}
	if snapshot.InFlightCount > 0 || len(snapshot.Dispatches) > 0 || hasNonTerminalWork(snapshot.Marking, f.topology) {
		return interfaces.RuntimeStatusActive
	}
	return interfaces.RuntimeStatusIdle
}

func hasNonTerminalWork(marking petri.MarkingSnapshot, topology *state.Net) bool {
	if topology == nil {
		return false
	}
	for _, token := range marking.Tokens {
		if token == nil || token.Color.DataType == factorytoken.DataTypeResource || token.Color.WorkTypeID == "" {
			continue
		}
		category := topology.StateCategoryForPlace(token.PlaceID)
		if category != state.StateCategoryTerminal && category != state.StateCategoryFailed {
			return true
		}
	}
	return false
}
