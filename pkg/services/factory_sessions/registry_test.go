package factorysessions_test

import (
	"context"
	"errors"
	"fmt"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	. "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"strconv"
	"sync/atomic"
	"testing"
)

var registryResponseEventIdentity atomic.Uint64

func registryResponseEventID() string {
	return fmt.Sprintf("registry-response-event-%d", registryResponseEventIdentity.Add(1))
}

func registrySessionID() string { return "550e8400-e29b-41d4-a716-446655440000" }

func TestRegistry_UpsertSelectAndRemove(t *testing.T) {
	registry := sessionregistry.New()

	defaultSession := livesession.New(DefaultSessionID, "/factories/alpha", "/workspace", "/workspace", TargetRef{Kind: TargetKindDefault}, "handle-default", true, "alpha", platformclock.Real{}, registrySessionID, registryResponseEventID)
	betaSession := livesession.New("session-beta", "/factories/beta", "/workspace", "/workspace", TargetRef{Kind: TargetKindNamed, Name: "beta"}, "handle-beta", false, "beta", platformclock.Real{}, registrySessionID, registryResponseEventID)

	registry.Upsert(defaultSession, true)
	if got := registry.Current(); got != defaultSession {
		t.Fatalf("Current() = %#v, want default session", got)
	}
	if registry.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", registry.Count())
	}

	registry.Upsert(betaSession, false)
	if got := registry.Current(); got != defaultSession {
		t.Fatalf("Current() after non-select upsert = %#v, want default session", got)
	}
	if registry.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", registry.Count())
	}

	if !registry.Select("session-beta") {
		t.Fatal("Select(session-beta) = false, want true")
	}
	if got := registry.Current(); got != betaSession {
		t.Fatalf("Current() after select = %#v, want beta session", got)
	}

	registry.Remove(DefaultSessionID)
	if got := registry.Current(); got != betaSession {
		t.Fatalf("Current() after removing default = %#v, want beta session", got)
	}
	if registry.Get(DefaultSessionID) != nil {
		t.Fatal("removed default session is still registered")
	}

	registry.Remove("session-beta")
	if registry.Current() != nil {
		t.Fatalf("Current() after removing all = %#v, want nil", registry.Current())
	}
	if got := registry.IDs(); len(got) != 0 {
		t.Fatalf("IDs() = %#v, want empty", got)
	}
}

func TestRegistry_SelectUnknownReturnsFalse(t *testing.T) {
	registry := sessionregistry.New()
	if registry.Select("missing") {
		t.Fatal("Select(missing) = true, want false")
	}
}

// The following compatibility-only cases prove retained selector acceptance.
// Canonical live identity is covered by the owner-private live-session tests.
func TestCompatibilityOnlyIsDefaultSessionSelector(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		want      bool
	}{
		{name: "alias", sessionID: DefaultSessionID, want: true},
		{name: "blank", sessionID: "   ", want: true},
		{name: "uuid", sessionID: "f47ac10b-58cc-4372-a567-0e02b2c3d479", want: false},
		{name: "named", sessionID: "session-beta", want: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := logicaltarget.IsLiveSessionDefaultSelector(tc.sessionID); got != tc.want {
				t.Fatalf("IsDefaultSessionSelector(%q) = %t, want %t", tc.sessionID, got, tc.want)
			}
		})
	}
}

func TestRegistry_CompatibilityOnlyDefaultSessionAliasLookupAndRemoval(t *testing.T) {
	registry := sessionregistry.New()
	defaultID := "550e8400-e29b-41d4-a716-446655440001"
	betaID := "550e8400-e29b-41d4-a716-446655440002"
	registry.Upsert(livesession.New(
		defaultID,
		"/factories/alpha",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		"handle-default",
		true,
		"alpha",
		platformclock.Real{},
		registrySessionID,
		registryResponseEventID,
	), true)
	registry.Upsert(livesession.New(
		betaID,
		"/factories/beta",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindNamed, Name: "beta"},
		"handle-beta",
		false,
		"beta",
		platformclock.Real{},
		registrySessionID,
		registryResponseEventID,
	), false)

	if got := registry.DefaultSession(); got == nil || got.ID != defaultID {
		t.Fatalf("DefaultSession() = %#v, want id %q", got, defaultID)
	}
	if got := registry.Get(DefaultSessionID); got == nil || got.ID != defaultID {
		t.Fatalf("Get(~default) = %#v, want id %q", got, defaultID)
	}
	if got := registry.Get(""); got == nil || got.ID != defaultID {
		t.Fatalf("Get(blank) = %#v, want id %q", got, defaultID)
	}
	if got := registry.Get(betaID); got == nil || got.ID != betaID {
		t.Fatalf("Get(beta) = %#v, want id %q", got, betaID)
	}

	registry.Remove(DefaultSessionID)
	if registry.Get(defaultID) != nil {
		t.Fatal("removed default session is still registered by uuid")
	}
	if registry.DefaultSession() != nil {
		t.Fatal("DefaultSession() after remove = non-nil, want nil")
	}
}

