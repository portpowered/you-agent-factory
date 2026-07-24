package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestNewRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		drop func(*invocationservice.Dependencies)
		want string
	}{
		{name: "Factory config", drop: func(deps *invocationservice.Dependencies) { deps.FactoryConfig = nil }, want: "Factory config reader"},
		{name: "Work peer", drop: func(deps *invocationservice.Dependencies) { deps.Work = nil }, want: "Work peer root"},
		{name: "result observer", drop: func(deps *invocationservice.Dependencies) { deps.Observe = nil }, want: "result observer"},
		{name: "interpolation", drop: func(deps *invocationservice.Dependencies) { deps.Interpolation = nil }, want: "interpolation service"},
		{name: "Work Type", drop: func(deps *invocationservice.Dependencies) { deps.WorkTypes = nil }, want: "Work Type service"},
		{name: "input reader", drop: func(deps *invocationservice.Dependencies) { deps.InputFiles = nil }, want: "input file reader"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			deps := validDependencies(nil)
			test.drop(&deps)
			got, err := New(deps)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() = %#v, %v; want error containing %q", got, err, test.want)
			}
			if got != nil {
				t.Fatalf("New() service = %#v, want nil", got)
			}
		})
	}
}

func TestNewIsInert(t *testing.T) {
	t.Parallel()

	calls := 0
	deps := validDependencies(&calls)
	got, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if got == nil {
		t.Fatal("New() service = nil")
	}
	if calls != 0 {
		t.Fatalf("runtime dependency calls during construction = %d, want 0", calls)
	}
}

func TestService_PrepareOwnsResolvedInvocationInput(t *testing.T) {
	t.Parallel()

	service, err := New(validDependencies(nil))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	cfg := &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
		Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
	}}}
	sourceKind := factorysessions.InvocationInputSourceKindText
	resolved, err := service.ResolveInvocationInput(cfg, factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "prepare-me"}},
	})
	if err != nil {
		t.Fatalf("ResolveInvocationInput(): %v", err)
	}
	wantSource := work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent)
	if resolved.Source != wantSource {
		t.Fatalf("resolved source = %q, want %q", resolved.Source, wantSource)
	}
	if len(resolved.Content) != 1 || resolved.Content[0].Text != "prepare-me" {
		t.Fatalf("resolved content = %#v, want prepared text content", resolved.Content)
	}
}

