// Package runtimebinding adapts opaque Factory Runtime handles to the
// canonical Factory Session state service. It is the intentional cross-domain
// seam; neither domain core imports the other.
package runtimebinding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

type LiveSessionResolver interface {
	Resolve(string) *livesession.LiveSession
}

type Registration struct {
	SessionID      string
	FactoryRootDir string
	Handle         RuntimeHandle
	Binding        factory.RuntimeBinding
	Target         factorysessions.Target
	Select         bool
}

// SyncActiveDirectory updates the process compatibility directory to match the
// selected runtime bundle. Domain services should use the bundle directly;
// this helper exists while legacy process configuration remains observable.
func SyncActiveDirectory(mu *sync.RWMutex, configured *string, factoryRoot string, bundle RuntimeInstance) {
	if mu == nil || configured == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if bundle == nil || strings.TrimSpace(bundle.Directory()) == "" {
		if root := strings.TrimSpace(factoryRoot); root != "" {
			*configured = root
		}
		return
	}
	*configured = bundle.Directory()
}

// StartInitial starts and registers the invocation's admitted Factory Session.
// The compatibility default is only one possible public selector; explicit
// local sessions must retain their own selector through startup and cleanup.
func StartInitial(
	readinessContext context.Context,
	runContext context.Context,
	state *sessionruntime.Service,
	runtimeState *State,
	sessionID string,
	factoryRootDir string,
	bundle RuntimeInstance,
	target factorysessions.Target,
	runtimeMode interfaces.RuntimeMode,
	lifecycle RuntimeLifecycle,
	stop func(RuntimeHandle) error,
	onSessionRemoved func(string),
) (RuntimeHandle, error) {
	if bundle == nil {
		return nil, fmt.Errorf("runtime bundle is required")
	}
	if state == nil || runtimeState == nil {
		return nil, fmt.Errorf("Factory Session runtime state is required")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("factory runtime lifecycle service is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	handle, err := lifecycle.Start(runContext, bundle)
	if err != nil {
		return nil, err
	}
	registeredSessionID := Register(state, Registration{
		SessionID: sessionID, FactoryRootDir: factoryRootDir,
		Handle: handle, Target: target, Select: true,
	})
	if strings.TrimSpace(registeredSessionID) == "" ||
		state.Resolve(sessionID) == nil {
		unregisterSession(state, sessionID, onSessionRemoved)
		if stop != nil {
			_ = stop(handle)
		}
		return nil, fmt.Errorf("register default Factory Session runtime")
	}
	runtimeState.ClearStartup()
	runtimeState.SetActive(runContext, registeredSessionID, handle)
	if err := lifecycle.WaitForStart(readinessContext, handle); err != nil {
		startupErr := HandleStartFailure(
			readinessContext, state, runtimeState,
			sessionID, handle, stop, err, runtimeMode, onSessionRemoved,
		)
		if startupErr == nil && readinessContext.Err() != nil {
			return nil, readinessContext.Err()
		}
		return nil, startupErr
	}
	return handle, nil
}

// Replace starts a replacement runtime, transfers the session registry entry
// and active selection, and then stops the previous runtime. The request
// context bounds readiness while the existing service context owns the new
// runtime after the request returns. The retirement callback runs only after
// the previous runtime has been stopped and its binding deactivated.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func Replace(
	readinessContext context.Context,
	state *sessionruntime.Service,
	runtimeState *State,
	session *livesession.LiveSession,
	replacement RuntimeInstance,
	serviceMode bool,
	lifecycle RuntimeLifecycle,
	startSidecars func(context.Context, RuntimeHandle) error,
	stop func(RuntimeHandle) error,
	report func(error),
	onPreviousRuntimeRetired func(string, *factorysessions.LiveRuntime, factory.RuntimeRecord),
) (*livesession.LiveSession, error) {
	if session == nil {
		return nil, fmt.Errorf("%w: session handle is unavailable", factorysessions.ErrSessionNotFound)
	}
	current := HandleFromSession(session)
	if current == nil {
		return nil, fmt.Errorf("%w: session handle is unavailable", factorysessions.ErrSessionNotFound)
	}
	previousRuntime := session.Runtime
	previousRecord := current.RuntimeInstance()
	preparedSpec := PreparedSpecFromSession(session)
	if state == nil || runtimeState == nil {
		return nil, fmt.Errorf("Factory Session runtime state is required")
	}
	runState := runtimeState.Active()
	serviceCtx := runtimeContext(readinessContext, runState)
	isActive := runState != nil && runState.SessionID == session.ID
	if lifecycle == nil {
		return nil, fmt.Errorf("factory runtime lifecycle service is required")
	}
	restoreSidecars := serviceMode
	if serviceMode {
		lifecycle.StopSidecars(current)
	}
	defer func() {
		if restoreSidecars && startSidecars != nil {
			_ = startSidecars(serviceCtx, current)
		}
	}()
	replacementHandle, err := lifecycle.Start(serviceCtx, replacement)
	if err != nil {
		return nil, err
	}
	if err := lifecycle.WaitForStart(readinessContext, replacementHandle); err != nil {
		_ = lifecycle.Stop(replacementHandle)
		return nil, fmt.Errorf("start replacement runtime: %w", err)
	}
	if serviceMode && startSidecars != nil {
		if err := startSidecars(serviceCtx, replacementHandle); err != nil {
			_ = lifecycle.Stop(replacementHandle)
			return nil, fmt.Errorf("start replacement runtime sidecars: %w", err)
		}
	}
	if err := lifecycle.PublishReplacement(readinessContext, current, replacement); err != nil && report != nil {
		report(err)
	}
	restoreSidecars = false

	updated := registerReplacementSession(
		state, runtimeState, session, replacement, replacementHandle,
		preparedSpec, serviceCtx, isActive,
	)
	if stop != nil {
		if err := stop(current); err != nil && !errors.Is(err, context.Canceled) && report != nil {
			report(fmt.Errorf("stop prior session runtime: %w", err))
		}
	}
	if err := deactivateRuntimeBinding(BindingForSession(session)); err != nil && report != nil {
		report(fmt.Errorf("deactivate prior Factory Runtime binding: %w", err))
	}
	if onPreviousRuntimeRetired != nil {
		onPreviousRuntimeRetired(session.ID, previousRuntime, previousRecord)
	}
	return updated, nil
}

func registerReplacementSession(
	state *sessionruntime.Service,
	runtimeState *State,
	session *livesession.LiveSession,
	replacement RuntimeInstance,
	replacementHandle RuntimeHandle,
	preparedSpec any,
	serviceCtx context.Context,
	isActive bool,
) *livesession.LiveSession {
	placement := session.Placement()
	executionBaseDir := strings.TrimSpace(session.ExecutionBaseDir)
	if replacement != nil && replacement.LoadedRuntimeConfig() != nil {
		if runtimeBaseDir := strings.TrimSpace(replacement.LoadedRuntimeConfig().RuntimeBaseDir()); runtimeBaseDir != "" {
			executionBaseDir = runtimeBaseDir
		}
	}
	state.RotateResponseStreams(session)
	var projectionOwner SessionProjectionOwner
	var activation interface{ Close(context.Context) error }
	var process roles.ProcessRuntime
	var diagnostics factory.RuntimeLogDiagnostics
	previous := SessionStateFrom(session)
	if previous != nil {
		projectionOwner = previous.Owner
		activation = previous.Activation
		process = previous.Process
		diagnostics = previous.Diagnostics
	}
	handle := &SessionState{
		Handle: replacementHandle, Instance: replacement,
		Spec: preparedSpec, Owner: projectionOwner, Activation: activation,
		Process: process, Diagnostics: diagnostics,
	}
	handle.inheritApplicationValues(previous)
	state.Register(sessionruntime.Registration{
		SessionID: session.ID, FactoryDir: replacement.Directory(),
		FolderPath: placement.FolderPath, ExecutionBaseDir: executionBaseDir,
		RuntimeFactorySessionID: session.RuntimeFactorySessionID,
		RuntimeEventSessionID:   session.RuntimeEventSessionID,
		Target:                  placement.Target,
		Handle:                  handle,
		Runtime: &factorysessions.LiveRuntime{
			Factory: replacement.RuntimeService(), BackendScopeID: replacement.BackendScope(),
			WorkAndEventIngress:   DeclaredWorkAndEventIngress(replacement.RuntimeService()),
			Clock:                 state.Clock(),
			RuntimeConfig:         loadedFactorySnapshotSource(replacement.LoadedRuntimeConfig()),
			LiveChangeEvents:      NewLiveChangeEventLog(replacement.RecordingLedger()),
			LiveChangeApplication: NewLiveChangeApplication(replacement.RuntimeService()),
			LiveChangeAdmission:   NewLiveChangeAdmission(replacement.RuntimeService()),
			LiveChangeLogger:      replacement.RuntimeLogger(),
		},
		Default: session.IsDefault, Project: placement.Project,
		Select: isActive, AddEventTypeRecorder: replacement.AddEventTypeRecorder,
		AddEventTypeRecorderWithReady: replacement.AddEventTypeRecorderWithReady,
	})
	updated := state.Resolve(session.ID)
	if isActive {
		runtimeState.SetActive(serviceCtx, session.ID, replacementHandle)
	}
	return updated
}

// Start launches a Factory Runtime, waits for readiness, starts service-mode
// sidecars, and registers the resulting live Factory Session.
func Start(
	readinessContext context.Context,
	state *sessionruntime.Service,
	runtimeState *State,
	factoryRootDir string,
	sessionID string,
	bundle RuntimeInstance,
	target factorysessions.Target,
	serviceMode bool,
	lifecycle RuntimeLifecycle,
	startSidecars func(context.Context, RuntimeHandle) error,
	stop func(RuntimeHandle) error,
) (RuntimeHandle, error) {
	if bundle == nil {
		return nil, fmt.Errorf("runtime bundle is required")
	}
	readinessCtx := readinessContext
	if readinessCtx == nil {
		readinessCtx = context.Background()
	}
	serviceCtx := readinessCtx
	if runtimeState != nil {
		if active := runtimeState.Active(); active != nil && active.Context != nil {
			serviceCtx = active.Context
		}
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("factory runtime lifecycle service is required")
	}
	handle, err := lifecycle.Start(serviceCtx, bundle)
	if err != nil {
		return nil, err
	}
	if err := lifecycle.WaitForStart(readinessCtx, handle); err != nil {
		stopStartedHandle(stop, handle)
		return nil, fmt.Errorf("start runtime session: %w", err)
	}
	if serviceMode && startSidecars != nil {
		if err := startSidecars(serviceCtx, handle); err != nil {
			stopStartedHandle(stop, handle)
			return nil, fmt.Errorf("start runtime session sidecars: %w", err)
		}
	}
	Register(state, Registration{
		SessionID: sessionID, FactoryRootDir: factoryRootDir,
		Handle: handle, Target: target,
	})
	return handle, nil
}

func stopStartedHandle(stop func(RuntimeHandle) error, handle RuntimeHandle) {
	if stop != nil {
		_ = stop(handle)
		return
	}
	if handle != nil {
		handle.CancelRun()
	}
}

func deactivateRuntimeBinding(binding factory.RuntimeBinding) error {
	if binding.IsZero() {
		return nil
	}
	if _, err := binding.Deactivate(context.Background()); err != nil && !errors.Is(err, factory.ErrRuntimeNotActive) {
		return err
	}
	return nil
}

func Register(state *sessionruntime.Service, input Registration) string {
	if state == nil || strings.TrimSpace(input.SessionID) == "" || input.Handle == nil || input.Handle.RuntimeInstance() == nil {
		return ""
	}
	bundle := input.Handle.RuntimeInstance()
	runtimeService := bundle.RuntimeService()
	if boundService := input.Binding.Service(); boundService != nil {
		runtimeService = boundService
	}
	runtimeBaseDir := ""
	if runtimeConfig := bundle.LoadedRuntimeConfig(); runtimeConfig != nil {
		runtimeBaseDir = runtimeConfig.RuntimeBaseDir()
	}
	metadata := sessionruntime.NormalizeRegistration(sessionruntime.RegistrationInput{
		FactoryRootDir: input.FactoryRootDir, BundleDir: bundle.Directory(), BundleFolder: bundle.FolderDirectory(),
		RuntimeBaseDir: runtimeBaseDir, Target: input.Target,
		PreparedSpec: PreparedSpecFromSession(state.Resolve(input.SessionID)),
	})
	var projectionOwner SessionProjectionOwner
	var activation interface{ Close(context.Context) error }
	var process roles.ProcessRuntime
	var diagnostics factory.RuntimeLogDiagnostics
	previousSession := state.Resolve(input.SessionID)
	previous := SessionStateFrom(previousSession)
	if previous != nil {
		projectionOwner = previous.Owner
		activation = previous.Activation
		process = previous.Process
		diagnostics = previous.Diagnostics
	}
	handle := &SessionState{Instance: bundle, Handle: input.Handle, Spec: metadata.PreparedSpec, Owner: projectionOwner, Activation: activation, Process: process, Diagnostics: diagnostics}
	handle.inheritApplicationValues(previous)
	// Initial lifecycle registration replaces the provisional opening record.
	// Carry its startup observations only for that handoff; runtime replacement
	// keeps its existing metadata policy.
	if previous != nil && previous.Handle == nil {
		handle.SkippedBoardRecordings = append([]string(nil), previous.SkippedBoardRecordings...)
		if previous.StartupRecovery != nil {
			recovery := *previous.StartupRecovery
			handle.StartupRecovery = &recovery
		}
	}
	// Opening already selected the canonical identity and the retained source
	// event scope. Keep both when attaching the running handle, including named
	// successors: their Work admission history still belongs to the source.
	var runtimeSessionID, eventSessionID string
	if previousSession != nil {
		runtimeSessionID = previousSession.RuntimeFactorySessionID
		eventSessionID = previousSession.RuntimeEventSessionID
	}
	return state.Register(sessionruntime.Registration{
		SessionID: input.SessionID, FactoryDir: metadata.FactoryDir, FolderPath: metadata.FolderPath,
		RuntimeFactorySessionID: runtimeSessionID, RuntimeEventSessionID: eventSessionID,
		ExecutionBaseDir: metadata.ExecutionBaseDir, Target: metadata.Target,
		Handle: handle,
		Runtime: &factorysessions.LiveRuntime{
			Factory: runtimeService, Binding: input.Binding, BackendScopeID: bundle.BackendScope(),
			WorkAndEventIngress:   DeclaredWorkAndEventIngress(runtimeService),
			Clock:                 state.Clock(),
			RuntimeConfig:         loadedFactorySnapshotSource(bundle.LoadedRuntimeConfig()),
			LiveChangeEvents:      NewLiveChangeEventLog(bundle.RecordingLedger()),
			LiveChangeApplication: NewLiveChangeApplication(runtimeService),
			LiveChangeAdmission:   NewLiveChangeAdmission(runtimeService),
			LiveChangeLogger:      bundle.RuntimeLogger(),
		},
		Default: logicaltarget.IsLiveSessionDefaultSelector(input.SessionID), Project: metadata.Project,
		Select: input.Select, AllocateDefaultID: true, AddEventTypeRecorder: bundle.AddEventTypeRecorder,
		AddEventTypeRecorderWithReady: bundle.AddEventTypeRecorderWithReady,
	})
}

// ServiceForLiveRuntime returns the current opaque Runtime capability and
// falls back to the hosted-era field while older openings are still in flight.
func ServiceForLiveRuntime(runtime *factorysessions.LiveRuntime) factory.Service {
	if runtime == nil {
		return nil
	}
	if service := runtime.Binding.Service(); service != nil {
		return service
	}
	return runtime.Factory
}

// ServiceForSession returns the session's bound Runtime capability without
// requiring callers to know how the session was hosted.
func ServiceForSession(session *livesession.LiveSession) factory.Service {
	if session == nil {
		return nil
	}
	return ServiceForLiveRuntime(session.Runtime)
}

// WorkAndEventIngressForService isolates the migration-only Work-submission
// and event-subscription boundary from the singular Factory Runtime Service
// contract. It is the one place Factory Sessions resolves that capability, so
// consumers hold the named factory.APIFactory contract instead of declaring
// their own narrow interfaces and recovering it per call.
func WorkAndEventIngressForService(runtime factory.Service) (factory.APIFactory, bool) {
	ingress, ok := runtime.(factory.APIFactory)
	if !ok || ingress == nil {
		return nil, false
	}
	return ingress, true
}

// DeclaredWorkAndEventIngress resolves the ingress a producer publishes on
// LiveRuntime.WorkAndEventIngress. It returns nil when the bound runtime does
// not serve the migration-only capability, which peers report exactly as the
// prior per-call recovery failure did.
func DeclaredWorkAndEventIngress(runtime factory.Service) factory.APIFactory {
	ingress, ok := WorkAndEventIngressForService(runtime)
	if !ok {
		return nil
	}
	return ingress
}

// WorkAndEventIngressForLiveRuntime returns the ingress declared when Factory
// Sessions bound the runtime.
func WorkAndEventIngressForLiveRuntime(runtime *factorysessions.LiveRuntime) (factory.APIFactory, bool) {
	if runtime == nil || runtime.WorkAndEventIngress == nil {
		return nil, false
	}
	return runtime.WorkAndEventIngress, true
}

// BindingForSession returns the opaque binding published for a live session.
func BindingForSession(session *livesession.LiveSession) factory.RuntimeBinding {
	if session == nil || session.Runtime == nil {
		return factory.RuntimeBinding{}
	}
	return session.Runtime.Binding
}

func SessionStateFrom(session *livesession.LiveSession) *SessionState {
	if session == nil {
		return nil
	}
	state, _ := session.Handle.(*SessionState)
	return state
}

func HandleFromSession(session *livesession.LiveSession) RuntimeHandle {
	state := SessionStateFrom(session)
	if state == nil {
		return nil
	}
	return state.Handle
}

func BundleFromSession(session *livesession.LiveSession) RuntimeInstance {
	state := SessionStateFrom(session)
	if state == nil {
		return nil
	}
	if state.Handle != nil {
		return state.Handle.RuntimeInstance()
	}
	return state.Instance
}

// CurrentBundle resolves the invocation-selected active runtime, its pre-start
// bundle, then the process compatibility default. Startup must precede the
// process default so concurrent explicit sessions retain their own runtime.
func CurrentBundle(
	state *sessionruntime.Service,
	runtimeState *State,
) RuntimeInstance {
	if runtimeState == nil {
		return nil
	}
	return runtimeState.Current(func() RuntimeInstance {
		if state == nil {
			return nil
		}
		handle := HandleFromSession(state.Default())
		if handle == nil {
			return nil
		}
		return handle.RuntimeInstance()
	})
}

// CanonicalEventsFromSession returns the event ledger for a live runtime
// without exposing its opaque handle representation to Session consumers.
func CanonicalEventsFromSession(session *livesession.LiveSession) []interfaces.FactoryEvent {
	instance := BundleFromSession(session)
	if instance == nil {
		return nil
	}
	return instance.CanonicalEvents()
}

// BackendScopeID resolves the process-configured scope before the session
// runtime bundle fallback.
func BackendScopeID(configured string, session *livesession.LiveSession) string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	if session != nil && session.Runtime != nil {
		if scope := strings.TrimSpace(session.Runtime.BackendScopeID); scope != "" {
			return scope
		}
	}
	if bundle := BundleFromSession(session); bundle != nil {
		return strings.TrimSpace(bundle.BackendScope())
	}
	return ""
}

