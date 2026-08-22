package protocol

import (
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestMapFactoryInvocationOutcome_StopReasonMapping(t *testing.T) {
	tests := []struct {
		name   string
		status factorysessions.InvocationTerminalStatus
		want   acpsdk.StopReason
	}{
		{"completed", factorysessions.InvocationTerminalStatusCompleted, acpsdk.StopReasonEndTurn},
		{"canceled", factorysessions.InvocationTerminalStatusCanceled, acpsdk.StopReasonCancelled},
		{"timed_out", factorysessions.InvocationTerminalStatusTimedOut, acpsdk.StopReasonCancelled},
		{"failed", factorysessions.InvocationTerminalStatusFailed, acpsdk.StopReasonEndTurn},
		{"unknown_future_status", factorysessions.InvocationTerminalStatus("SOME_FUTURE_STATUS"), acpsdk.StopReasonEndTurn},
		{"empty", factorysessions.InvocationTerminalStatus(""), acpsdk.StopReasonEndTurn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapFactoryInvocationOutcome(factorysessions.InvocationResult{Status: tt.status})
			if got.StopReason != tt.want {
				t.Errorf("MapFactoryInvocationOutcome(Status=%q).StopReason = %q, want %q", tt.status, got.StopReason, tt.want)
			}
		})
	}
}

