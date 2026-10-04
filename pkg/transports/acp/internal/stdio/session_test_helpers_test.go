// Shared ACP stdio fixtures for all focused behavioral suites.
// Keep cross-suite fakes and setup in this package-local helper.
package stdio

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/contextscope"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionswire "github.com/portpowered/infinite-you/pkg/services/chat_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"strconv"
	"sync"
	"testing"
	"time"
)

func testLogger() logging.Logger { return logging.NoopLogger{} }

func testInvocationScope(ctx context.Context) acp.InvocationScope { return contextscope.New(ctx) }

type fakeChatSessionsService struct {
	mu                                sync.Mutex
	createCalled                      bool
	created                           chatsessions.CreateSessionRequest
	createErr                         error
	sessionID                         string
	getSessionCalled                  bool
	getSessionReq                     chatsessions.GetSessionRequest
	getSessionReqs                    []chatsessions.GetSessionRequest
	getSessionResult                  chatsessions.GetSessionResult
	getSessionErrs                    []error
	getSessionResults                 []chatsessions.GetSessionResult
	getSessionErr                     error
	setTargetCalled                   bool
	setTargetReq                      chatsessions.SetTargetRequest
	setTargetResult                   chatsessions.SetTargetResult
	setTargetErr                      error
	startTurnCalled                   bool
	startTurnReq                      chatsessions.StartTurnRequest
	startTurnReqs                     []chatsessions.StartTurnRequest
	startTurnResult                   chatsessions.StartTurnResult
	startTurnResults                  []chatsessions.StartTurnResult
	startTurnErr                      error
	bindFactorySessionCalled          bool
	bindFactorySessionReq             chatsessions.BindFactorySessionRequest
	bindFactorySessionReqs            []chatsessions.BindFactorySessionRequest
	bindFactorySessionResult          chatsessions.BindFactorySessionResult
	bindFactorySessionErrs            []error
	bindFactorySessionErr             error
	recordPendingFactorySessionCalled bool
	recordPendingFactorySessionReq    chatsessions.RecordPendingFactorySessionRequest
	recordPendingFactorySessionReqs   []chatsessions.RecordPendingFactorySessionRequest
	recordPendingFactorySessionResult chatsessions.RecordPendingFactorySessionResult
	recordPendingFactorySessionErrs   []error
	recordPendingFactorySessionErr    error
	advanceTurnCalled                 bool
	advanceTurnReq                    chatsessions.AdvanceTurnRequest
	advanceTurnReqs                   []chatsessions.AdvanceTurnRequest
	advanceTurnErrs                   []error
	advanceTurnErr                    error
	attachments                       map[string]chatsessions.Attachment
	nextAttachmentID                  int
	attachErr                         error
	detachCalls                       []chatsessions.DetachRequest
	detachErr                         error
	acknowledgeAttachmentErr          error
	acknowledgeAttachmentErrs         []error
	acknowledgeAttachmentReqs         []chatsessions.AcknowledgeAttachmentRequest
	acknowledgeAttachmentPositionErr  bool
	requestControlReqs                []chatsessions.RequestControlRequest
	requestControlResult              chatsessions.RequestControlResult
	requestControlErr                 error
	lastControlIntent                 chatsessions.ControlIntent
	advanceControlReqs                []chatsessions.AdvanceControlRequest
	advanceControlResult              chatsessions.AdvanceControlResult
	advanceControlResults             []chatsessions.AdvanceControlResult
	advanceControlErr                 error
}

var _ chatsessions.Service = (*fakeChatSessionsService)(nil)