// StreamGenerationID resolves the canonical ledger generation, runtime
// snapshot generation, then runtime start timestamp.
func StreamGenerationID(session *livesession.LiveSession) string {
	if binding := BindingForSession(session); !binding.IsZero() {
		if runtime := binding.Service(); runtime != nil {
			observeResult, observeErr := runtime.Observe(context.Background(), factory.ObserveRequest{
				Scope: factory.ObservationScopeHealth,
			})
			if observeErr == nil {
				if generation := strings.TrimSpace(observeResult.Observation.Health.StreamGenerationID); generation != "" {
					return generation
				}
			}
		}
	}
	instance := BundleFromSession(session)
	if instance != nil {
		if generation := strings.TrimSpace(instance.StreamGeneration()); generation != "" {
			return generation
		}
		if runtime := instance.RuntimeService(); runtime != nil {
			observeResult, observeErr := runtime.Observe(context.Background(), factory.ObserveRequest{
				Scope: factory.ObservationScopeHealth,
			})
			if observeErr == nil {
				if generation := strings.TrimSpace(observeResult.Observation.Health.StreamGenerationID); generation != "" {
					return generation
				}
			}
		}
	}
	if instance != nil && !instance.StartTime().IsZero() {
		return instance.StartTime().UTC().Format(time.RFC3339Nano)
	}
	return ""
}

