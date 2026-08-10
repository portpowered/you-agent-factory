// Shared fixtures for the focused ACP stdio behavioral suites.
//
// Keep these package-local definitions in one file so moving tests between
// command, lifecycle, cancellation, and streaming suites does not duplicate
// setup behavior.
package stdio

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionswire "github.com/portpowered/infinite-you/pkg/services/chat_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// fakeChatSessionsService is a minimal chatsessions.Service test double.
// CreateSession, GetSession, SetTarget, turn operations, and control
// operations are configurable and tracked -- the methods this package's ACP
// handlers actually call. Every other method fails loudly: no handler calls
// them, and a call to one would itself be a defect worth catching. mu guards
// every field so the fake is safe under concurrent transport tests.
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
	// getSessionErrs, when non-empty, is consumed front-first across
	// successive GetSession calls. A nil entry lets that call return its
	// configured result, so tests can model a successful capture followed by a
	// failed re-read without making the initial control capture fail too.
	getSessionErrs []error
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

	requestControlReqs   []chatsessions.RequestControlRequest
	requestControlResult chatsessions.RequestControlResult
	requestControlErr    error
	lastControlIntent    chatsessions.ControlIntent

	advanceControlReqs   []chatsessions.AdvanceControlRequest
	advanceControlResult chatsessions.AdvanceControlResult
	// advanceControlResults, when non-empty, is consumed front-first across
	// successive calls so lifecycle tests can prove a final terminalization
	// rejection after an otherwise committed control.
	advanceControlResults []chatsessions.AdvanceControlResult
	advanceControlErr     error
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
	// cancelRelease, when non-nil, keeps Cancel in flight until the test
	// closes it. Together with cancelEntered this lets tests observe the
	// committed Chat control before its downstream effect returns.
	cancelRelease chan struct{}

	closeCalls     []string
	closeErr       error
	terminateCalls []terminateFactoryTargetCall
	// closeEntered and closeRelease mirror the deterministic Cancel controls:
	// they expose the exact point a captured Factory Session close begins so
	// a test can assert Chat's CLOSE intent is committed before fan-out.
	closeEntered chan struct{}
	closeRelease chan struct{}
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

func (f *fakeFactoryTargetService) SubscribeFactoryResponseEvents(
	_ context.Context,
	_ factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.responseCursor, f.responseErr
}

// fakeEventsService is a minimal events.Service test double, matching this
// package's existing fake convention (fakeChatSessionsService,
// fakeFactoryTargetService): Read and Subscribe are implemented (the two
// callers in this package that ever reach an events.Service --
// streamTurnUpdates uses only Read, liveDrainTurnUpdates uses only
// Subscribe). Embedding the interface unimplemented for the rest means a
// call to Append/AttachSource reaches a nil method value and panics, proving
// this package's streaming code never dispatches to either. It is backed by
// an in-memory, per-topic append-only record log with real
// events.AggregateSequence positions, so drainRecords' pagination and
// at-head/progress handling (via Read) and liveDrainTurnUpdates' delivery
// loop (via Subscribe) both run against real Cursor/ReadResult/Delivery
// semantics instead of a hand-simplified shortcut.
//
// This package deliberately does not wire a real events.Service/
// chatsessions.Service pair here (pkg/boundary forbids a transport test
// constructing a product service through its own wire package -- see
// docs/internal/baselines/transport-behavior-baseline.json's deletion-only
// contract): proving this transport's streaming behavior against the real,
// fully composed service graph is story ACP-L1-V2-T03-message-projectors-005's
// job ("prove streaming through the canonical application graph" via
// root.BuildProcess), not this unit-level package's.
type fakeEventsService struct {
	events.Service

	mu             sync.Mutex
	records        map[events.Topic][]events.Record
	evictedThrough map[events.Topic]events.AggregateSequence
	// cond wakes every blocked Subscription.Next call once seedRaw appends a
	// new record or a caller's context is canceled (see the small watcher
	// goroutine Subscribe spawns per wait to bridge ctx.Done into a
	// Broadcast). Lazily initialized by ensureCond so a fakeEventsService
	// literal that never calls Subscribe (the overwhelming majority of this
	// package's tests) pays no cost for it.
	cond *sync.Cond
	// subscribed is closed the first time Subscribe is called, letting a test
	// deterministically wait for a live drain's Subscribe call to actually
	// register before seeding a record it expects that drain to observe.
	// Lazily initialized (see ensureSubscribedChan) so either Subscribe or a
	// test's own waitForSubscriber call can safely be first.
	subscribed chan struct{}
	// readErr, when set, fails every Read call with this exact error -- for
	// tests proving streamTurnUpdates propagates a genuine Events.Read
	// dependency failure instead of swallowing it.
	readErr error
	// subscribeErr, when set, fails every Subscribe call with this exact
	// error -- for tests proving liveDrainTurnUpdates' own best-effort
	// no-op convention on a Subscribe failure.
	subscribeErr error
}

