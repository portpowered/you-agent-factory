package factorysessions

import (
	"context"
	"fmt"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"strings"
)

// RuntimeSidecars owns runtime-scoped background services without exposing
// their concrete host implementation to Factory Sessions consumers.
type RuntimeSidecars = runtimeports.RuntimeSidecarService

type DefinitionHost = factorydefinitions.SessionHost

// --- merged from invocation_contract.go ---

// Invocation root slice freezes request, resolved-input, result, timeout, and
// cancellation/error vocabulary on the singular Service aggregate. Peers
// consume these plain root contracts without importing private invocation
// subservice types or Work implementation packages beyond approved peer root
// contracts already present in root signatures:
//
//   - Request: InvocationRequest
//   - Resolved input: ResolvedInvocationInput
//   - Result: InvocationResult (+ InvocationTerminalStatus / InvocationErrorCode)
//   - Timeout: InvocationTimeout / DefaultInvocationTimeout (and TimeoutMillis on
//     InvocationRequest)
//   - Invalid input: *InvocationValidationError
//   - Timeout / caller-cancellation outcomes: InvocationResult with distinct
//     Status and ErrorCode values (TIMED_OUT / CANCELED)
//
// InvocationService is the narrow, owner-published Factory Sessions capability
// for one-shot invocation. It retains the singular Service as the only session
// authority: Service satisfies this interface structurally, and the interface
// neither constructs nor locates a session service.
//
// It intentionally exposes no live-session control, durable execution,
// opening, listing, or inspection method. Consumers that only invoke a captured
// Factory Session can therefore depend on this single owner-published operation.
type InvocationService interface {
	InvokeFactorySession(context.Context, string, InvocationRequest) (InvocationResult, error)
}

// Service satisfies InvocationService structurally. Keep this assertion at the
// owner root so a change to the published invocation contract cannot drift from
// the singular service implementation.
var _ InvocationService = (Service)(nil)

// InvocationResult is the plain root session-scoped outcome of one Factory
// Session invocation after input resolution and result selection.
type InvocationResult struct {
	RequestID     string
	TraceID       string
	Status        InvocationTerminalStatus
	PrimaryResult []work.WorkContentPart
	ErrorCode     string
	Message       string
	SessionID     string
	WorkID        string
	WorkName      string
	WorkState     string
}

// InvocationTerminalStatus is the Factory Session-owned terminal outcome for
// one invocation on the published root slice.
type InvocationTerminalStatus string

const (
	InvocationTerminalStatusCanceled  InvocationTerminalStatus = "CANCELED"
	InvocationTerminalStatusCompleted InvocationTerminalStatus = "COMPLETED"
	InvocationTerminalStatusFailed    InvocationTerminalStatus = "FAILED"
	InvocationTerminalStatusTimedOut  InvocationTerminalStatus = "TIMED_OUT"
)

// InvocationErrorCode is the stable Factory Session-owned failure code emitted
// with a non-completed invocation result on the published root slice.
type InvocationErrorCode string

const (
	InvocationErrorCodeCanceled       InvocationErrorCode = "INVOCATION_CANCELED"
	InvocationErrorCodeRuntimeFailure InvocationErrorCode = "INVOCATION_RUNTIME_FAILURE"
	InvocationErrorCodeTimedOut       InvocationErrorCode = "INVOCATION_TIMED_OUT"
)

// InvocationTimeout is the published root name for the Factory Sessions-owned
// lifecycle budget applied to one invocation.
type InvocationTimeout = ModelInvocationTimeout

// DefaultInvocationTimeout is the published default invocation lifecycle budget.
const DefaultInvocationTimeout = DefaultModelInvocationTimeout

// InvocationValidationError is the typed invalid-input failure published on the
// invocation root slice. Peers match it with errors.As without importing private
// invocation subservice types.
type InvocationValidationError struct {
	Field   string
	Message string
}

func (err *InvocationValidationError) Error() string {
	if err == nil {
		return "invocation validation error"
	}
	if err.Field == "" {
		return err.Message
	}
	if err.Message == "" {
		return fmt.Sprintf("invocation validation failed for %s", err.Field)
	}
	return fmt.Sprintf("%s: %s", err.Field, err.Message)
}