func ResponseStreamRuntimeFromSessionHandle(handle any) (factory.MetricsEmitter, *zap.Logger) {
	state, _ := handle.(*SessionState)
	if state == nil {
		return nil, nil
	}
	instance := state.Instance
	if state.Handle != nil {
		instance = state.Handle.RuntimeInstance()
	}
	if instance == nil {
		return nil, nil
	}
	return instance.RuntimeMetrics(), instance.RuntimeLogger()
}

func PreparedSpecFromSession(session *livesession.LiveSession) any {
	state := SessionStateFrom(session)
	if state == nil {
		return nil
	}
	return state.Spec
}

func RequireLiveSession(resolver LiveSessionResolver, sessionID string) (*livesession.LiveSession, error) {
	if resolver == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	session := resolver.Resolve(sessionID)
	handle := HandleFromSession(session)
	if session == nil || handle == nil || handle.RuntimeInstance() == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	return session, nil
}

// NextLiveSession returns another registered session backed by a live Factory
// Runtime. Selection policy belongs here so process hosts do not each maintain
// their own registry traversal.
func NextLiveSession(state *sessionruntime.Service, exceptSessionID string) *livesession.LiveSession {
	if state == nil || state.Registry() == nil {
		return nil
	}
	for _, sessionID := range state.Registry().IDs() {
		if sessionID == exceptSessionID {
			continue
		}
		session := state.Resolve(sessionID)
		if HandleFromSession(session) != nil {
			return session
		}
	}
	return nil
}

