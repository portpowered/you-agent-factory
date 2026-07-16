// backendsizecheck:ignore-file consolidated session runtime reads remain with runtime_sessions until dedicated service read seams split.
// pkgmaintcheck:ignore-file-lines consolidated session runtime reads remain with runtime_sessions until dedicated service read seams split.
package runtimehost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/metrics"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/factory/sessions/cursors"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factory/sessions/service"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	sessioncursor "github.com/portpowered/infinite-you/pkg/platform/cursors/session"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

const (
	DefaultFactorySessionID         = factorysessions.DefaultSessionID
	FactorySessionTargetKindDefault = factorysessions.TargetKindDefault
	FactorySessionTargetKindNamed   = factorysessions.TargetKindNamed
)

type (
	FactorySessionTargetKind          = factorysessions.TargetKind
	FactorySessionTargetRef           = factorysessions.TargetRef
	FactorySessionTarget              = factorysessions.Target
	FactorySessionOpenResult          = factorysessions.OpenResult
	liveFactorySession                = factorysessions.LiveSession
	inferenceProgressPublisherFactory func(sessionID string) workerprovider.InferenceProgressPublisher
	dispatchCompletionObserverFactory func(sessionID string) func(dispatchID string)
)

// FactoryCoordinator owns session tracking and runtime lifecycle orchestration.
type FactoryCoordinator interface {
	apisurface.SessionAPI
	ActivateNamedFactory(context.Context, string) error
	ListFactorySessions(context.Context) (factoryapi.ListFactorySessionsResponse, error)
	GetFactorySession(context.Context, string) (factoryapi.FactorySession, error)
	GetFactorySessionSyncPreflight(context.Context, string, interfaces.FactorySessionSyncPreflightOptions) (factoryapi.FactorySessionSyncPreflightResponse, error)
	GetFactorySessionResult(context.Context, string) (factoryapi.FactorySessionLiveResult, error)
	GetFactorySessionPartialResult(context.Context, string) (factoryapi.FactorySessionPartialResult, error)
	OpenFactorySession(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	CloseFactorySession(context.Context, string) error
	OpenFactorySessionFromFolder(context.Context, string, *FactorySessionTargetRef, bool, bool) (*FactorySessionOpenResult, error)
	SubmitWorkRequestForSession(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWorkForSession(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
	SubscribeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	GetEngineStateSnapshotForSession(context.Context, string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
	GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error)
	StartDefaultRuntime(context.Context, context.Context, bool) (*liveRuntimeHandle, error)
	StartBackgroundSessionWithMetadata(context.Context, string, *factoryRuntimeBundle, FactorySessionTarget) error
	StartLiveRuntimeSidecars(context.Context, *liveRuntimeHandle) error
	StopLiveRuntimeSidecars(*liveRuntimeHandle)
	StopLiveRuntime(*liveRuntimeHandle) error
	ShutdownOtherLiveSessions(*liveRuntimeHandle) error
	ReplaceSessionRuntime(context.Context, *factorysessions.LiveSession, string, *factoryRuntimeBundle) error
}

type runtimeCoordinator struct {
	host *Host
}

var _ apisurface.SessionAPI = (*runtimeCoordinator)(nil)

// SessionAPI returns the bounded canonical session collaborator used by the
// composed HTTP surface and the Host compatibility facade.
func (fs *Host) SessionAPI() apisurface.SessionAPI {
	if fs == nil {
		return nil
	}
	return fs.requireCoordinator()
}

func liveSessionHandle(session *factorysessions.LiveSession) *liveRuntimeHandle {
	if session == nil {
		return nil
	}
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	return state.handle
}

func liveSessionRuntimeState(session *factorysessions.LiveSession) *liveSessionState {
	if session == nil {
		return nil
	}
	state, _ := session.Handle.(*liveSessionState)
	return state
}

func liveSessionBundle(session *factorysessions.LiveSession) *factoryRuntimeBundle {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	if state.handle != nil {
		return state.handle.Bundle
	}
	return state.bundle
}

func liveSessionBuildSpec(session *factorysessions.LiveSession) *runtimebuild.SessionBuildSpec {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	return state.spec
}

type liveSessionRegistration struct {
	factoryDir       string
	folderPath       string
	executionBaseDir string
	targetRef        FactorySessionTargetRef
	project          string
	preparedSpec     *runtimebuild.SessionBuildSpec
}

func (fs *Host) buildLiveSessionRegistration(
	sessionID string,
	handle *liveRuntimeHandle,
	target FactorySessionTarget,
) liveSessionRegistration {
	registration := liveSessionRegistration{
		factoryDir: strings.TrimSpace(target.FactoryDir),
		folderPath: strings.TrimSpace(target.FolderPath),
		targetRef:  target.Ref,
		project:    strings.TrimSpace(target.Project),
	}
	if registration.factoryDir == "" && handle.Bundle != nil {
		registration.factoryDir = handle.Bundle.Dir
	}
	if registration.folderPath == "" {
		registration.folderPath = fs.factoryRootDir
	}
	if registration.folderPath == "" {
		registration.folderPath = registration.factoryDir
	}
	if registration.targetRef.Kind == "" {
		registration.targetRef = FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault}
	}
	if registration.project == "" {
		registration.project = filepath.Base(registration.folderPath)
	}
	registration.executionBaseDir = liveSessionExecutionBaseDir(handle, registration.folderPath, registration.factoryDir)
	if existing := liveSessionRuntimeState(fs.sessionByID(sessionID)); existing != nil {
		registration.preparedSpec = existing.spec
	}
	return registration
}

func liveSessionExecutionBaseDir(handle *liveRuntimeHandle, folderPath string, factoryDir string) string {
	executionBaseDir := ""
	if handle != nil && handle.Bundle != nil && handle.Bundle.RuntimeCfg != nil {
		executionBaseDir = strings.TrimSpace(handle.Bundle.RuntimeCfg.RuntimeBaseDir())
	}
	if executionBaseDir == "" {
		executionBaseDir = folderPath
	}
	if executionBaseDir == "" {
		executionBaseDir = factoryDir
	}
	return executionBaseDir
}

func newCoordinator(fs *Host) FactoryCoordinator {
	return &runtimeCoordinator{host: fs}
}

func (fs *Host) requireCoordinator() FactoryCoordinator {
	if fs == nil {
		return newCoordinator(nil)
	}
	if fs.coordinator == nil {
		fs.coordinator = newCoordinator(fs)
	}
	return fs.coordinator
}

func (fs *Host) registerLiveSession(
	sessionID string,
	handle *liveRuntimeHandle,
	target FactorySessionTarget,
	selectSession bool,
) string {
	if fs == nil || fs.sessions == nil || sessionID == "" || handle == nil {
		return ""
	}
	isDefault := factorysessions.IsDefaultSessionSelector(sessionID)
	if isDefault {
		if existing := fs.defaultSession(); existing != nil {
			sessionID = existing.ID
		} else {
			sessionID = factorysessions.NewSessionID()
		}
	}
	registration := fs.buildLiveSessionRegistration(sessionID, handle, target)
	session := factorysessions.NewLiveSession(
		sessionID,
		registration.factoryDir,
		registration.folderPath,
		registration.executionBaseDir,
		registration.targetRef,
		&liveSessionState{bundle: handle.Bundle, handle: handle, spec: registration.preparedSpec},
		isDefault,
		registration.project,
	)
	factorysessions.BindResponseEventCompletion(session, handle.Bundle.EventHistory.AddEventTypeRecorder)
	fs.sessions.Upsert(session, selectSession)
	return sessionID
}

func defaultSessionTargetFromRuntimeBundle(
	runtimeBundle *factoryRuntimeBundle,
	factoryRootDir string,
) FactorySessionTarget {
	target := FactorySessionTarget{
		Ref: FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
	}
	if runtimeBundle != nil {
		target.FactoryDir = runtimeBundle.Dir
		target.FolderPath = runtimeBundle.FolderPath
	}
	if strings.TrimSpace(target.FolderPath) == "" {
		target.FolderPath = factoryRootDir
	}
	if strings.TrimSpace(target.Project) == "" && target.FolderPath != "" {
		target.Project = filepath.Base(target.FolderPath)
	}
	return target
}

func (fs *Host) unregisterLiveSession(sessionID string) {
	if fs == nil || fs.sessions == nil {
		return
	}
	if factorysessions.IsDefaultSessionSelector(sessionID) {
		if session := fs.defaultSession(); session != nil {
			sessionID = session.ID
		}
	}
	fs.closeSessionResponseStreams(fs.sessionByID(sessionID))
	fs.sessions.Remove(sessionID)
}

func (fs *Host) currentSession() *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.Current()
}

func (fs *Host) defaultSession() *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	return fs.sessions.DefaultSession()
}