func TestMapFactoryInvocationOutcome_ProjectsOnlyTextPartsInStableOrder(t *testing.T) {
	result := factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "first"},
			{Type: work.WorkContentPartTypeImage, URL: "https://example.invalid/image.png"},
			{Type: work.WorkContentPartTypeText, Text: "second"},
			{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"k":"v"}`)},
			{Type: work.WorkContentPartTypeText, Text: "third"},
		},
	}

	got := MapFactoryInvocationOutcome(result)

	want := []string{"first", "second", "third"}
	if !reflect.DeepEqual(got.Text, want) {
		t.Fatalf("MapFactoryInvocationOutcome().Text = %#v, want %#v", got.Text, want)
	}
}

func TestMapFactoryInvocationOutcome_EmptyOrUnsupportedResultProducesNoFabricatedText(t *testing.T) {
	tests := []struct {
		name          string
		primaryResult []work.WorkContentPart
	}{
		{"nil primary result", nil},
		{"empty primary result", []work.WorkContentPart{}},
		{"only unsupported parts", []work.WorkContentPart{
			{Type: work.WorkContentPartTypeImage, URL: "https://example.invalid/image.png"},
			{Type: work.WorkContentPartTypeBinary, File: "artifact.bin"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapFactoryInvocationOutcome(factorysessions.InvocationResult{
				Status:        factorysessions.InvocationTerminalStatusCompleted,
				PrimaryResult: tt.primaryResult,
			})
			if len(got.Text) != 0 {
				t.Fatalf("MapFactoryInvocationOutcome().Text = %#v, want empty", got.Text)
			}
		})
	}
}

// TestMapFactoryInvocationOutcome_NeverSerializesRawResultFields proves the
// mapper never leaks an invocation's raw ErrorCode/Message/session/work
// identifiers into the bounded PromptOutcome projection it returns.
func TestMapFactoryInvocationOutcome_NeverSerializesRawResultFields(t *testing.T) {
	sentinel := "provider command: /usr/local/bin/agent --token=sk-live-ABC123"
	result := factorysessions.InvocationResult{
		Status:    factorysessions.InvocationTerminalStatusFailed,
		ErrorCode: string(factorysessions.InvocationErrorCodeRuntimeFailure),
		Message:   sentinel,
		SessionID: "fs-secret-session",
		WorkID:    "work-secret",
	}

	got := MapFactoryInvocationOutcome(result)

	v := reflect.ValueOf(got)
	if v.NumField() != 2 {
		t.Fatalf("PromptOutcome field count = %d, want exactly 2 (StopReason, Text)", v.NumField())
	}
	if len(got.Text) != 0 {
		t.Fatalf("MapFactoryInvocationOutcome().Text = %#v, want empty for a failed invocation with no primary result", got.Text)
	}
}

func TestMapFactoryOutcomes_AreDeterministic(t *testing.T) {
	invocation := factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "hello"},
		},
	}
	first := MapFactoryInvocationOutcome(invocation)
	second := MapFactoryInvocationOutcome(invocation)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("MapFactoryInvocationOutcome() is not deterministic: %+v vs %+v", first, second)
	}
}

// TestMapFactoryInvocationOutcome_FallsBackToStructuredParts proves a Factory
// that publishes a structured result still returns it.
//
// @you/deep-research publishes its synthesis as a JSON part rather than a text
// one. Projecting only text parts made it produce a completed turn -- end_turn,
// no error -- carrying no assistant message at all, which on the wire is
// indistinguishable from a Factory that ran and had nothing to say.
func TestMapFactoryInvocationOutcome_FallsBackToStructuredParts(t *testing.T) {
	tests := []struct {
		name          string
		primaryResult []work.WorkContentPart
		want          []string
	}{
		{
			name: "json only is serialized in published order",
			primaryResult: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"finding":"first"}`)},
				{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"finding":"second"}`)},
			},
			want: []string{`{"finding":"first"}`, `{"finding":"second"}`},
		},
		{
			// Text is what the Factory chose to say. Appending the structured
			// part beside it would show the same answer twice in two shapes.
			name: "text wins over json when both are published",
			primaryResult: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"finding":"structured"}`)},
				{Type: work.WorkContentPartTypeText, Text: "the prose answer"},
			},
			want: []string{"the prose answer"},
		},
		{
			name: "json alongside parts with no text form still projects the json",
			primaryResult: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeImage, URL: "https://example.invalid/image.png"},
				{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"finding":"only"}`)},
				{Type: work.WorkContentPartTypeBinary, File: "artifact.bin"},
			},
			want: []string{`{"finding":"only"}`},
		},
		{
			// A part that declares JSON but carries nothing is not a result;
			// projecting "" would render an empty assistant message.
			name: "blank json payload projects nothing",
			primaryResult: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeJSON, JSON: []byte("   ")},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapFactoryInvocationOutcome(factorysessions.InvocationResult{
				Status:        factorysessions.InvocationTerminalStatusCompleted,
				PrimaryResult: tt.primaryResult,
			})
			if !reflect.DeepEqual(got.Text, tt.want) {
				t.Fatalf("MapFactoryInvocationOutcome().Text = %#v, want %#v", got.Text, tt.want)
			}
		})
	}
}

// TestFactoryInvocationFailure_AnswersOnlyFailedOutcomes pins which outcomes
// reach ACP's JSON-RPC error channel at all. ACP has no failure stop reason,
// so a failed invocation cannot be told from a successful one inside a
// successful prompt response -- but a cancelled or timed-out turn has a
// truthful stop reason and must keep answering successfully.
func TestFactoryInvocationFailure_AnswersOnlyFailedOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		status    factorysessions.InvocationTerminalStatus
		wantError bool
	}{
		{"failed", factorysessions.InvocationTerminalStatusFailed, true},
		{"completed", factorysessions.InvocationTerminalStatusCompleted, false},
		{"canceled", factorysessions.InvocationTerminalStatusCanceled, false},
		{"timed out", factorysessions.InvocationTerminalStatusTimedOut, false},
		{"unknown", factorysessions.InvocationTerminalStatus("SOMETHING_ELSE"), false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := FactoryInvocationFailure(factorysessions.InvocationResult{
				Status:    testCase.status,
				ErrorCode: string(factorysessions.InvocationErrorCodeRuntimeFailure),
			})
			if testCase.wantError && got == nil {
				t.Fatalf("FactoryInvocationFailure(%q) = nil, want a JSON-RPC error", testCase.status)
			}
			if !testCase.wantError && got != nil {
				t.Fatalf("FactoryInvocationFailure(%q) = %+v, want nil", testCase.status, got)
			}
		})
	}
}

// TestFactoryInvocationFailure_DisclosesOnlyBoundedErrorCodes pins the whole
// disclosure rule in one place, including the branch an unrecognized code
// takes.
//
// The only reason an error code may cross this boundary is that its value set
// is closed: Factory Sessions owns it and it carries no free-form text. A code
// this transport does not recognize has no such guarantee -- it may be a new
// vocabulary entry, or diagnostic text that reached the field by mistake -- so
// it is replaced rather than forwarded. Both branches are asserted here
// directly, because relying on other packages' tests to reach them incidentally
// is what let this coverage lapse in the first place.
func TestFactoryInvocationFailure_DisclosesOnlyBoundedErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{"canceled", string(factorysessions.InvocationErrorCodeCanceled), string(factorysessions.InvocationErrorCodeCanceled)},
		{"runtime failure", string(factorysessions.InvocationErrorCodeRuntimeFailure), string(factorysessions.InvocationErrorCodeRuntimeFailure)},
		{"timed out", string(factorysessions.InvocationErrorCodeTimedOut), string(factorysessions.InvocationErrorCodeTimedOut)},
		{
			name: "unresolved primary result is outside the disclosed vocabulary",
			code: "INVOCATION_PRIMARY_RESULT_UNRESOLVED",
			want: string(factorysessions.InvocationErrorCodeRuntimeFailure),
		},
		{
			name: "diagnostic text is replaced rather than forwarded",
			code: "provider command: /usr/local/bin/agent --token=sk-live-ABC123",
			want: string(factorysessions.InvocationErrorCodeRuntimeFailure),
		},
		{"blank", "", string(factorysessions.InvocationErrorCodeRuntimeFailure)},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			failure := FactoryInvocationFailure(factorysessions.InvocationResult{
				Status:    factorysessions.InvocationTerminalStatusFailed,
				ErrorCode: testCase.code,
				Message:   "provider stderr: token=sk-live-ABC123",
			})
			if failure == nil {
				t.Fatal("FactoryInvocationFailure returned nil for a failed invocation")
			}
			data, ok := failure.Data.(map[string]any)
			if !ok {
				t.Fatalf("error data = %#v, want a bounded map", failure.Data)
			}
			if len(data) != 1 {
				t.Fatalf("error data = %#v, want exactly one member", data)
			}
			if got := data["reason"]; got != testCase.want {
				t.Fatalf("reason = %v, want %q", got, testCase.want)
			}
		})
	}
}