// DefaultSessionSuccessor resolves the selected non-default session used when
// a client reconnects through the compatibility default-session selector.
func DefaultSessionSuccessor(state *sessionruntime.Service, runtimeState *State) *livesession.LiveSession {
	if state == nil {
		return nil
	}
	if active := runtimeState.Active(); active != nil {
		sessionID := strings.TrimSpace(active.SessionID)
		if sessionID != "" && sessionID != factorysessions.DefaultSessionID {
			if session, err := RequireLiveSession(state, sessionID); err == nil {
				return session
			}
		}
	}
	current := state.Current()
	if current == nil || current.ID == factorysessions.DefaultSessionID {
		return nil
	}
	session, _ := RequireLiveSession(state, current.ID)
	return session
}

// StopSessionGeneration keeps all shutdown effects on the captured generation,
// even when a replacement is published before shutdown begins.
func StopSessionGeneration(
	state *sessionruntime.Service,
	runtimeState *State,
	session *livesession.LiveSession,
	stop func(RuntimeHandle) error,
) error {
	if err := CleanupSessionGeneration(session, stop); err != nil {
		return err
	}
	state.UnregisterGeneration(session)
	successor := state.Resolve(session.ID)
	if successor == nil {
		successor = NextLiveSession(state, session.ID)
	}
	runtimeState.RetireActive(session.ID, HandleFromSession(session), successor)
	return nil
}