func (fs *Host) sessionByID(sessionID string) *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return nil
	}
	if session := fs.sessions.Get(trimmed); session != nil {
		return session
	}
	for _, id := range fs.sessions.IDs() {
		session := fs.sessions.Get(id)
		if session != nil && factorysessions.CanonicalFactorySessionID(session) == trimmed {
			return session
		}
	}
	return nil
}

func (fs *Host) resolveLiveSessionSelector(sessionID string) (*factorysessions.LiveSession, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	if factorysessions.IsDefaultSessionSelector(sessionID) {
		session := fs.defaultSession()
		if session == nil {
			selector := strings.TrimSpace(sessionID)
			if selector == "" {
				selector = DefaultFactorySessionID
			}
			return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, selector)
		}
		handle := liveSessionHandle(session)
		if handle == nil || handle.Bundle == nil {
			selector := strings.TrimSpace(sessionID)
			if selector == "" {
				selector = DefaultFactorySessionID
			}
			return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, selector)
		}
		return session, nil
	}
	session := fs.sessionByID(sessionID)
	handle := liveSessionHandle(session)
	if session == nil || handle == nil || handle.Bundle == nil {
		return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	return session, nil
}

func (fs *Host) requireSession(sessionID string) (*factorysessions.LiveSession, error) {
	return fs.resolveLiveSessionSelector(sessionID)
}

func (fs *Host) sessionFactory(sessionID string) (factory.Factory, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return liveSessionHandle(session).Bundle.Factory, nil
}

func (fs *Host) sessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return liveSessionHandle(session).Bundle.RuntimeCfg, nil
}

func (fs *Host) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return fs.requireCoordinator().SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (c *runtimeCoordinator) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if c == nil || c.host == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("factory service is required")
	}
	c.host.activationMu.RLock()
	defer c.host.activationMu.RUnlock()
	return factoryservice.SubmitWorkRequest(ctx, c.host.currentRuntimeBundle(), request)
}

func (c *runtimeCoordinator) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	if c == nil || c.host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return factoryservice.SubscribeFactoryEvents(ctx, c.host.currentRuntimeBundle(), reconnect, scope)
}

func (c *runtimeCoordinator) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	if c == nil || c.host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return factoryservice.GetEngineStateSnapshot(ctx, c.host.currentRuntimeBundle())
}

func (c *runtimeCoordinator) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	if c == nil || c.host == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	return c.host.requireDefinitions().GetCurrentNamedFactory(ctx)
}

func (c *runtimeCoordinator) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	fs := c.host
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	return factoryservice.SubmitWorkRequest(ctx, liveSessionHandle(session).Bundle, request)
}

func (fs *Host) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	return fs.requireCoordinator().MoveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}

func (c *runtimeCoordinator) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	fs := c.host
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	return factoryservice.MoveWork(ctx, liveSessionHandle(session).Bundle, workID, stateName, work.WorkStateChangeSourceAPI, requestID)
}

// MoveWork applies a synchronous operator relocation on the current service-owned runtime.
func (fs *Host) MoveWork(ctx context.Context, workID, stateName string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	return factoryservice.MoveWork(ctx, fs.currentRuntimeBundle(), workID, stateName, source, requestID)
}

func (fs *Host) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	return fs.requireCoordinator().SubscribeFactoryEventsForSession(ctx, sessionID, reconnect)
}

func (fs *Host) SubscribeFactoryResponseEventsForSession(ctx context.Context, sessionID string, afterSequence int64, dispatchID string) (apisurface.FactoryResponseEventSubscription, error) {
	return fs.requireCoordinator().SubscribeFactoryResponseEventsForSession(ctx, sessionID, afterSequence, dispatchID)
}

func (c *runtimeCoordinator) SubscribeFactoryResponseEventsForSession(ctx context.Context, sessionID string, afterSequence int64, dispatchID string) (apisurface.FactoryResponseEventSubscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session := c.host.responseEventSession(sessionID)
	if session == nil {
		return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	if session.ResponseEvents == nil {
		return nil, fmt.Errorf("response event store unavailable for factory session %q", sessionID)
	}
	options := []responseeventstore.SubscribeOption{}
	if strings.TrimSpace(dispatchID) != "" {
		options = append(options, responseeventstore.WithDispatchFilter(dispatchID))
	}
	subscription, err := session.ResponseEvents.Subscribe(afterSequence, options...)
	if err != nil {
		if errors.Is(err, responseeventstore.ErrStoreExpired) {
			return nil, fmt.Errorf("%w: %s", apisurface.ErrFactoryResponseEventStreamExpired, sessionID)
		}
		return nil, err
	}
	return &factoryResponseEventSubscription{subscription: subscription}, nil
}

type factoryResponseEventSubscription struct {
	subscription *responseeventstore.Subscription
}

func (s *factoryResponseEventSubscription) Next(ctx context.Context) ([]apisurface.FactoryResponseEventRecord, error) {
	events, err := s.subscription.Next(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]apisurface.FactoryResponseEventRecord, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("serialize factory response event: %w", err)
		}
		records = append(records, apisurface.FactoryResponseEventRecord{Sequence: event.Sequence, Kind: string(event.Kind), Data: data})
	}
	return records, nil
}

func (s *factoryResponseEventSubscription) Detach() {
	if s != nil && s.subscription != nil {
		s.subscription.Detach()
	}
}

func (fs *Host) responseEventSession(sessionID string) *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	requestedID := strings.TrimSpace(sessionID)
	if requestedID == "" {
		return nil
	}
	if requestedID == factorysessions.DefaultSessionID {
		return fs.sessions.DefaultSession()
	}
	if session := fs.sessions.Get(requestedID); session != nil {
		return session
	}
	for _, registeredID := range fs.sessions.IDs() {
		session := fs.sessions.Get(registeredID)
		if factorysessions.CanonicalFactorySessionID(session) == requestedID {
			return session
		}
	}
	return nil
}

func (c *runtimeCoordinator) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	fs := c.host
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	stream, err := factoryservice.SubscribeFactoryEventsForSession(ctx, liveSessionHandle(session).Bundle, sessionID, reconnect)
	if err != nil || stream == nil || session == nil {
		return stream, err
	}
	stream.FactorySessionID = strings.TrimSpace(session.ID)
	stream.LogicalSessionKeyID = factorysessions.LogicalSessionKeyID(session)
	return stream, err
}

func (fs *Host) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return fs.requireCoordinator().GetEngineStateSnapshotForSession(ctx, sessionID)
}

func (c *runtimeCoordinator) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	fs := c.host
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return nil, err
	}
	return factoryservice.GetEngineStateSnapshot(ctx, liveSessionHandle(session).Bundle)
}

func (fs *Host) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return fs.FactoryDefinitionAPI().GetCurrentFactoryForSession(ctx, sessionID)
}

