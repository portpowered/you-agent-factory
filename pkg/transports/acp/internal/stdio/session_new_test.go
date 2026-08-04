package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// fakeChatSessionsService is a minimal chatsessions.Service test double.
// CreateSession, GetSession, SetTarget, and StartTurn are configurable and
// tracked -- the methods this package's session/new,
// session/set_config_option, and session/prompt handlers actually call.
// Every other method fails loudly: no handler calls them, and a call to one
// would itself be a defect worth catching. mu guards every field so the
// fake is safe under the concurrent/duplicate-delivery admission tests that
// call it from multiple goroutines against the same instance.
type fakeChatSessionsService struct {
	mu sync.Mutex

	createCalled bool
	created      chatsessions.CreateSessionRequest
	createErr    error
	sessionID    string

	getSessionCalled bool
	getSessionReq    chatsessions.GetSessionRequest
	getSessionReqs   []chatsessions.GetSessionRequest
	getSessionResult chatsessions.GetSessionResult
	// getSessionResults, when non-empty, is consumed front-first across
	// successive GetSession calls (one queued result per call) instead of
	// the single static getSessionResult -- for tests where a later
	// GetSession call (for example currentSessionVersion's fresh re-read
	// after a dispatch that may have advanced Session.Version concurrently)
	// must observe a different session snapshot than the admission-time
	// call did.
	getSessionResults []chatsessions.GetSessionResult
	getSessionErr     error

	setTargetCalled bool
	setTargetReq    chatsessions.SetTargetRequest
	setTargetResult chatsessions.SetTargetResult
	setTargetErr    error

	startTurnCalled bool
	startTurnReq    chatsessions.StartTurnRequest
	startTurnReqs   []chatsessions.StartTurnRequest
	startTurnResult chatsessions.StartTurnResult
	// startTurnResults, when non-empty, is consumed front-first across
	// successive StartTurn calls (one queued result per call) instead of the
	// single static startTurnResult -- for sequential tests that need a
	// later call to observe a different admitted episode snapshot (e.g. a
	// later turn's episode already carrying a Factory Session ID bound by an
	// earlier call).
	startTurnResults []chatsessions.StartTurnResult
	startTurnErr     error

	bindFactorySessionCalled bool
	bindFactorySessionReq    chatsessions.BindFactorySessionRequest
	bindFactorySessionReqs   []chatsessions.BindFactorySessionRequest
	bindFactorySessionResult chatsessions.BindFactorySessionResult
	// bindFactorySessionErrs, when non-empty, is consumed front-first across
	// successive BindFactorySession calls (one queued error per call, nil
	// meaning that call succeeds) instead of the single static
	// bindFactorySessionErr -- for retry tests that need one specific bind
	// attempt to fail while a later retry against the same pending identity
	// succeeds.
	bindFactorySessionErrs []error
	bindFactorySessionErr  error

	recordPendingFactorySessionCalled bool
	recordPendingFactorySessionReq    chatsessions.RecordPendingFactorySessionRequest
	recordPendingFactorySessionReqs   []chatsessions.RecordPendingFactorySessionRequest
	recordPendingFactorySessionResult chatsessions.RecordPendingFactorySessionResult
	// recordPendingFactorySessionErrs, when non-empty, is consumed front-first
	// across successive RecordPendingFactorySession calls (one queued error
	// per call, nil meaning that call succeeds) instead of the single static
	// recordPendingFactorySessionErr -- for retry tests that need one
	// specific record-pending attempt to fail while a later retry for the
	// same still-unbound episode succeeds.
	recordPendingFactorySessionErrs []error
	recordPendingFactorySessionErr  error

	advanceTurnCalled bool
	advanceTurnReq    chatsessions.AdvanceTurnRequest
	advanceTurnReqs   []chatsessions.AdvanceTurnRequest
	// advanceTurnErrs, when non-empty, is consumed front-first across
	// successive AdvanceTurn calls (one queued error per call, nil meaning
	// that call succeeds) instead of the single static advanceTurnErr -- for
	// fault-injection tests that need one specific AdvanceTurn call (e.g. the
	// admission-time transition to RUNNING) to succeed while a later one
	// (e.g. the terminal transition) fails, or vice versa.
	advanceTurnErrs []error
	advanceTurnErr  error

	// attachments and nextAttachmentID back a working, in-memory Attach/
	// AcknowledgeAttachment pair (unlike every other method below, which
	// fails loudly): streaming tests in prompt_stream_test.go need a real,
	// stateful attachment cursor to drive streamTurnUpdates/drainRecords
	// against, without pulling in the real chatsessions.Service
	// implementation (pkg/boundary forbids a transport test constructing a
	// product service directly -- see prompt_stream_test.go's own doc
	// comment). This intentionally does not enforce ExpectedVersion or
	// StreamHead the way the real Store does; those guards are chat_sessions'
	// own responsibility and are already covered by that service's own test
	// suite. This fake only needs to prove this transport package calls
	// Attach/AcknowledgeAttachment correctly and reacts correctly to their
	// results.
	attachments      map[string]chatsessions.Attachment
	nextAttachmentID int
	// attachErr, when set, fails every Attach call -- for tests proving
	// ensureAttachment/streamTurnUpdates propagate a genuine Factory Sessions
	// Attach failure rather than treating it like the ordinary "no attachment
	// yet" (ok=false) case.
	attachErr error
	// detachCalls records every Detach call this fake observed, in order --
	// disconnect-cleanup tests assert both which attachments were released
	// and that a turn-mutating method was never reached alongside them.
	detachCalls []chatsessions.DetachRequest
	detachErr   error
	// acknowledgeAttachmentErr, when set, fails every AcknowledgeAttachment
	// call with this exact error -- for tests proving drainRecords/
	// deliverReadTimeGap propagate a genuine (non-*AttachmentPositionError)
	// AcknowledgeAttachment failure instead of swallowing it.
	acknowledgeAttachmentErr error
	// acknowledgeAttachmentErrs, when non-empty, is consumed front-first
	// across successive AcknowledgeAttachment calls. A nil entry permits that
	// call to succeed, so retry tests can model one stale optimistic version
	// followed by a successful refreshed acknowledgement.
	acknowledgeAttachmentErrs []error
	// acknowledgeAttachmentReqs records each attempt so retry tests can prove
	// the refreshed session version reaches the next acknowledgement.
	acknowledgeAttachmentReqs []chatsessions.AcknowledgeAttachmentRequest
	// acknowledgeAttachmentPositionErr, when true, fails every
	// AcknowledgeAttachment call with a *chatsessions.AttachmentPositionError
	// -- the StreamHead-lag case drainRecords/deliverReadTimeGap treat as a
	// non-error "stop" rather than a failure.
	acknowledgeAttachmentPositionErr bool
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

// Attach mirrors the real resume policy: select the stored identity or sole candidate; reject a tie.
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

// Detach mirrors the real chatsessions.Service.Detach contract closely
// enough for disconnect-cleanup and reconnect tests: it marks the named
// attachment Detached in place (preserving its ID and AfterSequence for a
// later Resume, exactly like the real Store -- see Attach above) rather than
// removing it, and reports *chatsessions.NotFoundError for an attachment ID
// this fake never registered, matching the real Service's own
// unknown-attachment behavior.
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

func (f *fakeChatSessionsService) RequestControl(context.Context, chatsessions.RequestControlRequest) (chatsessions.RequestControlResult, error) {
	return chatsessions.RequestControlResult{}, errors.New("fakeChatSessionsService: RequestControl not implemented")
}

func (f *fakeChatSessionsService) AdvanceControl(context.Context, chatsessions.AdvanceControlRequest) (chatsessions.AdvanceControlResult, error) {
	return chatsessions.AdvanceControlResult{}, errors.New("fakeChatSessionsService: AdvanceControl not implemented")
}

func (f *fakeChatSessionsService) Sequence(context.Context, chatsessions.SequenceRequest) (chatsessions.SequenceResult, error) {
	return chatsessions.SequenceResult{}, errors.New("fakeChatSessionsService: Sequence not implemented")
}

func (f *fakeChatSessionsService) AdvanceStreamHead(context.Context, chatsessions.AdvanceStreamHeadRequest) (chatsessions.AdvanceStreamHeadResult, error) {
	return chatsessions.AdvanceStreamHeadResult{}, errors.New("fakeChatSessionsService: AdvanceStreamHead not implemented")
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

// fakeFactoryTargetCatalogService is a minimal
// chatsessions.FactoryTargetCatalogService test double.
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

// fakeFactoryTargetService is a minimal
// factorysessions.TargetExecutionService test double. mu guards every field
// for concurrent/duplicate-delivery tests.
type fakeFactoryTargetService struct {
	mu sync.Mutex

	startCalls  []factorysessions.StartRequest
	startResult factorysessions.AsyncStartResult
	startErr    error

	invokeCalls  []invokeFactoryTargetCall
	invokeResult factorysessions.InvocationResult
	invokeErr    error
	// invokeErrs, when non-empty, is consumed front-first across successive
	// InvokeFactorySession calls (one queued error per call, nil meaning that
	// call succeeds with invokeResult) instead of the single static
	// invokeErr -- for tests that need one specific dispatch to fail while a
	// later retry against the same (or a differently bound) identity
	// succeeds.
	invokeErrs []error
	// invokeEnter and invokeRelease, when both non-nil, make
	// InvokeFactorySession block: it closes invokeEnter the instant it is
	// entered (so a caller can deterministically wait, with no sleep, until
	// the call is genuinely in flight) and then blocks until invokeRelease
	// is itself closed before returning its configured result/error. This is
	// how a test proves a concurrently-dispatched "session/cancel" reaches
	// Cancel while an in-flight "session/prompt" is still blocked in its own
	// InvokeFactorySession call, rather than only after it already returned.
	invokeEnter   chan struct{}
	invokeRelease chan struct{}

	cancelCalls []cancelFactoryTargetCall
	cancelErr   error

	responseCursor *factorysessions.ResponseEventCursor
	responseErr    error
	// cancelEntered, when non-nil, is closed the instant the first Cancel
	// call is recorded, so a test can deterministically wait, with no sleep,
	// until cancellation has genuinely reached this fake rather than merely
	// having been written to the connection's input stream.
	cancelEntered chan struct{}

	closeCalls []string
	closeErr   error
}

type cancelFactoryTargetCall struct {
	sessionID string
	request   factorysessions.ControlRequest
}

type invokeFactoryTargetCall struct {
	sessionID string
	request   factorysessions.InvocationRequest
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
	defer f.mu.Unlock()
	f.cancelCalls = append(f.cancelCalls, cancelFactoryTargetCall{sessionID: sessionID, request: request})
	if len(f.cancelCalls) == 1 && f.cancelEntered != nil {
		close(f.cancelEntered)
	}
	if f.cancelErr != nil {
		return factorysessions.LifecycleControlResult{}, f.cancelErr
	}
	return factorysessions.LifecycleControlResult{}, nil
}

func (f *fakeFactoryTargetService) CloseFactorySession(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls = append(f.closeCalls, sessionID)
	return f.closeErr
}

func (f *fakeFactoryTargetService) SubscribeFactoryResponseEvents(
	_ context.Context,
	_ factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.responseCursor, f.responseErr
}

func defaultTestCatalogResult() chatsessions.ResolveFactoryTargetCatalogResult {
	return chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/factory-builder",
		Choices: []chatsessions.FactoryTargetCatalogChoice{
			{Value: "factory:@you/factory-builder", Name: "Factory Builder"},
			{Value: "factory:@you/review", Name: "Review"},
		},
	}
}

func newTestServer(chatSessions *fakeChatSessionsService, catalog *fakeFactoryTargetCatalogService, homeDir string) *Server {
	return newTestServerWithFactoryTarget(chatSessions, catalog, nil, homeDir)
}

// newTestServerWithFactoryTarget is newTestServer plus an explicit Factory
// Sessions delegation collaborator, for the ordinary-prompt-delegation tests
// that need to observe or fail StartAsync/InvokeFactorySession calls.
func newTestServerWithFactoryTarget(
	chatSessions *fakeChatSessionsService,
	catalog *fakeFactoryTargetCatalogService,
	factoryTarget factorysessions.TargetExecutionService,
	homeDir string,
) *Server {
	resolveHomeDir := func() (string, error) { return homeDir, nil }
	return New(nil, chatSessions, catalog, factoryTarget, nil, resolveHomeDir, nil)
}

// newTestServerWithResponseBridge is newTestServerWithFactoryTarget plus an
// explicit acp.ResponseBridge collaborator, for tests that need to observe
// dispatchFactoryInvocation actually reaching the injected response bridge
// (rather than calling InvokeFactorySession directly, the responseBridge==nil
// no-op path every other test in this package already exercises).
func newTestServerWithResponseBridge(
	chatSessions *fakeChatSessionsService,
	catalog *fakeFactoryTargetCatalogService,
	factoryTarget factorysessions.TargetExecutionService,
	homeDir string,
	responseBridge acp.ResponseBridge,
) *Server {
	resolveHomeDir := func() (string, error) { return homeDir, nil }
	return New(nil, chatSessions, catalog, factoryTarget, nil, resolveHomeDir, responseBridge)
}

func numberIdentityEnvelope(t *testing.T, connID identity.ConnectionID, wireID int64, method string, params string) envelope.Envelope {
	t.Helper()
	reqIdentity, err := identity.NewCorrelated(connID, identity.NewNumberJSONRPCID(wireID))
	if err != nil {
		t.Fatalf("NewCorrelated: %v", err)
	}
	return envelope.Envelope{Identity: reqIdentity, Method: method, Params: json.RawMessage(params)}
}

func stringIdentityEnvelope(t *testing.T, connID identity.ConnectionID, wireID string, method string, params string) envelope.Envelope {
	t.Helper()
	reqIdentity, err := identity.NewCorrelated(connID, identity.NewStringJSONRPCID(wireID))
	if err != nil {
		t.Fatalf("NewCorrelated: %v", err)
	}
	return envelope.Envelope{Identity: reqIdentity, Method: method, Params: json.RawMessage(params)}
}

const validSessionNewParams = `{"cwd":"/work/project","mcpServers":[]}`

func TestHandleSessionNewRejectsNonEmptyMcpServersBeforeAnyEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew,
		`{"cwd":"/work/project","mcpServers":[{"name":"x","command":"y"}]}`)

	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for non-empty mcpServers")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0: mcpServers rejection must happen before catalog resolution", len(catalog.calls))
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation for a rejected mcpServers request")
	}
}