func (f *fakeChatSessionsService) CreateSession(_ context.Context, req chatsessions.CreateSessionRequest) (chatsessions.CreateSessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalled = true
	f.created = req
	if f.createErr != nil {
		return chatsessions.CreateSessionResult{}, f.createErr
	}
	id := f.sessionID
	if id == "" {
		id = "session-1"
	}
	now := time.Unix(0, 1)
	return chatsessions.CreateSessionResult{Session: chatsessions.Session{
		ID:             id,
		State:          chatsessions.SessionStateCreated,
		SelectedTarget: req.InitialTarget,
		WorkingRoot:    req.WorkingRoot,
		CreatedAt:      now,
		UpdatedAt:      now,
	}}, nil
}
func (f *fakeChatSessionsService) GetSession(_ context.Context, req chatsessions.GetSessionRequest) (chatsessions.GetSessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getSessionCalled = true
	f.getSessionReq = req
	f.getSessionReqs = append(f.getSessionReqs, req)
	if len(f.getSessionErrs) > 0 {
		next := f.getSessionErrs[0]
		f.getSessionErrs = f.getSessionErrs[1:]
		if next != nil {
			return chatsessions.GetSessionResult{}, next
		}
	}
	if f.getSessionErr != nil {
		return chatsessions.GetSessionResult{}, f.getSessionErr
	}
	if len(f.getSessionResults) > 0 {
		next := f.getSessionResults[0]
		f.getSessionResults = f.getSessionResults[1:]
		return next, nil
	}
	return f.getSessionResult, nil
}
func (f *fakeChatSessionsService) SetTarget(_ context.Context, req chatsessions.SetTargetRequest) (chatsessions.SetTargetResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setTargetCalled = true
	f.setTargetReq = req
	if f.setTargetErr != nil {
		return chatsessions.SetTargetResult{}, f.setTargetErr
	}
	return f.setTargetResult, nil
}
func (f *fakeChatSessionsService) StartTurn(_ context.Context, req chatsessions.StartTurnRequest) (chatsessions.StartTurnResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startTurnCalled = true
	f.startTurnReq = req
	f.startTurnReqs = append(f.startTurnReqs, req)
	if f.startTurnErr != nil {
		return chatsessions.StartTurnResult{}, f.startTurnErr
	}
	if len(f.startTurnResults) > 0 {
		next := f.startTurnResults[0]
		f.startTurnResults = f.startTurnResults[1:]
		return next, nil
	}
	return f.startTurnResult, nil
}
func (f *fakeChatSessionsService) BindFactorySession(_ context.Context, req chatsessions.BindFactorySessionRequest) (chatsessions.BindFactorySessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bindFactorySessionCalled = true
	f.bindFactorySessionReq = req
	f.bindFactorySessionReqs = append(f.bindFactorySessionReqs, req)
	if len(f.bindFactorySessionErrs) > 0 {
		next := f.bindFactorySessionErrs[0]
		f.bindFactorySessionErrs = f.bindFactorySessionErrs[1:]
		if next != nil {
			return chatsessions.BindFactorySessionResult{}, next
		}
		return f.bindFactorySessionResult, nil
	}
	if f.bindFactorySessionErr != nil {
		return chatsessions.BindFactorySessionResult{}, f.bindFactorySessionErr
	}
	return f.bindFactorySessionResult, nil
}
func (f *fakeChatSessionsService) RecordPendingFactorySession(_ context.Context, req chatsessions.RecordPendingFactorySessionRequest) (chatsessions.RecordPendingFactorySessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recordPendingFactorySessionCalled = true
	f.recordPendingFactorySessionReq = req
	f.recordPendingFactorySessionReqs = append(f.recordPendingFactorySessionReqs, req)
	if len(f.recordPendingFactorySessionErrs) > 0 {
		next := f.recordPendingFactorySessionErrs[0]
		f.recordPendingFactorySessionErrs = f.recordPendingFactorySessionErrs[1:]
		if next != nil {
			return chatsessions.RecordPendingFactorySessionResult{}, next
		}
		return f.recordPendingFactorySessionResult, nil
	}
	if f.recordPendingFactorySessionErr != nil {
		return chatsessions.RecordPendingFactorySessionResult{}, f.recordPendingFactorySessionErr
	}
	return f.recordPendingFactorySessionResult, nil
}
func (f *fakeChatSessionsService) AdvanceTurn(_ context.Context, req chatsessions.AdvanceTurnRequest) (chatsessions.AdvanceTurnResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advanceTurnCalled = true
	f.advanceTurnReq = req
	f.advanceTurnReqs = append(f.advanceTurnReqs, req)
	if len(f.advanceTurnErrs) > 0 {
		next := f.advanceTurnErrs[0]
		f.advanceTurnErrs = f.advanceTurnErrs[1:]
		if next != nil {
			return chatsessions.AdvanceTurnResult{}, next
		}
		return chatsessions.AdvanceTurnResult{Turn: chatsessions.Turn{ID: req.TurnID, State: req.Next}}, nil
	}
	if f.advanceTurnErr != nil {
		return chatsessions.AdvanceTurnResult{}, f.advanceTurnErr
	}
	return chatsessions.AdvanceTurnResult{Turn: chatsessions.Turn{ID: req.TurnID, State: req.Next}}, nil
}
func (f *fakeChatSessionsService) Attach(_ context.Context, req chatsessions.AttachRequest) (chatsessions.AttachResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.attachErr != nil {
		return chatsessions.AttachResult{}, f.attachErr
	}
	if req.Resume && req.Interactive {
		if req.ResumeAttachmentID != "" {
			existing, found := f.attachments[req.ResumeAttachmentID]
			if !found || !existing.Detached || !existing.Interactive {
				return chatsessions.AttachResult{}, &chatsessions.NotFoundError{Value: "Attachment", ID: req.ResumeAttachmentID}
			}
			existing.ConnectionID = req.ConnectionID
			existing.Detached = false
			f.attachments[req.ResumeAttachmentID] = existing
			return chatsessions.AttachResult{Attachment: existing}, nil
		}
		var resumed chatsessions.Attachment
		candidates := 0
		for _, existing := range f.attachments {
			if existing.Detached && existing.Interactive {
				resumed = existing
				candidates++
			}
		}
		if candidates > 1 {
			return chatsessions.AttachResult{}, &chatsessions.AttachmentResumeAmbiguityError{SessionID: req.SessionID, CandidateCount: candidates}
		}
		if candidates == 1 {
			resumed.ConnectionID = req.ConnectionID
			resumed.Detached = false
			f.attachments[resumed.ID] = resumed
			return chatsessions.AttachResult{Attachment: resumed}, nil
		}
	}
	f.nextAttachmentID++
	attachment := chatsessions.Attachment{
		ID:           fmt.Sprintf("attachment-%d", f.nextAttachmentID),
		SessionID:    req.SessionID,
		ConnectionID: req.ConnectionID,
		Interactive:  req.Interactive,
	}
	if f.attachments == nil {
		f.attachments = make(map[string]chatsessions.Attachment)
	}
	f.attachments[attachment.ID] = attachment
	return chatsessions.AttachResult{Attachment: attachment}, nil
}
func (f *fakeChatSessionsService) Detach(_ context.Context, req chatsessions.DetachRequest) (chatsessions.DetachResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detachCalls = append(f.detachCalls, req)
	if f.detachErr != nil {
		return chatsessions.DetachResult{}, f.detachErr
	}
	attachment, ok := f.attachments[req.AttachmentID]
	if !ok {
		return chatsessions.DetachResult{}, &chatsessions.NotFoundError{Value: "Attachment", ID: req.AttachmentID}
	}
	attachment.Detached = true
	f.attachments[req.AttachmentID] = attachment
	return chatsessions.DetachResult{}, nil
}
func (f *fakeChatSessionsService) RequestControl(_ context.Context, req chatsessions.RequestControlRequest) (chatsessions.RequestControlResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requestControlReqs = append(f.requestControlReqs, req)
	if f.requestControlErr != nil {
		return chatsessions.RequestControlResult{}, f.requestControlErr
	}
	if f.requestControlResult.Intent.State != "" {
		f.lastControlIntent = f.requestControlResult.Intent
		return f.requestControlResult, nil
	}
	intent := chatsessions.ControlIntent{
		RequestID:       req.RequestID,
		SessionID:       req.SessionID,
		TurnID:          f.getSessionResult.Session.ActiveTurnID,
		TargetEpisode:   f.getSessionResult.Episode.Number,
		ExpectedVersion: req.ExpectedVersion,
		Action:          req.Action,
		State:           chatsessions.ControlIntentStateRequested,
		RequestedAt:     time.Unix(0, 1),
	}
	f.lastControlIntent = intent
	return chatsessions.RequestControlResult{Intent: intent}, nil
}
func (f *fakeChatSessionsService) AdvanceControl(_ context.Context, req chatsessions.AdvanceControlRequest) (chatsessions.AdvanceControlResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advanceControlReqs = append(f.advanceControlReqs, req)
	if f.advanceControlErr != nil {
		return chatsessions.AdvanceControlResult{}, f.advanceControlErr
	}
	if len(f.advanceControlResults) > 0 {
		next := f.advanceControlResults[0]
		f.advanceControlResults = f.advanceControlResults[1:]
		f.lastControlIntent = next.Intent
		return next, nil
	}
	if f.advanceControlResult.Intent.State != "" {
		f.lastControlIntent = f.advanceControlResult.Intent
		return f.advanceControlResult, nil
	}
	intent := f.lastControlIntent
	if intent.RequestID != req.RequestID || intent.SessionID != req.SessionID {
		intent.RequestID = req.RequestID
		intent.SessionID = req.SessionID
	}
	intent.State = req.Next
	f.lastControlIntent = intent
	return chatsessions.AdvanceControlResult{Intent: intent}, nil
}
func (f *fakeChatSessionsService) Sequence(ctx context.Context, req chatsessions.SequenceRequest) (chatsessions.SequenceResult, error) {
	return recordFakePromptHistory(f, ctx, req)
}
func (f *fakeChatSessionsService) AdvanceStreamHead(ctx context.Context, req chatsessions.AdvanceStreamHeadRequest) (chatsessions.AdvanceStreamHeadResult, error) {
	return advanceFakePromptHistoryStreamHead(f, ctx, req)
}
func (f *fakeChatSessionsService) AcknowledgeAttachment(_ context.Context, req chatsessions.AcknowledgeAttachmentRequest) (chatsessions.AcknowledgeAttachmentResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acknowledgeAttachmentReqs = append(f.acknowledgeAttachmentReqs, req)
	if len(f.acknowledgeAttachmentErrs) > 0 {
		next := f.acknowledgeAttachmentErrs[0]
		f.acknowledgeAttachmentErrs = f.acknowledgeAttachmentErrs[1:]
		if next != nil {
			return chatsessions.AcknowledgeAttachmentResult{}, next
		}
	}
	if f.acknowledgeAttachmentPositionErr {
		return chatsessions.AcknowledgeAttachmentResult{}, &chatsessions.AttachmentPositionError{
			SessionID: req.SessionID, AttachmentID: req.AttachmentID, Requested: uint64(req.AfterSequence),
		}
	}
	if f.acknowledgeAttachmentErr != nil {
		return chatsessions.AcknowledgeAttachmentResult{}, f.acknowledgeAttachmentErr
	}
	attachment, ok := f.attachments[req.AttachmentID]
	if !ok {
		return chatsessions.AcknowledgeAttachmentResult{}, &chatsessions.NotFoundError{Value: "Attachment", ID: req.AttachmentID}
	}
	if attachment.AfterSequence >= uint64(req.AfterSequence) {
		return chatsessions.AcknowledgeAttachmentResult{Attachment: attachment, Outcome: chatsessions.AcknowledgeAttachmentOutcomeAlreadyCurrent}, nil
	}
	attachment.AfterSequence = uint64(req.AfterSequence)
	f.attachments[req.AttachmentID] = attachment
	return chatsessions.AcknowledgeAttachmentResult{Attachment: attachment, Outcome: chatsessions.AcknowledgeAttachmentOutcomeAdvanced}, nil
}