func (c *runtimeCoordinator) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return c.host.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
}

func (fs *Host) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	result, err := fs.requireSessionGateway().OpenFactorySession(ctx, factorysession.OpenRequestFromAPI(request))
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	return fs.openFactorySessionResponse(result)
}

func (c *runtimeCoordinator) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	if c.host == nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("factory service is required")
	}
	return c.host.OpenFactorySession(ctx, request)
}

func (fs *Host) openFactorySessionResponse(result *FactorySessionOpenResult) (factoryapi.OpenFactorySessionResponse, error) {
	if result == nil || result.SessionID == "" {
		return factorysession.OpenResultToAPI(result, nil), nil
	}
	session, err := fs.requireSession(result.SessionID)
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	return factorysession.OpenResultToAPI(result, session), nil
}

func (fs *Host) CloseFactorySession(ctx context.Context, sessionID string) error {
	return fs.requireSessionGateway().CloseFactorySession(ctx, sessionID)
}

func (c *runtimeCoordinator) CloseFactorySession(ctx context.Context, sessionID string) error {
	if c.host == nil {
		return fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().CloseFactorySession(ctx, sessionID)
}

func (fs *Host) openFactorySession(ctx context.Context, factoryDir string) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("factory service is required")
	}
	sessionID := factorysessions.NewSessionID()
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, factoryDir, factoryDir, sessionID)
	if err != nil {
		return "", err
	}
	if err := fs.startBackgroundSession(ctx, sessionID, replacement); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (fs *Host) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *FactorySessionTargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*FactorySessionOpenResult, error) {
	return fs.requireSessionGateway().OpenFactorySessionFromFolder(ctx, folderPath, target, validateOnly, initNewFactory)
}

func (c *runtimeCoordinator) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *FactorySessionTargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*FactorySessionOpenResult, error) {
	if c.host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return c.host.requireSessionGateway().OpenFactorySessionFromFolder(ctx, folderPath, target, validateOnly, initNewFactory)
}

func (fs *Host) openFactorySessionForTarget(ctx context.Context, target FactorySessionTarget) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("factory service is required")
	}
	sessionID := factorysessions.NewSessionID()
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, target.FolderPath, target.FactoryDir, sessionID)
	if err != nil {
		return "", err
	}
	if err := fs.StartBackgroundSessionWithMetadata(ctx, sessionID, replacement, target); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (fs *Host) startBackgroundSession(ctx context.Context, sessionID string, runtimeBundle *factoryRuntimeBundle) error {
	return fs.StartBackgroundSessionWithMetadata(ctx, sessionID, runtimeBundle, FactorySessionTarget{
		Ref: FactorySessionTargetRef{
			Kind: FactorySessionTargetKindDefault,
		},
		FactoryDir: runtimeBundle.Dir,
		FolderPath: runtimeBundle.Dir,
		Project:    filepath.Base(runtimeBundle.Dir),
	})
}

//nolint:contextcheck // The request context bounds startup waiting, while the active service runtime context owns the long-lived session runtime and sidecars.
func (fs *Host) StartBackgroundSessionWithMetadata(
	ctx context.Context,
	sessionID string,
	runtimeBundle *factoryRuntimeBundle,
	target FactorySessionTarget,
) error {
	return fs.requireCoordinator().StartBackgroundSessionWithMetadata(ctx, sessionID, runtimeBundle, target)
}

func (c *runtimeCoordinator) StartBackgroundSessionWithMetadata(
	ctx context.Context,
	sessionID string,
	runtimeBundle *factoryRuntimeBundle,
	target FactorySessionTarget,
) error {
	fs := c.host
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	if runtimeBundle == nil {
		return fmt.Errorf("runtime bundle is required")
	}
	serviceCtx := ctx
	if runState := fs.currentRunState(); runState != nil && runState.ctx != nil {
		serviceCtx = runState.ctx
	}
	handle := fs.startLiveRuntime(serviceCtx, runtimeBundle)
	if err := fs.waitForLiveRuntimeStart(ctx, handle); err != nil {
		_ = fs.StopLiveRuntime(handle)
		return fmt.Errorf("start runtime session: %w", err)
	}
	if runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) == interfaces.RuntimeModeService {
		if err := fs.StartLiveRuntimeSidecars(serviceCtx, handle); err != nil {
			_ = fs.StopLiveRuntime(handle)
			return fmt.Errorf("start runtime session sidecars: %w", err)
		}
	}
	fs.registerLiveSession(sessionID, handle, target, false)
	return nil
}

func (fs *Host) stopFactorySession(sessionID string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	session, err := fs.resolveLiveSessionSelector(sessionID)
	if err != nil {
		return err
	}
	handle := liveSessionHandle(session)
	if handle == nil {
		return fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	sessionID = session.ID

	runState := fs.currentRunState()
	if runState != nil && runState.sessionID == sessionID {
		successor := fs.nextLiveSessionAfterStop(sessionID)
		if successor != nil {
			fs.setRunState(runState.ctx, successor.ID, liveSessionHandle(successor))
		} else {
			fs.clearRunState()
		}
	}

	fs.unregisterLiveSession(sessionID)
	if err := fs.StopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (fs *Host) runSessionID() string {
	if fs == nil {
		return DefaultFactorySessionID
	}
	if runState := fs.currentRunState(); runState != nil && strings.TrimSpace(runState.sessionID) != "" {
		return runState.sessionID
	}
	if session := fs.defaultSession(); session != nil {
		return session.ID
	}
	return DefaultFactorySessionID
}

func (fs *Host) requireIdleRuntimeForSession(
	ctx context.Context,
	sessionID string,
) error {
	snapshot, err := fs.GetEngineStateSnapshotForSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("read session runtime status: %w", err)
	}
	if snapshot.RuntimeStatus != interfaces.RuntimeStatusIdle {
		return fmt.Errorf("%w: current runtime status is %s", ErrFactoryActivationRequiresIdle, snapshot.RuntimeStatus)
	}
	if snapshotHasActiveWork(snapshot) {
		return fmt.Errorf("%w: current runtime has active work", ErrFactoryActivationRequiresIdle)
	}
	return nil
}

func sessionServiceContext(ctx context.Context, runState *hostRunState) context.Context {
	if runState != nil && runState.ctx != nil {
		return runState.ctx
	}
	return ctx
}

func (fs *Host) startReplacementSessionRuntime(
	ctx context.Context,
	serviceCtx context.Context,
	replacement *factoryRuntimeBundle,
) (*liveRuntimeHandle, error) {
	serviceMode := runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) == interfaces.RuntimeModeService
	return factoryservice.StartReplacement(factoryservice.StartReplacementInput{
		ReadinessCtx:                ctx,
		ServiceCtx:                  serviceCtx,
		Bundle:                      replacement,
		Clock:                       fs.clock,
		AttachSidecars:              fs.StartLiveRuntimeSidecars,
		AttachSidecarsInServiceMode: serviceMode,
	})
}

//nolint:contextcheck // The request context bounds the save/startup wait, while the long-lived service runtime context owns the replacement session runtime and sidecars after the request returns.
func (fs *Host) ReplaceSessionRuntime(
	ctx context.Context,
	session *factorysessions.LiveSession,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	return fs.requireCoordinator().ReplaceSessionRuntime(ctx, session, name, replacement)
}

