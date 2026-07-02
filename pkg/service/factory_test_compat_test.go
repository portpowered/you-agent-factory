package service

import (
	"context"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

func registerLiveSession(
	svc *FactoryService,
	sessionID string,
	handle *liveRuntimeHandle,
	target FactorySessionTarget,
	selectSession bool,
) {
	svc.RegisterLiveSessionForTest(sessionID, handle, target, selectSession)
}

func requireSessionForTest(svc *FactoryService, sessionID string) (*factorysessions.LiveSession, error) {
	return runtimehost.RequireSessionForTest(svc, sessionID)
}

func requireIdleRuntime(svc *FactoryService, ctx context.Context) error {
	return svc.RequireIdleRuntimeForTest(ctx)
}

func currentRuntimeConfig(svc *FactoryService) *factoryconfig.LoadedFactoryConfig {
	return svc.CurrentRuntimeConfig()
}

func currentFactoryDefinitionVersionAtRoot(
	svc *FactoryService,
	rootDir string,
	name factoryapi.FactoryName,
) (factoryapi.HybridLogicalTimestamp, error) {
	return svc.CurrentFactoryDefinitionVersionAtRootForTest(rootDir, name)
}

func inferenceProgressPublisher(
	svc *FactoryService,
	sessionID string,
	logger *zap.Logger,
) workerprovider.InferenceProgressPublisher {
	return svc.InferenceProgressPublisherForTest(sessionID, logger)
}

func sessionResponseStream(
	svc *FactoryService,
	session *factorysessions.LiveSession,
	dispatchID string,
) *factorysessions.SessionResponseStream {
	return svc.SessionResponseStreamForTest(session, dispatchID)
}

func sessionResponseStreams(
	svc *FactoryService,
	session *factorysessions.LiveSession,
) *factorysessions.SessionResponseStreamSet {
	return svc.SessionResponseStreamsForTest(session)
}

func probeFactorySessionTarget(
	svc *FactoryService,
	folderPath string,
	factoryDir string,
	ref factorysessions.TargetRef,
) (factorysessions.Target, bool, *factorysessions.DiscoveryFailure) {
	return svc.ProbeFactorySessionTargetForTest(folderPath, factoryDir, ref)
}

func sessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	return runtimehost.SessionFactoryPersistRootForTest(serviceRootDir, session)
}

func modelEventDiagnostics(success *interfaces.WorkDiagnostics, err error) *factoryapi.SafeWorkDiagnostics {
	return runtimehost.ModelEventDiagnosticsForTest(success, err)
}

func modelEventErrorClass(err error) string {
	return runtimehost.ModelEventErrorClassForTest(err)
}

func newRecordingModelRunner(
	inner workers.Runner,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	recorder func(factoryapi.FactoryEvent),
	now func() time.Time,
) workers.Runner {
	return runtimehost.NewRecordingModelRunnerForTest(inner, factoryCfg, workerDef, recorder, now)
}

func modelEventContext(request interfaces.RunnerExecutionRequest, eventTime time.Time) factoryapi.FactoryEventContext {
	return runtimehost.ModelEventContextForTest(request, eventTime)
}

func submitCronTick(
	svc *FactoryService,
	ctx context.Context,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	return svc.SubmitCronTickForTest(ctx, ws, firedAt)
}

func isCanceledServiceStartup(ctx context.Context, err error) bool {
	return runtimehost.IsCanceledServiceStartupForTest(ctx, err)
}

func openFactorySessionFromDir(svc *FactoryService, ctx context.Context, factoryDir string) (string, error) {
	return svc.OpenFactorySessionForTest(ctx, factoryDir)
}

func currentLiveRuntime(svc *FactoryService) *liveRuntimeHandle {
	return svc.CurrentLiveRuntimeForTest()
}

func waitForLiveRuntimeStart(svc *FactoryService, ctx context.Context, handle *liveRuntimeHandle) error {
	return svc.WaitForLiveRuntimeStartForTest(ctx, handle)
}

func stopFactorySession(svc *FactoryService, sessionID string) error {
	return svc.StopFactorySessionForTest(sessionID)
}

func currentFactory(svc *FactoryService) factory.Factory {
	return svc.CurrentFactoryForTest()
}

func sessionScopedRecordPath(basePath string, sessionID string) string {
	return runtimehost.SessionScopedRecordPathForTest(basePath, sessionID)
}

func runtimeWorkflowContext(cfg *interfaces.FactoryConfig, sessionID string) *factory_context.FactoryContext {
	return runtimehost.RuntimeWorkflowContextForTest(cfg, sessionID)
}

func liveSessionRuntimeState(session *factorysessions.LiveSession) *runtimehost.LiveSessionState {
	return runtimehost.LiveSessionRuntimeStateForTest(session)
}

func newSessionDispatchCompletionObserverFactory(sessions *factorysessions.Registry) runtimehost.DispatchCompletionObserverFactory {
	return runtimehost.NewSessionDispatchCompletionObserverFactory(sessions)
}

func factorySessionBackendScopeID(svc *FactoryService, session *factorysessions.LiveSession) string {
	return svc.FactorySessionBackendScopeIDForTest(session)
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