type fakeFactoryTargetCatalogService struct {
	calls  []chatsessions.ResolveFactoryTargetCatalogRequest
	result chatsessions.ResolveFactoryTargetCatalogResult
	err    error
}

var _ chatsessions.FactoryTargetCatalogService = (*fakeFactoryTargetCatalogService)(nil)

func (f *fakeFactoryTargetCatalogService) ResolveFactoryTargetCatalog(
	_ context.Context,
	req chatsessions.ResolveFactoryTargetCatalogRequest,
) (chatsessions.ResolveFactoryTargetCatalogResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, f.err
	}
	return f.result, nil
}

type fakeFactoryTargetService struct {
	factorysessions.Service
	mu                sync.Mutex
	startCalls        []factorysessions.StartRequest
	sessionStartCalls []factorysessions.SessionStartRequest
	startResult       factorysessions.AsyncStartResult
	startErr          error
	startEnter        chan struct{}
	startRelease      chan struct{}
	invokeCalls       []invokeFactoryTargetCall
	invokeResult      factorysessions.InvocationResult
	invokeErr         error
	invokeErrs        []error
	invokeEnter       chan struct{}
	invokeRelease     chan struct{}
	cancelCalls       []cancelFactoryTargetCall
	cancelErr         error
	cancelOutcome     factorysessions.LifecycleControlOutcome
	responseCursor    *factorysessions.ResponseEventCursor
	responseErr       error
	cancelEntered     chan struct{}
	cancelRelease     chan struct{}
	closeCalls        []string
	closeErr          error
	terminateCalls    []terminateFactoryTargetCall
	controlCalls      []factorysessions.SessionControlRequest
	closeEntered      chan struct{}
	closeRelease      chan struct{}
}
type cancelFactoryTargetCall struct {
	sessionID string
	request   factorysessions.ControlRequest
}
type invokeFactoryTargetCall struct {
	sessionID string
	request   factorysessions.InvocationRequest
}
type terminateFactoryTargetCall struct {
	sessionID string
	request   factorysessions.ControlRequest
}