func TestLogicalSessionKeyID_DefaultTargetUsesStableKey(t *testing.T) {
	session := &livesession.LiveSession{
		SessionState: livesession.SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{
			Kind: TargetKindDefault,
		},
	}
	if got := logicaltarget.LegacyLiveSessionKeyID(session); got != "/workspace/root::default::" {
		t.Fatalf("LogicalSessionKeyID(default) = %q, want /workspace/root::default::", got)
	}
}

func TestLogicalSessionKeyID_NamedTargetIncludesFactoryName(t *testing.T) {
	session := &livesession.LiveSession{
		SessionState: livesession.SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{
			Kind: TargetKindNamed,
			Name: "beta",
		},
	}
	if got := logicaltarget.LegacyLiveSessionKeyID(session); got != "/workspace/root::named::beta" {
		t.Fatalf("LogicalSessionKeyID(named) = %q, want /workspace/root::named::beta", got)
	}
}

func TestRegistry_FindByLogicalSessionKeyID_ReturnsMatchingSession(t *testing.T) {
	registry := sessionregistry.New()
	defaultSession := &livesession.LiveSession{
		ID: "session-default",
		SessionState: livesession.SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{Kind: TargetKindDefault},
	}
	namedSession := &livesession.LiveSession{
		ID: "session-beta",
		SessionState: livesession.SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{Kind: TargetKindNamed, Name: "beta"},
	}
	registry.Upsert(defaultSession, true)
	registry.Upsert(namedSession, false)

	if got := registry.FindByLogicalSessionKeyID("/workspace/root::default::"); got != defaultSession {
		t.Fatalf("FindByLogicalSessionKeyID(default) = %#v, want default session", got)
	}
	if got := registry.FindByLogicalSessionKeyID("/workspace/root::named::beta"); got != namedSession {
		t.Fatalf("FindByLogicalSessionKeyID(named) = %#v, want named session", got)
	}
	if got := registry.FindByLogicalSessionKeyID("/workspace/other::default::"); got != nil {
		t.Fatalf("FindByLogicalSessionKeyID(missing) = %#v, want nil", got)
	}
}

// --- merged from durable_execution_contract_characterization_test.go ---

// peerDurableExecutionFake exercises the published durable-execution root slice
// through the singular Service. It compiles against only the Sessions root
// package and never imports factory_sessions/internal or nested execution
// implementation packages.
type peerDurableExecutionFake struct {
	*peerRootServiceFake
	starts    map[string]DurableAsyncStartResult
	sessions  map[string]DurableInspectResult
	lifecycle map[string]LifecycleStatus
	resumeErr map[string]error
	startErr  map[string]error
}

func newPeerDurableExecutionFake() *peerDurableExecutionFake {
	return &peerDurableExecutionFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		starts:              make(map[string]DurableAsyncStartResult),
		sessions:            make(map[string]DurableInspectResult),
		lifecycle:           make(map[string]LifecycleStatus),
		resumeErr:           make(map[string]error),
		startErr:            make(map[string]error),
	}
}

var _ Service = (*peerDurableExecutionFake)(nil)

func (fake *peerDurableExecutionFake) StartAsync(
	_ context.Context,
	req DurableStartRequest,
) (DurableAsyncStartResult, error) {
	if err, ok := fake.startErr[req.RequestID]; ok {
		return DurableAsyncStartResult{}, err
	}
	if result, ok := fake.starts[req.RequestID]; ok {
		return result, nil
	}
	return DurableAsyncStartResult{}, &DurableValidationError{
		Field:   "source",
		Message: "source is required",
	}
}

