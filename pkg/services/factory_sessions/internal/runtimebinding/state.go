package runtimebinding

import (
	"context"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/workersettings"
	"sync"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// SessionProjectionOwner is the existing per-session runtime state needed to
// project a live session. It is retained on the canonical session record.
type SessionProjectionOwner interface {
	BuildSessionProjectionContext(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error)
}

// ActiveRuntime is Factory Session's selection of one running runtime handle.
type ActiveRuntime struct {
	Context   context.Context
	SessionID string
	Handle    RuntimeHandle
}

// State owns startup and active runtime selection without depending on a
// concrete Factory Runtime host.
type State struct {
	startupMu sync.RWMutex
	startup   RuntimeInstance
	activeMu  sync.RWMutex
	active    *ActiveRuntime
}

func (s *State) Startup() RuntimeInstance {
	if s == nil {
		return nil
	}
	s.startupMu.RLock()
	defer s.startupMu.RUnlock()
	return s.startup
}

func (s *State) SetStartup(instance RuntimeInstance) {
	if s == nil {
		return
	}
	s.startupMu.Lock()
	s.startup = instance
	s.startupMu.Unlock()
}

func (s *State) ClearStartup() { s.SetStartup(nil) }

func (s *State) Active() *ActiveRuntime {
	if s == nil {
		return nil
	}
	s.activeMu.RLock()
	defer s.activeMu.RUnlock()
	if s.active == nil {
		return nil
	}
	copy := *s.active
	return &copy
}

func (s *State) ActiveHandle() RuntimeHandle {
	active := s.Active()
	if active == nil {
		return nil
	}
	return active.Handle
}

func (s *State) SetActive(ctx context.Context, sessionID string, handle RuntimeHandle) {
	if s == nil {
		return
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if ctx == nil {
		s.active = nil
		return
	}
	s.active = &ActiveRuntime{Context: ctx, SessionID: sessionID, Handle: handle}
}

func (s *State) ClearActive() {
	if s == nil {
		return
	}
	s.activeMu.Lock()
	s.active = nil
	s.activeMu.Unlock()
}

// RetireActive changes selection only after successful cleanup, and only if
// the retired run still owns it. A replacement or peer selected during cleanup
// keeps its own active context and handle.
func (s *State) RetireActive(sessionID string, handle RuntimeHandle, successor *livesession.LiveSession) {
	if s == nil {
		return
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if s.active == nil || s.active.SessionID != sessionID || s.active.Handle != handle {
		return
	}
	if successor == nil {
		s.active = nil
		return
	}
	s.active = &ActiveRuntime{Context: s.active.Context, SessionID: successor.ID, Handle: HandleFromSession(successor)}
}

func (s *State) Current(defaultInstance func() RuntimeInstance) RuntimeInstance {
	if handle := s.ActiveHandle(); handle != nil {
		if instance := handle.RuntimeInstance(); instance != nil {
			return instance
		}
	}
	// A newly opened SessionRuntime owns its startup instance. The process-wide
	// default may belong to another concurrent invocation and must not replace
	// that invocation-local startup authority before StartInitial registers it.
	if instance := s.Startup(); instance != nil {
		return instance
	}
	if defaultInstance != nil {
		if instance := defaultInstance(); instance != nil {
			return instance
		}
	}
	return nil
}

func runtimeContext(fallback context.Context, active *ActiveRuntime) context.Context {
	if active != nil && active.Context != nil {
		return active.Context
	}
	return fallback
}

// SessionState is the opaque Factory Runtime payload retained by a live
// Factory Session.
type SessionState struct {
	Instance        RuntimeInstance
	Handle          RuntimeHandle
	Spec            any
	Owner           SessionProjectionOwner
	Invoker         roles.CanonicalSessionInvoker
	ModelInvoker    workers.ModelInvoker
	ModelInvocation modelinvocation.RuntimeModelInvocation
	InputResolver   roles.InvocationInputResolver
	// Process and Diagnostics are application lifecycle values owned by this
	// canonical session record. The process root routes transport commands by
	// session ID instead of retaining another runtime-opening graph.
	Process        roles.ProcessRuntime
	Diagnostics    factoryruntime.RuntimeLogDiagnostics
	FactoryRuntime factoryruntime.Service
	ModelsScope    models.RuntimeScopeRef
	Logger         *zap.Logger
	Reader         roles.RuntimeReader
	Projections    recordings.ProjectionService
	Clock          factoryruntime.Clock
	// ProjectionBackendScope retains the opening override as a keyed fact.
	ProjectionBackendScope string
	CurrentBoardRecordPath string
	OperatorSettingsPath   string
	Recordings             recordings.Service
	ReplayMetadataWarnings []recordings.MetadataMismatchWarning
	ResumeRecoveryMetadata *recordings.ResumeRecoveryMetadata
	OrderlyStop            func(context.Context) error
	// Activation retains lifecycle cleanup on the canonical session record.
	Activation interface{ Close(context.Context) error }
	// workerSessionsMu guards workerSessions, which Factory Session start binds
	// after the session is already resolvable by fleet observation reads.
	workerSessionsMu   sync.RWMutex
	workerSessions     workersessions.ObservationService
	mockWorkersMu      sync.RWMutex
	mockWorkers        *workers.MockWorkersConfig
	operatorDefaultsMu sync.RWMutex
	operatorDefaults   operatorsettings.ResolvedDefaults
	workerSettingsMu   sync.RWMutex
	workerSettings     *factoryruntime.JavaScriptWorkerSettings
	startRequestMu     sync.RWMutex
	startRequestID     string
	controlMu          sync.Mutex
	lastControlKey     string
	lastControlResult  factorysessions.SessionControlResult
}

func (s *SessionState) inheritApplicationValues(previous *SessionState) {
	if s == nil || previous == nil {
		return
	}
	s.FactoryRuntime = previous.FactoryRuntime
	s.ModelsScope = previous.ModelsScope
	s.ModelInvocation = previous.ModelInvocation
	s.SetWorkerSessions(previous.WorkerSessionsObservation())
	s.SetMockWorkers(previous.MockWorkersConfig())
	s.SetOperatorDefaults(previous.OperatorDefaults())
	s.SetWorkerSettings(previous.WorkerSettingsSnapshot())
	s.Logger = previous.Logger
	s.Reader = previous.Reader
	s.Projections = previous.Projections
	s.Clock = previous.Clock
	s.CurrentBoardRecordPath = previous.CurrentBoardRecordPath
	s.OperatorSettingsPath = previous.OperatorSettingsPath
	s.Recordings = previous.Recordings
	s.ReplayMetadataWarnings = append([]recordings.MetadataMismatchWarning(nil), previous.ReplayMetadataWarnings...)
	if previous.ResumeRecoveryMetadata != nil {
		metadata := *previous.ResumeRecoveryMetadata
		s.ResumeRecoveryMetadata = &metadata
	}
	s.OrderlyStop = previous.OrderlyStop
}

// SetWorkerSessions publishes the Worker Sessions observation service bound to
// this session. Start calls it after the session is already resolvable.
func (s *SessionState) SetWorkerSessions(observation workersessions.ObservationService) {
	if s == nil {
		return
	}
	s.workerSessionsMu.Lock()
	s.workerSessions = observation
	s.workerSessionsMu.Unlock()
}

// WorkerSessionsObservation returns the Worker Sessions observation service
// bound to this session. Concurrent readers must use it instead of the field.
func (s *SessionState) WorkerSessionsObservation() workersessions.ObservationService {
	if s == nil {
		return nil
	}
	s.workerSessionsMu.RLock()
	observation := s.workerSessions
	s.workerSessionsMu.RUnlock()
	return observation
}

func (s *SessionState) SetMockWorkers(config *workers.MockWorkersConfig) {
	if s == nil {
		return
	}
	s.mockWorkersMu.Lock()
	s.mockWorkers = config.Clone()
	s.mockWorkersMu.Unlock()
}

func (s *SessionState) MockWorkersConfig() *workers.MockWorkersConfig {
	if s == nil {
		return nil
	}
	s.mockWorkersMu.RLock()
	config := s.mockWorkers.Clone()
	s.mockWorkersMu.RUnlock()
	return config
}

func (s *SessionState) SetOperatorDefaults(defaults operatorsettings.ResolvedDefaults) {
	if s == nil {
		return
	}
	s.operatorDefaultsMu.Lock()
	s.operatorDefaults = defaults
	s.operatorDefaultsMu.Unlock()
}

func (s *SessionState) OperatorDefaults() operatorsettings.ResolvedDefaults {
	if s == nil {
		return operatorsettings.ResolvedDefaults{}
	}
	s.operatorDefaultsMu.RLock()
	defaults := s.operatorDefaults
	s.operatorDefaultsMu.RUnlock()
	return defaults
}

func (s *SessionState) SetWorkerSettings(settings *factoryruntime.JavaScriptWorkerSettings) {
	if s == nil {
		return
	}
	s.workerSettingsMu.Lock()
	s.workerSettings = workersettings.Clone(settings)
	s.workerSettingsMu.Unlock()
}

func (s *SessionState) WorkerSettingsSnapshot() *factoryruntime.JavaScriptWorkerSettings {
	if s == nil {
		return nil
	}
	s.workerSettingsMu.RLock()
	settings := workersettings.Clone(s.workerSettings)
	s.workerSettingsMu.RUnlock()
	return settings
}

func (s *SessionState) SetStartRequestID(requestID string) {
	if s == nil {
		return
	}
	s.startRequestMu.Lock()
	s.startRequestID = requestID
	s.startRequestMu.Unlock()
}

func (s *SessionState) StartRequestID() string {
	if s == nil {
		return ""
	}
	s.startRequestMu.RLock()
	defer s.startRequestMu.RUnlock()
	return s.startRequestID
}

// ApplyControlOnce serializes controls on one canonical session generation.
// A repeated committed control ID returns its original result.
func (s *SessionState) ApplyControlOnce(key string, apply func() (factorysessions.SessionControlResult, error)) (factorysessions.SessionControlResult, error) {
	if s == nil {
		return factorysessions.SessionControlResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	if key != "" && key == s.lastControlKey {
		return s.lastControlResult, nil
	}
	result, err := apply()
	if err == nil && key != "" {
		s.lastControlKey = key
		s.lastControlResult = result
	}
	return result, err
}

func (s *SessionState) CanReplaceTerminatedSession() bool {
	if s == nil {
		return false
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	return (s.lastControlResult.Operation == factorysessions.SessionControlCancel || s.lastControlResult.Operation == factorysessions.SessionControlTerminate) &&
		s.lastControlResult.Status == factorysessions.LifecycleStatusSucceeded
}

// InheritTerminalControl carries the committed cancellation fence across a
// same-ID replacement. Retried Chat controls then replay the old outcome
// instead of terminating the newly started runtime.
func (s *SessionState) InheritTerminalControl(previous *SessionState) {
	if s == nil || previous == nil || s == previous {
		return
	}
	previous.controlMu.Lock()
	key := previous.lastControlKey
	result := previous.lastControlResult
	previous.controlMu.Unlock()
	if key == "" || (result.Operation != factorysessions.SessionControlCancel && result.Operation != factorysessions.SessionControlTerminate) {
		return
	}
	s.controlMu.Lock()
	s.lastControlKey = key
	s.lastControlResult = result
	s.controlMu.Unlock()
}