func (f *fakeFactoryTargetService) Start(
	_ context.Context,
	request factorysessions.SessionStartRequest,
) (factorysessions.SessionStartResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls = append(f.startCalls, factorysessions.StartRequest{
		RequestID: request.Correlation.RequestID,
		Source:    request.Source,
		Args:      request.Args,
	})
	f.sessionStartCalls = append(f.sessionStartCalls, request)
	if enter, release := f.startEnter, f.startRelease; enter != nil && release != nil {
		f.mu.Unlock()
		close(enter)
		<-release
		f.mu.Lock()
	}
	if f.startErr != nil {
		return factorysessions.SessionStartResult{}, f.startErr
	}
	if f.startResult.SessionID != "" {
		return factorysessions.SessionStartResult{SessionID: f.startResult.SessionID}, nil
	}
	return factorysessions.SessionStartResult{SessionID: request.SessionID}, nil
}
func (f *fakeFactoryTargetService) Invoke(
	_ context.Context,
	request factorysessions.SessionInvokeRequest,
) (factorysessions.InvocationResult, error) {
	f.mu.Lock()
	legacy := factorysessions.InvocationRequest{Content: work.CloneWorkContentParts(request.Content), ContentProvided: request.ContentProvided}
	if request.ContentProvided {
		sourceKind := factorysessions.InvocationInputSourceKindText
		legacy.SourceKind = &sourceKind
	}
	requestID := request.Correlation.RequestID
	legacy.RequestID = &requestID
	f.invokeCalls = append(f.invokeCalls, invokeFactoryTargetCall{sessionID: request.SessionID, request: legacy})
	enter, release := f.invokeEnter, f.invokeRelease
	f.mu.Unlock()
	if enter != nil && release != nil {
		close(enter)
		<-release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.invokeErrs) > 0 {
		next := f.invokeErrs[0]
		f.invokeErrs = f.invokeErrs[1:]
		if next != nil {
			return factorysessions.InvocationResult{}, next
		}
		return f.invokeResult, nil
	}
	if f.invokeErr != nil {
		return factorysessions.InvocationResult{}, f.invokeErr
	}
	return f.invokeResult, nil
}
func testStartResolver(_ context.Context, factoryTargetID, workingRoot, requestID string) (factorysessions.SessionStartRequest, error) {
	return factorysessions.SessionStartRequest{
		Mode:           factorysessions.SessionOperationModeLive,
		ActivationOnly: true,
		Correlation:    factorysessions.SessionOperationCorrelation{RequestID: requestID},
		Source: factorysessions.Source{
			Kind:      factoryruntime.WorkflowSourceKindFactoryID,
			FactoryID: factoryTargetID,
		},
		Args: map[string]any{"workingRoot": workingRoot},
	}, nil
}
func (f *fakeFactoryTargetService) StartAsync(
	_ context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls = append(f.startCalls, request)
	if f.startErr != nil {
		return factorysessions.AsyncStartResult{}, f.startErr
	}
	return f.startResult, nil
}
func (f *fakeFactoryTargetService) InvokeFactorySession(
	_ context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	f.mu.Lock()
	f.invokeCalls = append(f.invokeCalls, invokeFactoryTargetCall{sessionID: sessionID, request: request})
	enter, release := f.invokeEnter, f.invokeRelease
	f.mu.Unlock()
	if enter != nil && release != nil {
		close(enter)
		<-release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.invokeErrs) > 0 {
		next := f.invokeErrs[0]
		f.invokeErrs = f.invokeErrs[1:]
		if next != nil {
			return factorysessions.InvocationResult{}, next
		}
		return f.invokeResult, nil
	}
	if f.invokeErr != nil {
		return factorysessions.InvocationResult{}, f.invokeErr
	}
	return f.invokeResult, nil
}
func (f *fakeFactoryTargetService) Cancel(
	_ context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	f.mu.Lock()
	f.cancelCalls = append(f.cancelCalls, cancelFactoryTargetCall{sessionID: sessionID, request: request})
	if len(f.cancelCalls) == 1 && f.cancelEntered != nil {
		close(f.cancelEntered)
	}
	release, err := f.cancelRelease, f.cancelErr
	f.mu.Unlock()
	if release != nil {
		<-release
	}
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return factorysessions.LifecycleControlResult{}, nil
}
func (f *fakeFactoryTargetService) Control(
	_ context.Context,
	request factorysessions.SessionControlRequest,
) (factorysessions.SessionControlResult, error) {
	f.mu.Lock()
	f.controlCalls = append(f.controlCalls, request)
	sessionID, operation := request.SessionID, request.Operation
	control := request.Control
	release, closeRelease, closeErr, outcome := f.cancelRelease, f.closeRelease, f.closeErr, f.cancelOutcome
	err := f.cancelErr
	switch operation {
	case factorysessions.SessionControlTerminate:
		f.terminateCalls = append(f.terminateCalls, terminateFactoryTargetCall{sessionID: sessionID, request: control})
	case factorysessions.SessionControlClose:
		f.closeCalls = append(f.closeCalls, sessionID)
		if len(f.closeCalls) == 1 && f.closeEntered != nil {
			close(f.closeEntered)
		}
		release, err = closeRelease, closeErr
	case factorysessions.SessionControlCancel:
		f.cancelCalls = append(f.cancelCalls, cancelFactoryTargetCall{sessionID: sessionID, request: control})
		if len(f.cancelCalls) == 1 && f.cancelEntered != nil {
			close(f.cancelEntered)
		}
	default:
		f.mu.Unlock()
		return factorysessions.SessionControlResult{}, nil
	}
	f.mu.Unlock()
	if release != nil {
		<-release
	}
	if err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	return factorysessions.SessionControlResult{Outcome: outcome}, nil
}
func (f *fakeFactoryTargetService) CloseFactorySession(_ context.Context, sessionID string) error {
	f.mu.Lock()
	f.closeCalls = append(f.closeCalls, sessionID)
	if len(f.closeCalls) == 1 && f.closeEntered != nil {
		close(f.closeEntered)
	}
	release, err := f.closeRelease, f.closeErr
	f.mu.Unlock()
	if release != nil {
		<-release
	}
	return err
}
func (f *fakeFactoryTargetService) TerminateFactorySession(
	_ context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) error {
	f.mu.Lock()
	f.terminateCalls = append(f.terminateCalls, terminateFactoryTargetCall{sessionID: sessionID, request: request})
	release, err := f.closeRelease, f.closeErr
	f.mu.Unlock()
	if release != nil {
		<-release
	}
	return err
}
func (f *fakeFactoryTargetService) SubscribeFactoryResponseEvents(
	_ context.Context,
	_ factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.responseCursor, f.responseErr
}
func sequentialIDGenerator(prefix string) func() string {
	n := 0
	return func() string {
		n++
		return prefix + "-" + strconv.Itoa(n)
	}
}