func (fake *peerDurableExecutionFake) ResumeInterruptedSession(
	_ context.Context,
	sessionID string,
	_ DurableResumeRequest,
) (DurableAsyncStartResult, error) {
	if err, ok := fake.resumeErr[sessionID]; ok {
		return DurableAsyncStartResult{}, err
	}
	if result, ok := fake.starts[sessionID]; ok {
		return result, nil
	}
	return DurableAsyncStartResult{}, ErrDurableSessionNotFound
}

func (fake *peerDurableExecutionFake) GetSession(
	_ context.Context,
	sessionID string,
) (DurableInspectResult, error) {
	if result, ok := fake.sessions[sessionID]; ok {
		return result, nil
	}
	return DurableInspectResult{}, ErrDurableSessionNotFound
}

func (fake *peerDurableExecutionFake) Pause(
	_ context.Context,
	sessionID string,
	_ DurableControlRequest,
) (DurableControlResult, error) {
	return fake.applyDurableControl(sessionID, LifecycleControlPause)
}

func (fake *peerDurableExecutionFake) Resume(
	_ context.Context,
	sessionID string,
	_ DurableControlRequest,
) (DurableControlResult, error) {
	return fake.applyDurableControl(sessionID, LifecycleControlResume)
}

func (fake *peerDurableExecutionFake) applyDurableControl(
	sessionID string,
	operation LifecycleControlKind,
) (DurableControlResult, error) {
	status, ok := fake.lifecycle[sessionID]
	if !ok {
		return DurableControlResult{}, ErrDurableSessionNotFound
	}
	switch status {
	case LifecycleStatusSucceeded, LifecycleStatusFailed:
		return DurableControlResult{}, &DurableControlError{
			Operation: operation,
			Outcome:   LifecycleControlOutcomeTerminalSession,
			Status:    status,
			Message:   string(LifecycleControlOutcomeTerminalSession),
		}
	case LifecycleStatusRunning:
		if operation == LifecycleControlPause {
			fake.lifecycle[sessionID] = LifecycleStatusPaused
			return DurableControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   LifecycleControlOutcomeAccepted,
				Status:    LifecycleStatusPaused,
			}, nil
		}
	}
	return DurableControlResult{}, &DurableControlError{
		Operation: operation,
		Outcome:   LifecycleControlOutcomeInvalidState,
		Status:    status,
		Message:   string(LifecycleControlOutcomeInvalidState),
	}
}

