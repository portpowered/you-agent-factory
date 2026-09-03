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
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	sessionstream "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
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

// StartDefault starts and registers the default Factory Session before waiting
// for readiness so service-mode clients may close it during startup.
func StartDefault(
	readinessContext context.Context,
	runContext context.Context,
	state *sessionruntime.Service,
	runtimeState *State,
	factoryRootDir string,
	bundle RuntimeInstance,
	target factorysessions.Target,
	serviceMode bool,
	runtimeMode interfaces.RuntimeMode,
	lifecycle RuntimeLifecycle,
	startSidecars func(context.Context, RuntimeHandle) error,
	stop func(RuntimeHandle) error,
	onSessionRemoved func(string),
) (RuntimeHandle, error) {
	return StartInitial(
		readinessContext, runContext, state, runtimeState,
		factorysessions.DefaultSessionID, factoryRootDir, bundle, target,
		serviceMode, runtimeMode, lifecycle, startSidecars, stop, onSessionRemoved,
	)
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
	serviceMode bool,
	runtimeMode interfaces.RuntimeMode,
	lifecycle RuntimeLifecycle,
	startSidecars func(context.Context, RuntimeHandle) error,
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
		return nil, HandleStartFailure(
			readinessContext, state, runtimeState,
			sessionID, handle, stop, err, runtimeMode, onSessionRemoved,
		)
	}
	if serviceMode && startSidecars != nil {
		if err := startSidecars(runContext, handle); err != nil {
			if SessionClosedDuringStartup(state, sessionID, runtimeMode) {
				unregisterSession(state, sessionID, onSessionRemoved)
				return nil, nil
			}
			runtimeState.ClearActive()
			unregisterSession(state, sessionID, onSessionRemoved)
			if stop != nil {
				_ = stop(handle)
			}
			return nil, err
		}
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
	executionBaseDir := strings.TrimSpace(session.ExecutionBaseDir)
	if replacement != nil && replacement.LoadedRuntimeConfig() != nil {
		if runtimeBaseDir := strings.TrimSpace(replacement.LoadedRuntimeConfig().RuntimeBaseDir()); runtimeBaseDir != "" {
			executionBaseDir = runtimeBaseDir
		}
	}
	state.RotateResponseStreams(session)
	state.Register(sessionruntime.Registration{
		SessionID: session.ID, FactoryDir: replacement.Directory(),
		FolderPath: session.FolderPath, ExecutionBaseDir: executionBaseDir,
		RuntimeFactorySessionID: session.RuntimeFactorySessionID,
		RuntimeEventSessionID:   session.RuntimeEventSessionID,
		Target:                  session.Target,
		Handle: &SessionState{
			Handle: replacementHandle, Instance: replacement,
			Spec: preparedSpec,
		},
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
		Default: session.IsDefault, Project: session.Project,
		Select: isActive, AddEventTypeRecorder: replacement.AddEventTypeRecorder,
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
	return state.Register(sessionruntime.Registration{
		SessionID: input.SessionID, FactoryDir: metadata.FactoryDir, FolderPath: metadata.FolderPath,
		ExecutionBaseDir: metadata.ExecutionBaseDir, Target: metadata.Target,
		Handle: &SessionState{Instance: bundle, Handle: input.Handle, Spec: metadata.PreparedSpec},
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
// Sessions bound the runtime, falling back to the bound Runtime capability
// while older openings that predate the declared field are still in flight.
func WorkAndEventIngressForLiveRuntime(runtime *factorysessions.LiveRuntime) (factory.APIFactory, bool) {
	if runtime == nil {
		return nil, false
	}
	if runtime.WorkAndEventIngress != nil {
		return runtime.WorkAndEventIngress, true
	}
	return WorkAndEventIngressForService(ServiceForLiveRuntime(runtime))
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

// CurrentBundle resolves the process-selected active runtime, the default
// Factory Session runtime, then the pre-start bundle.
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

// NewStreamManager composes provider response streams from the canonical
// Session registry and Factory Runtime telemetry resolver.
func NewStreamManager(state *sessionruntime.Service) *sessionstream.Manager {
	if state == nil {
		return nil
	}
	return sessionstream.NewManagerWithResponseService(
		state,
		sessionruntime.NewResponseStreamObserver(ResponseStreamRuntimeFromSessionHandle),
		state.ResponseStreams(),
		state.ResponseEventService(),
	)
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

// StopSession removes one live session, updates active-runtime selection, and
// stops its Factory Runtime through the injected lifecycle edge.
func StopSession(
	state *sessionruntime.Service,
	runtimeState *State,
	sessionID string,
	stop func(RuntimeHandle) error,
) error {
	if state == nil {
		return fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	session := state.Resolve(sessionID)
	if session == nil {
		return fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	handle := HandleFromSession(session)
	if handle == nil {
		return fmt.Errorf("%w: session handle is unavailable", factorysessions.ErrSessionNotFound)
	}
	sessionID = session.ID
	binding := BindingForSession(session)
	if active := runtimeState.Active(); active != nil && active.SessionID == sessionID {
		if successor := NextLiveSession(state, sessionID); successor != nil {
			runtimeState.SetActive(active.Context, successor.ID, HandleFromSession(successor))
		} else {
			runtimeState.ClearActive()
		}
	}
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
	if err := errors.Join(cleanupErrs...); err != nil {
		return err
	}
	state.Unregister(sessionID)
	return nil
}

// FailStartup clears active selection, unregisters the startup session, and
// joins runtime-stop failure with the original readiness error.
func FailStartup(
	state *sessionruntime.Service,
	runtimeState *State,
	sessionID string,
	handle RuntimeHandle,
	stop func(RuntimeHandle) error,
	startupErr error,
) error {
	runtimeState.ClearActive()
	if state != nil {
		state.Unregister(sessionID)
	}
	if handle == nil || stop == nil {
		return startupErr
	}
	if stopErr := stop(handle); stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		return errors.Join(startupErr, stopErr)
	}
	return startupErr
}

// DefaultSessionClosedDuringStartup reports the expected service-mode race in
// which a client closes the default session while startup is still waiting.
func DefaultSessionClosedDuringStartup(
	state *sessionruntime.Service,
	mode interfaces.RuntimeMode,
) bool {
	return SessionClosedDuringStartup(state, factorysessions.DefaultSessionID, mode)
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
	if SessionClosedDuringStartup(state, sessionID, mode) {
		runtimeState.ClearActive()
		unregisterSession(state, sessionID, onSessionRemoved)
		if stop != nil {
			_ = stop(handle)
		}
		return nil
	}
	runtimeState.ClearActive()
	unregisterSession(state, sessionID, onSessionRemoved)
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
	for _, sessionID := range state.Registry().IDs() {
		session := state.Resolve(sessionID)
		if session == nil {
			continue
		}
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
		state.Unregister(sessionID)
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

// LegacyInvocationSourcesForService resolves the paired compatibility
// capabilities still needed by invocation observation in one boundary check.
func LegacyInvocationSourcesForService(runtime factory.Service) (legacysnapshot.Provider, LegacyEventSource, error) {
	observation, err := LegacyObservationForService(runtime)
	if err != nil {
		return nil, nil, err
	}
	events, err := LegacyEventSourceForService(runtime)
	return observation, events, err
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