// CleanupSessionGeneration stops and deactivates a captured runtime without
// retiring its registration. Failed cleanup retains the record for retry.
func CleanupSessionGeneration(session *livesession.LiveSession, stop func(RuntimeHandle) error) error {
	handle := HandleFromSession(session)
	if handle == nil {
		return fmt.Errorf("%w: session handle is unavailable", factorysessions.ErrSessionNotFound)
	}
	binding := BindingForSession(session)
	var cleanupErrs []error
	if stop != nil {
		if err := stop(handle); err != nil &&
			!errors.Is(err, context.Canceled) &&
			!errors.Is(err, factory.ErrAlreadyStopped) &&
			!errors.Is(err, factory.ErrNotRunning) {
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	if err := deactivateRuntimeBinding(binding); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("deactivate Factory Runtime binding: %w", err))
	}
	return errors.Join(cleanupErrs...)
}

// FailStartup stops the failed runtime and closes its activation before
// retiring the record. Incomplete cleanup retains the record for a later close.
// The original readiness error is preserved alongside any cleanup failure.
func FailStartup(
	state *sessionruntime.Service,
	runtimeState *State,
	sessionID string,
	handle RuntimeHandle,
	stop func(RuntimeHandle) error,
	startupErr error,
) error {
	// Capture cleanup ownership before stopping the run: stopping or closing
	// activation may publish a replacement under this same public session ID.
	var session *livesession.LiveSession
	if state != nil {
		session = state.Resolve(sessionID)
		if HandleFromSession(session) != handle {
			session = nil
		}
	}
	runtimeState.RetireActive(sessionID, handle, nil)
	if handle != nil && stop != nil {
		if stopErr := stop(handle); stopErr != nil && !errors.Is(stopErr, context.Canceled) {
			startupErr = errors.Join(startupErr, stopErr)
			if !handle.Completed() {
				return startupErr
			}
		}
	}
	if session != nil {
		bound := SessionStateFrom(session)
		if bound != nil && bound.Activation != nil {
			if err := bound.Activation.Close(context.Background()); err != nil {
				startupErr = errors.Join(startupErr, err)
				var terminal *recordings.RecordingCleanupError
				if !errors.As(err, &terminal) || !terminal.Complete {
					return startupErr
				}
			}
		}
		state.UnregisterGeneration(session)
	}
	return startupErr
}