func TestDurableExecutionRootContract_StartAndInspectSuccess(t *testing.T) {
	t.Parallel()

	fake := newPeerDurableExecutionFake()
	sessionID := "dur-sess-alpha"
	requestID := "req-durable-start-1"
	fake.starts[requestID] = DurableAsyncStartResult{
		SessionID:        sessionID,
		Status:           string(LifecycleStatusRunning),
		OrchestratorKind: "JAVASCRIPT",
		SourceHash:       "source-hash-alpha",
	}
	fake.sessions[sessionID] = DurableInspectResult{
		SessionID:        sessionID,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		SourceHash:       "source-hash-alpha",
	}
	fake.lifecycle[sessionID] = LifecycleStatusRunning

	var service Service = fake
	ctx := context.Background()

	started, err := service.StartAsync(ctx, DurableStartRequest{
		RequestID: requestID,
		Source:    Source{Kind: "WORKFLOW_NAME", WorkflowName: "demo"},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.SessionID != sessionID || started.Status != string(LifecycleStatusRunning) {
		t.Fatalf("StartAsync = %#v, want published durable start success for %q", started, sessionID)
	}

	inspected, err := service.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if inspected.SessionID != sessionID || inspected.Status != LifecycleStatusRunning {
		t.Fatalf("GetSession = %#v, want durable inspect success shape", inspected)
	}

	paused, err := service.Pause(ctx, sessionID, DurableControlRequest{Reason: "operator-pause"})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.SessionID != sessionID ||
		paused.Outcome != LifecycleControlOutcomeAccepted ||
		paused.Status != LifecycleStatusPaused {
		t.Fatalf("Pause = %#v, want accepted durable control result", paused)
	}
}

func TestDurableExecutionRootContract_TypedFailures(t *testing.T) {
	t.Parallel()

	fake := newPeerDurableExecutionFake()
	terminalID := "dur-sess-terminal"
	checkpointID := "dur-sess-missing-checkpoint"
	fake.lifecycle[terminalID] = LifecycleStatusSucceeded
	fake.sessions[terminalID] = DurableInspectResult{
		SessionID: terminalID,
		Status:    LifecycleStatusSucceeded,
	}
	fake.startErr["req-invalid-source"] = &DurableValidationError{
		Field:   "source",
		Message: "source.kind is invalid",
	}
	fake.startErr["req-invalid-policy"] = &DurableValidationError{
		Field:   "requestedPolicy",
		Message: "requestedPolicy is invalid",
	}
	fake.resumeErr[checkpointID] = &DurableResumeError{
		Outcome:   DurableResumeOutcomeMissingCheckpoint,
		Status:    LifecycleStatusPaused,
		Field:     "checkpointSummary",
		Message:   string(DurableResumeOutcomeMissingCheckpoint),
		SessionID: checkpointID,
	}

	var service Service = fake
	ctx := context.Background()

	_, err := service.StartAsync(ctx, DurableStartRequest{RequestID: "req-invalid-source"})
	var invalidSource *DurableValidationError
	if !errors.As(err, &invalidSource) || invalidSource.Field != "source" {
		t.Fatalf("StartAsync invalid source = %v, want *DurableValidationError field=source", err)
	}

	_, err = service.StartAsync(ctx, DurableStartRequest{RequestID: "req-invalid-policy"})
	var invalidPolicy *DurableValidationError
	if !errors.As(err, &invalidPolicy) || invalidPolicy.Field != "requestedPolicy" {
		t.Fatalf("StartAsync invalid policy = %v, want *DurableValidationError field=requestedPolicy", err)
	}

	_, err = service.GetSession(ctx, "dur-sess-missing")
	if !errors.Is(err, ErrDurableSessionNotFound) {
		t.Fatalf("GetSession missing = %v, want ErrDurableSessionNotFound", err)
	}

	_, err = service.ResumeInterruptedSession(ctx, checkpointID, DurableResumeRequest{RequestID: "resume-1"})
	var missingCheckpoint *DurableResumeError
	if !errors.As(err, &missingCheckpoint) {
		t.Fatalf("ResumeInterruptedSession = %v, want *DurableResumeError", err)
	}
	if missingCheckpoint.Outcome != DurableResumeOutcomeMissingCheckpoint {
		t.Fatalf("ResumeInterruptedSession outcome = %q, want MISSING_CHECKPOINT", missingCheckpoint.Outcome)
	}
	if errors.Is(err, ErrDurableSessionNotFound) {
		t.Fatal("missing checkpoint must stay distinct from ErrDurableSessionNotFound")
	}

	_, err = service.Pause(ctx, terminalID, DurableControlRequest{})
	var rejected *DurableControlError
	if !errors.As(err, &rejected) {
		t.Fatalf("Pause terminal = %v, want *DurableControlError", err)
	}
	if rejected.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("Pause outcome = %q, want TERMINAL_SESSION", rejected.Outcome)
	}
	if errors.Is(err, ErrDurableSessionNotFound) {
		t.Fatal("rejected lifecycle control must stay distinct from ErrDurableSessionNotFound")
	}
}

// --- merged from invocation_contract_characterization_test.go ---

// peerInvocationSurfaceFake exercises the published invocation root slice while
// remaining a singular Service implementer. It compiles against only the
// Sessions root package (plus approved Work content types already present in
// root signatures) and never imports factory_sessions/internal or a separately
// published peer-facing invoker interface.
type peerInvocationSurfaceFake struct {
	*peerRootServiceFake
	outcomes map[string]InvocationResult
	failures map[string]error
}

func newPeerInvocationSurfaceFake() *peerInvocationSurfaceFake {
	return &peerInvocationSurfaceFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		outcomes:            make(map[string]InvocationResult),
		failures:            make(map[string]error),
	}
}

var _ Service = (*peerInvocationSurfaceFake)(nil)

func invocationRequestKey(sessionID string, request InvocationRequest) string {
	requestID := ""
	if request.RequestID != nil {
		requestID = *request.RequestID
	}
	source := ""
	if request.SourceKind != nil {
		source = string(*request.SourceKind)
	}
	timeout := int64(-1)
	if request.TimeoutMillis != nil {
		timeout = *request.TimeoutMillis
	}
	return sessionID + "|" + requestID + "|" + source + "|" + strconv.FormatInt(timeout, 10)
}