func TestHandleSessionNewCreatesOneSessionAndReturnsProjectedPicker(t *testing.T) {
	chatSessions := &fakeChatSessionsService{sessionID: "session-42"}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	connID := identity.NewConnectionID()
	env := numberIdentityEnvelope(t, connID, 7, acpsdk.AgentMethodSessionNew, validSessionNewParams)

	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr != nil {
		t.Fatalf("handleSessionNew() error = %+v, want success", rpcErr)
	}

	var resp acpsdk.NewSessionResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if string(resp.SessionId) != "session-42" {
		t.Fatalf("sessionId = %q, want session-42", resp.SessionId)
	}
	if len(resp.ConfigOptions) != 1 || resp.ConfigOptions[0].Select == nil {
		t.Fatalf("configOptions = %+v, want exactly one select option", resp.ConfigOptions)
	}
	option := resp.ConfigOptions[0].Select
	if option.Id != "target" || option.Name != "Factory" || option.Type != "select" {
		t.Fatalf("option = %+v, want id=target name=Factory type=select", option)
	}
	if string(option.CurrentValue) != "factory:@you/factory-builder" {
		t.Fatalf("currentValue = %q, want factory:@you/factory-builder", option.CurrentValue)
	}
	if option.Options.Ungrouped == nil || len(*option.Options.Ungrouped) != 2 {
		t.Fatalf("options = %+v, want 2 ungrouped choices", option.Options)
	}

	if len(catalog.calls) != 1 {
		t.Fatalf("catalog resolved %d times, want exactly 1", len(catalog.calls))
	}
	if catalog.calls[0].ClientWorkingRoot != "/work/project" {
		t.Fatalf("ClientWorkingRoot = %q, want the validated editor cwd /work/project", catalog.calls[0].ClientWorkingRoot)
	}

	if !chatSessions.createCalled {
		t.Fatal("CreateSession was not called, want exactly one session creation")
	}
	wantTarget := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/factory-builder"}
	if chatSessions.created.InitialTarget != wantTarget {
		t.Fatalf("InitialTarget = %+v, want %+v", chatSessions.created.InitialTarget, wantTarget)
	}
	if chatSessions.created.WorkingRoot != "/work/project" {
		t.Fatalf("WorkingRoot = %q, want the validated editor cwd /work/project", chatSessions.created.WorkingRoot)
	}

	wantIdentity := chatsessions.RequestIdentity{
		Kind:            chatsessions.RequestIdentityKindJSONRPCNumber,
		ConnectionID:    string(connID),
		JSONRPCNumberID: "7",
	}
	if chatSessions.created.RequestID != wantIdentity {
		t.Fatalf("RequestID = %+v, want %+v", chatSessions.created.RequestID, wantIdentity)
	}
}