var _ events.Service = (*fakeEventsService)(nil)

// Read mirrors the real Store.Read's own outcome decision
// (pkg/services/events/internal/service/read.go), including its "from+1 ==
// earliest is not a gap" boundary, so a test that calls markEvictedThrough
// exercises streamTurnUpdates against the same events.ReadOutcomeGap/
// events.GapFacts shape production Read would report, not a
// hand-simplified stand-in.
func (f *fakeEventsService) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.readErr != nil {
		return events.ReadResult{}, f.readErr
	}

	all := f.records[req.Topic]
	head := events.AggregateSequence(len(all))
	earliest := f.evictedThrough[req.Topic] + 1
	retained := events.RetainedRange{Topic: req.Topic, Earliest: earliest, Head: head}

	from := req.From.Position
	switch {
	case from == head:
		return events.ReadResult{Outcome: events.ReadOutcomeAtHead, Next: req.From, Retained: retained}, nil
	case from > head:
		return events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor}, nil
	case earliest > 1 && from+1 < earliest:
		return events.ReadResult{
			Outcome: events.ReadOutcomeGap,
			Gap:     &events.GapFacts{Topic: req.Topic, Requested: from, EarliestRetained: earliest, Head: head},
		}, nil
	}

	start := int(from)
	end := min(start+req.Limit, len(all))
	page := all[start:end]
	next := events.Cursor{Topic: req.Topic, Position: page[len(page)-1].ID.Position}
	return events.ReadResult{Records: page, Next: next, Retained: retained, Outcome: events.ReadOutcomeProgress}, nil
}

// markEvictedThrough simulates retention having evicted every position up
// to and including through on sessionID's topic, so a later Read from a
// cursor at or before through observes events.ReadOutcomeGap the same way a
// real Events retention policy would report it.
func (f *fakeEventsService) markEvictedThrough(sessionID string, through events.AggregateSequence) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.evictedThrough == nil {
		f.evictedThrough = make(map[events.Topic]events.AggregateSequence)
	}
	f.evictedThrough[chatsessions.EventsTopic(sessionID)] = through
}

// seed appends one (kind, phase, payload) record onto sessionID's
// chat-session topic, in the exact chatsessions.SequencedItem envelope
// shape a real chatsessions.Service.Sequence call commits (see
// prompt_stream.go's package doc for why no such production caller exists
// yet), so streamTurnUpdates/drainRecords decode and project it exactly as
// they would a genuine committed record.
func (f *fakeEventsService) seed(t *testing.T, sessionID string, kind workers.Kind, phase workers.Phase, payload any) {
	t.Helper()
	f.seedItem(t, sessionID, "", "", kind, phase, payload)
}

// seedItem is seed plus explicit sequencer-assigned itemID/parentItemID, for
// tests that assert item lineage survives projection (Chat Sessions assigns
// stable item identity at sequencing time -- see final-proposal.md §5.3 --
// so a record observed through Events already carries it).
func (f *fakeEventsService) seedItem(t *testing.T, sessionID, itemID, parentItemID string, kind workers.Kind, phase workers.Phase, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal seed payload: %v", err)
	}
	itemRaw, err := json.Marshal(chatsessions.SequencedItem{
		ItemID: itemID, ParentItemID: parentItemID, Kind: kind, Phase: phase, Payload: raw,
	})
	if err != nil {
		t.Fatalf("marshal seed envelope: %v", err)
	}
	f.seedRaw(sessionID, itemRaw)
}