func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

func newChatSessionsStore(prefix string) (chatsessions.Service, error) {
	return chatsessionswire.NewService(
		sequentialIDGenerator(prefix),
		fixedClock(time.Unix(0, 1)),
		stubEventsAppender{},
		stubEventsReader{},
	)
}

type stubEventsAppender struct{}

func (stubEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, nil
}

type stubEventsReader struct{}

func (stubEventsReader) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, nil
}
func promptTextParams(sessionID, text string) string {
	raw, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}
func setConfigOptionParams(sessionID, value string) string {
	return `{"sessionId":"` + sessionID + `","configId":"target","value":"` + value + `"}`
}
func sessionAt(id string, target string, version uint64, workingRoot string) chatsessions.GetSessionResult {
	now := time.Unix(0, 1)
	return chatsessions.GetSessionResult{Session: chatsessions.Session{
		ID:    id,
		State: chatsessions.SessionStateCreated,
		SelectedTarget: chatsessions.ChatTargetRef{
			Kind: chatsessions.ChatTargetKindFactory,
			Ref:  target,
		},
		Version:     version,
		WorkingRoot: workingRoot,
		CreatedAt:   now,
		UpdatedAt:   now,
	}}
}
func catalogResultWithCurrent(current string) chatsessions.ResolveFactoryTargetCatalogResult {
	return chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: current,
		Choices: []chatsessions.FactoryTargetCatalogChoice{
			{Value: "factory:@you/factory-builder", Name: "Factory Builder"},
			{Value: "factory:@you/review", Name: "Review"},
		},
	}
}

