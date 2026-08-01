// Package automations is the public Automations service boundary.
package automations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	automationcontracts "github.com/portpowered/infinite-you/pkg/services/automations/internal/contracts"
	cronwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/wire"
	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// Service is the singular Automations root authority. It exposes only the
// detached reconciliation, source lifecycle, and cursor/status contract.
// Runtime-sidecar and construction capabilities remain optional implementation
// seams owned by their consuming runtime or Wire package.
type Service interface {
	Reconcile(context.Context, ReconcileRequest) (ReconcileResult, error)
	StartSource(context.Context, StartSourceRequest) (StartSourceResult, error)
	StopSource(context.Context, StopSourceRequest) (StopSourceResult, error)
	WaitSource(context.Context, WaitSourceRequest) (WaitSourceResult, error)
	SourceStatus(context.Context, SourceStatusRequest) (SourceStatusResult, error)
	GetStatus(context.Context, GetStatusRequest) (GetStatusResult, error)
	GetCursor(context.Context, GetCursorRequest) (GetCursorResult, error)
}

const (
	InvocationScheduleEveryTag                  = "agent_factory.schedule.every"
	InvocationScheduleWorkstationTag            = "agent_factory.schedule.workstation"
	InvocationScheduleTriggerAtStartTag         = "agent_factory.schedule.trigger_at_start"
	InvocationScheduleMaxConsecutiveFailuresTag = "agent_factory.schedule.max_consecutive_failures"
)

// InvocationScheduleRequest carries the authored CRON graph, the pending
// invocation Work, and runtime-owned effects needed by scheduled executions.
type InvocationScheduleRequest struct {
	FactoryDir    string
	FactoryConfig *factorydefinitions.FactoryConfig
	RuntimeConfig factorydefinitions.RuntimeConfigLookup
	WorkRequest   work.WorkRequest
	// ResumeSequence continues deterministic scheduled Work identity after a
	// runtime has reconstructed an accepted controller from canonical state.
	ResumeSequence int64
	// SuppressTriggerAtStart prevents restart recovery from treating an
	// already-admitted controller as a new invocation.
	SuppressTriggerAtStart bool
	Submitter              WorkRequestSubmitter
	Observe                InvocationScheduleObserver
	FailController         func(context.Context, string) error
}

// InvocationScheduleObservation reports whether the controller and its most
// recent scheduled execution are active, plus consecutive terminal failures.
type InvocationScheduleObservation struct {
	ControllerActive    bool
	ExecutionActive     bool
	ConsecutiveFailures int
}

// InvocationScheduleObserver reads orchestration-neutral schedule facts from
// the owning runtime without giving Automations mutable runtime state.
type InvocationScheduleObserver func(context.Context, InvocationScheduleObservationRequest) (InvocationScheduleObservation, error)

// InvocationScheduleObservationRequest identifies one invocation schedule.
type InvocationScheduleObservationRequest struct {
	ControllerWorkID  string
	ControllerTraceID string
	ExecutionWorkType string
}

// PreparedInvocationSchedules owns validated inert schedules. Commit starts
// them after Work admission; Abort releases them when admission fails.
type PreparedInvocationSchedules struct {
	// CommitFunc starts all prepared schedules using accepted controller
	// identity. It is populated only by Automations-owned schedule assembly.
	CommitFunc func(work.WorkRequestSubmitResult)
	// AbortFunc releases all prepared schedules without starting them. It is
	// populated only by Automations-owned schedule assembly.
	AbortFunc func()
}

// Commit starts all prepared schedules using accepted controller identity.
func (prepared PreparedInvocationSchedules) Commit(result work.WorkRequestSubmitResult) {
	if prepared.CommitFunc != nil {
		prepared.CommitFunc(result)
	}
}

// Abort releases all prepared schedules without starting them.
func (prepared PreparedInvocationSchedules) Abort() {
	if prepared.AbortFunc != nil {
		prepared.AbortFunc()
	}
}

// Root is the transport-injectable Automations boundary for reconciliation,
// source lifecycle, and cursor/status operations. It is a thin value adapter
// around the singular Service contract.
//
// Constructing a Root performs no work. Callers set Operations once when
// composing a peer and invoke explicit methods on Root.
type Root struct {
	Operations Service
}

