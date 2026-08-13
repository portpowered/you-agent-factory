package factorysessions

import (
	"encoding/json"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestLifecyclePolicyHelpersCoverStableTransitions(t *testing.T) {
	terminal := []LifecycleStatus{
		LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated,
	}
	for _, status := range terminal {
		if !isTerminalLifecycleStatus(status) {
			t.Fatalf("isTerminalLifecycleStatus(%q) = false", status)
		}
	}
	for _, status := range []LifecycleStatus{LifecycleStatusQueued, LifecycleStatusRunning, ""} {
		if isTerminalLifecycleStatus(status) {
			t.Fatalf("isTerminalLifecycleStatus(%q) = true", status)
		}
	}

	if !allowsRetryDispatchOnTerminal(LifecycleStatusFailed) || allowsRetryDispatchOnTerminal(LifecycleStatusSucceeded) {
		t.Fatal("retry-dispatch terminal policy is incorrect")
	}
	for _, status := range []LifecycleStatus{LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming} {
		if !allowsInterruptDispatchOnSession(status) {
			t.Fatalf("interrupt-dispatch should be allowed for %q", status)
		}
	}
	if allowsInterruptDispatchOnSession(LifecycleStatusSucceeded) {
		t.Fatal("interrupt-dispatch should be rejected for a terminal session")
	}

	cases := []struct {
		name   string
		status LifecycleStatus
		op     LifecycleControlKind
		want   LifecycleControlOutcome
	}{
		{"empty status", "", LifecycleControlPause, LifecycleControlOutcomeInvalidState},
		{"interrupted resume", LifecycleStatusInterrupted, LifecycleControlResume, LifecycleControlOutcomeAccepted},
		{"failed retry", LifecycleStatusFailed, LifecycleControlRetryDispatch, LifecycleControlOutcomeAccepted},
		{"succeeded retry", LifecycleStatusSucceeded, LifecycleControlRetryDispatch, LifecycleControlOutcomeTerminalSession},
		{"canceled cancel", LifecycleStatusCanceled, LifecycleControlCancel, LifecycleControlOutcomeNoOp},
		{"terminated terminate", LifecycleStatusTerminated, LifecycleControlTerminate, LifecycleControlOutcomeNoOp},
		{"terminal pause", LifecycleStatusSucceeded, LifecycleControlPause, LifecycleControlOutcomeTerminalSession},
		{"running pause", LifecycleStatusRunning, LifecycleControlPause, LifecycleControlOutcomeAccepted},
		{"paused pause", LifecycleStatusPaused, LifecycleControlPause, LifecycleControlOutcomeNoOp},
		{"queued pause", LifecycleStatusQueued, LifecycleControlPause, LifecycleControlOutcomeInvalidState},
		{"paused resume", LifecycleStatusPaused, LifecycleControlResume, LifecycleControlOutcomeAccepted},
		{"resuming resume", LifecycleStatusResuming, LifecycleControlResume, LifecycleControlOutcomeNoOp},
		{"queued resume", LifecycleStatusQueued, LifecycleControlResume, LifecycleControlOutcomeInvalidState},
		{"canceling cancel", LifecycleStatusCanceling, LifecycleControlCancel, LifecycleControlOutcomeNoOp},
		{"running cancel", LifecycleStatusRunning, LifecycleControlCancel, LifecycleControlOutcomeAccepted},
		{"failed cancel", LifecycleStatusFailed, LifecycleControlCancel, LifecycleControlOutcomeTerminalSession},
		{"queued terminate", LifecycleStatusQueued, LifecycleControlTerminate, LifecycleControlOutcomeAccepted},
		{"failed terminate", LifecycleStatusFailed, LifecycleControlTerminate, LifecycleControlOutcomeTerminalSession},
		{"awaiting approval", LifecycleStatusAwaitingApproval, LifecycleControlApprove, LifecycleControlOutcomeAccepted},
		{"running approve", LifecycleStatusRunning, LifecycleControlApprove, LifecycleControlOutcomeInvalidState},
		{"running retry", LifecycleStatusRunning, LifecycleControlRetryDispatch, LifecycleControlOutcomeAccepted},
		{"queued retry", LifecycleStatusQueued, LifecycleControlRetryDispatch, LifecycleControlOutcomeInvalidState},
		{"unknown operation", LifecycleStatusRunning, LifecycleControlKind("unknown"), LifecycleControlOutcomeInvalidState},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := evaluateLifecycleControl(test.op, test.status); got != test.want {
				t.Fatalf("evaluateLifecycleControl(%q, %q) = %q, want %q", test.op, test.status, got, test.want)
			}
		})
	}
}