// seedMalformed appends one record onto sessionID's topic whose payload does
// not decode as chatsessions.SequencedItem, standing in for a committed
// record this transport cannot make sense of -- proving drainRecords rejects
// it as errMalformedSequencedEnvelope instead of panicking or silently
// skipping it.
func (f *fakeEventsService) seedMalformed(sessionID string) {
	f.seedRaw(sessionID, json.RawMessage(`{"kind":`))
}

func (f *fakeEventsService) seedRaw(sessionID string, payload json.RawMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	topic := chatsessions.EventsTopic(sessionID)
	if f.records == nil {
		f.records = make(map[events.Topic][]events.Record)
	}
	position := events.AggregateSequence(len(f.records[topic]) + 1)
	f.records[topic] = append(f.records[topic], events.Record{
		ID:      events.RecordID{Topic: topic, Position: position},
		Payload: payload,
	})
	if f.cond != nil {
		f.cond.Broadcast()
	}
}

// ensureCond lazily initializes cond under f.mu, safe to call from Subscribe
// or seedRaw regardless of call order.
func (f *fakeEventsService) ensureCond() {
	if f.cond == nil {
		f.cond = sync.NewCond(&f.mu)
	}
}

// ensureSubscribedChan lazily initializes and returns subscribed under f.mu,
// safe to call from Subscribe or waitForSubscriber regardless of call order.
func (f *fakeEventsService) ensureSubscribedChan() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscribed == nil {
		f.subscribed = make(chan struct{})
	}
	return f.subscribed
}

// waitForSubscriber blocks until Subscribe has been called at least once,
// letting a test deterministically know a live drain is already listening
// before it seeds a record the drain is expected to observe -- without a
// fixed sleep.
func (f *fakeEventsService) waitForSubscriber(t *testing.T) {
	t.Helper()
	select {
	case <-f.ensureSubscribedChan():
	case <-time.After(2 * time.Second):
		t.Fatal("fakeEventsService: no Subscribe call observed within timeout")
	}
}

// Subscribe returns a Subscription (a plain, blocking func(ctx) Delivery --
// see events.Subscription's own doc comment) that replays req.Topic's
// already-recorded events strictly in order starting after req.From, then
// blocks until seedRaw appends a new one or ctx is done. It mirrors Read's
// own retention-gap boundary (earliest > 1 && position+1 < earliest) so a
// test can exercise DeliveryGap the same way it exercises
// events.ReadOutcomeGap via markEvictedThrough.
func (f *fakeEventsService) Subscribe(_ context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	f.mu.Lock()
	if f.subscribeErr != nil {
		f.mu.Unlock()
		return nil, f.subscribeErr
	}
	f.ensureCond()
	f.mu.Unlock()

	subscribed := f.ensureSubscribedChan()
	select {
	case <-subscribed:
	default:
		close(subscribed)
	}

	topic := req.Topic
	position := req.From.Position
	return events.Subscription(func(ctx context.Context) events.Delivery {
		f.mu.Lock()
		defer f.mu.Unlock()
		for {
			all := f.records[topic]
			head := events.AggregateSequence(len(all))
			earliest := f.evictedThrough[topic] + 1
			switch {
			case earliest > 1 && position+1 < earliest:
				gap := &events.GapFacts{Topic: topic, Requested: position, EarliestRetained: earliest, Head: head}
				position = earliest - 1
				return events.Delivery{Kind: events.DeliveryGap, Gap: gap}
			case position < head:
				rec := all[int(position)]
				position = rec.ID.Position
				return events.Delivery{Kind: events.DeliveryRecord, Record: rec, Cursor: events.Cursor{Topic: topic, Position: rec.ID.Position}}
			}

			if ctx.Err() != nil {
				return events.Delivery{Kind: events.DeliveryCanceled}
			}
			waitDone := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					f.mu.Lock()
					f.cond.Broadcast()
					f.mu.Unlock()
				case <-waitDone:
				}
			}()
			f.cond.Wait()
			close(waitDone)
			if ctx.Err() != nil {
				return events.Delivery{Kind: events.DeliveryCanceled}
			}
		}
	}), nil
}