// SessionClosedDuringStartup reports the expected service-mode race in which
// a client closes the admitted session while startup is still waiting.
func SessionClosedDuringStartup(
	state *sessionruntime.Service,
	sessionID string,
	mode interfaces.RuntimeMode,
) bool {
	if mode != interfaces.RuntimeModeService {
		return false
	}
	return state == nil || state.Resolve(sessionID) == nil
}

// HandleStartFailure clears a failed default runtime, preserving the expected
// close-during-startup behavior and joining any shutdown failure.
func HandleStartFailure(
	ctx context.Context,
	state *sessionruntime.Service,
	runtimeState *State,
	sessionID string,
	handle RuntimeHandle,
	stop func(RuntimeHandle) error,
	startErr error,
	mode interfaces.RuntimeMode,
	onSessionRemoved func(string),
) error {
	// Readiness may have published a replacement or selected a peer before
	// returning. Cleanup owns the failed run, never a fresh lookup by ID.
	var failed *livesession.LiveSession
	if state != nil {
		failed = state.Resolve(sessionID)
		if HandleFromSession(failed) != handle {
			failed = nil
		}
	}
	if active := runtimeState.Active(); active != nil && active.Handle == handle {
		runtimeState.RetireActive(active.SessionID, handle, nil)
	}
	retireFailed := func() {
		if failed != nil {
			state.UnregisterGeneration(failed)
		}
		if onSessionRemoved != nil && (state == nil || state.Resolve(sessionID) == nil) {
			onSessionRemoved(sessionID)
		}
	}
	if SessionClosedDuringStartup(state, sessionID, mode) {
		retireFailed()
		if stop != nil {
			_ = stop(handle)
		}
		return nil
	}
	retireFailed()
	var stopErr error
	if stop != nil {
		stopErr = stop(handle)
	}
	if ctx != nil && ctx.Err() != nil && errors.Is(startErr, context.Canceled) {
		if stopErr != nil &&
			!errors.Is(stopErr, context.Canceled) &&
			!errors.Is(stopErr, factory.ErrAlreadyStopped) {
			return stopErr
		}
		return nil
	}
	wrapped := fmt.Errorf("start runtime: %w", startErr)
	if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		return errors.Join(wrapped, stopErr)
	}
	return wrapped
}