func TestHandleSessionNewConvertsStringWireIDs(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	connID := identity.NewConnectionID()
	env := stringIdentityEnvelope(t, connID, "req-abc", acpsdk.AgentMethodSessionNew, validSessionNewParams)

	if _, rpcErr := server.handleSessionNew(context.Background(), env); rpcErr != nil {
		t.Fatalf("handleSessionNew() error = %+v, want success", rpcErr)
	}

	wantIdentity := chatsessions.RequestIdentity{
		Kind:            chatsessions.RequestIdentityKindJSONRPCString,
		ConnectionID:    string(connID),
		JSONRPCStringID: "req-abc",
	}
	if chatSessions.created.RequestID != wantIdentity {
		t.Fatalf("RequestID = %+v, want %+v", chatSessions.created.RequestID, wantIdentity)
	}
}

func TestHandleSessionNewKeepsEqualWireIDsDistinctAcrossConnections(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	firstConn := identity.NewConnectionID()
	firstEnv := numberIdentityEnvelope(t, firstConn, 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	if _, rpcErr := server.handleSessionNew(context.Background(), firstEnv); rpcErr != nil {
		t.Fatalf("first handleSessionNew() error = %+v", rpcErr)
	}
	firstIdentity := chatSessions.created.RequestID

	secondConn := identity.NewConnectionID()
	secondEnv := numberIdentityEnvelope(t, secondConn, 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	if _, rpcErr := server.handleSessionNew(context.Background(), secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionNew() error = %+v", rpcErr)
	}
	secondIdentity := chatSessions.created.RequestID

	if firstIdentity == secondIdentity {
		t.Fatalf("wire id 1 on two different connections converted to the same RequestIdentity %+v", firstIdentity)
	}
}

func TestHandleSessionNewCatalogFailureReturnsNoSuccessAndNoSessionCreation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{Err: chatsessions.ErrFactoryTargetCatalogUnavailable}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for a catalog failure")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on catalog failure", result)
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation after a catalog failure")
	}
}

