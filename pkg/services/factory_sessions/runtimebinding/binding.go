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
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimemetrics "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtime"
	sessionstream "github.com/portpowered/infinite-you/pkg/services/factory_sessions/stream"
	"go.uber.org/zap"
)

type LiveSessionResolver interface {
	Resolve(string) *factorysessions.LiveSession
}

type Registration struct {
	SessionID      string
	FactoryRootDir string
	Handle         factory.HostedHandle
	Target         factorysessions.Target
	Select         bool
}

// SyncActiveDirectory updates the process compatibility directory to match the
// selected runtime bundle. Domain services should use the bundle directly;
// this helper exists while legacy process configuration remains observable.
func SyncActiveDirectory(mu *sync.RWMutex, configured *string, factoryRoot string, bundle factory.HostedInstance) {
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
	bundle factory.HostedInstance,
	target factorysessions.Target,
	serviceMode bool,
	runtimeMode interfaces.RuntimeMode,
	lifecycle factory.Lifecycle,
	startSidecars func(context.Context, factory.HostedHandle) error,
	stop func(factory.HostedHandle) error,
) (factory.HostedHandle, error) {
	if bundle == nil {
		return nil, fmt.Errorf("runtime bundle is required")
	}
	if state == nil || runtimeState == nil {
		return nil, fmt.Errorf("Factory Session runtime state is required")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("factory runtime lifecycle service is required")
	}
	handle, err := lifecycle.Start(runContext, bundle)
	if err != nil {
		return nil, err
	}
	registeredSessionID := Register(state, Registration{
		SessionID: factorysessions.DefaultSessionID, FactoryRootDir: factoryRootDir,
		Handle: handle, Target: target, Select: true,
	})
	if strings.TrimSpace(registeredSessionID) == "" ||
		state.Resolve(factorysessions.DefaultSessionID) == nil {
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
			factorysessions.DefaultSessionID, handle, stop, err, runtimeMode,
		)
	}
	if serviceMode && startSidecars != nil {
		if err := startSidecars(runContext, handle); err != nil {
			if DefaultSessionClosedDuringStartup(state, runtimeMode) {
				return nil, nil
			}
			runtimeState.ClearActive()
			state.Unregister(factorysessions.DefaultSessionID)
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
// runtime after the request returns.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func Replace(
	readinessContext context.Context,
	state *sessionruntime.Service,
	runtimeState *State,
	session *factorysessions.LiveSession,
	replacement factory.HostedInstance,
	serviceMode bool,
	lifecycle factory.Lifecycle,
	startSidecars func(context.Context, factory.HostedHandle) error,
	stop func(factory.HostedHandle) error,
	report func(error),
) (*factorysessions.LiveSession, error) {
	if session == nil {
		return nil, fmt.Errorf("%w: session handle is unavailable", factorysessions.ErrSessionNotFound)
	}
	current := HandleFromSession(session)
	if current == nil {
		return nil, fmt.Errorf("%w: session handle is unavailable", factorysessions.ErrSessionNotFound)
	}
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

	executionBaseDir := strings.TrimSpace(session.ExecutionBaseDir)
	if replacement != nil && replacement.LoadedRuntimeConfig() != nil {
		if runtimeBaseDir := strings.TrimSpace(replacement.LoadedRuntimeConfig().RuntimeBaseDir()); runtimeBaseDir != "" {
			executionBaseDir = runtimeBaseDir
		}
	}
	state.CloseResponseStreams(session)
	state.Register(sessionruntime.Registration{
		SessionID: session.ID, FactoryDir: replacement.Directory(),
		FolderPath: session.FolderPath, ExecutionBaseDir: executionBaseDir,
		Target: session.Target,
		Handle: &SessionState{
			Handle: replacementHandle, Instance: replacement,
			Spec: preparedSpec,
		},
		Runtime: &factorysessions.LiveRuntime{
			Factory: replacement.RuntimeService(), BackendScopeID: replacement.BackendScope(),
			RuntimeConfig: loadedFactorySnapshotSource(replacement.LoadedRuntimeConfig()),
		},
		Default: session.IsDefault, Project: session.Project,
		Select: isActive, AddEventTypeRecorder: replacement.AddEventTypeRecorder,
	})
	updated := state.Resolve(session.ID)
	if updated != nil {
		updated.RuntimeFactorySessionID = session.RuntimeFactorySessionID
	}
	if isActive {
		runtimeState.SetActive(serviceCtx, session.ID, replacementHandle)
	}
	if stop != nil {
		if err := stop(current); err != nil && !errors.Is(err, context.Canceled) && report != nil {
			report(fmt.Errorf("stop prior session runtime: %w", err))
		}
	}
	return updated, nil
}

// Start launches a Factory Runtime, waits for readiness, starts service-mode
// sidecars, and registers the resulting live Factory Session.
func Start(
	readinessContext context.Context,
	state *sessionruntime.Service,
	runtimeState *State,
	factoryRootDir string,
	sessionID string,
	bundle factory.HostedInstance,
	target factorysessions.Target,
	serviceMode bool,
	lifecycle factory.Lifecycle,
	startSidecars func(context.Context, factory.HostedHandle) error,
	stop func(factory.HostedHandle) error,
) (factory.HostedHandle, error) {
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

func stopStartedHandle(stop func(factory.HostedHandle) error, handle factory.HostedHandle) {
	if stop != nil {
		_ = stop(handle)
		return
	}
	if handle != nil {
		handle.CancelRun()
	}
}

func Register(state *sessionruntime.Service, input Registration) string {
	if state == nil || strings.TrimSpace(input.SessionID) == "" || input.Handle == nil || input.Handle.RuntimeInstance() == nil {
		return ""
	}
	bundle := input.Handle.RuntimeInstance()
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
		Handle:  &SessionState{Instance: bundle, Handle: input.Handle, Spec: metadata.PreparedSpec},
		Runtime: &factorysessions.LiveRuntime{Factory: bundle.RuntimeService(), BackendScopeID: bundle.BackendScope(), RuntimeConfig: loadedFactorySnapshotSource(bundle.LoadedRuntimeConfig())},
		Default: factorysessions.IsDefaultSessionSelector(input.SessionID), Project: metadata.Project,
		Select: input.Select, AllocateDefaultID: true, AddEventTypeRecorder: bundle.AddEventTypeRecorder,
	})
}