func unregisterSession(
	state *sessionruntime.Service,
	sessionID string,
	onSessionRemoved func(string),
) {
	if state != nil {
		state.Unregister(sessionID)
	}
	if onSessionRemoved != nil {
		onSessionRemoved(sessionID)
	}
}

// ShutdownOtherLiveSessions stops and unregisters every session except the
// supplied runtime handle.
func ShutdownOtherLiveSessions(
	state *sessionruntime.Service,
	except RuntimeHandle,
	stop func(RuntimeHandle) error,
) error {
	if state == nil || state.Registry() == nil {
		return nil
	}
	var errs []error
	// Capture every generation before the first shutdown effect. A stop can
	// publish a replacement for itself or for a later session in the traversal.
	// Those newly admitted runs do not belong to this shutdown window.
	sessions := make([]*livesession.LiveSession, 0, state.Registry().Count())
	for _, sessionID := range state.Registry().IDs() {
		session := state.Resolve(sessionID)
		if session != nil {
			sessions = append(sessions, session)
		}
	}
	for _, session := range sessions {
		handle := HandleFromSession(session)
		if handle == except {
			continue
		}
		binding := BindingForSession(session)
		if handle != nil && stop != nil {
			if err := stop(handle); err != nil && !errors.Is(err, context.Canceled) {
				errs = append(errs, err)
			}
		}
		if err := deactivateRuntimeBinding(binding); err != nil {
			errs = append(errs, err)
		}
		state.UnregisterGeneration(session)
	}
	return errors.Join(errs...)
}