func TestService_InvokeFactorySessionOwnsPrepareCommandObserve(t *testing.T) {
	t.Parallel()

	deps := validDependencies(nil)
	factoryConfigCalls := 0
	observeCalls := 0
	peer := &fakeWorkPeer{
		submitResult: work.WorkRequestSubmitResult{RequestID: "request-owned-1", TraceID: "trace-owned-1", Accepted: true},
	}
	var observedInput legacyinvocation.SessionInvocationWaitInput

	deps.FactoryConfig = func(sessionID string) (*factorydefinitions.FactoryConfig, error) {
		factoryConfigCalls++
		if sessionID != "session-owned-1" {
			t.Fatalf("FactoryConfig session ID = %q, want session-owned-1", sessionID)
		}
		return &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
		}}}, nil
	}
	deps.Work = peer
	deps.Observe = func(_ context.Context, sessionID string, input legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		observeCalls++
		if sessionID != "session-owned-1" {
			t.Fatalf("Observe session ID = %q, want session-owned-1", sessionID)
		}
		observedInput = input
		return completedObservation("request-owned-1", "trace-owned-1", "owned-result"), nil
	}

	var owned invocationservice.Service
	owned, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := owned.InvokeFactorySession(context.Background(), "session-owned-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello-owned"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted ||
		result.RequestID != "request-owned-1" ||
		result.TraceID != "trace-owned-1" {
		t.Fatalf("result = %#v, want completed owned outcome", result)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "owned-result" {
		t.Fatalf("primary result = %#v, want owned-result from observe path", result.PrimaryResult)
	}
	if factoryConfigCalls != 1 || peer.submitCalls != 1 || observeCalls != 1 {
		t.Fatalf("prepare/command/observe calls = config:%d submit:%d observe:%d, want 1 each", factoryConfigCalls, peer.submitCalls, observeCalls)
	}
	assertCommandedPreparedWork(t, peer.lastRequest, "hello-owned")
	if observedInput.RequestID != "request-owned-1" || observedInput.TraceID != "trace-owned-1" {
		t.Fatalf("observe input = %#v, want commanded request/trace identity", observedInput)
	}
}

func TestService_ObserveCompletedYieldsSessionScopedResult(t *testing.T) {
	t.Parallel()

	const (
		sessionID = "session-observe-complete"
		requestID = "request-observe-complete"
		traceID   = "trace-observe-complete"
	)
	peer := &fakeWorkPeer{
		submitResult: work.WorkRequestSubmitResult{RequestID: requestID, TraceID: traceID, Accepted: true},
	}
	deps := validDependencies(nil)
	deps.Work = peer
	var observedSession string
	deps.Observe = func(_ context.Context, gotSessionID string, input legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		observedSession = gotSessionID
		if input.RequestID != requestID || input.TraceID != traceID {
			t.Fatalf("observe wait input = %#v, want request/trace identity", input)
		}
		return completedObservation(requestID, traceID, "observe-complete"), nil
	}

	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := service.InvokeFactorySession(context.Background(), sessionID, factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "observe-me"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", result.Status)
	}
	if result.RequestID != requestID || result.TraceID != traceID {
		t.Fatalf("result identity = %#v, want request/trace %s/%s", result, requestID, traceID)
	}
	if result.SessionID != sessionID {
		t.Fatalf("SessionID = %q, want session-scoped %q", result.SessionID, sessionID)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "observe-complete" {
		t.Fatalf("PrimaryResult = %#v, want observe-complete", result.PrimaryResult)
	}
	if observedSession != sessionID {
		t.Fatalf("observe session ID = %q, want %q", observedSession, sessionID)
	}
	if peer.submitCalls != 1 {
		t.Fatalf("Work peer admission calls = %d, want 1", peer.submitCalls)
	}
}

func TestService_ObservePartialResultAvailabilityAsTypedFailedOutcome(t *testing.T) {
	t.Parallel()

	const (
		sessionID   = "session-observe-partial"
		requestID   = "request-observe-partial"
		traceID     = "trace-observe-partial"
		partialText = "partial answer before non-completed stop"
	)
	peer := &fakeWorkPeer{
		submitResult: work.WorkRequestSubmitResult{RequestID: requestID, TraceID: traceID, Accepted: true},
	}
	deps := validDependencies(nil)
	deps.Work = peer
	deps.Observe = func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		return partialCaptureFailedObservation(requestID, traceID, partialText), nil
	}

	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := service.InvokeFactorySession(context.Background(), sessionID, factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial-me"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusFailed {
		t.Fatalf("status = %q, want FAILED typed non-completed outcome", result.Status)
	}
	if result.ErrorCode != string(factorydefinitions.InvocationErrorCodeRuntimeFailure) {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, factorydefinitions.InvocationErrorCodeRuntimeFailure)
	}
	if result.RequestID != requestID || result.TraceID != traceID {
		t.Fatalf("result identity = %#v, want request/trace %s/%s", result, requestID, traceID)
	}
	if result.SessionID != sessionID {
		t.Fatalf("SessionID = %q, want session-scoped %q", result.SessionID, sessionID)
	}
	if result.WorkID != "work-partial-1" || result.WorkState == "" {
		t.Fatalf("work context = workID=%q workState=%q, want typed Work identity on partial-capture failure", result.WorkID, result.WorkState)
	}
	if result.Status == factorydefinitions.InvocationTerminalStatusCompleted {
		t.Fatal("partial-capture failure must not promote to COMPLETED")
	}
	for _, part := range result.PrimaryResult {
		if part.Text == partialText {
			t.Fatalf("PrimaryResult = %#v, want partial capture excluded from completed primary-result selection", result.PrimaryResult)
		}
	}
	if result.ErrorCode == "" || result.Message == "" {
		t.Fatalf("result = %#v, want typed ErrorCode/Message rather than an untyped side channel", result)
	}
}