func SessionStateFrom(session *factorysessions.LiveSession) *SessionState {
	if session == nil {
		return nil
	}
	state, _ := session.Handle.(*SessionState)
	return state
}

func HandleFromSession(session *factorysessions.LiveSession) factory.HostedHandle {
	state := SessionStateFrom(session)
	if state == nil {
		return nil
	}
	return state.Handle
}

func BundleFromSession(session *factorysessions.LiveSession) factory.HostedInstance {
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
) factory.HostedInstance {
	if runtimeState == nil {
		return nil
	}
	return runtimeState.Current(func() factory.HostedInstance {
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
func CanonicalEventsFromSession(session *factorysessions.LiveSession) []interfaces.FactoryEvent {
	instance := BundleFromSession(session)
	if instance == nil {
		return nil
	}
	return instance.CanonicalEvents()
}

// BackendScopeID resolves the process-configured scope before the session
// runtime bundle fallback.
func BackendScopeID(configured string, session *factorysessions.LiveSession) string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	if bundle := BundleFromSession(session); bundle != nil {
		return strings.TrimSpace(bundle.BackendScope())
	}
	return ""
}

// StreamGenerationID resolves the canonical ledger generation, runtime
// snapshot generation, then runtime start timestamp.
func StreamGenerationID(session *factorysessions.LiveSession) string {
	instance := BundleFromSession(session)
	if instance != nil {
		if generation := strings.TrimSpace(instance.StreamGeneration()); generation != "" {
			return generation
		}
		if runtime := instance.RuntimeService(); runtime != nil {
			snapshot, err := runtime.GetEngineStateSnapshot(context.Background())
			if err == nil && snapshot != nil {
				if generation := strings.TrimSpace(snapshot.StreamGenerationID); generation != "" {
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
	return sessionstream.NewManagerWithDependencies(
		state,
		sessionruntime.NewResponseStreamObserver(ResponseStreamRuntimeFromSessionHandle),
		state.ResponseStreams(),
	)
}

func ResponseStreamRuntimeFromSessionHandle(handle any) (runtimemetrics.MetricsEmitter, *zap.Logger) {
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

func PreparedSpecFromSession(session *factorysessions.LiveSession) any {
	state := SessionStateFrom(session)
	if state == nil {
		return nil
	}
	return state.Spec
}

func RequireLiveSession(resolver LiveSessionResolver, sessionID string) (*factorysessions.LiveSession, error) {
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
func NextLiveSession(state *sessionruntime.Service, exceptSessionID string) *factorysessions.LiveSession {
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
func DefaultSessionSuccessor(state *sessionruntime.Service, runtimeState *State) *factorysessions.LiveSession {
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
	stop func(factory.HostedHandle) error,
) error {
	session, err := RequireLiveSession(state, sessionID)
	if err != nil {
		return err
	}
	handle := HandleFromSession(session)
	sessionID = session.ID
	if active := runtimeState.Active(); active != nil && active.SessionID == sessionID {
		if successor := NextLiveSession(state, sessionID); successor != nil {
			runtimeState.SetActive(active.Context, successor.ID, HandleFromSession(successor))
		} else {
			runtimeState.ClearActive()
		}
	}
	state.Unregister(sessionID)
	if stop == nil {
		return nil
	}
	if err := stop(handle); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// FailStartup clears active selection, unregisters the startup session, and
// joins runtime-stop failure with the original readiness error.
func FailStartup(
	state *sessionruntime.Service,
	runtimeState *State,
	sessionID string,
	handle factory.HostedHandle,
	stop func(factory.HostedHandle) error,
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
	if mode != interfaces.RuntimeModeService {
		return false
	}
	return state == nil || state.Resolve(factorysessions.DefaultSessionID) == nil
}

// HandleStartFailure clears a failed default runtime, preserving the expected
// close-during-startup behavior and joining any shutdown failure.
func HandleStartFailure(
	ctx context.Context,
	state *sessionruntime.Service,
	runtimeState *State,
	sessionID string,
	handle factory.HostedHandle,
	stop func(factory.HostedHandle) error,
	startErr error,
	mode interfaces.RuntimeMode,
) error {
	if DefaultSessionClosedDuringStartup(state, mode) {
		runtimeState.ClearActive()
		if stop != nil {
			_ = stop(handle)
		}
		return nil
	}
	runtimeState.ClearActive()
	if state != nil {
		state.Unregister(sessionID)
	}
	var stopErr error
	if stop != nil {
		stopErr = stop(handle)
	}
	if ctx != nil && ctx.Err() != nil && errors.Is(startErr, context.Canceled) {
		if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
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

// ShutdownOtherLiveSessions stops and unregisters every session except the
// supplied runtime handle.
func ShutdownOtherLiveSessions(
	state *sessionruntime.Service,
	except factory.HostedHandle,
	stop func(factory.HostedHandle) error,
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
		if handle != nil && stop != nil {
			if err := stop(handle); err != nil && !errors.Is(err, context.Canceled) {
				errs = append(errs, err)
			}
		}
		state.Unregister(sessionID)
	}
	return errors.Join(errs...)
}

func BundleForSession(resolver LiveSessionResolver, sessionID string) (factory.HostedInstance, error) {
	session, err := RequireLiveSession(resolver, sessionID)
	if err != nil {
		return nil, err
	}
	return HandleFromSession(session).RuntimeInstance(), nil
}

func FactoryForSession(resolver LiveSessionResolver, sessionID string) (factory.Service, error) {
	bundle, err := BundleForSession(resolver, sessionID)
	if err != nil {
		return nil, err
	}
	return bundle.RuntimeService(), nil
}

func RuntimeConfigForSession(resolver LiveSessionResolver, sessionID string) (interfaces.LoadedFactorySource, error) {
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