func BundleForSession(resolver LiveSessionResolver, sessionID string) (RuntimeInstance, error) {
	session, err := RequireLiveSession(resolver, sessionID)
	if err != nil {
		return nil, err
	}
	return HandleFromSession(session).RuntimeInstance(), nil
}

func FactoryForSession(resolver LiveSessionResolver, sessionID string) (factory.Service, error) {
	if resolver != nil {
		if session := resolver.Resolve(sessionID); session != nil {
			if runtime := ServiceForSession(session); runtime != nil {
				return runtime, nil
			}
		}
	}
	bundle, err := BundleForSession(resolver, sessionID)
	if err != nil {
		return nil, err
	}
	return bundle.RuntimeService(), nil
}

// LegacyObservationForService isolates migration-era Petri snapshot access
// from the singular Factory Runtime Service contract.
func LegacyObservationForService(runtime factory.Service) (legacysnapshot.Provider, error) {
	observation, ok := runtime.(legacysnapshot.Provider)
	if !ok || observation == nil {
		return nil, fmt.Errorf("legacy Factory Runtime observation is unavailable")
	}
	return observation, nil
}

// LegacyEventSource is the migration-only event-history capability retained
// while Factory Session invocation still derives its observation from the
// legacy snapshot and canonical Factory Event stream together.
type LegacyEventSource interface {
	GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error)
}

// LegacyEventSourceForService isolates migration-era event-history access from
// the singular Factory Runtime Service contract.
func LegacyEventSourceForService(runtime factory.Service) (LegacyEventSource, error) {
	source, ok := runtime.(LegacyEventSource)
	if !ok || source == nil {
		return nil, fmt.Errorf("legacy Factory Runtime event history is unavailable")
	}
	return source, nil
}

func RuntimeConfigForSession(resolver LiveSessionResolver, sessionID string) (interfaces.LoadedFactorySource, error) {
	if resolver != nil {
		if session := resolver.Resolve(sessionID); session != nil && session.Runtime != nil && session.Runtime.RuntimeConfig != nil {
			return session.Runtime.RuntimeConfig, nil
		}
	}
	bundle, err := BundleForSession(resolver, sessionID)
	if err != nil {
		return nil, err
	}
	runtimeConfig := loadedFactorySnapshotSource(bundle.LoadedRuntimeConfig())
	if runtimeConfig == nil {
		return nil, fmt.Errorf("loaded runtime config is unavailable")
	}
	return runtimeConfig, nil
}

func loadedFactorySnapshotSource(runtimeConfig factory.LoadedConfig) interfaces.LoadedFactorySource {
	if runtimeConfig == nil {
		return nil
	}
	loaded, _ := runtimeConfig.(interfaces.LoadedFactorySource)
	return loaded
}

// ReplacementExecutionBaseDir preserves an existing session execution root
// and otherwise applies the canonical folder/factory/process fallback order.
func ReplacementExecutionBaseDir(resolver LiveSessionResolver, folderPath, factoryDir, sessionID, processDefault string) string {
	if resolver != nil {
		if session := resolver.Resolve(sessionID); session != nil {
			if executionBaseDir := strings.TrimSpace(session.ExecutionBaseDir); executionBaseDir != "" {
				return executionBaseDir
			}
		}
	}
	if folderPath = strings.TrimSpace(folderPath); folderPath != "" {
		return folderPath
	}
	if factoryDir = strings.TrimSpace(factoryDir); factoryDir != "" {
		return factoryDir
	}
	return strings.TrimSpace(processDefault)
}