// InvokeFactorySession is a peer-local callable using the published root
// vocabulary. It is intentionally not a separately published root Invoker
// interface; invocation stays under the singular Service aggregate.
func (fake *peerInvocationSurfaceFake) InvokeFactorySession(
	_ context.Context,
	sessionID string,
	request InvocationRequest,
) (InvocationResult, error) {
	key := invocationRequestKey(sessionID, request)
	if err, ok := fake.failures[key]; ok {
		return InvocationResult{}, err
	}
	if result, ok := fake.outcomes[key]; ok {
		return result, nil
	}
	return InvocationResult{}, &InvocationValidationError{
		Field:   "content",
		Message: "invocation input is required",
	}
}

func TestInvocationRootContract_ValidRequestMapsToSessionScopedOutcome(t *testing.T) {
	t.Parallel()

	fake := newPeerInvocationSurfaceFake()
	sessionID := "sess-invoke-alpha"
	requestID := "req-invoke-1"
	sourceKind := InvocationInputSourceKindText
	request := InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		RequestID:       &requestID,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	}
	fake.outcomes[invocationRequestKey(sessionID, request)] = InvocationResult{
		RequestID: requestID,
		TraceID:   "trace-invoke-1",
		Status:    InvocationTerminalStatusCompleted,
		SessionID: sessionID,
		WorkID:    "work-1",
		WorkName:  "task",
		WorkState: "done",
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "ok"},
		},
	}

	var service Service = fake
	if _, err := service.GetFactorySession(context.Background(), "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("singular Service read path = %v, want ErrSessionNotFound", err)
	}

	resolved := ResolvedInvocationInput{
		Source:  work.InputSourcePositionalText,
		Content: request.Content,
	}
	if len(resolved.Content) != 1 || resolved.Content[0].Text != "hello" {
		t.Fatalf("ResolvedInvocationInput = %#v, want published resolved-input shape", resolved)
	}

	timeout := InvocationTimeout(DefaultInvocationTimeout)
	if timeout <= 0 {
		t.Fatalf("InvocationTimeout = %v, want positive published timeout budget", timeout)
	}

	result, err := fake.InvokeFactorySession(context.Background(), sessionID, request)
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	if result.Status != InvocationTerminalStatusCompleted ||
		result.SessionID != sessionID ||
		result.RequestID != requestID {
		t.Fatalf("InvokeFactorySession = %#v, want completed session-scoped outcome", result)
	}
}

func TestInvocationRootContract_TypedFailuresAreDistinct(t *testing.T) {
	t.Parallel()

	fake := newPeerInvocationSurfaceFake()
	sessionID := "sess-invoke-beta"
	sourceKind := InvocationInputSourceKindText

	invalid := InvocationRequest{
		ContentProvided: false,
		RequestID:       strPtr("req-invalid"),
	}
	timeoutMillis := int64(1)
	timedOut := InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		RequestID:       strPtr("req-timeout"),
		TimeoutMillis:   &timeoutMillis,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	}
	canceled := InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		RequestID:       strPtr("req-cancel"),
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	}

	fake.failures[invocationRequestKey(sessionID, invalid)] = &InvocationValidationError{
		Field:   "content",
		Message: "content is required when args are omitted",
	}
	fake.outcomes[invocationRequestKey(sessionID, timedOut)] = InvocationResult{
		RequestID: "req-timeout",
		Status:    InvocationTerminalStatusTimedOut,
		ErrorCode: string(InvocationErrorCodeTimedOut),
		SessionID: sessionID,
		Message:   "invocation timed out",
	}
	fake.outcomes[invocationRequestKey(sessionID, canceled)] = InvocationResult{
		RequestID: "req-cancel",
		Status:    InvocationTerminalStatusCanceled,
		ErrorCode: string(InvocationErrorCodeCanceled),
		SessionID: sessionID,
		Message:   "invocation canceled by caller",
	}

	_, err := fake.InvokeFactorySession(context.Background(), sessionID, invalid)
	var invalidInput *InvocationValidationError
	if !errors.As(err, &invalidInput) || invalidInput.Field != "content" {
		t.Fatalf("invalid input = %v, want *InvocationValidationError field=content", err)
	}

	timeoutResult, err := fake.InvokeFactorySession(context.Background(), sessionID, timedOut)
	if err != nil {
		t.Fatalf("timeout InvokeFactorySession: %v", err)
	}
	if timeoutResult.Status != InvocationTerminalStatusTimedOut ||
		timeoutResult.ErrorCode != string(InvocationErrorCodeTimedOut) {
		t.Fatalf("timeout result = %#v, want TIMED_OUT typed outcome", timeoutResult)
	}

	cancelResult, err := fake.InvokeFactorySession(context.Background(), sessionID, canceled)
	if err != nil {
		t.Fatalf("cancel InvokeFactorySession: %v", err)
	}
	if cancelResult.Status != InvocationTerminalStatusCanceled ||
		cancelResult.ErrorCode != string(InvocationErrorCodeCanceled) {
		t.Fatalf("cancel result = %#v, want CANCELED typed outcome", cancelResult)
	}

	if timeoutResult.Status == cancelResult.Status ||
		timeoutResult.ErrorCode == cancelResult.ErrorCode ||
		timeoutResult.Status == InvocationTerminalStatusCompleted {
		t.Fatal("timeout and cancellation typed outcomes must stay distinguishable from each other and success")
	}
	if cancelResult.ErrorCode == string(InvocationErrorCodeTimedOut) ||
		cancelResult.Status == InvocationTerminalStatusTimedOut {
		t.Fatal("caller-cancellation must stay distinct from timeout typed outcome")
	}
}

