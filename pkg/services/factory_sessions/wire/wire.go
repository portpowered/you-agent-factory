// Package wire is the Factory Sessions service composition boundary.
//
// Wire performs construction only, returns the singular factorysessions.Service
// root interface, and starts no lifecycle components. Parent-private identity
// and response-stream collaborators are supplied through focused owner providers;
// peers depend on Service rather than owner internals or construction ports.
package wire

import (
	"fmt"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation/packagedtts"
	invocationruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation/runtimeadapter"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/requestpreparation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	factorysessionroot "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	durableexecutionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/wire"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	identitywire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/wire"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	sessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	sessionstream "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewRequestPreparation constructs the private request-normalization
// implementation for injection into Factory Sessions-owned transports.
func NewRequestPreparation() RequestPreparation {
	return requestpreparation.New()
}

// NewWorkStopSummaryProjector constructs the owner-private stopped-state
// projection policy for injection into Work-owned read adapters.
func NewWorkStopSummaryProjector() factorysessions.WorkStopSummaryProjector {
	return func(request factorysessions.WorkStopSummaryRequest) *factorysessions.StopSummary {
		return sessionprojection.ProjectWorkStopSummary(
			request.SessionID,
			request.Snapshot,
			request.Token,
			request.SessionStopSummary,
		)
	}
}

// NewLiveChangeCoordinator constructs the one process-scoped admission
// coordinator shared by live and durable Factory Session execution.
func NewLiveChangeCoordinator() factorysessioncontracts.LiveChangeCoordinator {
	return livechange.NewCoordinator()
}

// Identity is the owner-private normalization capability supplied by canonical Wire.
type Identity = identity.Service

// ResponseStreams allocates session-owned stores using the selected effects.
type ResponseStreams = responsestreamservice.Service

// NewIdentity constructs reusable identity behavior without allocating session state.
func NewIdentity(resolveSymlinks factorysessions.LogicalTargetResolveSymlinks, resolveHome factorysessions.HomeDirectoryResolver) (Identity, error) {
	return identitywire.NewService(resolveSymlinks, resolveHome)
}

// NewResponseStreams constructs reusable response behavior with the selected logger.
func NewResponseStreams(eventIDs factorysessions.ResponseEventIDGenerator, limits *factorysessions.ResponseEventRetentionLimits, eventsService events.Service, logger logging.Logger) (ResponseStreams, error) {
	return responsestreamwire.NewService(eventIDs, limits, eventsService, logger)
}

// SessionRegistry is the explicit process-owned live session directory.
type SessionRegistry = sessionregistry.Service

// ResponseStreamRegistry is the paired dispatch-stream registry.
type ResponseStreamRegistry = responsestream.Registry

// SessionState is the independent live session authority.
type SessionState = sessionruntime.Service

// InvocationAuthority selects the addressed generation for invocation queries.
type InvocationAuthority = sessionservice.InvocationAuthority

// InvocationService is the completed owner-private engine supplied to Assembly.
type InvocationService = roles.InvocationService
type InvocationTelemetry = sessioninvocation.SessionInvocationTelemetry
type InvocationSpecialCase = sessioninvocation.SessionInvocationSpecialCase

// NewInvocationOwner constructs the single reusable invocation engine.
func NewInvocationOwner(authority InvocationAuthority, control SessionScopeControl, telemetry InvocationTelemetry, specialCase InvocationSpecialCase, interpolation factorydefinitions.InvocationInterpolationService, workTypes factorydefinitions.InvocationWorkTypeService, inputFiles fileeffects.InvocationInputReader, workPolicy InvocationWorkPolicy) (InvocationService, error) {
	if inputFiles == nil {
		return nil, fmt.Errorf("construct invocation: invocation input reader is required")
	}
	return sessioninvocation.NewSessionOwner(authority, control, telemetry, specialCase, interpolation, workTypes, inputFiles, workPolicy), nil
}

// NewInvocationTelemetry preserves selected metrics and addressed runtime logging.
func NewInvocationTelemetry(state *SessionState, observability factorydefinitions.TTSObservabilityService, metrics InvocationMetricsRecorder, logger *zap.Logger) InvocationTelemetry {
	return packagedtts.NewTelemetry(observability,
		func(metric sessioninvocation.SessionInvocationMetric) {
			metrics.RecordInvocationMetric(factorysessions.InvocationMetric{Name: metric.Name, Labels: metric.Labels})
		},
		func(record sessioninvocation.SessionInvocationLogRecord) {
			selectedLogger := logger
			if sessionID, ok := record.Fields["session_id"].(string); ok {
				if session := state.Resolve(sessionID); session != nil {
					if bound := runtimebinding.SessionStateFrom(session); bound != nil && bound.Logger != nil {
						selectedLogger = bound.Logger
					}
				}
			}
			invocationruntime.WriteLogRecord(selectedLogger, record)
		},
	)
}

func NewInvocationSpecialCase(observability factorydefinitions.TTSObservabilityService) InvocationSpecialCase {
	return packagedtts.NewSpecialCase(observability)
}

// NewInvocationWorkPolicy supplies pure return policy without the Work root cycle.
type InvocationWorkPolicy interface{ work.Service }

// DisabledInvocationMetricsRecorder is the explicit optional-edge policy.
type DisabledInvocationMetricsRecorder struct{}

func (DisabledInvocationMetricsRecorder) RecordInvocationMetric(factorysessions.InvocationMetric) {}

// NewInvocationAuthority constructs the independent invocation query adapter.
func NewInvocationAuthority(state *SessionState, scheduler platformclock.TimerSource, projector factoryruntime.WorldStateProjector) InvocationAuthority {
	return sessionservice.NewInvocationAuthority(state, scheduler, projector)
}

type SessionScopeActivation = sessionservice.SessionScopeActivation

func NewScopeActivation(state *SessionState) SessionScopeActivation {
	return sessionservice.NewScopeActivation(state)
}

// SessionScopeControl owns addressed cancellation and stop independently of the gateway.
type SessionScopeControl = sessionservice.SessionScopeControl

// NewScopeControl constructs control over the canonical session authority.
func NewScopeControl(state *SessionState, stop factoryruntime.RuntimeStopOperation, logger *zap.Logger) SessionScopeControl {
	return sessionservice.NewScopeControl(state, stop, logger)
}

// StreamManager supplies provider progress and dispatch completion factories.
type StreamManager = sessionservice.StreamManager

// StreamObserver supplies runtime telemetry for response streams.
type StreamObserver = sessionstream.Observer

// NewSessionRegistry allocates the process-owned live directory.
func NewSessionRegistry() SessionRegistry {
	return sessionregistry.New()
}

// NewResponseStreamRegistry allocates dispatch state through the selected response owner.
func NewResponseStreamRegistry(responses ResponseStreams, clock factoryruntime.Clock) (*ResponseStreamRegistry, error) {
	return responses.NewStreamRegistry(clock)
}

// NewSessionState constructs authority over the explicitly paired registries.
func NewSessionState(registry SessionRegistry, responseRegistry *ResponseStreamRegistry, clock factoryruntime.Clock, eventIDs factorysessions.ResponseEventIDGenerator, sessionIDs factorysessions.SessionIDGenerator, responses ResponseStreams) *SessionState {
	return sessionruntime.NewWithResponseService(registry, responseRegistry, nil, clock, eventIDs, sessionIDs, responses)
}

// NewStreamObserver constructs telemetry using the existing runtime handle resolver.
func NewStreamObserver() StreamObserver {
	return sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle)
}