// DetachedOperations is the canonical detached view over an existing Factory
// Sessions operation owner. It owns no registry and constructs no child
// service; each method translates a value-only request to the existing live
// or durable owner operation and returns a detached projection. Transport
// compatibility capabilities remain explicit on the owner and are not
// folded back into the aggregate Service.
type DetachedOperationsOwner interface {
	Service
	LiveControlService
	LiveResultService
}

type DetachedOperations struct {
	owner DetachedOperationsOwner
}

// Bind attaches the P4-A operation view to the already-composed Sessions root.
// Binding is inert: it retains the owner and constructs no child service.
func (operations *DetachedOperations) Bind(owner DetachedOperationsOwner) (DetachedService, error) {
	if owner == nil {
		return nil, ErrDetachedServiceUnavailable
	}
	if operations == nil {
		operations = &DetachedOperations{}
	}
	operations.owner = owner
	return operations, nil
}

func (operations *DetachedOperations) Start(
	ctx context.Context,
	request SessionStartRequest,
) (SessionStartResult, error) {
	if err := validateDetachedStartRequest(request); err != nil {
		return SessionStartResult{}, err
	}
	if operations == nil || operations.owner == nil {
		return SessionStartResult{}, ErrDetachedServiceUnavailable
	}

	switch request.Mode {
	case SessionOperationModeLive:
		opened, err := operations.owner.OpenFactorySession(ctx, OpenRequest{
			FolderPath:     strings.TrimSpace(request.FolderPath),
			Target:         cloneTargetRef(request.Target),
			ValidateOnly:   request.ValidateOnly,
			InitNewFactory: request.InitNewFactory,
		})
		if err != nil {
			return SessionStartResult{}, err
		}
		if opened == nil {
			return SessionStartResult{}, fmt.Errorf("open Factory Session returned no result")
		}
		live := detachedOpenResult(*opened)
		return SessionStartResult{
			SessionID: opened.SessionID,
			Mode:      SessionOperationModeLive,
			Status:    "OPENED",
			Live:      &live,
		}, nil
	case SessionOperationModeDurable:
		legacyRequest := legacyStartRequest(request)
		if request.Synchronous {
			started, err := operations.owner.StartSync(ctx, legacyRequest)
			if err != nil {
				return SessionStartResult{}, err
			}
			return SessionStartResult{
				SessionID: started.SessionID,
				Mode:      SessionOperationModeDurable,
				Status:    startedStatus(started.AsyncStartResult.Status, string(started.SyncOutcome)),
				Sync:      cloneSyncStartResult(&started),
			}, nil
		}
		started, err := operations.owner.StartAsync(ctx, legacyRequest)
		if err != nil {
			return SessionStartResult{}, err
		}
		return SessionStartResult{
			SessionID: started.SessionID,
			Mode:      SessionOperationModeDurable,
			Status:    started.Status,
			Async:     cloneAsyncStartResult(&started),
		}, nil
	default:
		return SessionStartResult{}, detachedRequestError("mode", "mode must be live or durable")
	}
}

func (operations *DetachedOperations) Invoke(
	ctx context.Context,
	request SessionInvokeRequest,
) (InvocationResult, error) {
	if err := validateSessionID(request.SessionID); err != nil {
		return InvocationResult{}, err
	}
	if request.Wait.TimeoutMillis < 0 {
		return InvocationResult{}, detachedRequestError("wait.timeoutMillis", "timeout must not be negative")
	}
	if operations == nil || operations.owner == nil {
		return InvocationResult{}, ErrDetachedServiceUnavailable
	}

	legacyRequest := InvocationRequest{}
	if request.Input != nil {
		legacyRequest.PreparedInvocationInput = request.Input.Clone()
		legacyRequest.ContentProvided = true
		sourceKind := InvocationInputSourceKind(request.Input.Source)
		legacyRequest.SourceKind = &sourceKind
	}
	if request.Correlation.RequestID != "" {
		requestID := strings.TrimSpace(request.Correlation.RequestID)
		legacyRequest.RequestID = &requestID
	}
	if request.Wait.TimeoutMillis > 0 {
		timeoutMillis := request.Wait.TimeoutMillis
		legacyRequest.TimeoutMillis = &timeoutMillis
	}
	return operations.owner.InvokeFactorySession(ctx, request.SessionID, legacyRequest)
}