func rootOperationsAvailable(operations Service) bool {
	if operations == nil {
		return false
	}

	value := reflect.ValueOf(operations)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func (r Root) Reconcile(
	ctx context.Context,
	request ReconcileRequest,
) (ReconcileResult, error) {
	if !rootOperationsAvailable(r.Operations) {
		return ReconcileResult{}, unavailableRootError("Reconcile")
	}
	return r.Operations.Reconcile(ctx, request)
}

func (r Root) StartSource(
	ctx context.Context,
	request StartSourceRequest,
) (StartSourceResult, error) {
	if !rootOperationsAvailable(r.Operations) {
		return StartSourceResult{}, unavailableRootError("StartSource")
	}
	return r.Operations.StartSource(ctx, request)
}

func (r Root) StopSource(
	ctx context.Context,
	request StopSourceRequest,
) (StopSourceResult, error) {
	if !rootOperationsAvailable(r.Operations) {
		return StopSourceResult{}, unavailableRootError("StopSource")
	}
	return r.Operations.StopSource(ctx, request)
}

func (r Root) WaitSource(
	ctx context.Context,
	request WaitSourceRequest,
) (WaitSourceResult, error) {
	if !rootOperationsAvailable(r.Operations) {
		return WaitSourceResult{}, unavailableRootError("WaitSource")
	}
	return r.Operations.WaitSource(ctx, request)
}

func (r Root) SourceStatus(
	ctx context.Context,
	request SourceStatusRequest,
) (SourceStatusResult, error) {
	if !rootOperationsAvailable(r.Operations) {
		return SourceStatusResult{}, unavailableRootError("SourceStatus")
	}
	return r.Operations.SourceStatus(ctx, request)
}

func (r Root) GetStatus(
	ctx context.Context,
	request GetStatusRequest,
) (GetStatusResult, error) {
	if !rootOperationsAvailable(r.Operations) {
		return GetStatusResult{}, unavailableRootError("GetStatus")
	}
	return r.Operations.GetStatus(ctx, request)
}

func (r Root) GetCursor(
	ctx context.Context,
	request GetCursorRequest,
) (GetCursorResult, error) {
	if !rootOperationsAvailable(r.Operations) {
		return GetCursorResult{}, unavailableRootError("GetCursor")
	}
	return r.Operations.GetCursor(ctx, request)
}

// ReconcileRequest carries desired automation specs and observed instance facts
// for one convergence pass. It stays free of WaitGroup, Factory Definition, and
// cron/poller/watcher implementation types.
type ReconcileRequest struct {
	Desired  []DesiredSpec
	Observed []ObservedInstance
}

// DesiredSpec is the plain desired automation configuration peers supply.
type DesiredSpec struct {
	AutomationID string
	SourceID     string
	Kind         string
	State        DesiredLifecycleState
}

// ObservedInstance is a detached observation of one live automation instance.
type ObservedInstance struct {
	AutomationID string
	SourceID     string
	InstanceID   string
	State        ObservedLifecycleState
}

// ReconcileResult is the detached set of convergence outcomes peers consume.
type ReconcileResult struct {
	Outcomes              []ConvergenceOutcome
	GeneratedWorkRequests []GeneratedWorkRequestOutcome
}

// ConvergenceOutcome reports how one automation identity converged.
type ConvergenceOutcome struct {
	AutomationID string
	SourceID     string
	InstanceID   string
	Action       ConvergenceAction
	Desired      DesiredLifecycleState
	Observed     ObservedLifecycleState
	Convergence  ConvergenceStatus
}

// DesiredLifecycleState is the requested lifecycle state for a source.
// It is intentionally distinct from ObservedLifecycleState because desired
// state cannot represent transitional or failed observations.
type DesiredLifecycleState string

const (
	DesiredLifecycleRunning DesiredLifecycleState = "running"
	DesiredLifecycleStopped DesiredLifecycleState = "stopped"
)

// ObservedLifecycleState is a detached fact about a source instance.
type ObservedLifecycleState string

const (
	ObservedLifecyclePending   ObservedLifecycleState = "pending"
	ObservedLifecycleStarting  ObservedLifecycleState = "starting"
	ObservedLifecycleRunning   ObservedLifecycleState = "running"
	ObservedLifecycleStopping  ObservedLifecycleState = "stopping"
	ObservedLifecycleStopped   ObservedLifecycleState = "stopped"
	ObservedLifecycleFailed    ObservedLifecycleState = "failed"
	ObservedLifecycleCancelled ObservedLifecycleState = "cancelled"
)

// ConvergenceStatus classifies the relationship between desired and observed
// state without requiring callers to parse status or error text.
type ConvergenceStatus string

const (
	ConvergenceStatusConverged   ConvergenceStatus = "converged"
	ConvergenceStatusProgressing ConvergenceStatus = "progressing"
	ConvergenceStatusFailed      ConvergenceStatus = "failed"
	ConvergenceStatusCancelled   ConvergenceStatus = "cancelled"
)

// ConvergenceAction reports the logical reconciliation action.
type ConvergenceAction string

const (
	ConvergenceActionCreated   ConvergenceAction = "created"
	ConvergenceActionUpdated   ConvergenceAction = "updated"
	ConvergenceActionUnchanged ConvergenceAction = "unchanged"
	ConvergenceActionRemoved   ConvergenceAction = "removed"
)

// SourceIdentity is the stable Automation-owned identity shared by every source
// kind and lifecycle command.
type SourceIdentity struct {
	AutomationID string
	SourceID     string
}

// Cursor is an opaque durable source position. Peers may persist and return the
// value but must not interpret its contents.
type Cursor string

// SourceObservation is a detached source fact suitable for status inspection
// and restart recovery.
type SourceObservation struct {
	Identity   SourceIdentity
	InstanceID string
	State      ObservedLifecycleState
	Cursor     Cursor
}

// LifecycleOutcome reports a command's desired state and latest observation.
// Idempotent is true when the source already matched the requested state.
type LifecycleOutcome struct {
	Desired     DesiredLifecycleState
	Observation SourceObservation
	Convergence ConvergenceStatus
	Idempotent  bool
}

// StartSourceRequest asks Automations to reconcile one source to running.
// Resume, when present, supplies the last committed cursor and observed facts
// from a previous process without exposing private persistence.
type StartSourceRequest struct {
	Identity SourceIdentity
	Kind     string
	Resume   *SourceObservation
}

// StartSourceResult reports the running reconciliation observation.
type StartSourceResult struct {
	Outcome LifecycleOutcome
}

// StopSourceRequest asks Automations to reconcile one source to stopped.
type StopSourceRequest struct {
	Identity SourceIdentity
}

// StopSourceResult reports the stopped reconciliation observation.
type StopSourceResult struct {
	Outcome LifecycleOutcome
}

// WaitSourceRequest waits for a source to reach one desired lifecycle state.
type WaitSourceRequest struct {
	Identity SourceIdentity
	Desired  DesiredLifecycleState
}

// WaitSourceResult reports the latest lifecycle observation.
type WaitSourceResult struct {
	Outcome LifecycleOutcome
}

// SourceStatusRequest observes the current lifecycle status of one source.
type SourceStatusRequest struct {
	Identity SourceIdentity
}

// SourceStatusResult is the detached lifecycle status peers consume.
type SourceStatusResult struct {
	Observation SourceObservation
}

// GetStatusRequest queries the detached status of one automation instance by
// opaque instance identity. It stays free of Runtime sidecar, WaitGroup, and
// cron/poller/watcher implementation types.
type GetStatusRequest struct {
	InstanceID string
}

// GetStatusResult is the detached instance status peers consume for progress
// inspection without importing source implementation packages.
type GetStatusResult struct {
	AutomationID string
	InstanceID   string
	Status       ObservedLifecycleState
}

// GetCursorRequest reads cursor/checkpoint recovery facts for one instance.
// ExpectedCursor, when non-empty, is an optimistic concurrency token peers can
// supply to detect stale cursors.
type GetCursorRequest struct {
	InstanceID     string
	ExpectedCursor Cursor
}

// GetCursorResult is the detached cursor and checkpoint observation peers use
// for recovery without importing cron/poller/watcher types.
type GetCursorResult struct {
	AutomationID string
	InstanceID   string
	Cursor       Cursor
	Checkpoint   string
}

// GeneratedWorkRequestIdentity identifies one logical Work Request emitted by
// an Automation source without exposing the Work service's request type.
type GeneratedWorkRequestIdentity struct {
	AutomationID string
	SourceID     string
	RequestID    string
}

// GeneratedWorkRequest is detached Automation-owned request data. Payload is
// copied at the service boundary; PayloadReference can identify payload data
// held by a caller without publishing a storage contract.
type GeneratedWorkRequest struct {
	Identity         GeneratedWorkRequestIdentity
	Payload          []byte
	PayloadReference string
}

// WorkRequestAdmissionStatus classifies the downstream disposition of one
// generated Work Request.
type WorkRequestAdmissionStatus string

const (
	WorkRequestAdmissionAccepted  WorkRequestAdmissionStatus = "accepted"
	WorkRequestAdmissionRejected  WorkRequestAdmissionStatus = "rejected"
	WorkRequestAdmissionDuplicate WorkRequestAdmissionStatus = "duplicate"
)

// WorkRequestRejectionReason is a typed reason callers can branch on without
// parsing a downstream error message.
type WorkRequestRejectionReason string

const (
	WorkRequestRejectedInvalidPayload WorkRequestRejectionReason = "invalid_payload"
	WorkRequestRejectedPolicy         WorkRequestRejectionReason = "policy"
	WorkRequestRejectedUnavailable    WorkRequestRejectionReason = "unavailable"
)

// GeneratedWorkRequestOutcome is the detached admission fact for one generated
// request. OriginalRequestID is populated for duplicate admission so callers
// can identify the first logical emission.
type GeneratedWorkRequestOutcome struct {
	Request           GeneratedWorkRequest
	Status            WorkRequestAdmissionStatus
	RejectionReason   WorkRequestRejectionReason
	OriginalRequestID string
}

// ErrorCode classifies typed Automations root failures peers can branch on.
type ErrorCode string

const (
	ErrorCodeNotReady  ErrorCode = "not_ready"
	ErrorCodeInvalid   ErrorCode = "invalid"
	ErrorCodeNotFound  ErrorCode = "not_found"
	ErrorCodeConflict  ErrorCode = "conflict"
	ErrorCodeCancelled ErrorCode = "cancelled"
	ErrorCodeFailed    ErrorCode = "failed"
)

var (
	// ErrNotReady reports that the Automations root is not ready for published
	// contract operations.
	ErrNotReady = errors.New("automations: service not ready")
	// ErrInvalidRequest reports that a published Automations request was rejected.
	ErrInvalidRequest = errors.New("automations: invalid request")
	// ErrNotFound reports that a referenced Automations entity was not found.
	ErrNotFound = errors.New("automations: not found")
	// ErrConflict reports that an Automations operation conflicted with observed state.
	ErrConflict = errors.New("automations: conflict")
	// ErrSupervisionFailed reports that an Automation source effect failed.
	ErrSupervisionFailed = errors.New("automations: source supervision failed")
)

// Error is the typed Automations root failure peers distinguish without parsing
// free-form implementation details.
type Error struct {
	Op   string
	Code ErrorCode
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return "automations: error"
	}
	if e.Err == nil {
		return fmt.Sprintf("automations: %s: %s", e.Op, e.Code)
	}
	return fmt.Sprintf("automations: %s: %s: %v", e.Op, e.Code, e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	return errors.Is(e.Err, target)
}

func unavailableRootError(op string) *Error {
	return &Error{
		Op:   op,
		Code: ErrorCodeNotReady,
		Err:  ErrNotReady,
	}
}

var (
	cronService = cronwire.NewService()

	ValidateCronSchedule  = cronService.ValidateCronSchedule
	ParseCronJitter       = cronService.ParseCronJitter
	ParseCronExpiryWindow = cronService.ParseCronExpiryWindow
)

// WorkRequestSubmitter admits one Work Request generated by an automation.
// The root owns this narrow effect shape; nested source services keep their
// cursor and lifecycle collaborators private.
type WorkRequestSubmitter func(context.Context, work.WorkRequest) error

// FilesystemInputReader reads the watched input tree selected by Automations.
// It is a root-owned effect port, not a nested watcher service contract.
type FilesystemInputReader = automationcontracts.FilesystemInputReader

// FilesystemDirectoryWalker traverses a watched input tree.
type FilesystemDirectoryWalker func(string, fs.WalkDirFunc) error

// FilesystemWatcher supervises one configured input root. Construction is
// inert; PreseedInputs and Watch perform effects only when explicitly invoked.
type FilesystemWatcher = automationcontracts.FilesystemWatcher

// FilesystemWatcherConfig carries inert construction inputs for one watcher.
type FilesystemWatcherConfig struct {
	Dir               string
	Logger            *zap.Logger
	KnownWorkTypes    []string
	ValidStatesByType map[string]map[string]bool
	Files             FilesystemInputReader
	WalkDirectory     FilesystemDirectoryWalker
	WorkRequestIDs    work.RequestIDGenerator
	Submitter         WorkRequestSubmitter
	DebounceWindow    time.Duration
}

// HostedWorkSubmitter admits one normalized Work Request produced by a hosted
// poller.
type HostedWorkSubmitter = hostedsources.WorkSubmitter

// HostedPollers is the provider-specific polling capability supervised by the
// Automations service. The implementation contract remains owned by the
// private hosted-sources service.
type HostedPollers = hostedsources.HostedPollers

// Clock is the automation time source needed for scheduling and supervision.
type Clock = platformclock.Source