const validSessionNewParams = `{"cwd":"/work/project","mcpServers":[]}`

func newTestServer(chatSessions *fakeChatSessionsService, catalog *fakeFactoryTargetCatalogService, homeDir string) *Server {
	return newTestServerWithFactoryTarget(chatSessions, catalog, nil, homeDir)
}
func newTestServerWithFactoryTarget(
	chatSessions *fakeChatSessionsService,
	catalog *fakeFactoryTargetCatalogService,
	factoryTarget factorysessions.Service,
	homeDir string,
) *Server {
	resolveHomeDir := func() (string, error) { return homeDir, nil }
	return New(testLogger(),
		chatSessions, catalog, factoryTarget, nil, resolveHomeDir, nil, nil, testStartResolver, testInvocationScope,
	)
}
func newTestServerWithResponseBridge(
	chatSessions *fakeChatSessionsService,
	catalog *fakeFactoryTargetCatalogService,
	factoryTarget factorysessions.Service,
	homeDir string,
	responseBridge acp.ResponseBridge,
) *Server {
	resolveHomeDir := func() (string, error) { return homeDir, nil }
	return New(testLogger(),
		chatSessions, catalog, factoryTarget, nil, resolveHomeDir, responseBridge, nil, testStartResolver, testInvocationScope,
	)
}
func numberIdentityEnvelope(t *testing.T, connID identity.ConnectionID, wireID int64, method string, params string) envelope.Envelope {
	t.Helper()
	reqIdentity, err := identity.NewCorrelated(connID, identity.NewNumberJSONRPCID(wireID))
	if err != nil {
		t.Fatalf("NewCorrelated: %v", err)
	}
	return envelope.Envelope{Identity: reqIdentity, Method: method, Params: json.RawMessage(params)}
}
func mintedIdentityEnvelope(t *testing.T, method string, params string) envelope.Envelope {
	t.Helper()
	minted, err := identity.NewMinted("transport-minted-id")
	if err != nil {
		t.Fatalf("identity.NewMinted: %v", err)
	}
	return envelope.Envelope{Identity: minted, Method: method, Params: json.RawMessage(params)}
}
func wantAdvanceTurnSequence(t *testing.T, chatSessions *fakeChatSessionsService, sessionID, turnID string, want ...chatsessions.TurnState) {
	t.Helper()
	if len(chatSessions.advanceTurnReqs) != len(want) {
		t.Fatalf("AdvanceTurn call count = %d, want %d (%v)", len(chatSessions.advanceTurnReqs), len(want), want)
	}
	for i, req := range chatSessions.advanceTurnReqs {
		if req.SessionID != sessionID {
			t.Fatalf("AdvanceTurn[%d] SessionID = %q, want %q", i, req.SessionID, sessionID)
		}
		if req.TurnID != turnID {
			t.Fatalf("AdvanceTurn[%d] TurnID = %q, want %q", i, req.TurnID, turnID)
		}
		if req.Next != want[i] {
			t.Fatalf("AdvanceTurn[%d] Next = %q, want %q", i, req.Next, want[i])
		}
	}
}
func newActiveBoundControlSession(t *testing.T, factorySessionID string) (chatsessions.Service, chatsessions.Session, chatsessions.Turn) {
	t.Helper()
	store, err := newChatSessionsStore("control")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	created, err := store.CreateSession(context.Background(), chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "control-setup", JSONRPCNumberID: "1"},
		WorkingRoot:   "/work/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	started, err := store.StartTurn(context.Background(), chatsessions.StartTurnRequest{
		RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCNumber, ConnectionID: "control-setup", JSONRPCNumberID: "2"},
		SessionID:       created.Session.ID,
		ExpectedVersion: created.Session.Version,
	})
	if err != nil {
		t.Fatalf("StartTurn() error = %v", err)
	}
	if _, err := store.AdvanceTurn(context.Background(), chatsessions.AdvanceTurnRequest{
		SessionID: started.Session.ID, TurnID: started.Turn.ID, Next: chatsessions.TurnStateRunning,
	}); err != nil {
		t.Fatalf("AdvanceTurn(RUNNING) error = %v", err)
	}
	bound, err := store.BindFactorySession(context.Background(), chatsessions.BindFactorySessionRequest{
		SessionID: started.Session.ID, ExpectedVersion: started.Session.Version,
		Episode: started.Episode.Number, TurnID: started.Turn.ID, FactorySessionID: factorySessionID,
	})
	if err != nil {
		t.Fatalf("BindFactorySession() error = %v", err)
	}
	return store, bound.Session, chatsessions.Turn{ID: started.Turn.ID, Episode: started.Turn.Episode, State: chatsessions.TurnStateRunning}
}
func admittedTurnResult(id, target string, version uint64, workingRoot, turnID, factorySessionID string) chatsessions.StartTurnResult {
	targetRef := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: target}
	return chatsessions.StartTurnResult{
		Session: chatsessions.Session{
			ID: id, State: chatsessions.SessionStateActive,
			SelectedTarget: targetRef, TargetEpisode: 1, ActiveTurnID: turnID,
			Version: version, WorkingRoot: workingRoot,
		},
		Turn: chatsessions.Turn{
			ID: turnID, Episode: 1, State: chatsessions.TurnStateAdmitted,
		},
		Episode: chatsessions.TargetEpisode{
			Number: 1, State: chatsessions.TargetEpisodeStateOpen,
			Target: targetRef, FactorySessionID: factorySessionID,
		},
	}
}

