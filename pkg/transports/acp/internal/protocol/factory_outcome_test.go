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