func (operations *DetachedOperations) Activate(
	ctx context.Context,
	request SessionActivateRequest,
) (SessionActivateResult, error) {
	if err := validateSessionID(request.SessionID); err != nil {
		return SessionActivateResult{}, err
	}
	if operations == nil || operations.owner == nil {
		return SessionActivateResult{}, ErrDetachedServiceUnavailable
	}
	name := strings.TrimSpace(request.FactoryName)
	if name == "" {
		name = strings.TrimSpace(request.Definition.FactoryID)
	}
	if name == "" {
		return SessionActivateResult{}, detachedRequestError("factoryName", "factory name is required")
	}
	if err := operations.owner.ActivateNamedFactory(ctx, name); err != nil {
		return SessionActivateResult{}, err
	}
	return SessionActivateResult{
		SessionID:   request.SessionID,
		FactoryName: name,
		Activated:   true,
	}, nil
}

func (operations *DetachedOperations) Get(
	ctx context.Context,
	request SessionGetRequest,
) (SessionGetResult, error) {
	if err := validateSessionID(request.SessionID); err != nil {
		return SessionGetResult{}, err
	}
	if operations == nil || operations.owner == nil {
		return SessionGetResult{}, ErrDetachedServiceUnavailable
	}

	switch request.Mode {
	case SessionOperationModeLive:
		projection, err := operations.owner.GetFactorySession(ctx, request.SessionID)
		if err != nil {
			return SessionGetResult{}, err
		}
		return SessionGetResult{Session: detachedLiveSessionView(projection)}, nil
	case SessionOperationModeDurable:
		projection, err := operations.owner.GetSession(ctx, request.SessionID)
		if err != nil {
			return SessionGetResult{}, err
		}
		return SessionGetResult{Session: detachedDurableView(projection)}, nil
	default:
		return SessionGetResult{}, detachedRequestError("mode", "mode must be live or durable")
	}
}

func (operations *DetachedOperations) List(
	ctx context.Context,
	request SessionListRequest,
) (SessionListResult, error) {
	if operations == nil || operations.owner == nil {
		return SessionListResult{}, ErrDetachedServiceUnavailable
	}

	result := SessionListResult{Mode: request.Mode}
	switch request.Mode {
	case SessionOperationModeLive:
		live, err := operations.owner.ListFactorySessions(ctx)
		if err != nil {
			return SessionListResult{}, err
		}
		result.Sessions = make([]SessionView, 0, len(live))
		for _, projection := range live {
			result.Sessions = append(result.Sessions, detachedLiveReadView(projection))
		}
		return result, nil
	case SessionOperationModeDurable:
		durable, err := operations.owner.ListSessions(ctx, ListSessionsRequest{
			Scope:   SessionListScopePersisted,
			Filters: cloneSessionListFilters(request.Filters),
		})
		if err != nil {
			return SessionListResult{}, err
		}
		result.Sessions = durableSessionViews(durable.DurableSessions)
		return result, nil
	case SessionOperationModeAll:
		live, err := operations.owner.ListFactorySessions(ctx)
		if err != nil {
			return SessionListResult{}, err
		}
		durable, err := operations.owner.ListSessions(ctx, ListSessionsRequest{
			Scope:   SessionListScopePersisted,
			Filters: cloneSessionListFilters(request.Filters),
		})
		if err != nil {
			return SessionListResult{}, err
		}
		result.Sessions = make([]SessionView, 0, len(live)+len(durable.DurableSessions))
		for _, projection := range live {
			result.Sessions = append(result.Sessions, detachedLiveReadView(projection))
		}
		result.Sessions = append(result.Sessions, durableSessionViews(durable.DurableSessions)...)
		return result, nil
	default:
		return SessionListResult{}, detachedRequestError("mode", "mode must be live, durable, or all")
	}
}

