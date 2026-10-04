package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// applicationSessionState resolves application lifecycle values from the
// canonical live-session record. No application runtime registry is allocated.
func (r *Root) applicationSessionState(sessionID string) (*runtimebinding.SessionState, error) {
	if r == nil || r.Assembly == nil {
		return nil, fmt.Errorf("Factory Sessions process root is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("application Factory Session ID is required")
	}
	session := r.Resolve(sessionID)
	if session == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Process == nil {
		return nil, fmt.Errorf("%w: application process for session %q", factorysessions.ErrRuntimeNotAvailable, sessionID)
	}
	return bound, nil
}

// RunApplicationTransport hosts the selected Factory Session through its
// process-owned transport. The runtime remains attached to one session record.
func (r *Root) RunApplicationTransport(ctx context.Context, sessionID string, handler http.Handler) error {
	if handler == nil {
		return fmt.Errorf("run Factory Session transport: HTTP handler is required")
	}
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return err
	}
	return bound.Process.RunTransport(ctx, handler)
}

// StopApplicationRuntime ends the process lifecycle before orderly recording
// flush and final session cleanup run in the initializer plan.
func (r *Root) StopApplicationRuntime(ctx context.Context, sessionID string) error {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		if errors.Is(err, factorysessions.ErrSessionNotFound) {
			return nil
		}
		return err
	}
	return bound.Process.Stop(ctx)
}

// CloseApplicationSession completes the initializer plan's resource phase.
func (r *Root) CloseApplicationSession(ctx context.Context, sessionID string) error {
	_, err := r.Control(ctx, factorysessions.SessionControlRequest{
		SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive,
		Operation: factorysessions.SessionControlClose,
	})
	if errors.Is(err, factorysessions.ErrSessionNotFound) {
		return nil
	}
	return err
}

func (r *Root) ApplicationCleanSnapshot(ctx context.Context, sessionID string) (factoryruntime.CleanInvocationSnapshot, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return factoryruntime.CleanInvocationSnapshot{}, err
	}
	if bound.FactoryRuntime == nil {
		return factoryruntime.CleanInvocationSnapshot{}, factoryruntime.ErrNotRunning
	}
	return bound.FactoryRuntime.CleanInvocationSnapshot(ctx)
}

func (r *Root) ApplicationControlWaitToComplete(sessionID string, request factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil || bound.FactoryRuntime == nil {
		return factoryruntime.WaitToCompleteResult{}
	}
	return bound.FactoryRuntime.ControlWaitToComplete(request)
}

// ApplicationReady returns the selected session's runtime-host readiness.
func (r *Root) ApplicationReady(sessionID string) (<-chan factorysessions.RuntimeHostBinding, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return nil, err
	}
	ready, ok := bound.Process.(interface {
		RuntimeHostReady() <-chan factorysessions.RuntimeHostBinding
	})
	if !ok {
		return nil, nil
	}
	return ready.RuntimeHostReady(), nil
}

// ApplicationDiagnostics reports artifact locations selected for this session.
func (r *Root) ApplicationDiagnostics(sessionID string) (factoryruntime.RuntimeLogDiagnostics, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return factoryruntime.RuntimeLogDiagnostics{}, err
	}
	return bound.Diagnostics, nil
}

// SessionPresentation contains only values that vary with the selected live
// session. Process-owned services are accessed directly from Root.
type SessionPresentation struct {
	FactoryRuntime       factoryruntime.Service
	ModelsScope          models.RuntimeScopeRef
	ModelInvoker         workers.ModelInvoker
	WorkerSessions       workersessions.ObservationService
	Logger               *zap.Logger
	Reader               roles.RuntimeReader
	Projections          recordings.ProjectionService
	Clock                factoryruntime.Clock
	MetricsRootDir       string
	OperatorSettingsPath string
	Recordings           recordings.Service
}

func (r *Root) SessionPresentation(sessionID string) (SessionPresentation, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return SessionPresentation{}, err
	}
	return SessionPresentation{
		FactoryRuntime:       bound.FactoryRuntime,
		ModelsScope:          bound.ModelsScope,
		ModelInvoker:         bound.ModelInvoker,
		WorkerSessions:       bound.WorkerSessionsObservation(),
		Logger:               bound.Logger,
		Reader:               bound.Reader,
		Projections:          bound.Projections,
		Clock:                bound.Clock,
		MetricsRootDir:       bound.Diagnostics.MetricsRootDir,
		OperatorSettingsPath: bound.OperatorSettingsPath,
		Recordings:           bound.Recordings,
	}, nil
}

func (r *Root) ApplicationReplayMetadataWarnings(sessionID string) ([]recordings.MetadataMismatchWarning, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return nil, err
	}
	return append([]recordings.MetadataMismatchWarning(nil), bound.ReplayMetadataWarnings...), nil
}

func (r *Root) ApplicationResumeRecoveryMetadata(sessionID string) (*recordings.ResumeRecoveryMetadata, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return nil, err
	}
	if bound.ResumeRecoveryMetadata == nil {
		return nil, nil
	}
	metadata := *bound.ResumeRecoveryMetadata
	return &metadata, nil
}

func (r *Root) StopApplicationOrderly(ctx context.Context, sessionID string) error {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		if errors.Is(err, factorysessions.ErrSessionNotFound) {
			return nil
		}
		return err
	}
	if bound.OrderlyStop == nil {
		return nil
	}
	return bound.OrderlyStop(ctx)
}

func (r *Root) FactoryDefinitionsService() factorydefinitions.Service { return r.factoryDefinitions }
func (r *Root) WorkService() work.Service                             { return r.workService }
func (r *Root) ModelsService() models.Service                         { return r.modelService }
func (r *Root) WorkersService() workers.Service                       { return r.workerService }
func (r *Root) ProviderSessionsService() providersessions.Service     { return r.providerSessions }
func (r *Root) RecordingsService() recordings.Service                 { return r.recordingsService }
func (r *Root) WorkflowPreviewService() factoryruntime.WorkflowPreviewOperation {
	return r.workflowPreview
}
func (r *Root) WorkerPromptsService() workers.PromptTemplates {
	prompts, _ := r.workerService.(workers.PromptTemplates)
	return prompts
}