// sequentialIDGenerator returns a chatsessionswire.IDGenerator that produces
// prefix-1, prefix-2, ... on successive calls, for tests that construct a
// real chatsessions.Store and need deterministic, human-readable identities.
func sequentialIDGenerator(prefix string) chatsessionswire.IDGenerator {
	n := 0
	return func() string {
		n++
		return prefix + "-" + strconv.Itoa(n)
	}
}

// fixedClock returns a chatsessionswire.Clock that always reports at, for
// tests that construct a real chatsessions.Store and do not exercise
// timestamp behavior.
func fixedClock(at time.Time) chatsessionswire.Clock {
	return func() time.Time { return at }
}

// stubEventsAppender is a minimal chatsessionswire.EventsAppender double for
// tests that construct a real chatsessions.Store but do not themselves
// exercise Sequence.
type stubEventsAppender struct{}

func (stubEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, nil
}

// stubEventsReader is a minimal chatsessionswire.EventsReader double for
// tests that construct a real chatsessions.Store but do not themselves
// exercise AcknowledgeAttachment.
type stubEventsReader struct{}

func (stubEventsReader) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, nil
}

// promptTextParams builds a raw session/prompt params payload carrying one
// text content block, the shape a client sends for a typed "/factory
// <value>" command or an ordinary chat message alike.
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

// setConfigOptionParams builds a raw session/set_config_option value-id
// params payload addressing the Factory target select option.
func setConfigOptionParams(sessionID, value string) string {
	return `{"sessionId":"` + sessionID + `","configId":"target","value":"` + value + `"}`
}

// sessionAt builds a GetSessionResult for a session addressed by id, with the
// given selected target, version, and WorkingRoot -- mirroring what a prior
// successful "session/new" call would have produced.
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
	return New(nil, chatSessions, catalog, factoryTarget, nil, resolveHomeDir, nil, nil)
}

// newTestServerWithResponseBridge is newTestServerWithFactoryTarget plus an
// explicit acp.ResponseBridge collaborator, for tests that need to observe
// dispatchFactoryInvocation actually reaching the injected response bridge
// (rather than calling InvokeFactorySession directly, the responseBridge==nil
// no-op path every other test in this package already exercises, nil).
func newTestServerWithResponseBridge(
	chatSessions *fakeChatSessionsService,
	catalog *fakeFactoryTargetCatalogService,
	factoryTarget factorysessions.TargetExecutionService,
	homeDir string,
	responseBridge acp.ResponseBridge,
) *Server {
	resolveHomeDir := func() (string, error) { return homeDir, nil }
	return New(nil, chatSessions, catalog, factoryTarget, nil, resolveHomeDir, responseBridge, nil)
}