func (operations *DetachedOperations) Control(
	ctx context.Context,
	request SessionControlRequest,
) (SessionControlResult, error) {
	if err := validateSessionID(request.SessionID); err != nil {
		return SessionControlResult{}, err
	}
	if operations == nil || operations.owner == nil {
		return SessionControlResult{}, ErrDetachedServiceUnavailable
	}
	control := request.Control
	if control.RequestID == "" {
		control.RequestID = strings.TrimSpace(request.Correlation.RequestID)
	}
	if control.TurnID == "" {
		control.TurnID = strings.TrimSpace(request.Correlation.TurnID)
	}

	switch request.Mode {
	case SessionOperationModeLive:
		return operations.controlLive(ctx, request, control)
	case SessionOperationModeDurable:
		return operations.controlDurable(ctx, request, control)
	default:
		return SessionControlResult{}, detachedRequestError("mode", "mode must be live or durable")
	}
}

func (operations *DetachedOperations) ReadResult(
	ctx context.Context,
	request SessionResultReadRequest,
) (SessionResultReadResult, error) {
	if err := validateSessionID(request.SessionID); err != nil {
		return SessionResultReadResult{}, err
	}
	if operations == nil || operations.owner == nil {
		return SessionResultReadResult{}, ErrDetachedServiceUnavailable
	}

	switch request.Mode {
	case SessionOperationModeDurable:
		result, err := operations.owner.GetResult(ctx, request.SessionID, request.Request)
		if err != nil {
			return SessionResultReadResult{}, err
		}
		return SessionResultReadResult{
			SessionID: request.SessionID,
			Mode:      SessionOperationModeDurable,
			Status:    string(result.ResultStatus),
			Durable:   cloneDurableResult(result),
		}, nil
	case SessionOperationModeLive:
		if request.Request.Mode == ResultModePartial {
			result, err := operations.owner.GetFactorySessionPartialResult(ctx, request.SessionID)
			if err != nil {
				return SessionResultReadResult{}, err
			}
			return SessionResultReadResult{
				SessionID: request.SessionID,
				Mode:      SessionOperationModeLive,
				Status:    "PARTIAL",
				Live: &SessionLiveResult{
					SessionID:         result.SessionID,
					Status:            "PARTIAL",
					CheckpointRefs:    cloneCheckpointRefs(result.CheckpointRefs),
					ResultArtifactRef: cloneArtifactRef(result.PartialResultArtifactRef),
				},
			}, nil
		}
		result, err := operations.owner.GetFactorySessionResult(ctx, request.SessionID)
		if err != nil {
			return SessionResultReadResult{}, err
		}
		return SessionResultReadResult{
			SessionID: request.SessionID,
			Mode:      SessionOperationModeLive,
			Status:    fmt.Sprint(result.Status),
			Live: &SessionLiveResult{
				SessionID:         result.SessionID,
				Status:            fmt.Sprint(result.Status),
				CheckpointRefs:    cloneCheckpointRefs(result.CheckpointRefs),
				ResultArtifactRef: cloneArtifactRef(result.ResultArtifactRef),
			},
		}, nil
	default:
		return SessionResultReadResult{}, detachedRequestError("mode", "mode must be live or durable")
	}
}

func (operations *DetachedOperations) PrepareSync(
	_ context.Context,
	request SessionSyncPreparationRequest,
) (SessionPreparedSyncStart, error) {
	start := cloneDetachedStartRequest(request.Start)
	if start.Mode != SessionOperationModeDurable {
		return SessionPreparedSyncStart{}, detachedRequestError("start.mode", "synchronous preparation requires durable mode")
	}
	if start.Correlation.RequestID == "" {
		return SessionPreparedSyncStart{}, detachedRequestError("start.correlation.requestId", "request id is required")
	}
	if start.Wait.TimeoutMillis < 0 || request.Wait.TimeoutMillis < 0 {
		return SessionPreparedSyncStart{}, detachedRequestError("wait.timeoutMillis", "timeout must not be negative")
	}
	if request.Wait.TimeoutMillis > 0 || request.Wait.CancelOnTimeout {
		start.Wait = request.Wait
	}
	start.Synchronous = true
	return SessionPreparedSyncStart{Request: start, Wait: start.Wait}, nil
}