func TestHandleSessionNewCreateSessionFailureReturnsNoSuccess(t *testing.T) {
	chatSessions := &fakeChatSessionsService{createErr: errors.New("create session boom")}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for a session creation failure")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on session creation failure", result)
	}
}

func TestHandleSessionNewFailureNeverLeaksRawCause(t *testing.T) {
	sensitive := "sk_live_should_never_leak"
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{err: errors.New(sensitive)}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	_, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection")
	}
	encoded, err := json.Marshal(rpcErr)
	if err != nil {
		t.Fatalf("marshal rpc error: %v", err)
	}
	if strings.Contains(string(encoded), sensitive) {
		t.Fatalf("rpc error %s leaked the raw dependency cause", encoded)
	}
}

// TestServeDispatchesSessionNewOverRealJSONRPCFraming proves session/new is
// reachable through the same newline-delimited JSON-RPC stdio framing
// "initialize" already uses, not just through handleSessionNew called
// directly: the whole Serve -> scan line -> dispatchRequest ->
// handleSessionNew -> writeResponse path produces one complete, correctly
// correlated, successful JSON-RPC response.
func TestServeDispatchesSessionNewOverRealJSONRPCFraming(t *testing.T) {
	chatSessions := &fakeChatSessionsService{sessionID: "session-real"}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	input := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/work/project","mcpServers":[]}}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	resp := assertSingleResponseLine(t, out)
	if string(resp.ID) != "1" {
		t.Fatalf("id = %s, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("error = %+v, want a successful result", resp.Error)
	}

	var sessionResp acpsdk.NewSessionResponse
	if err := json.Unmarshal(resp.Result, &sessionResp); err != nil {
		t.Fatalf("unmarshal session/new result: %v", err)
	}
	if string(sessionResp.SessionId) != "session-real" {
		t.Fatalf("sessionId = %q, want session-real", sessionResp.SessionId)
	}
	if !chatSessions.createCalled {
		t.Fatal("CreateSession was not called over the real Serve path")
	}
}

// TestServeRespondsMethodNotFoundForEveryUnimplementedMethodStillExcludesSessionNew
// guards against a regression that would put "session/new" back on the
// unimplemented-methods list in TestServeRespondsMethodNotFoundForEveryUnimplementedMethod:
// a server with no session/new collaborators configured must still dispatch
// the method (and report a bounded internal failure), never method-not-found.
func TestServeRespondsMethodNotFoundForEveryUnimplementedMethodStillExcludesSessionNew(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":9,"method":"session/new","params":{"cwd":"/work/project","mcpServers":[]}}` + "\n"
	out := &bytes.Buffer{}
	server := New(nil, nil, nil, nil, nil, nil, nil)
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	resp := assertSingleResponseLine(t, out)
	if resp.Error == nil {
		t.Fatal("error = nil, want a bounded failure for an unconfigured session/new server")
	}
	if resp.Error.Code == -32601 {
		t.Fatal("error code = method-not-found (-32601), want session/new to be dispatched, not rejected as unsupported")
	}
}

func TestHandleSessionNewWithoutCollaboratorsReportsBoundedFailure(t *testing.T) {
	server := New(nil, nil, nil, nil, nil, nil, nil)
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)

	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a bounded failure when collaborators are unset")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil", result)
	}
}