func numberIdentityEnvelope(t *testing.T, connID identity.ConnectionID, wireID int64, method string, params string) envelope.Envelope {
	t.Helper()
	reqIdentity, err := identity.NewCorrelated(connID, identity.NewNumberJSONRPCID(wireID))
	if err != nil {
		t.Fatalf("NewCorrelated: %v", err)
	}
	return envelope.Envelope{Identity: reqIdentity, Method: method, Params: json.RawMessage(params)}
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

const streamingTestSessionID = "session-1"

// newStreamingTestServer builds a Server against fakeChatSessionsService and
// fakeEventsService test doubles (not a real, wired chatsessions.Service/
// events.Service pair -- see fakeEventsService's own doc comment for why),
// with a session whose target episode is already bound to factorySessionID:
// every admitted turn in these tests reaches invokeFactorySessionForEpisode,
// not the start/bind branch, since that branch's own behavior is already
// covered by session_prompt_test.go and is orthogonal to what this file
// proves about streaming. turnIDs queues one chatsessions.StartTurnResult
// per call this test drives handleSessionPrompt through, letting a
// multi-turn test observe a fresh admitted Turn identity each time.
func newStreamingTestServer(t *testing.T, factoryTarget *fakeFactoryTargetService, turnIDs ...string) (*Server, *fakeEventsService) {
	t.Helper()

	session := chatsessions.Session{ID: streamingTestSessionID, Version: 1, WorkingRoot: "/work/project", TargetEpisode: 1}
	episode := chatsessions.TargetEpisode{
		Number:           1,
		Target:           chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/review"},
		FactorySessionID: "fs-1",
	}
	startTurnResults := make([]chatsessions.StartTurnResult, len(turnIDs))
	for i, turnID := range turnIDs {
		startTurnResults[i] = chatsessions.StartTurnResult{
			Session: session,
			Turn:    chatsessions.Turn{ID: turnID, State: chatsessions.TurnStateAdmitted},
			Episode: episode,
		}
	}

	chatSessions := &fakeChatSessionsService{
		getSessionResult: chatsessions.GetSessionResult{Session: session},
		startTurnResults: startTurnResults,
	}
	eventsSvc := &fakeEventsService{}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	resolveHomeDir := func() (string, error) { return "/home/operator", nil }
	server := New(nil, chatSessions, catalog, factoryTarget, eventsSvc, resolveHomeDir, nil, nil)
	return server, eventsSvc
}

func assistantMessagePayload(text string) workers.MessagePayload {
	return workers.MessagePayload{
		Role:          "assistant",
		ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: text}},
	}
}

func fallbackInvokeResult(text string) factorysessions.InvocationResult {
	return factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}},
	}
}

// TestStreamTurnUpdatesDeliversSeededMessageAndSuppressesV1Fallback proves a
// canonical MESSAGE record already sequenced onto the Chat Session topic
// before a turn dispatches is delivered as exactly one agent_message_chunk
// through streaming, and that the V1 synchronous final-text fallback never
// also fires -- even though the fake Factory Sessions outcome carries its
// own non-empty primary-result text.

// captureNotifier returns a promptNotifier that appends every notification
// it receives, and the slice it appends into.
func captureNotifier() (promptNotifier, *[]acpsdk.SessionNotification) {
	var notified []acpsdk.SessionNotification
	return func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}, &notified
}

// TestStreamTurnUpdatesTwoIndependentAttachmentsObserveIdenticalRecordsWithOneExecution
// proves story ACP-L1-V2-T03-message-projectors-004's AC1: two attachments
// to one Chat Session -- connection A driving the actual "session/prompt"
// turn (the only Factory execution), and connection B independently
// draining the same already-sequenced records through its own attachment
// and cursor, never itself dispatching a Factory turn -- observe the exact
// same eligible records, in the same order, with the same sequencer-assigned
// ItemID, while each attachment's own delivery cursor advances
// independently of the other's.

// wantAdvanceTurnSequence asserts AdvanceTurn was called exactly once per
// entry in want, in order, each against sessionID/turnID, ending with the
// corresponding TurnState -- proving an admitted turn passes through the
// legal RUNNING intermediate state on its way to an explicit terminal state,
// never skipped and never left non-terminal.
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

// TestHandleSessionPromptFirstTurnAdvancesByInvokeOutcome proves a
// first-turn (start) admission's terminal advancement tracks the follow-up
// InvokeFactorySession call's own published terminal status exactly, the same
// way a later (invoke) turn's does -- not a hardcoded TurnStateCompleted
// regardless of outcome. StartAsync itself only opens the runtime
// (AsyncStartResult carries no terminal status at all); this turn's actual
// published outcome is entirely the immediate follow-up invoke's.

// newActiveBoundControlSession creates a real active Chat turn with its exact
// episode bound to factorySessionID. Control tests use it rather than a call
// recorder so NOOP and SUPERSEDED outcomes come from Store's real aggregate
// transitions.
func newActiveBoundControlSession(t *testing.T, factorySessionID string) (chatsessions.Service, chatsessions.Session, chatsessions.Turn) {
	t.Helper()
	store, err := chatsessionswire.NewService(sequentialIDGenerator("control"), fixedClock(time.Unix(0, 1)), stubEventsAppender{}, stubEventsReader{})
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