func (operations *DetachedOperations) Subscribe(
	ctx context.Context,
	request SessionResponseSubscriptionRequest,
) (SessionResponseSubscriptionResult, error) {
	if err := validateSessionID(request.SessionID); err != nil {
		return SessionResponseSubscriptionResult{}, err
	}
	if request.AfterSequence < 0 {
		return SessionResponseSubscriptionResult{}, detachedRequestError("afterSequence", "sequence must not be negative")
	}
	if operations == nil || operations.owner == nil {
		return SessionResponseSubscriptionResult{}, ErrDetachedServiceUnavailable
	}
	cursor, err := operations.owner.SubscribeFactoryResponseEvents(ctx, ResponseEventSubscriptionRequest{
		SessionID:     request.SessionID,
		AfterSequence: request.AfterSequence,
		DispatchID:    request.DispatchID,
		Kinds:         append([]ResponseEventKind(nil), request.Kinds...),
	})
	if err != nil {
		return SessionResponseSubscriptionResult{}, err
	}
	return SessionResponseSubscriptionResult{Cursor: cursor}, nil
}

func (operations *DetachedOperations) controlLive(
	ctx context.Context,
	request SessionControlRequest,
	control ControlRequest,
) (SessionControlResult, error) {
	if request.Operation == SessionControlClose || request.Operation == SessionControlCancel || request.Operation == SessionControlTerminate {
		if err := operations.owner.CloseFactorySession(ctx, request.SessionID); err != nil {
			return SessionControlResult{}, err
		}
		return SessionControlResult{
			SessionID: request.SessionID,
			Mode:      SessionOperationModeLive,
			Operation: request.Operation,
			Closed:    true,
		}, nil
	}
	var (
		result LifecycleControlResult
		err    error
	)
	switch request.Operation {
	case SessionControlPause:
		result, err = operations.owner.PauseLiveFactorySession(ctx, request.SessionID, control)
	case SessionControlResume:
		result, err = operations.owner.ResumeLiveFactorySession(ctx, request.SessionID, control)
	default:
		return SessionControlResult{}, detachedRequestError("operation", "unsupported live control operation")
	}
	if err != nil {
		return SessionControlResult{}, err
	}
	return detachedControlResult(request, result), nil
}

func (operations *DetachedOperations) controlDurable(
	ctx context.Context,
	request SessionControlRequest,
	control ControlRequest,
) (SessionControlResult, error) {
	switch request.Operation {
	case SessionControlPause:
		return operations.forwardDurableControl(ctx, request, control, operations.owner.Pause)
	case SessionControlResume:
		return operations.forwardDurableControl(ctx, request, control, operations.owner.Resume)
	case SessionControlCancel:
		return operations.forwardDurableControl(ctx, request, control, operations.owner.Cancel)
	case SessionControlTerminate:
		return operations.forwardDurableControl(ctx, request, control, operations.owner.Terminate)
	case SessionControlRecover:
		return operations.controlRecover(ctx, request, control)
	case SessionControlApprove:
		return operations.controlApprove(ctx, request, control)
	case SessionControlRetryDispatch:
		return operations.controlRetryDispatch(ctx, request, control)
	case SessionControlInterruptDispatch:
		return operations.controlInterruptDispatch(ctx, request, control)
	default:
		return SessionControlResult{}, detachedRequestError("operation", "unsupported durable control operation")
	}
}

func (operations *DetachedOperations) forwardDurableControl(
	ctx context.Context,
	request SessionControlRequest,
	control ControlRequest,
	operation func(context.Context, string, ControlRequest) (LifecycleControlResult, error),
) (SessionControlResult, error) {
	result, err := operation(ctx, request.SessionID, control)
	if err != nil {
		return SessionControlResult{}, err
	}
	return detachedControlResult(request, result), nil
}

func (operations *DetachedOperations) controlRecover(
	ctx context.Context,
	request SessionControlRequest,
	control ControlRequest,
) (SessionControlResult, error) {
	recovery := ResumeSessionRequest{RequestID: control.RequestID}
	if request.Recover != nil {
		recovery = *request.Recover
		if recovery.RequestID == "" {
			recovery.RequestID = control.RequestID
		}
	}
	started, err := operations.owner.ResumeInterruptedSession(ctx, request.SessionID, recovery)
	if err != nil {
		return SessionControlResult{}, err
	}
	return SessionControlResult{
		SessionID: request.SessionID,
		Mode:      SessionOperationModeDurable,
		Operation: request.Operation,
		Status:    LifecycleStatus(started.Status),
		Recovery:  cloneAsyncStartResult(&started),
	}, nil
}