func TestService_PrepareCommandsWorkThroughCTRWorkPeerRoot(t *testing.T) {
	t.Parallel()

	peer := &fakeWorkPeer{
		submitResult: work.WorkRequestSubmitResult{
			RequestID: "request-peer-1", TraceID: "trace-peer-1", Accepted: true,
			Works: []work.WorkRequestSubmittedWork{{Name: "task", WorkTypeName: "task", WorkID: "work-peer-1"}},
		},
	}
	deps := validDependencies(nil)
	deps.Work = peer
	deps.Observe = func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		return completedObservation("request-peer-1", "trace-peer-1", "peer-done"), nil
	}

	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := service.InvokeFactorySession(context.Background(), "session-peer-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "command-me"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted ||
		result.RequestID != "request-peer-1" ||
		result.TraceID != "trace-peer-1" {
		t.Fatalf("result = %#v, want completed peer admission identity", result)
	}
	if peer.submitCalls != 1 {
		t.Fatalf("Work peer admission calls = %d, want 1", peer.submitCalls)
	}
	if peer.lastSessionID != "session-peer-1" {
		t.Fatalf("commanded session ID = %q, want session-peer-1", peer.lastSessionID)
	}
	assertCommandedPreparedWork(t, peer.lastRequest, "command-me")
	if peer.lastRequest.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("commanded WorkRequest type = %q, want %q", peer.lastRequest.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if len(peer.lastRequest.Works) != 1 || peer.lastRequest.Works[0].WorkTypeID != "task" {
		t.Fatalf("commanded Works = %#v, want one task Work through peer root", peer.lastRequest.Works)
	}
}

func TestServiceCoordinatesSubmissionAndTerminalResult(t *testing.T) {
	t.Parallel()

	peer := &fakeWorkPeer{
		submitResult: work.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1", Accepted: true},
	}
	deps := validDependencies(nil)
	deps.Work = peer
	deps.Observe = func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		return completedObservation("request-1", "trace-1", "done"), nil
	}
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := service.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted || result.RequestID != "request-1" || result.TraceID != "trace-1" {
		t.Fatalf("result = %#v, want completed request-1/trace-1", result)
	}
	if peer.lastSessionID != "session-1" {
		t.Fatalf("commanded session ID = %q, want session-1", peer.lastSessionID)
	}
	assertCommandedPreparedWork(t, peer.lastRequest, "hello")
}

func TestServiceCompletesCancellationExactlyOnce(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	deps := validDependencies(nil)
	observeCalls := 0
	deps.Observe = func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		observeCalls++
		cancel()
		return legacyinvocation.SessionInvocationObservation{}, nil
	}
	deps.WaitNext = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := service.InvokeFactorySession(ctx, "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCanceled || result.ErrorCode != string(factorydefinitions.InvocationErrorCodeCanceled) {
		t.Fatalf("result = %#v, want canceled", result)
	}
	if observeCalls != 1 {
		t.Fatalf("observation calls = %d, want 1", observeCalls)
	}
}

func TestServiceCompletesTimeoutExactlyOnce(t *testing.T) {
	t.Parallel()

	deps := validDependencies(nil)
	observeCalls := 0
	deps.Observe = func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
		observeCalls++
		return legacyinvocation.SessionInvocationObservation{}, nil
	}
	deps.WaitNext = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	sourceKind := factorysessions.InvocationInputSourceKindText
	timeoutMillis := int64(1)
	result, err := service.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		TimeoutMillis:   &timeoutMillis,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession(): %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusTimedOut || result.ErrorCode != string(factorydefinitions.InvocationErrorCodeTimedOut) {
		t.Fatalf("result = %#v, want timed out", result)
	}
	if observeCalls != 1 {
		t.Fatalf("observation calls = %d, want 1", observeCalls)
	}
}

func TestServicePreservesDependencyFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("runtime unavailable")
	deps := validDependencies(nil)
	deps.FactoryConfig = func(string) (*factorydefinitions.FactoryConfig, error) { return nil, want }
	service, err := New(deps)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	_, err = service.InvokeFactorySession(context.Background(), "session-1", factorysessions.InvocationRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("InvokeFactorySession() error = %v, want %v", err, want)
	}
}

func validDependencies(calls *int) invocationservice.Dependencies {
	count := func() {
		if calls != nil {
			*calls++
		}
	}
	peer := &fakeWorkPeer{
		submitResult: work.WorkRequestSubmitResult{RequestID: "request-1", TraceID: "trace-1", Accepted: true},
		onSubmit:     count,
	}
	return invocationservice.Dependencies{
		FactoryConfig: func(string) (*factorydefinitions.FactoryConfig, error) {
			count()
			return &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
				Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
			}}}, nil
		},
		Work: peer,
		Observe: func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
			count()
			return completedObservation("request-1", "trace-1", "done"), nil
		},
		Interpolation: factorydefinitionfixtures.InvocationInterpolation{},
		WorkTypes:     staticWorkType("task"),
		InputFiles: func(string) ([]byte, error) {
			count()
			return nil, nil
		},
	}
}