func strPtr(value string) *string { return &value }

// --- merged from service_authority_characterization_test.go ---

// peerRootServiceFake is a peer-owned fake of the published Factory Sessions
// root Service. It intentionally imports only the Sessions root package (plus
// approved peer roots already present in root signatures) and never imports
// factory_sessions/internal.
type peerRootServiceFake struct {
	peerExecutionStub
	sessions map[string]SessionProjection
}

func newPeerRootServiceFake() *peerRootServiceFake {
	return &peerRootServiceFake{
		sessions: make(map[string]SessionProjection),
	}
}

var _ Service = (*peerRootServiceFake)(nil)

func (fake *peerRootServiceFake) ForRuntime(OpeningBindingRequest) (Service, error) {
	return fake, nil
}

func (fake *peerRootServiceFake) OpenFactorySession(context.Context, OpenRequest) (*OpenResult, error) {
	return &OpenResult{SessionID: DefaultSessionID}, nil
}

func (fake *peerRootServiceFake) OpenFactorySessionFromFolder(context.Context, string, *TargetRef, bool, bool) (*OpenResult, error) {
	return &OpenResult{SessionID: DefaultSessionID}, nil
}

func (fake *peerRootServiceFake) ListFactorySessions(context.Context) ([]ReadProjection, error) {
	return nil, nil
}

func (fake *peerRootServiceFake) GetFactorySession(_ context.Context, sessionID string) (SessionProjection, error) {
	if projection, ok := fake.sessions[sessionID]; ok {
		return projection, nil
	}
	return SessionProjection{}, ErrSessionNotFound
}

func (fake *peerRootServiceFake) GetFactorySessionSyncPreflight(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor, *factorydefinitions.FactorySessionLogicalResolveHint) (SyncPreflightResult, error) {
	return SyncPreflightResult{}, ErrSessionNotFound
}

func (fake *peerRootServiceFake) GetFactorySessionResult(context.Context, string) (factoryruntime.LiveSessionResult, error) {
	return factoryruntime.LiveSessionResult{}, ErrSessionNotFound
}

func (fake *peerRootServiceFake) GetFactorySessionPartialResult(context.Context, string) (factoryruntime.PartialSessionResult, error) {
	return factoryruntime.PartialSessionResult{}, ErrSessionNotFound
}

func (fake *peerRootServiceFake) SubscribeFactoryResponseEvents(context.Context, ResponseEventSubscriptionRequest) (*ResponseEventCursor, error) {
	return nil, ErrSessionNotFound
}

func (fake *peerRootServiceFake) SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	return nil, ErrSessionNotFound
}

func (fake *peerRootServiceFake) ProbeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error {
	return ErrSessionNotFound
}