func (operations *DetachedOperations) controlApprove(
	ctx context.Context,
	request SessionControlRequest,
	control ControlRequest,
) (SessionControlResult, error) {
	approve := ApproveRequest{ControlRequest: control}
	if request.Approve != nil {
		approve = cloneApproveRequest(*request.Approve)
		if approve.RequestID == "" {
			approve.RequestID = control.RequestID
		}
	}
	result, err := operations.owner.Approve(ctx, request.SessionID, approve)
	if err != nil {
		return SessionControlResult{}, err
	}
	return detachedControlResult(request, result), nil
}

func (operations *DetachedOperations) controlRetryDispatch(
	ctx context.Context,
	request SessionControlRequest,
	control ControlRequest,
) (SessionControlResult, error) {
	retry := RetryDispatchRequest{ControlRequest: control}
	if request.Retry != nil {
		retry = cloneRetryRequest(*request.Retry)
		if retry.RequestID == "" {
			retry.RequestID = control.RequestID
		}
	}
	result, err := operations.owner.RetryDispatch(ctx, request.SessionID, retry)
	if err != nil {
		return SessionControlResult{}, err
	}
	return detachedControlResult(request, result), nil
}

func (operations *DetachedOperations) controlInterruptDispatch(
	ctx context.Context,
	request SessionControlRequest,
	control ControlRequest,
) (SessionControlResult, error) {
	interrupt := InterruptDispatchRequest{ControlRequest: control}
	if request.Interrupt != nil {
		interrupt = cloneInterruptRequest(*request.Interrupt)
		if interrupt.RequestID == "" {
			interrupt.RequestID = control.RequestID
		}
	}
	result, err := operations.owner.InterruptDispatch(ctx, request.SessionID, interrupt)
	if err != nil {
		return SessionControlResult{}, err
	}
	return detachedControlResult(request, result), nil
}
func detachedControlResult(request SessionControlRequest, result LifecycleControlResult) SessionControlResult {
	return SessionControlResult{
		SessionID:         request.SessionID,
		Mode:              request.Mode,
		Operation:         request.Operation,
		Outcome:           result.Outcome,
		Status:            result.Status,
		Detail:            result.Detail,
		ApprovalPreviewID: result.ApprovalPreviewID,
		DispatchID:        result.DispatchID,
		RetryDispatchID:   result.RetryDispatchID,
		Links:             result.Links,
	}
}

func legacyStartRequest(request SessionStartRequest) StartRequest {
	source := cloneSource(request.Source)
	if source.Kind == "" && strings.TrimSpace(request.Definition.FactoryID) != "" {
		source.Kind = factoryruntime.WorkflowSourceKindFactoryID
		source.FactoryID = strings.TrimSpace(request.Definition.FactoryID)
	}
	legacy := StartRequest{
		RequestID:       strings.TrimSpace(request.Correlation.RequestID),
		Source:          source,
		Args:            cloneAnyMap(request.Args),
		RequestedPolicy: cloneAnyMap(request.Policy),
		Orchestrator:    cloneOrchestratorOverride(request.Orchestrator),
		Runtime:         cloneRuntimeOptions(request.RuntimeOptions),
	}
	if len(legacy.Args) == 0 && request.Input != nil && request.Input.NormalizedArguments != nil {
		legacy.Args = normalizedArgumentsToValues(request.Input.NormalizedArguments)
	}
	if request.Wait.TimeoutMillis > 0 || request.Wait.CancelOnTimeout {
		timeout := request.Wait.TimeoutMillis
		legacy.Wait = &WaitOptions{TimeoutMillis: &timeout, CancelOnTimeout: request.Wait.CancelOnTimeout}
	}
	return legacy
}

func validateDetachedStartRequest(request SessionStartRequest) error {
	if request.Wait.TimeoutMillis < 0 {
		return detachedRequestError("wait.timeoutMillis", "timeout must not be negative")
	}
	switch request.Mode {
	case SessionOperationModeLive:
		return nil
	case SessionOperationModeDurable:
		if strings.TrimSpace(request.Correlation.RequestID) == "" {
			return detachedRequestError("correlation.requestId", "request id is required")
		}
		return nil
	default:
		return detachedRequestError("mode", "mode must be live or durable")
	}
}

func validateSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return detachedRequestError("sessionId", "session id is required")
	}
	return nil
}

func detachedRequestError(field, message string) error {
	return &DetachedRequestError{Field: field, Message: message}
}

func startedStatus(status, fallback string) string {
	if strings.TrimSpace(status) != "" {
		return status
	}
	return fallback
}

func detachedLiveReadView(projection ReadProjection) SessionView {
	return detachedLiveView(projection.Context, projection.Runtime, projection.RuntimeAvailable)
}

func detachedLiveSessionView(projection SessionProjection) SessionView {
	runtimeAvailable := projection.Context.Session != nil && projection.Context.Session.Runtime != nil
	return detachedLiveView(projection.Context, projection.Runtime, runtimeAvailable)
}

func detachedOpenResult(opened OpenResult) SessionOpenResult {
	result := SessionOpenResult{
		SessionID:             opened.SessionID,
		Targets:               append([]Target(nil), opened.Targets...),
		InitializedNewFactory: opened.InitsNewFactory,
		FolderPath:            opened.FolderPath,
	}
	if opened.Session == nil {
		return result
	}
	status := "OPENED"
	runtimeAvailable := opened.Session.Runtime != nil
	if opened.Session.Runtime != nil && opened.Session.Runtime.Status != "" {
		status = opened.Session.Runtime.Status
	}
	view := detachedLiveView(
		ProjectionContext{Session: opened.Session, FactorySessionID: opened.Session.ID},
		RuntimeProjection{},
		runtimeAvailable,
	)
	view.Status = status
	result.Session = &view
	return result
}

func detachedLiveView(
	context ProjectionContext,
	runtime RuntimeProjection,
	runtimeAvailable bool,
) SessionView {
	view := SessionView{
		Mode:             SessionOperationModeLive,
		RuntimeAvailable: runtimeAvailable,
		Status:           runtime.Status,
	}
	if context.Session != nil {
		session := context.Session
		view.SessionID = session.ID
		view.FactoryDir = session.FactoryDir
		view.FolderPath = session.FolderPath
		view.Project = session.Project
		view.IsDefault = session.IsDefault
		view.Target = session.Target
	}
	if view.SessionID == "" {
		view.SessionID = context.FactorySessionID
	}
	return view
}

func detachedDurableView(projection SessionReadResult) SessionView {
	view := SessionView{
		SessionID:        projection.SessionID,
		Mode:             SessionOperationModeDurable,
		Status:           string(projection.Status),
		OrchestratorKind: projection.OrchestratorKind,
		SourceRef:        projection.ResolvedSource.SourceRef,
		SourceHash:       projection.SourceHash,
	}
	if projection.ResultSummary != nil {
		view.ResultStatus = projection.ResultSummary.ResultStatus
	}
	return view
}

func durableSessionViews(summaries []DurableSessionListSummary) []SessionView {
	views := make([]SessionView, 0, len(summaries))
	for _, summary := range summaries {
		views = append(views, SessionView{
			SessionID:        summary.SessionID,
			Mode:             SessionOperationModeDurable,
			Status:           string(summary.Status),
			OrchestratorKind: summary.OrchestratorKind,
			SourceRef:        summary.ResolvedSource.SourceRef,
			SourceHash:       summary.SourceHash,
		})
	}
	return views
}

func cloneSessionListFilters(filters SessionListFilters) SessionListFilters {
	filters.Statuses = append([]LifecycleStatus(nil), filters.Statuses...)
	filters.OrchestratorKinds = append([]string(nil), filters.OrchestratorKinds...)
	return filters
}

func clonePreparedInput(input *work.PreparedInvocationInput) *work.PreparedInvocationInput {
	if input == nil {
		return nil
	}
	return input.Clone()
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneAnyValue(value)
	}
	return cloned
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneAnyValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func normalizedArgumentsToValues(arguments *work.NormalizedArguments) map[string]any {
	if arguments == nil || len(arguments.Arguments) == 0 {
		return nil
	}
	values := make(map[string]any, len(arguments.Arguments))
	for name, argument := range arguments.Arguments {
		if len(argument.Values) == 1 {
			values[name] = argument.Values[0]
			continue
		}
		values[name] = append([]string(nil), argument.Values...)
	}
	return values
}