// mintedIdentityEnvelope builds an envelope carrying a transport-minted
// (non-connection-correlated) identity, the one RequestIdentity shape
// chatRequestIdentity always rejects: "session/new" and "session/prompt" are
// always real inbound JSON-RPC requests, so they only ever see a correlated
// identity in production, but the boundary must still fail closed rather
// than panic or fabricate a connection if it ever saw one.
func mintedIdentityEnvelope(t *testing.T, method string, params string) envelope.Envelope {
	t.Helper()
	minted, err := identity.NewMinted("transport-minted-id")
	if err != nil {
		t.Fatalf("identity.NewMinted: %v", err)
	}
	return envelope.Envelope{Identity: minted, Method: method, Params: json.RawMessage(params)}
}

func TestChatRequestIdentityRejectsNonCorrelatedIdentity(t *testing.T) {
	minted, err := identity.NewMinted("transport-minted-id")
	if err != nil {
		t.Fatalf("identity.NewMinted: %v", err)
	}
	if _, err := chatRequestIdentity(minted); err == nil {
		t.Fatal("chatRequestIdentity() error = nil, want a rejection for a non-connection-correlated identity")
	}
}

func TestHandleSessionNewIdentityFailureReturnsNoEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := mintedIdentityEnvelope(t, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection for a non-correlated identity")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0: identity conversion must fail before catalog resolution", len(catalog.calls))
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation for a rejected identity")
	}
}

