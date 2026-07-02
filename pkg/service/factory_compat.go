// Package service provides compatibility aliases and legacy wire helpers for the
// extracted runtime host. Authoritative composition lives in pkg/initializer and
// pkg/composebridge; runtime/session ownership lives in pkg/runtimehost.
package service

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

const defaultFactorySessionID = runtimehost.DefaultFactorySessionID

const invocationInputSourceStructuredArgs = runtimehost.InvocationInputSourceStructuredArgs

type resolvedSessionInvocationInput = runtimehost.ResolvedSessionInvocationInput

func resolveSessionInvocationInput(
	cfg *interfaces.FactoryConfig,
	request factoryapi.InvocationRequest,
) (resolvedSessionInvocationInput, error) {
	return runtimehost.ResolveSessionInvocationInput(cfg, request)
}

func ProvideModelServiceCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) ModelService {
	return runtimehost.ProvideModelServiceCollaborator(shell, cfg)
}

func AttachModelServiceCollaborator(shell FactoryServiceShell, modelAPI ModelService) *FactoryService {
	return runtimehost.AttachModelServiceCollaborator(shell, modelAPI)
}

func ProvideFactorySaveCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) runtimehost.FactorySaveSaver {
	return runtimehost.ProvideFactorySaveCollaborator(shell, cfg)
}

func AttachFactorySaveCollaborator(shell FactoryServiceShell, factorySave runtimehost.FactorySaveSaver) *FactoryService {
	return runtimehost.AttachFactorySaveCollaborator(shell, factorySave)
}

func ProvideSessionGatewayCollaborator(shell FactoryServiceShell, cfg *FactoryServiceConfig) runtimehost.SessionGateway {
	return runtimehost.ProvideSessionGatewayCollaborator(shell, cfg)
}

func AttachSessionGatewayCollaborator(shell FactoryServiceShell, gateway runtimehost.SessionGateway) *FactoryService {
	return runtimehost.AttachSessionGatewayCollaborator(shell, gateway)
}

// ValidateReplayModeConfig rejects incompatible replay and record mode combinations.
func ValidateReplayModeConfig(cfg *FactoryServiceConfig) error {
	return runtimehost.ValidateReplayModeConfig(cfg)
}

func newInferenceProgressPublisherFactory(sessions *factorysessions.Registry, logger *zap.Logger) runtimehost.InferenceProgressPublisherFactory {
	return runtimehost.NewInferenceProgressPublisherFactory(sessions, logger)
}

// serviceCoordinatorPolicyFromConfig is the legacy test helper name.
func serviceCoordinatorPolicyFromConfig(cfg *FactoryServiceConfig) runtimehost.CoordinatorPolicy {
	return runtimehost.CoordinatorPolicyFromConfig(cfg)
}
func NewWorkersSchedulerService(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	logger *zap.Logger,
	hostedWorkers hostedworkers.Config,
) *workersservice.Service {
	return runtimehost.NewWorkersSchedulerService(cfg, clock, logger, hostedWorkers)
}

type (
	FactoryService              = runtimehost.Host
	FactoryCore                 = runtimehost.Core
	FactoryServiceConfig        = runtimehost.Config
	FactoryServiceShell         = runtimehost.HostShell
	ComposeCollaboratorSnapshot = runtimehost.ComposeCollaboratorSnapshot
	LocalModelDomain            = runtimehost.LocalModelDomain
	FactoryDefinitionService    = runtimehost.FactoryDefinitionService
	SimpleDashboardRenderInput  = runtimehost.SimpleDashboardRenderInput
	SimpleDashboardRenderer     = runtimehost.SimpleDashboardRenderer
	APIServerStarter            = runtimehost.APIServerStarter
	RuntimeFileLoggingPolicy    = runtimehost.RuntimeFileLoggingPolicy
	RuntimeMetricsPolicy        = runtimehost.RuntimeMetricsPolicy
	RuntimeLogDiagnostics       = runtimehost.RuntimeLogDiagnostics
	InvocationMetricsRecorder   = runtimehost.InvocationMetricsRecorder
	InvocationMetric            = runtimehost.InvocationMetric
	ModelPullMetricsRecorder    = runtimehost.ModelPullMetricsRecorder
)

var (
	ErrFactoryActivationRequiresIdle = runtimehost.ErrFactoryActivationRequiresIdle
	ErrInvalidNamedFactoryName       = runtimehost.ErrInvalidNamedFactoryName
	ErrInvalidNamedFactory           = runtimehost.ErrInvalidNamedFactory
	ErrCurrentFactoryNotFound        = runtimehost.ErrCurrentFactoryNotFound
)

const (
	RuntimeFileLoggingPolicyEnabled  = runtimehost.RuntimeFileLoggingPolicyEnabled
	RuntimeFileLoggingPolicyDisabled = runtimehost.RuntimeFileLoggingPolicyDisabled
	RuntimeMetricsPolicyEnabled      = runtimehost.RuntimeMetricsPolicyEnabled
	RuntimeMetricsPolicyDisabled     = runtimehost.RuntimeMetricsPolicyDisabled
)

type (
	localModelCacheLayout       = localmodels.CacheLayout
	localModelLoadRequest       = localmodels.LoadRequest
	localModelInvocationRequest = localmodels.InvocationRequest
	localModelHandle            = localmodels.Handle
	liveFactorySession          = factorysessions.LiveSession
)

const (
	FactorySessionTargetKindDefault = runtimehost.FactorySessionTargetKindDefault
	FactorySessionTargetKindNamed   = runtimehost.FactorySessionTargetKindNamed
)