func cloneArtifactRef(ref *factorydefinitions.FactoryArtifactRef) *factorydefinitions.FactoryArtifactRef {
	if ref == nil {
		return nil
	}
	cloned := *ref
	if ref.ContentHash != nil {
		contentHash := *ref.ContentHash
		cloned.ContentHash = &contentHash
	}
	if ref.SizeBytes != nil {
		sizeBytes := *ref.SizeBytes
		cloned.SizeBytes = &sizeBytes
	}
	return &cloned
}

func cloneCheckpointRefs(refs []factorydefinitions.FactorySessionJavaScriptCheckpointEventRef) []factorydefinitions.FactorySessionJavaScriptCheckpointEventRef {
	if len(refs) == 0 {
		return nil
	}
	cloned := make([]factorydefinitions.FactorySessionJavaScriptCheckpointEventRef, len(refs))
	for index, ref := range refs {
		cloned[index] = ref
		cloned[index].ArtifactRef = cloneArtifactRef(ref.ArtifactRef)
		if ref.Label != nil {
			label := *ref.Label
			cloned[index].Label = &label
		}
		if ref.Summary != nil {
			summary := *ref.Summary
			cloned[index].Summary = &summary
		}
		if ref.Timestamp != nil {
			timestamp := *ref.Timestamp
			cloned[index].Timestamp = &timestamp
		}
	}
	return cloned
}

func cloneAsyncStartResult(result *AsyncStartResult) *AsyncStartResult {
	if result == nil {
		return nil
	}
	cloned := *result
	cloned.Policy.Requested = cloneAnyMap(result.Policy.Requested)
	cloned.Policy.Effective = cloneAnyMap(result.Policy.Effective)
	cloned.ResolvedSource = cloneResolvedSource(result.ResolvedSource)
	return &cloned
}

func cloneSyncStartResult(result *SyncStartResult) *SyncStartResult {
	if result == nil {
		return nil
	}
	cloned := *result
	if async := cloneAsyncStartResult(&result.AsyncStartResult); async != nil {
		cloned.AsyncStartResult = *async
	}
	cloned.Result = append([]byte(nil), result.Result...)
	return &cloned
}

func cloneResolvedSource(source ResolvedSource) ResolvedSource {
	cloned := source
	cloned.ResolutionOrder = append([]string(nil), source.ResolutionOrder...)
	cloned.Metadata = cloneStringMap(source.Metadata)
	cloned.Agents = make(map[string]factorydefinitions.FactoryOrchestratorJavaScriptAgent, len(source.Agents))
	for name, agent := range source.Agents {
		cloned.Agents[name] = agent
	}
	cloned.ArgsSchema = append([]byte(nil), source.ArgsSchema...)
	cloned.DefaultPolicy = append([]byte(nil), source.DefaultPolicy...)
	return cloned
}

func cloneDurableResult(result ResultReadResult) *SessionDurableResult {
	cloned := &SessionDurableResult{
		SessionID:        result.SessionID,
		Status:           result.ResultStatus,
		SessionStatus:    result.SessionStatus,
		Mode:             result.Mode,
		IncludeArtifacts: result.IncludeArtifacts,
		PrimaryResult:    append([]byte(nil), result.PrimaryResult...),
		ArtifactIDs:      append([]string(nil), result.ArtifactIDs...),
		ArtifactRefs:     append([]ArtifactRefSummary(nil), result.ArtifactRefs...),
	}
	if result.Failure != nil {
		failure := *result.Failure
		cloned.Failure = &failure
	}
	if result.Availability != nil {
		availability := *result.Availability
		cloned.Availability = &availability
	}
	return cloned
}

func cloneApproveRequest(request ApproveRequest) ApproveRequest {
	cloned := request
	cloned.ApprovedPolicy = cloneAnyMap(request.ApprovedPolicy)
	return cloned
}

func cloneRetryRequest(request RetryDispatchRequest) RetryDispatchRequest {
	return request
}

func cloneInterruptRequest(request InterruptDispatchRequest) InterruptDispatchRequest {
	return request
}
