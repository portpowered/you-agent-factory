package factorysessions_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// peerInvocationSurfaceFake exercises the published invocation root slice while
// remaining a singular Service implementer. It compiles against only the
// Sessions root package (plus approved Work content types already present in
// root signatures) and never imports factory_sessions/internal or a separately
// published peer-facing invoker interface.
type peerInvocationSurfaceFake struct {
	*peerRootServiceFake
	outcomes map[string]factorysessions.InvocationResult
	failures map[string]error
}

func newPeerInvocationSurfaceFake() *peerInvocationSurfaceFake {
	return &peerInvocationSurfaceFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		outcomes:            make(map[string]factorysessions.InvocationResult),
		failures:            make(map[string]error),
	}
}

var _ factorysessions.Service = (*peerInvocationSurfaceFake)(nil)

func invocationRequestKey(sessionID string, request factorysessions.InvocationRequest) string {
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
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	key := invocationRequestKey(sessionID, request)
	if err, ok := fake.failures[key]; ok {
		return factorysessions.InvocationResult{}, err
	}
	if result, ok := fake.outcomes[key]; ok {
		return result, nil
	}
	return factorysessions.InvocationResult{}, &factorysessions.InvocationValidationError{
		Field:   "content",
		Message: "invocation input is required",
	}
}

func TestInvocationRootContract_ValidRequestMapsToSessionScopedOutcome(t *testing.T) {
	t.Parallel()

	fake := newPeerInvocationSurfaceFake()
	sessionID := "sess-invoke-alpha"
	requestID := "req-invoke-1"
	sourceKind := factorysessions.InvocationInputSourceKindText
	request := factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		RequestID:       &requestID,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	}
	fake.outcomes[invocationRequestKey(sessionID, request)] = factorysessions.InvocationResult{
		RequestID: requestID,
		TraceID:   "trace-invoke-1",
		Status:    factorysessions.InvocationTerminalStatusCompleted,
		SessionID: sessionID,
		WorkID:    "work-1",
		WorkName:  "task",
		WorkState: "done",
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "ok"},
		},
	}

	var service factorysessions.Service = fake
	if _, err := service.GetFactorySession(context.Background(), "missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("singular Service read path = %v, want ErrSessionNotFound", err)
	}

	resolved := factorysessions.ResolvedInvocationInput{
		Source:  work.InputSourcePositionalText,
		Content: request.Content,
	}
	if len(resolved.Content) != 1 || resolved.Content[0].Text != "hello" {
		t.Fatalf("ResolvedInvocationInput = %#v, want published resolved-input shape", resolved)
	}

	timeout := factorysessions.InvocationTimeout(factorysessions.DefaultInvocationTimeout)
	if timeout <= 0 {
		t.Fatalf("InvocationTimeout = %v, want positive published timeout budget", timeout)
	}

	result, err := fake.InvokeFactorySession(context.Background(), sessionID, request)
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	if result.Status != factorysessions.InvocationTerminalStatusCompleted ||
		result.SessionID != sessionID ||
		result.RequestID != requestID {
		t.Fatalf("InvokeFactorySession = %#v, want completed session-scoped outcome", result)
	}
}

func TestInvocationRootContract_TypedFailuresAreDistinct(t *testing.T) {
	t.Parallel()

	fake := newPeerInvocationSurfaceFake()
	sessionID := "sess-invoke-beta"
	sourceKind := factorysessions.InvocationInputSourceKindText

	invalid := factorysessions.InvocationRequest{
		ContentProvided: false,
		RequestID:       strPtr("req-invalid"),
	}
	timeoutMillis := int64(1)
	timedOut := factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		RequestID:       strPtr("req-timeout"),
		TimeoutMillis:   &timeoutMillis,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	}
	canceled := factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		RequestID:       strPtr("req-cancel"),
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	}

	fake.failures[invocationRequestKey(sessionID, invalid)] = &factorysessions.InvocationValidationError{
		Field:   "content",
		Message: "content is required when args are omitted",
	}
	fake.outcomes[invocationRequestKey(sessionID, timedOut)] = factorysessions.InvocationResult{
		RequestID: "req-timeout",
		Status:    factorysessions.InvocationTerminalStatusTimedOut,
		ErrorCode: string(factorysessions.InvocationErrorCodeTimedOut),
		SessionID: sessionID,
		Message:   "invocation timed out",
	}
	fake.outcomes[invocationRequestKey(sessionID, canceled)] = factorysessions.InvocationResult{
		RequestID: "req-cancel",
		Status:    factorysessions.InvocationTerminalStatusCanceled,
		ErrorCode: string(factorysessions.InvocationErrorCodeCanceled),
		SessionID: sessionID,
		Message:   "invocation canceled by caller",
	}

	_, err := fake.InvokeFactorySession(context.Background(), sessionID, invalid)
	var invalidInput *factorysessions.InvocationValidationError
	if !errors.As(err, &invalidInput) || invalidInput.Field != "content" {
		t.Fatalf("invalid input = %v, want *InvocationValidationError field=content", err)
	}

	timeoutResult, err := fake.InvokeFactorySession(context.Background(), sessionID, timedOut)
	if err != nil {
		t.Fatalf("timeout InvokeFactorySession: %v", err)
	}
	if timeoutResult.Status != factorysessions.InvocationTerminalStatusTimedOut ||
		timeoutResult.ErrorCode != string(factorysessions.InvocationErrorCodeTimedOut) {
		t.Fatalf("timeout result = %#v, want TIMED_OUT typed outcome", timeoutResult)
	}

	cancelResult, err := fake.InvokeFactorySession(context.Background(), sessionID, canceled)
	if err != nil {
		t.Fatalf("cancel InvokeFactorySession: %v", err)
	}
	if cancelResult.Status != factorysessions.InvocationTerminalStatusCanceled ||
		cancelResult.ErrorCode != string(factorysessions.InvocationErrorCodeCanceled) {
		t.Fatalf("cancel result = %#v, want CANCELED typed outcome", cancelResult)
	}

	if timeoutResult.Status == cancelResult.Status ||
		timeoutResult.ErrorCode == cancelResult.ErrorCode ||
		timeoutResult.Status == factorysessions.InvocationTerminalStatusCompleted {
		t.Fatal("timeout and cancellation typed outcomes must stay distinguishable from each other and success")
	}
	if cancelResult.ErrorCode == string(factorysessions.InvocationErrorCodeTimedOut) ||
		cancelResult.Status == factorysessions.InvocationTerminalStatusTimedOut {
		t.Fatal("caller-cancellation must stay distinct from timeout typed outcome")
	}
}

func strPtr(value string) *string { return &value }