type staticWorkType string

func (workType staticWorkType) DefaultWorkType(*factorydefinitions.FactoryConfig) (string, error) {
	return string(workType), nil
}

// fakeWorkPeer is a CTR-WORK peer-root admission stand-in. It uses only
// work.Service request/result vocabulary and never imports Work implementation.
type fakeWorkPeer struct {
	submitResult  work.WorkRequestSubmitResult
	submitErr     error
	submitCalls   int
	lastSessionID string
	lastRequest   work.WorkRequest
	onSubmit      func()
}

func (f *fakeWorkPeer) SubmitWorkRequestForSession(
	_ context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	f.submitCalls++
	f.lastSessionID = sessionID
	f.lastRequest = request
	if f.onSubmit != nil {
		f.onSubmit()
	}
	return f.submitResult, f.submitErr
}

func assertCommandedPreparedWork(t *testing.T, request work.WorkRequest, wantText string) {
	t.Helper()
	if request.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("commanded WorkRequest type = %q, want %q", request.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if len(request.Works) != 1 {
		t.Fatalf("commanded Works = %#v, want exactly one prepared Work", request.Works)
	}
	got := request.Works[0]
	if got.WorkTypeID != "task" {
		t.Fatalf("commanded WorkTypeID = %q, want task", got.WorkTypeID)
	}
	if len(got.Content) != 1 || got.Content[0].Text != wantText {
		t.Fatalf("commanded content = %#v, want prepared text %q", got.Content, wantText)
	}
}

func completedObservation(requestID, traceID, text string) legacyinvocation.SessionInvocationObservation {
	item := work.FactoryWorkItem{
		ID: "work-1", WorkTypeID: "task", State: "done", TraceID: traceID,
		Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}},
	}
	return legacyinvocation.SessionInvocationObservation{WorldState: factorydefinitions.FactoryWorldState{
		WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{requestID: {
			RequestID: requestID, TraceID: traceID, WorkItems: []work.FactoryWorkItem{item},
		}},
		TerminalWorkByID: map[string]factorydefinitions.FactoryTerminalWork{item.ID: {WorkItem: item, Status: "done"}},
	}}
}

// partialCaptureFailedObservation models non-completed Work that still carries
// captured content. ResolvePrimaryResult must not treat that content as a
// completed primary result; ClassifyFailedInvocation surfaces the typed failure.
func partialCaptureFailedObservation(requestID, traceID, partialText string) legacyinvocation.SessionInvocationObservation {
	initial := work.FactoryWorkItem{
		ID: "work-partial-1", WorkTypeID: "task", State: "draft", TraceID: traceID,
		DisplayName: "Goal", PlaceID: "task:init",
		Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "submitted"}},
	}
	failed := work.FactoryWorkItem{
		ID: "work-partial-1", WorkTypeID: "task", State: "failed", TraceID: traceID,
		DisplayName: "Goal", PlaceID: "task:failed",
		Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: partialText}},
	}
	state := factorydefinitions.FactoryWorldState{
		PayloadLineage:      work.WorkPayloadLineageProjection{},
		WorkItemsByID:       map[string]work.FactoryWorkItem{failed.ID: failed},
		WorkRequestsByID:    map[string]factorydefinitions.WorkRequestPayload{},
		TerminalWorkByID:    map[string]factorydefinitions.FactoryTerminalWork{},
		FailedWorkItemsByID: map[string]work.FactoryWorkItem{failed.ID: failed},
	}
	state.WorkRequestsByID[requestID] = factorydefinitions.WorkRequestPayload{
		RequestID: requestID, TraceID: traceID, Type: work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []work.FactoryWorkItem{initial},
	}
	state.PayloadLineage.RecordWorkRequestSnapshot(1, requestID, initial)
	state.TerminalWorkByID[failed.ID] = factorydefinitions.FactoryTerminalWork{WorkItem: failed, Status: "FAILED"}
	return legacyinvocation.SessionInvocationObservation{WorldState: state, ActiveWork: false}
}