func (c *runtimeCoordinator) ReplaceSessionRuntime(
	ctx context.Context,
	session *factorysessions.LiveSession,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	fs := c.host
	if session == nil {
		return fmt.Errorf("%w: session handle is unavailable", apisurface.ErrFactorySessionNotFound)
	}
	handle := liveSessionHandle(session)
	if handle == nil {
		return fmt.Errorf("%w: session handle is unavailable", apisurface.ErrFactorySessionNotFound)
	}
	previousScope, previousScopeErr := fs.sessionPersistenceScopeFromSession(ctx, session)
	runState := fs.currentRunState()
	serviceCtx := sessionServiceContext(ctx, runState)
	isActiveSession := runState != nil && runState.sessionID == session.ID

	serviceMode := runtimeModeOrDefault(fs.coordinatorPolicy().runtimeMode) == interfaces.RuntimeModeService
	attempt := &factoryservice.ReplacementAttempt{
		Current:         handle,
		ServiceCtx:      serviceCtx,
		ServiceMode:     serviceMode,
		RestoreSidecars: fs.StartLiveRuntimeSidecars,
	}
	attempt.Begin()
	defer attempt.End()

	replacementHandle, err := fs.startReplacementSessionRuntime(ctx, serviceCtx, replacement)
	if err != nil {
		return err
	}

	fs.publishFactoryChangeEvent(ctx, handle, replacement)
	attempt.Commit()
	executionBaseDir := strings.TrimSpace(session.ExecutionBaseDir)
	if replacement.RuntimeCfg != nil {
		if runtimeBaseDir := strings.TrimSpace(replacement.RuntimeCfg.RuntimeBaseDir()); runtimeBaseDir != "" {
			executionBaseDir = runtimeBaseDir
		}
	}
	fs.closeSessionResponseStreams(session)
	replacementSession := factorysessions.NewLiveSession(
		session.ID,
		replacement.Dir,
		session.FolderPath,
		executionBaseDir,
		session.Target,
		&liveSessionState{handle: replacementHandle, spec: liveSessionBuildSpec(session)},
		session.IsDefault,
		session.Project,
	)
	replacementSession.RuntimeFactorySessionID = session.RuntimeFactorySessionID
	replacementSession.ResponseEvents = factorysessions.NewSessionResponseEventStore(
		factorysessions.CanonicalFactorySessionID(replacementSession),
	)
	factorysessions.BindResponseEventCompletion(replacementSession, replacement.EventHistory.AddEventTypeRecorder)
	fs.sessions.Upsert(replacementSession, isActiveSession)
	if isActiveSession {
		fs.setRunState(serviceCtx, session.ID, replacementHandle)
	}
	if err := fs.StopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
		fs.logger.Warn("prior session runtime shutdown failed", zap.Error(err), zap.String("session_id", session.ID))
	}
	if previousScopeErr == nil {
		if updated := fs.sessionByID(session.ID); updated != nil {
			if currentScope, err := fs.sessionPersistenceScopeFromSession(ctx, updated); err == nil {
				if diagnostic, ok := factorysessioncursors.IdentityMismatchDiagnostic(
					previousScope,
					currentScope,
					session.ID,
				); ok {
					fs.recordSessionPersistenceInvalidation(diagnostic)
				}
			}
		}
	}
	return nil
}

func (fs *Host) nextLiveSessionAfterStop(sessionID string) *factorysessions.LiveSession {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	for _, id := range fs.sessions.IDs() {
		if id == sessionID {
			continue
		}
		next := fs.sessionByID(id)
		if next != nil && liveSessionHandle(next) != nil {
			return next
		}
	}
	return nil
}

func (fs *Host) discoverFactorySessionTargets(folderPath string) ([]FactorySessionTarget, error) {
	return factorysessions.DiscoverTargets(folderPath, fs.probeFactorySessionTarget)
}

func (fs *Host) probeFactorySessionTarget(
	folderPath string,
	factoryDir string,
	ref factorysessions.TargetRef,
) (factorysessions.Target, bool, *factorysessions.DiscoveryFailure) {
	if fs == nil {
		return factorysessions.Target{}, false, nil
	}
	loaded, err := configload.LoadRuntimeConfigFromFactoryDir(factoryDir, fs.coordinatorPolicy().workstationLoader)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && !configload.IsFactoryLayoutNotFound(err) && !configload.IsNamedFactoryNotFound(err) {
			fs.logFactorySessionTargetProbeFailure(folderPath, factoryDir, ref, err)
			return factorysessions.Target{}, false, &factorysessions.DiscoveryFailure{
				FactoryDir: factoryDir,
				Ref:        ref,
				Summary:    err.Error(),
			}
		}
		return factorysessions.Target{}, false, nil
	}

	project := ""
	if cfg := loaded.FactoryConfig(); cfg != nil {
		project = strings.TrimSpace(cfg.Project)
		if project == "" {
			project = strings.TrimSpace(cfg.Name)
		}
	}
	return factorysessions.BuildTargetFromConfig(folderPath, factoryDir, ref, project), true, nil
}

func (fs *Host) logFactorySessionTargetProbeFailure(
	folderPath string,
	factoryDir string,
	ref factorysessions.TargetRef,
	err error,
) {
	if fs == nil || err == nil {
		return
	}
	logger := fs.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	fields := []zap.Field{
		zap.String("submitted_folder_path", folderPath),
		zap.String("target_factory_dir", factoryDir),
		zap.String("target_kind", string(ref.Kind)),
		zap.String("target_display_name", factorysessions.TargetDisplayName(ref)),
		zap.String("failure_summary", err.Error()),
		zap.Error(err),
	}
	if ref.Kind == factorysessions.TargetKindNamed && strings.TrimSpace(ref.Name) != "" {
		fields = append(fields, zap.String("target_name", strings.TrimSpace(ref.Name)))
	}
	logger.Error("factory session discovery target runtime config load failed", fields...)
}

func (fs *Host) waitForServiceModeStartupWorkReadability(ctx context.Context, serviceMode bool) error {
	policy := fs.coordinatorPolicy()
	if !serviceMode || policy.workFile == "" || policy.apiServerReady == nil || policy.port <= 0 || policy.apiServerStarter == nil {
		return nil
	}
	apiServerExit := fs.apiServerExit
	select {
	case <-policy.apiServerReady:
	case err := <-apiServerExit:
		return startupReadinessError(err)
	case <-ctx.Done():
		return ctx.Err()
	}

	timer := time.NewTimer(hostModeStartupWorkReadabilityDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case err := <-apiServerExit:
		return startupReadinessError(err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (fs *Host) failServiceModeStartup(currentRuntime *liveRuntimeHandle, startupErr error) error {
	fs.clearRunState()
	fs.unregisterLiveSession(DefaultFactorySessionID)
	if currentRuntime == nil {
		return startupErr
	}
	if stopErr := fs.StopLiveRuntime(currentRuntime); stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		return errors.Join(startupErr, stopErr)
	}
	return startupErr
}

func startupReadinessError(err error) error {
	if err == nil {
		return fmt.Errorf("wait for service-mode startup work readiness: API server stopped before signaling readiness")
	}
	return fmt.Errorf("wait for service-mode startup work readiness: %w", err)
}

func (fs *Host) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	reads, err := fs.requireSessionGateway().ListFactorySessions(ctx)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return factorysession.ReadProjectionsToAPI(reads), nil
}

func (c *runtimeCoordinator) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	if c.host == nil {
		return factoryapi.ListFactorySessionsResponse{}, fmt.Errorf("factory service is required")
	}
	return c.host.ListFactorySessions(ctx)
}

func (fs *Host) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	projection, err := fs.requireSessionGateway().GetFactorySession(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	return factorysession.SessionResponseToAPI(projection), nil
}

func (c *runtimeCoordinator) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	if c.host == nil {
		return factoryapi.FactorySession{}, fmt.Errorf("factory service is required")
	}
	return c.host.GetFactorySession(ctx, sessionID)
}

func (fs *Host) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	options interfaces.FactorySessionSyncPreflightOptions,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	logicalResolve := logicalResolveHintFromSyncPreflightOptions(options)
	result, err := fs.requireSessionGateway().GetFactorySessionSyncPreflight(ctx, sessionID, options.Reconnect, logicalResolve)
	if err != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}
	response := factorysession.SyncPreflightResultToAPI(result)
	fs.recordSessionPersistenceInvalidationFromPreflight(response)
	return response, nil
}