type (
	FactorySessionTargetKind = runtimehost.FactorySessionTargetKind
	FactorySessionTargetRef  = runtimehost.FactorySessionTargetRef
	FactorySessionTarget     = runtimehost.FactorySessionTarget
	FactorySessionOpenResult = runtimehost.FactorySessionOpenResult
	liveSessionState         = runtimehost.LiveSessionState
)

func liveSessionHandle(session *factorysessions.LiveSession) *liveRuntimeHandle {
	return runtimehost.LiveSessionHandle(session)
}

const (
	runtimeMetricLifecycleStarted               = runtimehost.RuntimeMetricLifecycleStarted
	runtimeMetricLifecycleStopped               = runtimehost.RuntimeMetricLifecycleStopped
	runtimeMetricStateActive                    = runtimehost.RuntimeMetricStateActive
	runtimeMetricStateIdle                      = runtimehost.RuntimeMetricStateIdle
	runtimeMetricStatePaused                    = runtimehost.RuntimeMetricStatePaused
	runtimeMetricStateFailed                    = runtimehost.RuntimeMetricStateFailed
	runtimeMetricQueueInFlight                  = runtimehost.RuntimeMetricQueueInFlight
	runtimeMetricQueueSubmissionCount           = runtimehost.RuntimeMetricQueueSubmissionCount
	runtimeMetricDispatchStarted                = runtimehost.RuntimeMetricDispatchStarted
	runtimeMetricDispatchComplete               = runtimehost.RuntimeMetricDispatchComplete
	runtimeMetricDispatchDuration               = runtimehost.RuntimeMetricDispatchDuration
	runtimeMetricDispatchRetries                = runtimehost.RuntimeMetricDispatchRetries
	runtimeMetricDispatchCost                   = runtimehost.RuntimeMetricDispatchCost
	runtimeMetricProviderRequest                = runtimehost.RuntimeMetricProviderRequest
	runtimeMetricProviderComplete               = runtimehost.RuntimeMetricProviderComplete
	runtimeMetricProviderFailed                 = runtimehost.RuntimeMetricProviderFailed
	runtimeMetricProviderDuration               = runtimehost.RuntimeMetricProviderDuration
	runtimeMetricProviderInputTok               = runtimehost.RuntimeMetricProviderInputTok
	runtimeMetricProviderOutputTok              = runtimehost.RuntimeMetricProviderOutputTok
	runtimeMetricProviderCost                   = runtimehost.RuntimeMetricProviderCost
	runtimeMetricScriptStarted                  = runtimehost.RuntimeMetricScriptStarted
	runtimeMetricScriptComplete                 = runtimehost.RuntimeMetricScriptComplete
	runtimeMetricScriptDuration                 = runtimehost.RuntimeMetricScriptDuration
	runtimeMetricScriptTimedOut                 = runtimehost.RuntimeMetricScriptTimedOut
	runtimeMetricScriptFailed                   = runtimehost.RuntimeMetricScriptFailed
	runtimeMetricSessionResponseStreamPublished = runtimehost.RuntimeMetricSessionResponseStreamPublished
	runtimeMetricSessionResponseStreamCompacted = runtimehost.RuntimeMetricSessionResponseStreamCompacted
	runtimeMetricSessionResponseStreamDegraded  = runtimehost.RuntimeMetricSessionResponseStreamDegraded
	runtimeMetricLifecycleControl               = runtimehost.RuntimeMetricLifecycleControl
)

const (
	modelPullMetricAttempts      = runtimehost.ModelPullMetricAttempts
	modelPullMetricSuccess       = runtimehost.ModelPullMetricSuccess
	modelPullMetricFailure       = runtimehost.ModelPullMetricFailure
	modelPullMetricSourceFailure = runtimehost.ModelPullMetricSourceFailure
)

const (
	invocationMetricNormalizationAttempts = runtimehost.InvocationMetricNormalizationAttempts
	invocationMetricNormalizationSuccess  = runtimehost.InvocationMetricNormalizationSuccess
	invocationMetricNormalizationFailure  = runtimehost.InvocationMetricNormalizationFailure
	invocationMetricInterpolationFailure  = runtimehost.InvocationMetricInterpolationFailure
	invocationMetricAttempts              = runtimehost.InvocationMetricAttempts
	invocationMetricSuccess               = runtimehost.InvocationMetricSuccess
	invocationMetricFailure               = runtimehost.InvocationMetricFailure
	invocationMetricUnresolvedPrimary     = runtimehost.InvocationMetricUnresolvedPrimary
	invocationMetricFallbackPolicyUsed    = runtimehost.InvocationMetricFallbackPolicyUsed
	invocationMetricResultType            = runtimehost.InvocationMetricResultType
)

// NewFactoryServiceFromCore wraps a composed core in the compatibility alias.
func NewFactoryServiceFromCore(core *FactoryCore) *FactoryService {
	return runtimehost.NewHostFromCore(core)
}

// BuildFactoryService loads factory configuration and constructs the compatibility host.
func BuildFactoryService(ctx context.Context, cfg *FactoryServiceConfig) (*FactoryService, error) {
	core, err := BuildFactoryCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	host := NewFactoryServiceFromCore(core)
	shell := FactoryServiceShell{Host: host}
	host = AttachModelServiceCollaborator(shell, ProvideModelServiceCollaborator(shell, cfg))
	host = AttachFactorySaveCollaborator(
		FactoryServiceShell{Host: host},
		ProvideFactorySaveCollaborator(FactoryServiceShell{Host: host}, cfg),
	)
	return AttachSessionGatewayCollaborator(
		FactoryServiceShell{Host: host},
		ProvideSessionGatewayCollaborator(FactoryServiceShell{Host: host}, cfg),
	), nil
}