func (fake *peerRootServiceFake) ReadDurableFactorySessionEventStream(context.Context, string, EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error) {
	return nil, ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) ProbeDurableFactorySessionEvents(context.Context, string, EventReconnectRequest) error {
	return ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) GetEngineStateSnapshotForSession(context.Context, string) (*factoryruntime.LegacyEngineObservation, error) {
	return nil, ErrSessionNotFound
}

func (fake *peerRootServiceFake) PauseLiveFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrSessionNotFound
}

func (fake *peerRootServiceFake) ResumeLiveFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrSessionNotFound
}

func (fake *peerRootServiceFake) CloseFactorySession(context.Context, string) error {
	return ErrSessionNotFound
}

func (fake *peerRootServiceFake) PauseDurableFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) ResumeDurableFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) CancelDurableFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) TerminateDurableFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) ApproveDurableFactorySession(context.Context, string, ApproveRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) RetryDurableFactorySessionDispatch(context.Context, string, RetryDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) InterruptDurableFactorySessionDispatch(context.Context, string, InterruptDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}

// peerExecutionStub satisfies the durable ExecutionService methods embedded in
// the singular root Service so a peer can compile against one aggregate authority.
type peerExecutionStub struct {
	Service
}

func (peerExecutionStub) StartAsync(context.Context, StartRequest) (AsyncStartResult, error) {
	return AsyncStartResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) StartSync(context.Context, StartRequest) (SyncStartResult, error) {
	return SyncStartResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) ResumeInterruptedSession(context.Context, string, ResumeSessionRequest) (AsyncStartResult, error) {
	return AsyncStartResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) GetSession(context.Context, string) (SessionReadResult, error) {
	return SessionReadResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) Pause(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) Resume(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) Cancel(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) Terminate(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) Approve(context.Context, string, ApproveRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) RetryDispatch(context.Context, string, RetryDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) InterruptDispatch(context.Context, string, InterruptDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) GetResult(context.Context, string, ResultRequest) (ResultReadResult, error) {
	return ResultReadResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) ListDispatches(context.Context, string) (ListDispatchesResult, error) {
	return ListDispatchesResult{}, nil
}
func (peerExecutionStub) QueryDispatches(context.Context, DispatchQueryRequest) (ListDispatchesResult, error) {
	return ListDispatchesResult{}, nil
}
func (peerExecutionStub) GetDispatch(context.Context, string, string) (DispatchDetail, error) {
	return DispatchDetail{}, ErrDispatchNotFound
}
func (peerExecutionStub) ListArtifacts(context.Context, string) (ListArtifactsResult, error) {
	return ListArtifactsResult{}, nil
}
func (peerExecutionStub) GetArtifact(context.Context, string, string) (ArtifactDetail, error) {
	return ArtifactDetail{}, ErrArtifactNotFound
}
func (peerExecutionStub) ReadEvents(context.Context, string, EventReconnectRequest) (EventReadResult, error) {
	return EventReadResult{}, ErrDurableSessionNotFound
}
func (peerExecutionStub) ListSessions(context.Context, ListSessionsRequest) (ListSessionsResult, error) {
	return ListSessionsResult{}, nil
}

func TestSingularRootServiceAuthority_PeerFakeReadNotFound(t *testing.T) {
	t.Parallel()

	var service Service = newPeerRootServiceFake()
	ctx := context.Background()

	projection, err := service.GetFactorySession(ctx, "missing-session")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("GetFactorySession error = %v, want ErrSessionNotFound", err)
	}
	if projection.Context.FactorySessionID != "" || projection.Runtime.Status != "" {
		t.Fatalf("GetFactorySession projection = %#v, want empty identity on not-found", projection)
	}

	listed, err := service.ListFactorySessions(ctx)
	if err != nil {
		t.Fatalf("ListFactorySessions error = %v, want nil", err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListFactorySessions len = %d, want empty list", len(listed))
	}

	opened, err := service.OpenFactorySession(ctx, OpenRequest{FolderPath: "/factories/demo"})
	if err != nil {
		t.Fatalf("OpenFactorySession error = %v, want nil", err)
	}
	if opened == nil || opened.SessionID == "" {
		t.Fatalf("OpenFactorySession result = %#v, want reachable open path through singular root", opened)
	}
}