func (c *runtimeCoordinator) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	options interfaces.FactorySessionSyncPreflightOptions,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	if c.host == nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, fmt.Errorf("factory service is required")
	}
	return c.host.GetFactorySessionSyncPreflight(ctx, sessionID, options)
}

func logicalResolveHintFromSyncPreflightOptions(
	options interfaces.FactorySessionSyncPreflightOptions,
) *interfaces.FactorySessionLogicalResolveHint {
	backendScopeID := ""
	if options.BackendScopeID != nil {
		backendScopeID = strings.TrimSpace(*options.BackendScopeID)
	}
	logicalSessionKeyID := ""
	if options.LogicalSessionKeyID != nil {
		logicalSessionKeyID = strings.TrimSpace(*options.LogicalSessionKeyID)
	}
	if backendScopeID == "" && logicalSessionKeyID == "" {
		return nil
	}
	return &interfaces.FactorySessionLogicalResolveHint{
		BackendScopeID:      backendScopeID,
		LogicalSessionKeyID: logicalSessionKeyID,
	}
}

type sessionSyncPreflightTarget struct {
	session    *factorysessions.LiveSession
	remapped   bool
	unresolved bool
}

func (fs *Host) resolveSessionSyncPreflightTarget(
	sessionID string,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
) (sessionSyncPreflightTarget, error) {
	if fs == nil {
		return sessionSyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	if session, err := fs.requireSession(sessionID); err == nil {
		return sessionSyncPreflightTarget{session: session}, nil
	} else if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		return sessionSyncPreflightTarget{}, err
	}

	if strings.TrimSpace(sessionID) == DefaultFactorySessionID {
		if session := fs.preflightDefaultSessionSuccessor(); session != nil {
			return sessionSyncPreflightTarget{session: session, remapped: true}, nil
		}
	}

	if hasLogicalResolveHint(logicalResolve) {
		return fs.resolveSessionSyncPreflightByLogicalKey(sessionID, logicalResolve)
	}

	return sessionSyncPreflightTarget{}, nil
}

func hasLogicalResolveHint(hint *interfaces.FactorySessionLogicalResolveHint) bool {
	if hint == nil {
		return false
	}
	return strings.TrimSpace(hint.BackendScopeID) != "" &&
		strings.TrimSpace(hint.LogicalSessionKeyID) != ""
}

func (fs *Host) resolveSessionSyncPreflightByLogicalKey(
	requestedSessionID string,
	hint *interfaces.FactorySessionLogicalResolveHint,
) (sessionSyncPreflightTarget, error) {
	serviceScope := factorySessionBackendScopeID(fs, nil)
	if serviceScope == "" || strings.TrimSpace(hint.BackendScopeID) != serviceScope {
		return sessionSyncPreflightTarget{unresolved: true}, nil
	}
	session := fs.sessions.FindByLogicalSessionKeyID(hint.LogicalSessionKeyID)
	if session == nil {
		return sessionSyncPreflightTarget{unresolved: true}, nil
	}
	remapped := strings.TrimSpace(requestedSessionID) != "" &&
		session.ID != strings.TrimSpace(requestedSessionID)
	return sessionSyncPreflightTarget{session: session, remapped: remapped}, nil
}

func (fs *Host) preflightDefaultSessionSuccessor() *factorysessions.LiveSession {
	if fs == nil {
		return nil
	}
	if runState := fs.currentRunState(); runState != nil {
		successorID := strings.TrimSpace(runState.sessionID)
		if successorID != "" && successorID != DefaultFactorySessionID {
			if session, err := fs.requireSession(successorID); err == nil {
				return session
			}
		}
	}
	current := fs.currentSession()
	if current == nil || current.ID == DefaultFactorySessionID {
		return nil
	}
	if session, err := fs.requireSession(current.ID); err == nil {
		return session
	}
	return nil
}

func (fs *Host) buildSessionProjectionContext(
	ctx context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if session == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("%w", apisurface.ErrFactorySessionNotFound)
	}
	runtimeCfg, err := fs.sessionRuntimeConfig(session.ID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	factoryCfg := runtimeCfg.FactoryConfig()
	projectionCtx := factorysessions.ProjectionContext{
		Session:          session,
		FactoryCfg:       factoryCfg,
		BackendScopeID:   strings.TrimSpace(liveSessionBundle(session).BackendScopeID),
		RuntimeStartedAt: liveSessionBundle(session).StartedAtUTC,
		Now:              time.Now().UTC(),
	}
	snapshot, err := fs.GetEngineStateSnapshotForSession(ctx, session.ID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	projectionCtx.Snapshot = snapshot
	projectionCtx.LifecycleControlStatus = snapshot.LifecycleControlStatus
	checkpointStore := (*factorysessions.JavaScriptCheckpointStore)(nil)
	if interfaces.IsJavaScriptOrchestratorFactory(factoryCfg) {
		checkpointStore = fs.requireSessionGateway().JavaScriptCheckpointStore(session)
		projectionCtx.JavaScriptCheckpoints = checkpointStore.List()
	}
	projectionCtx.JavaScript, err = fs.projectJavaScriptRuntimeState(session, checkpointStore, snapshot.TickCount)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	projectionCtx.Enabled = factorysessions.EnabledTransitionsForSnapshot(ctx, snapshot, runtimeCfg)
	return projectionCtx, nil
}

func (fs *Host) projectJavaScriptRuntimeState(
	session *factorysessions.LiveSession,
	checkpointStore *factorysessions.JavaScriptCheckpointStore,
	selectedTick int,
) (*interfaces.FactorySessionJavaScriptRuntimeState, error) {
	state := (*interfaces.FactorySessionJavaScriptRuntimeState)(nil)
	handle := liveSessionHandle(session)
	if handle != nil && handle.Bundle != nil && handle.Bundle.EventHistory != nil {
		worldState, err := projections.ReconstructCanonicalFactoryWorldState(handle.Bundle.EventHistory.CanonicalEvents(), selectedTick)
		if err != nil {
			return nil, err
		}
		state = worldState.JavaScriptRuntime
	}
	return factorysessions.JavaScriptRuntimeStateFromCheckpoints(checkpointStore, state), nil
}

func (fs *Host) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	result, err := fs.requireSessionGateway().GetFactorySessionResult(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionLiveResult{}, err
	}
	return apisurface.WorkflowSessionLiveResultToAPI(result), nil
}

func (c *runtimeCoordinator) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	if c.host == nil {
		return factoryapi.FactorySessionLiveResult{}, fmt.Errorf("factory service is required")
	}
	return c.host.GetFactorySessionResult(ctx, sessionID)
}

func (fs *Host) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	result, err := fs.requireSessionGateway().GetFactorySessionPartialResult(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionPartialResult{}, err
	}
	return apisurface.WorkflowSessionPartialResultToAPI(result), nil
}

func (c *runtimeCoordinator) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	if c.host == nil {
		return factoryapi.FactorySessionPartialResult{}, fmt.Errorf("factory service is required")
	}
	return c.host.GetFactorySessionPartialResult(ctx, sessionID)
}

func (fs *Host) javascriptCheckpointStoreDirect(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	if state.javascriptCheckpoints == nil {
		state.javascriptCheckpoints = factorysessions.NewJavaScriptCheckpointStore()
	}
	return state.javascriptCheckpoints
}

func (fs *Host) sessionResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	state := liveSessionRuntimeState(session)
	if state == nil {
		return nil
	}
	state.responseStreamsOnce.Do(func() {
		state.responseStreams = fs.newSessionResponseStreamSetInstance()
	})
	return state.responseStreams
}