// GatewayStreams is the complete stream role supplied directly to the gateway.
type GatewayStreams = sessionstream.Manager

// NewStreamManager constructs reusable streams over the supplied authority and response owner.
func NewStreamManager(state *SessionState, observer StreamObserver, responseRegistry *ResponseStreamRegistry, responses ResponseStreams) *GatewayStreams {
	return sessionstream.NewManagerWithResponseService(state, observer, responseRegistry, responses)
}

// ProcessDurableScope supplies independent project and resume queries.
type ProcessDurableScope = sessionservice.ProcessDurableScope

func NewProcessDurableScope(state *SessionState) ProcessDurableScope {
	return sessionservice.NewProcessDurableScope(state)
}

// RecordedHistory supplies independent recorded-session inventory reads.
type RecordedHistory = sessionservice.RecordedHistory

func NewRecordedHistory(resolveHome factorysessions.HomeDirectoryResolver, inventory recordings.RecordedSessionInventory) RecordedHistory {
	return sessionservice.NewRecordedHistory(resolveHome, inventory)
}

// NewRuntimeAssembly builds the one owner-private Factory Sessions assembly
// used by peer roots while canonical Wire completes the rest of the process
// graph. It is an assembly capability, not a second published Service root.
func NewRuntimeAssembly(
	registry SessionRegistry,
	state *SessionState,
	streams StreamManager,
	invoker roles.InvocationService,
	control SessionScopeControl,
	activation SessionScopeActivation,
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService Identity,
	responseStreams ResponseStreams,
	clock factoryruntime.Clock,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordedHistory RecordedHistory,
) (RuntimeAssembly, error) {
	assembly, err := factorysessionroot.NewAssembly(
		registry, state, streams, invoker, control, activation,
		newJavaScriptCheckpointStore,
		sessionResultProjection,
		eventIDs,
		sessionIDs,
		resolveHome,
		directoryInspection,
		namedPaths,
		initialWorkFiles,
		identityService,
		responseStreams,
		clock,
		liveChangeCoordinator,
		recordedHistory,
	)
	if err != nil {
		return nil, err
	}
	if assembly == nil {
		return nil, fmt.Errorf("construct Factory Sessions: implementation rejected its dependencies")
	}
	return assembly, nil
}

// NewServiceFromAssembly binds the already-composed owner assembly to the
// existing process root.
func NewServiceFromAssembly(
	assembly RuntimeAssembly,
	root *factorysessionroot.Root,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (*factorysessionroot.Root, error) {
	service, err := factorysessionroot.NewRootFromAssembly(
		assembly,
		root,
		liveChangeCoordinator,
	)
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, fmt.Errorf("construct Factory Sessions: implementation rejected its dependencies")
	}
	return service, nil
}

func NewDurableExecution(
	projectRoot string,
	persistencePolicy factorysessions.PersistencePolicy,
	stores RuntimePersistenceStoreFactory,
	childExecutorMode string,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	workerPresetIDs map[string]struct{},
	workerSettings factoryruntime.JavaScriptWorkerSettings,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	generateResponseEventID factorysessions.ResponseEventIDGenerator,
	responseStreams ResponseStreams,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (durableexecution.Service, error) {
	return durableexecutionwire.NewDurable(
		projectRoot, persistencePolicy, stores, childExecutorMode, clock, syncWaits,
		checkpointSummaries, workflows, orchestration, workflows,
		workerPresetIDs, workerSettings,
		recordingWriter, generateSessionID, generateResponseEventID, responseStreams,
		liveChangeCoordinator,
	)
}