func TestClassifyDependencyFailureMapsContextCauseToRequestCancelled(t *testing.T) {
	cancelled := classifyDependencyFailure(context.Canceled)
	deadlineExceeded := classifyDependencyFailure(fmt.Errorf("wrapped: %w", context.DeadlineExceeded))
	generic := classifyDependencyFailure(errors.New("boom"))

	wantCancelled := acpsdk.NewRequestCancelled(map[string]any{"reason": "cancelled"})
	if cancelled.Code != wantCancelled.Code {
		t.Fatalf("classifyDependencyFailure(context.Canceled) code = %d, want %d (request-cancelled)", cancelled.Code, wantCancelled.Code)
	}
	if deadlineExceeded.Code != wantCancelled.Code {
		t.Fatalf("classifyDependencyFailure(wrapped DeadlineExceeded) code = %d, want %d (request-cancelled)", deadlineExceeded.Code, wantCancelled.Code)
	}
	if generic.Code == wantCancelled.Code {
		t.Fatalf("classifyDependencyFailure(plain error) code = %d, want a code distinct from request-cancelled", generic.Code)
	}
}

func TestHandleSessionNewResolveHomeDirFailureReturnsNoEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := New(nil, chatSessions, catalog, nil, nil, func() (string, error) { return "", errors.New("resolve home dir boom") }, nil)

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection when resolveHomeDir fails")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 when resolveHomeDir fails", len(catalog.calls))
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation when resolveHomeDir fails")
	}
}

func TestHandleSessionNewBlankHomeDirFailureReturnsNoEffect(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: defaultTestCatalogResult()}
	server := New(nil, chatSessions, catalog, nil, nil, func() (string, error) { return "", nil }, nil)

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection when resolveHomeDir returns a blank home directory")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for a blank home directory", len(catalog.calls))
	}
}

func TestHandleSessionNewConfigProjectionFailureReturnsNoSessionCreation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{}
	catalog := &fakeFactoryTargetCatalogService{result: chatsessions.ResolveFactoryTargetCatalogResult{
		CurrentTarget: "factory:@you/factory-builder",
		Choices:       nil,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionNew, validSessionNewParams)
	result, rpcErr := server.handleSessionNew(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionNew() error = nil, want a rejection when the catalog projects no picker choices")
	}
	if result != nil {
		t.Fatalf("handleSessionNew() result = %q, want nil on rejection", result)
	}
	if chatSessions.createCalled {
		t.Fatal("CreateSession was called, want no session creation when picker projection fails")
	}
}