func (fs *Host) sessionResponseStream(
	session *factorysessions.LiveSession,
	dispatchID string,
) *factorysessions.SessionResponseStream {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return nil
	}
	return streams.Stream(dispatchID)
}

func (fs *Host) closeSessionResponseStreams(session *factorysessions.LiveSession) {
	fs.requireSessionGateway().CloseSessionResponseStreams(session)
}

func (fs *Host) closeSessionResponseStreamsDirect(session *factorysessions.LiveSession) {
	session.CloseResponseEvents()
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return
	}
	streams.Close()
}

func (fs *Host) closeSessionResponseStreamDispatchDirect(
	session *factorysessions.LiveSession,
	dispatchID string,
) bool {
	streams := fs.sessionResponseStreams(session)
	if streams == nil {
		return false
	}
	return streams.CloseDispatch(dispatchID)
}

func (fs *Host) SubscribeSessionResponseStream(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	return fs.requireSessionGateway().SubscribeSessionResponseStream(sessionID, dispatchID, afterSequence)
}

func (fs *Host) SessionResponseStreamDispatchIDs(sessionID string) ([]string, error) {
	return fs.requireSessionGateway().SessionResponseStreamDispatchIDs(sessionID)
}

func (fs *Host) newSessionResponseStreamInstance() *factorysessions.SessionResponseStream {
	if fs != nil && fs.newSessionResponseStream != nil {
		return fs.newSessionResponseStream()
	}
	return factorysessions.NewSessionResponseStream()
}

func (fs *Host) newSessionResponseStreamSetInstance() *factorysessions.SessionResponseStreamSet {
	return factorysessions.NewSessionResponseStreamSetWithFactory(func() *factorysessions.SessionResponseStream {
		return fs.newSessionResponseStreamInstance()
	})
}

func factorySessionBackendScopeID(fs *Host, session *factorysessions.LiveSession) string {
	if fs != nil && fs.cfg != nil {
		if backendScopeID := strings.TrimSpace(fs.cfg.BackendScopeID); backendScopeID != "" {
			return backendScopeID
		}
	}
	if bundle := liveSessionBundle(session); bundle != nil {
		return strings.TrimSpace(bundle.BackendScopeID)
	}
	return ""
}

func factorySessionStreamGenerationID(_ *Host, session *factorysessions.LiveSession) string {
	if session == nil {
		return ""
	}
	if handle := liveSessionHandle(session); handle != nil && handle.Bundle != nil {
		if handle.Bundle.EventHistory != nil {
			if streamGenerationID := strings.TrimSpace(handle.Bundle.EventHistory.StreamGenerationID()); streamGenerationID != "" {
				return streamGenerationID
			}
		}
		if handle.Bundle.Factory != nil {
			snapshot, err := handle.Bundle.Factory.GetEngineStateSnapshot(context.Background())
			if err == nil && snapshot != nil {
				if streamGenerationID := strings.TrimSpace(snapshot.StreamGenerationID); streamGenerationID != "" {
					return streamGenerationID
				}
			}
		}
	}
	if bundle := liveSessionBundle(session); bundle != nil {
		if startedAt := bundle.StartedAtUTC; !startedAt.IsZero() {
			return startedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return ""
}

func NewInferenceProgressPublisherFactory(
	sessions *factorysessions.Registry,
	logger *zap.Logger,
) inferenceProgressPublisherFactory {
	if sessions == nil {
		return nil
	}
	gateway := newSessionGatewayService(&Host{sessions: sessions})
	return gateway.InferenceProgressPublisherFactory(logger)
}

func NewSessionDispatchCompletionObserverFactory(
	sessions *factorysessions.Registry,
) dispatchCompletionObserverFactory {
	if sessions == nil {
		return nil
	}
	gateway := newSessionGatewayService(&Host{sessions: sessions})
	return gateway.DispatchCompletionObserverFactory()
}

func (fs *Host) inferenceProgressPublisher(
	sessionID string,
	logger *zap.Logger,
) workerprovider.InferenceProgressPublisher {
	if fs == nil {
		return nil
	}
	factory := fs.requireSessionGateway().InferenceProgressPublisherFactory(logger)
	if factory == nil {
		return nil
	}
	return factory(sessionID)
}

func (fs *Host) observeResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(event.DispatchID),
		Reason:     string(event.Kind),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamPublished, fields)
}

func (fs *Host) observeResponseStreamCompaction(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	summary responsestream.CompactionSummary,
) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(dispatchID),
		Reason:     string(summary.Reason),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamCompacted, fields)
	if handle := liveSessionHandle(session); handle != nil && handle.Bundle != nil && handle.Bundle.Logger != nil {
		handle.Bundle.Logger.Warn("session response stream compacted internal provider progress",
			zap.String("dispatch_id", dispatchID),
			zap.String("compaction_reason", string(summary.Reason)),
			zap.Int("dropped_sequence_count", summary.DroppedSequenceCount),
			zap.Int64("first_retained_sequence", summary.FirstRetainedSequence),
			zap.Int64("last_dropped_sequence", summary.LastDroppedSequence),
		)
	}
}

func (fs *Host) observeResponseStreamDegraded(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	reason string,
	fallbackLogger *zap.Logger,
	err error,
) {
	fields := metrics.Fields{
		DispatchID: strings.TrimSpace(dispatchID),
		Reason:     strings.TrimSpace(reason),
	}
	emitSessionResponseStreamMetric(session, sessionID, runtimeMetricSessionResponseStreamDegraded, fields)

	log := fallbackLogger
	if handle := liveSessionHandle(session); handle != nil && handle.Bundle != nil && handle.Bundle.Logger != nil {
		log = handle.Bundle.Logger
	}
	if log == nil {
		return
	}
	logFields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("dispatch_id", strings.TrimSpace(dispatchID)),
		zap.String("reason", strings.TrimSpace(reason)),
	}
	if err != nil {
		logFields = append(logFields, zap.Error(err))
	}
	log.Warn("internal provider progress publication degraded", logFields...)
}

func emitSessionResponseStreamMetric(
	session *factorysessions.LiveSession,
	sessionID string,
	name string,
	fields metrics.Fields,
) {
	handle := liveSessionHandle(session)
	if handle == nil || handle.Bundle == nil {
		return
	}
	if fields.DispatchID == "" {
		fields.DispatchID = sessionID
	}
	if err := handle.Bundle.MetricsEmitter().Counter(context.Background(), name, 1, fields); err != nil {
		handle.Bundle.RuntimeLogger().Warn("session response stream metric emission failed",
			zap.String("metric_name", name),
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}
}

var _ apisurface.DurableSessionExecutionAPI = (*Host)(nil)
var _ apisurface.DurableSessionListingAPI = (*Host)(nil)
var _ apisurface.DurableSessionLifecycleAPI = (*Host)(nil)

func (fs *Host) StartDurableFactorySessionAsync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	return fs.DurableExecutionAPI().StartDurableFactorySessionAsync(ctx, request)
}

func (fs *Host) StartDurableFactorySessionSync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	return fs.DurableExecutionAPI().StartDurableFactorySessionSync(ctx, request)
}

func (fs *Host) durableExecutionService() factorysessionexecution.Service {
	if fs == nil {
		return nil
	}
	return fs.durableExecution
}

func (fs *Host) ListDurableFactorySessions(
	ctx context.Context,
	params factoryapi.ListFactorySessionsParams,
) (factoryapi.ListFactorySessionsResponse, error) {
	return fs.DurableExecutionAPI().ListDurableFactorySessions(ctx, params)
}

// ListDurableExecutionSessions returns the shared durable session listing projection
// used by API merge logic before workspace rows are combined.
func (fs *Host) ListDurableExecutionSessions(
	ctx context.Context,
	req factorysessionexecution.ListSessionsRequest,
) (factorysessionexecution.ListSessionsResult, error) {
	execution := fs.durableExecutionService()
	if execution == nil {
		return factorysessionexecution.ListSessionsResult{}, factorysessionexecution.ErrServiceNotConfigured
	}
	return execution.ListSessions(ctx, req)
}