type controlRecordingChatSessions struct {
	chatsessions.Service
	requestEntered chan struct{}
	requestRelease <-chan struct{}
	requestOnce    sync.Once
	commitEntered  chan struct{}
	commitRelease  <-chan struct{}
	commitOnce     sync.Once
	mu             sync.Mutex
	requests       []chatsessions.RequestControlRequest
	advances       []chatsessions.AdvanceControlResult
}

func (s *controlRecordingChatSessions) RequestControl(
	ctx context.Context,
	req chatsessions.RequestControlRequest,
) (chatsessions.RequestControlResult, error) {
	if s.requestEntered != nil {
		s.requestOnce.Do(func() { close(s.requestEntered) })
		if s.requestRelease != nil {
			<-s.requestRelease
		}
	}
	result, err := s.Service.RequestControl(ctx, req)
	if err == nil {
		s.mu.Lock()
		s.requests = append(s.requests, req)
		s.mu.Unlock()
	}
	return result, err
}
func (s *controlRecordingChatSessions) AdvanceControl(
	ctx context.Context,
	req chatsessions.AdvanceControlRequest,
) (chatsessions.AdvanceControlResult, error) {
	if req.Next == chatsessions.ControlIntentStateCommitted && s.commitEntered != nil {
		s.commitOnce.Do(func() { close(s.commitEntered) })
		if s.commitRelease != nil {
			<-s.commitRelease
		}
	}
	result, err := s.Service.AdvanceControl(ctx, req)
	if err == nil {
		s.mu.Lock()
		s.advances = append(s.advances, result)
		s.mu.Unlock()
	}
	return result, err
}
func (s *controlRecordingChatSessions) snapshotControls() (
	[]chatsessions.RequestControlRequest,
	[]chatsessions.AdvanceControlResult,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	requests := append([]chatsessions.RequestControlRequest(nil), s.requests...)
	advances := append([]chatsessions.AdvanceControlResult(nil), s.advances...)
	return requests, advances
}
func waitForChannel(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}
