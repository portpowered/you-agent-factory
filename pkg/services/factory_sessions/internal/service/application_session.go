package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	runtime := selectedRuntimeService(bound)
	if runtime == nil {
		return factoryruntime.CleanInvocationSnapshot{}, factoryruntime.ErrNotRunning
	}
	return runtime.CleanInvocationSnapshot(ctx)
}

func (r *Root) ApplicationControlWaitToComplete(sessionID string, request factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return factoryruntime.WaitToCompleteResult{}
	}
	runtime := selectedRuntimeService(bound)
	if runtime == nil {
		return factoryruntime.WaitToCompleteResult{}
	}
	return runtime.ControlWaitToComplete(request)
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

// SessionPresentation exposes the presentation values for one selected live
// session. HTTP peer services are injected directly by process composition.
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
}

func (r *Root) SessionPresentation(sessionID string) (SessionPresentation, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return SessionPresentation{}, err
	}
	return SessionPresentation{
		FactoryRuntime:       selectedRuntimeService(bound),
		ModelsScope:          bound.ModelsScope,
		ModelInvoker:         selectedModelInvocation{facts: selectedModelFacts(bound, sessionID), operation: r.modelInvocation},
		WorkerSessions:       bound.WorkerSessionsObservation(),
		Logger:               bound.Logger,
		Reader:               r.factorySessionsRuntimeAssembly,
		Projections:          r.recordingProjections,
		Clock:                bound.Clock,
		MetricsRootDir:       bound.Diagnostics.MetricsRootDir,
		OperatorSettingsPath: bound.OperatorSettingsPath,
	}, nil
}

// selectedModelInvocation adapts one live generation to the Models HTTP
// invocation boundary. Captured facts keep a retained presentation scoped even
// when the current session or its runtime generation changes.
type selectedModelInvocation struct {
	facts     modelinvocation.RuntimeModelInvocation
	operation modelinvocation.RuntimeModelInvocationOperation
}

func (s selectedModelInvocation) InvokeModel(ctx context.Context, name string, request models.Request) (models.Result, error) {
	return s.operation.InvokeRuntimeModel(ctx, s.facts, name, request)
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

// CurrentBoardRecordingArtifact addresses the configured host board for exactly
// one scope. It supplies only a legacy candidate; Recordings validates its facts.
func (r *Root) CurrentBoardRecordingArtifact(ctx context.Context, scope string) (recordings.RecordingArtifactReference, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	bound, err := r.applicationSessionState(factorysessions.DefaultSessionID)
	if err != nil {
		return "", &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorMissingHistory, Cause: err}
	}
	if strings.TrimSpace(scope) == "" {
		return "", recordings.ErrInvalidProjectionScope
	}
	return recordings.RecordingArtifactReference(factoryruntime.RecordingPath(bound.CurrentBoardRecordPath).ForSession(scope)), nil
}

// ApplicationSkippedBoardRecordings returns paths only, never decoder causes.
func (r *Root) ApplicationSkippedBoardRecordings(sessionID string) ([]string, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), bound.SkippedBoardRecordings...), nil
}

// selectedRuntimeService reads the acquired generation rather than a copied
// service peer inherited by replacement registration.
func selectedRuntimeService(bound *runtimebinding.SessionState) factoryruntime.Service {
	if bound.Handle != nil {
		return bound.Handle.RuntimeInstance().RuntimeService()
	}
	if bound.Instance == nil {
		return nil
	}
	return bound.Instance.RuntimeService()
}