func (fs *Host) GetDurableFactorySession(
	ctx context.Context,
	sessionID string,
) (factoryapi.FactorySessionDurableReadModel, error) {
	return fs.DurableExecutionAPI().GetDurableFactorySession(ctx, sessionID)
}

func (fs *Host) GetDurableFactorySessionResult(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetFactorySessionResultsParams,
) (factoryapi.FactorySessionResult, error) {
	return fs.DurableExecutionAPI().GetDurableFactorySessionResult(ctx, sessionID, params)
}

func (fs *Host) ReadDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetEventsBySessionIdParams,
) (*interfaces.FactoryEventStream, error) {
	return fs.DurableExecutionAPI().ReadDurableFactorySessionEvents(ctx, sessionID, params)
}

func (fs *Host) ListDurableFactorySessionDispatches(
	ctx context.Context,
	sessionID string,
	params factoryapi.ListFactorySessionDispatchesParams,
) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	return fs.DurableExecutionAPI().ListDurableFactorySessionDispatches(ctx, sessionID, params)
}

func (fs *Host) GetDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID, dispatchID string,
) (factoryapi.FactoryDispatch, error) {
	return fs.DurableExecutionAPI().GetDurableFactorySessionDispatch(ctx, sessionID, dispatchID)
}

func (fs *Host) ListDurableFactorySessionArtifacts(
	ctx context.Context,
	sessionID string,
) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	return fs.DurableExecutionAPI().ListDurableFactorySessionArtifacts(ctx, sessionID)
}

func (fs *Host) GetDurableFactorySessionArtifact(
	ctx context.Context,
	sessionID, artifactID string,
) (factoryapi.FactorySessionArtifactDetail, error) {
	return fs.DurableExecutionAPI().GetDurableFactorySessionArtifact(ctx, sessionID, artifactID)
}

func (fs *Host) PauseDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.DurableExecutionAPI().PauseDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) ResumeDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.DurableExecutionAPI().ResumeDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) CancelDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.DurableExecutionAPI().CancelDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) TerminateDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.DurableExecutionAPI().TerminateDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) ApproveDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionApproveRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.DurableExecutionAPI().ApproveDurableFactorySession(ctx, sessionID, request)
}

func (fs *Host) RetryDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionRetryDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.DurableExecutionAPI().RetryDurableFactorySessionDispatch(ctx, sessionID, request)
}

func (fs *Host) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.DurableExecutionAPI().InterruptDurableFactorySessionDispatch(ctx, sessionID, request)
}
func (fs *Host) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireCoordinator().PauseLiveFactorySession(ctx, sessionID, request)
}

func (c *runtimeCoordinator) PauseLiveFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if c == nil || c.host == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("factory service is required")
	}
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := c.host.requireSessionGateway().PauseLiveFactorySession(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *Host) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return fs.requireCoordinator().ResumeLiveFactorySession(ctx, sessionID, request)
}

func (c *runtimeCoordinator) ResumeLiveFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if c == nil || c.host == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("factory service is required")
	}
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := c.host.requireSessionGateway().ResumeLiveFactorySession(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (fs *Host) observeLiveLifecycleControl(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
	err error,
) {
	if fs == nil {
		return
	}

	outcomeClass := lifecycleControlOutcomeClass(outcome, err)
	fields := liveLifecycleControlLogFields(sessionID, operation, outcomeClass, status, control)
	switch outcomeClass {
	case lifecycleControlOutcomeClassNotFound,
		string(factorysessionexecution.LifecycleControlOutcomeInvalidState),
		string(factorysessionexecution.LifecycleControlOutcomeTerminalSession):
		fs.logger.Warn("factory session lifecycle control rejected", fields...)
	default:
		fs.logger.Info("factory session lifecycle control", fields...)
	}

	fs.emitLiveLifecycleControlMetric(sessionID, operation, outcomeClass)
}

func lifecycleControlOutcomeClass(
	outcome factorysessionexecution.LifecycleControlOutcome,
	err error,
) string {
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return lifecycleControlOutcomeClassNotFound
		}
		var controlErr *factorysessionexecution.ControlError
		if errors.As(err, &controlErr) {
			return string(controlErr.Outcome)
		}
		return "ERROR"
	}
	if outcome == "" {
		return "ERROR"
	}
	return string(outcome)
}

const (
	lifecycleControlOutcomeClassNotFound = "NOT_FOUND"
)

func liveLifecycleControlLogFields(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	outcomeClass string,
	status factorysessionexecution.LifecycleStatus,
	control factorysessionexecution.ControlRequest,
) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("operation", string(operation)),
		zap.String("outcome", outcomeClass),
	}
	if status != "" {
		fields = append(fields, zap.String("lifecycle_control_status", string(status)))
	}
	if requestID := control.RequestID; requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}
	return fields
}

func (fs *Host) recordSessionPersistenceInvalidationFromPreflight(
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	if diagnostic, ok := factorysessioncursors.InvalidationFromPreflight(apisurface.FactorySessionCursorPreflightResult(response)); ok {
		fs.recordSessionPersistenceInvalidation(diagnostic)
	}
}

func (fs *Host) recordSessionPersistenceInvalidation(
	diagnostic factorysessioncursors.InvalidationDiagnostic,
) {
	if fs == nil {
		return
	}
	fs.sessionPersistenceObserver().Record(diagnostic)
}

func (fs *Host) sessionPersistenceObserver() sessioncursor.Observer {
	return sessioncursor.Observer{
		Logger: sessionPersistenceZapLogger{logger: fs.logger},
	}
}

type sessionPersistenceZapLogger struct {
	logger *zap.Logger
}

func (l sessionPersistenceZapLogger) Info(msg string, fields map[string]string) {
	if l.logger == nil {
		return
	}
	zapFields := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		zapFields = append(zapFields, zap.String(key, value))
	}
	l.logger.Info(msg, zapFields...)
}

func (fs *Host) sessionPersistenceScopeFromSession(
	ctx context.Context,
	session *factorysessions.LiveSession,
) (factorysessioncursors.IdentityScope, error) {
	if fs == nil || session == nil {
		return factorysessioncursors.IdentityScope{}, fmt.Errorf("factory service is required")
	}
	projectionCtx, err := fs.buildSessionProjectionContext(ctx, session)
	if err != nil {
		return factorysessioncursors.IdentityScope{}, err
	}
	runtime := factorysessions.ProjectRuntimeContract(projectionCtx)
	scope := factorysessioncursors.IdentityScope{
		BackendScopeID:      factorySessionBackendScopeID(fs, session),
		LogicalSessionKeyID: controlplane.LogicalSessionKeyID(session),
		FactorySessionID:    strings.TrimSpace(session.ID),
	}
	if runtime.StreamIdentity != nil {
		scope.BackendScopeID = strings.TrimSpace(runtime.StreamIdentity.BackendScopeID)
		scope.FactorySessionID = strings.TrimSpace(runtime.StreamIdentity.FactorySessionID)
		scope.StreamGenerationID = strings.TrimSpace(runtime.StreamIdentity.StreamGenerationID)
	}
	return factorysessioncursors.NormalizeScope(scope), nil
}

func (fs *Host) emitLiveLifecycleControlMetric(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	outcomeClass string,
) {
	if fs == nil {
		return
	}
	session, err := fs.requireSession(sessionID)
	if err != nil {
		return
	}
	bundle := liveSessionHandle(session).Bundle
	if bundle == nil {
		return
	}
	bundle.EmitMetricCounter(runtimeMetricLifecycleControl, 1, metrics.Fields{
		Outcome: outcomeClass,
		Reason:  string(operation),
	})
}