func TestLifecycleProjectionHelpersBuildDetachedValues(t *testing.T) {
	withoutEvents := inspectionLinksForSession("session-a", false)
	if withoutEvents.Events != "" || withoutEvents.Results != "/factory-sessions/session-a/results" {
		t.Fatalf("inspection links without events = %#v", withoutEvents)
	}
	withEvents := lifecycleControlLinksForSession("session-a", true)
	if withEvents.Events != "/factory-sessions/session-a/events" || withEvents.Dispatches == "" {
		t.Fatalf("lifecycle control links = %#v", withEvents)
	}
	if got := emptySessionUsage(); got.Resources == nil || len(got.Resources) != 0 {
		t.Fatalf("empty session usage = %#v, want non-nil empty resources", got)
	}

	for _, test := range []struct {
		state string
		want  LifecycleStatus
	}{
		{"RUNNING", LifecycleStatusRunning},
		{" idle ", LifecycleStatusRunning},
		{"PAUSED", LifecycleStatusPaused},
		{"COMPLETED", LifecycleStatusSucceeded},
		{"FAILED", LifecycleStatusFailed},
		{"unknown", ""},
	} {
		if got := lifecycleStatusFromFactoryRuntimeState(test.state); got != test.want {
			t.Fatalf("lifecycleStatusFromFactoryRuntimeState(%q) = %q, want %q", test.state, got, test.want)
		}
	}

	if got := liveLifecycleControlLinksForSession(" session-a "); got.Results != "/factory-sessions/session-a/result" {
		t.Fatalf("live lifecycle links = %#v", got)
	}
	if got := liveLifecycleControlLogFields("session-a", LifecycleControlPause, "ACCEPTED", LifecycleStatusRunning, ControlRequest{}); len(got) != 4 {
		t.Fatalf("log fields without request id = %d, want 4", len(got))
	}
	if got := liveLifecycleControlLogFields("session-a", LifecycleControlPause, "ACCEPTED", "", ControlRequest{RequestID: "request-a"}); len(got) != 4 {
		t.Fatalf("log fields with request id = %d, want 4", len(got))
	}
}

func TestLifecycleOutcomeClassAndEventMaterialization(t *testing.T) {
	if got := lifecycleControlOutcomeClass("", ErrDurableSessionNotFound); got != LifecycleControlOutcomeClassNotFound {
		t.Fatalf("not-found outcome class = %q", got)
	}
	controlErr := &ControlError{Outcome: LifecycleControlOutcomeConflict}
	if got := lifecycleControlOutcomeClass("", controlErr); got != string(LifecycleControlOutcomeConflict) {
		t.Fatalf("control-error outcome class = %q", got)
	}
	if got := lifecycleControlOutcomeClass("", errors.New("boom")); got != "ERROR" {
		t.Fatalf("generic outcome class = %q", got)
	}
	if got := lifecycleControlOutcomeClass("", nil); got != "ERROR" {
		t.Fatalf("empty outcome class = %q", got)
	}

	payload := json.RawMessage(`{"value":1}`)
	raw, err := json.Marshal(factorydefinitions.FactoryEvent{Id: "event-a", Payload: payload})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	stream := materializeEventReadStream(EventReadResult{
		Events: []json.RawMessage{raw, json.RawMessage("{")},
	})
	if len(stream.History) != 1 || stream.History[0].Id != "event-a" || string(stream.History[0].Payload) != string(payload) {
		t.Fatalf("materialized history = %#v", stream.History)
	}
	select {
	case _, ok := <-stream.Events:
		if ok {
			t.Fatal("materialized event stream channel is open")
		}
	default:
		t.Fatal("materialized event stream channel was not closed")
	}

	validation := newValidationError("name", "name is required")
	if validation.Field != "name" || validation.Message != "name is required" {
		t.Fatalf("validation error = %#v", validation)
	}
}

func TestLiveChangeContractsNormalizeAndClassifyErrors(t *testing.T) {
	assertLiveChangeErrorClassification(t)
	assertLiveChangeNormalization(t)
}

func assertLiveChangeErrorClassification(t *testing.T) {
	t.Helper()
	var nilError *LiveChangeError
	if nilError.Error() != "live change error" || nilError.Unwrap() != nil || nilError.Is(nil) {
		t.Fatal("nil LiveChangeError methods are not stable")
	}
	invalid := NewLiveChangeError(LiveChangeErrorInvalidRequest, "  invalid  ")
	if invalid.Error() != "invalid" || !errors.Is(invalid, ErrLiveChangeInvalidRequest) {
		t.Fatalf("NewLiveChangeError() = %v", invalid)
	}
	notFound := &LiveChangeError{Code: LiveChangeErrorSessionNotFound}
	if !errors.Is(notFound, ErrLiveChangeSessionNotFound) || !errors.Is(notFound, ErrSessionNotFound) {
		t.Fatal("session-not-found live change error lost stable sentinels")
	}
	wrapped := errors.New("cause")
	if !errors.Is((&LiveChangeError{Cause: wrapped}).Unwrap(), wrapped) {
		t.Fatal("live change cause was not retained for local matching")
	}
}

func assertLiveChangeNormalization(t *testing.T) {
	t.Helper()
	normalized, err := NormalizeLiveChangeRequest(LiveChangeRequest{
		RequestID:        " request-a ",
		ExpectedRevision: 2,
		Operation:        "  SET ",
		TargetID:         " target-a ",
		RequestedValue:   json.RawMessage(`{"b":2,"a":1}`),
		Reason:           "  operator   request  ",
	})
	if err != nil {
		t.Fatalf("NormalizeLiveChangeRequest() error = %v", err)
	}
	if normalized.ChangeID != "live-change/request-a" || normalized.Operation != "set" || normalized.Reason != "operator request" || string(normalized.RequestedValue) != `{"a":1,"b":2}` {
		t.Fatalf("normalized live change = %#v", normalized)
	}
	for _, request := range []LiveChangeRequest{
		{ExpectedRevision: -1, RequestID: "id", Operation: "set", TargetID: "target", RequestedValue: json.RawMessage(`1`)},
		{RequestID: "id", TargetID: "target", RequestedValue: json.RawMessage(`1`)},
		{RequestID: "id", Operation: "set", RequestedValue: json.RawMessage(`1`)},
		{RequestID: "id", Operation: "set", TargetID: "target", RequestedValue: json.RawMessage("{")},
	} {
		if _, err := NormalizeLiveChangeRequest(request); err == nil {
			t.Fatalf("NormalizeLiveChangeRequest(%#v) unexpectedly succeeded", request)
		}
	}
}