// sessionGateway is the injectable session gateway collaborator seam.
type SessionGateway interface {
	OpenFactorySession(context.Context, factorysessions.OpenRequest) (*FactorySessionOpenResult, error)
	OpenFactorySessionFromFolder(context.Context, string, *FactorySessionTargetRef, bool, bool) (*FactorySessionOpenResult, error)
	ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error)
	GetFactorySession(context.Context, string) (factorysessions.ProjectionContext, error)
	GetFactorySessionSyncPreflight(context.Context, string, *interfaces.FactoryEventReconnectCursor, *interfaces.FactorySessionLogicalResolveHint) (factorysessions.SyncPreflightResult, error)
	GetFactorySessionResult(context.Context, string) (workflowresult.LiveSessionResult, error)
	GetFactorySessionPartialResult(context.Context, string) (workflowresult.PartialSessionResult, error)
	PauseLiveFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	ResumeLiveFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	CloseFactorySession(context.Context, string) error
	PauseDurableFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	ResumeDurableFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	CancelDurableFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	TerminateDurableFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	ApproveDurableFactorySession(context.Context, string, factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error)
	RetryDurableFactorySessionDispatch(context.Context, string, factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error)
	InterruptDurableFactorySessionDispatch(context.Context, string, factorysessionexecution.InterruptDispatchRequest) (factorysessionexecution.LifecycleControlResult, error)
	SubscribeSessionResponseStream(sessionID string, dispatchID string, afterSequence int64) (*factorysessions.SessionResponseStreamSubscription, error)
	SessionResponseStreamDispatchIDs(sessionID string) ([]string, error)
	CloseSessionResponseStreams(session *factorysessions.LiveSession)
	JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore
	InferenceProgressPublisherFactory(logger *zap.Logger) func(sessionID string) workerprovider.InferenceProgressPublisher
	DispatchCompletionObserverFactory() func(sessionID string) func(string)
}

var _ SessionGateway = (*factorysessionservice.Service)(nil)

type sessionGatewayHost struct {
	*Host
}

var _ factorysessionservice.Host = sessionGatewayHost{}

func (h sessionGatewayHost) DiscoverTargets(folderPath string) ([]factorysessions.Target, error) {
	if h.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.discoverFactorySessionTargets(folderPath)
}

func (h sessionGatewayHost) InitializeFactoryScaffold(factoryDir string) error {
	if err := initcmd.Init(initcmd.InitConfig{
		Dir:         factoryDir,
		Diagnostics: io.Discard,
	}); err != nil {
		return factorysessions.NewValidationError(
			factorysessions.ValidationReasonUnreadable,
			"folderPath",
			fmt.Errorf("initialize factory scaffold: %w", err),
		)
	}
	return nil
}

func (h sessionGatewayHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	if h.Host == nil {
		return "", fmt.Errorf("factory service is required")
	}
	return h.openFactorySessionForTarget(ctx, target)
}

func (h sessionGatewayHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if h.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.requireSession(sessionID)
}

func (h sessionGatewayHost) ListLiveSessionIDs() []string {
	if h.Host == nil || h.Host.sessions == nil {
		return nil
	}
	return h.Host.sessions.IDs()
}

func (h sessionGatewayHost) GetLiveSession(sessionID string) *factorysessions.LiveSession {
	if h.Host == nil {
		return nil
	}
	return h.Host.sessionByID(sessionID)
}

func (h sessionGatewayHost) BuildSessionProjectionContext(
	ctx context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.Host == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("factory service is required")
	}
	return h.Host.buildSessionProjectionContext(ctx, session)
}

func (h sessionGatewayHost) ResolveSyncPreflightTarget(
	sessionID string,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
) (controlplane.SyncPreflightTarget, error) {
	if h.Host == nil {
		return controlplane.SyncPreflightTarget{}, fmt.Errorf("factory service is required")
	}
	target, err := h.Host.resolveSessionSyncPreflightTarget(sessionID, logicalResolve)
	return controlplane.SyncPreflightTarget{
		Session:    target.session,
		Remapped:   target.remapped,
		Unresolved: target.unresolved,
	}, err
}

func (h sessionGatewayHost) BackendScopeID() string {
	if h.Host == nil {
		return ""
	}
	return factorySessionBackendScopeID(h.Host, nil)
}

func (h sessionGatewayHost) StreamGenerationID(session *factorysessions.LiveSession) string {
	if h.Host == nil {
		return ""
	}
	return factorySessionStreamGenerationID(h.Host, session)
}

func (h sessionGatewayHost) LiveSessionEvents(session *factorysessions.LiveSession) []interfaces.FactoryEvent {
	handle := liveSessionHandle(session)
	if handle == nil || handle.Bundle == nil || handle.Bundle.EventHistory == nil {
		return nil
	}
	return handle.Bundle.EventHistory.CanonicalEvents()
}

func (h sessionGatewayHost) SessionFactory(sessionID string) (factory.Factory, error) {
	if h.Host == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.Host.sessionFactory(sessionID)
}

func (h sessionGatewayHost) StopLiveSession(sessionID string) error {
	if h.Host == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.Host.stopFactorySession(sessionID)
}

func (h sessionGatewayHost) ObserveLiveLifecycleControl(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
	err error,
) {
	if h.Host == nil {
		return
	}
	h.Host.observeLiveLifecycleControl(sessionID, operation, control, outcome, status, err)
}

func (h sessionGatewayHost) DurableExecution() factorysessionexecution.Service {
	if h.Host == nil {
		return nil
	}
	return h.Host.durableExecutionService()
}

func (h sessionGatewayHost) ResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if h.Host == nil {
		return nil
	}
	return h.Host.sessionResponseStreams(session)
}

func (h sessionGatewayHost) NewResponseStream() *factorysessions.SessionResponseStream {
	if h.Host == nil {
		return factorysessions.NewSessionResponseStream()
	}
	return h.Host.newSessionResponseStreamInstance()
}

func (h sessionGatewayHost) CloseResponseStreams(session *factorysessions.LiveSession) {
	if h.Host == nil {
		return
	}
	h.Host.closeSessionResponseStreamsDirect(session)
}

func (h sessionGatewayHost) CloseResponseStreamDispatch(session *factorysessions.LiveSession, dispatchID string) bool {
	if h.Host == nil {
		return false
	}
	return h.Host.closeSessionResponseStreamDispatchDirect(session, dispatchID)
}

func (h sessionGatewayHost) JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if h.Host == nil {
		return nil
	}
	return h.Host.javascriptCheckpointStoreDirect(session)
}

func (h sessionGatewayHost) ObserveResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event) {
	if h.Host == nil {
		return
	}
	h.Host.observeResponseStreamPublished(session, sessionID, event)
}

func (h sessionGatewayHost) ObserveResponseStreamCompaction(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	summary responsestream.CompactionSummary,
) {
	if h.Host == nil {
		return
	}
	h.Host.observeResponseStreamCompaction(session, sessionID, dispatchID, summary)
}

func (h sessionGatewayHost) ObserveResponseStreamDegraded(
	session *factorysessions.LiveSession,
	sessionID string,
	dispatchID string,
	reason string,
	fallbackLogger *zap.Logger,
	err error,
) {
	if h.Host == nil {
		return
	}
	h.Host.observeResponseStreamDegraded(session, sessionID, dispatchID, reason, fallbackLogger, err)
}

func newSessionGatewayService(fs *Host) *factorysessionservice.Service {
	return factorysessionservice.New(sessionGatewayHost{fs})
}

func (fs *Host) requireSessionGateway() SessionGateway {
	if fs == nil {
		return newSessionGatewayService(nil)
	}
	if fs.sessionGateway == nil {
		fs.sessionGateway = newSessionGatewayService(fs)
	}
	return fs.sessionGateway
}
