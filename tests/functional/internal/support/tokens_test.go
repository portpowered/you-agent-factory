package support_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFirstInputWorkID_ReadsPublicWorkFieldsWithoutReturningToken(t *testing.T) {
	raw := []any{
		map[string]any{
			"id": "tok-1",
			"color": map[string]any{
				"work_id": "work-parent",
				"payload": json.RawMessage(`{"title":"chapter"}`),
				"tags":    map[string]string{"lane": "parser"},
			},
		},
	}

	if got := support.FirstInputWorkID(raw); got != "work-parent" {
		t.Fatalf("FirstInputWorkID = %q, want %q", got, "work-parent")
	}
	if got := string(support.FirstInputPayload(raw)); got != `{"title":"chapter"}` {
		t.Fatalf("FirstInputPayload = %q, want chapter JSON", got)
	}
	tags := support.FirstInputTags(raw)
	if tags["lane"] != "parser" {
		t.Fatalf("FirstInputTags = %#v, want lane=parser", tags)
	}
	if got := support.FirstInputWorkID(nil); got != "" {
		t.Fatalf("FirstInputWorkID(nil) = %q, want empty", got)
	}
	if got := support.FirstInputWorkID([]any{}); got != "" {
		t.Fatalf("FirstInputWorkID(empty) = %q, want empty", got)
	}
}

func TestFirstInputWorkID_AcceptsJSONCompatibleTokenSlice(t *testing.T) {
	type color struct {
		WorkID  string            `json:"work_id"`
		Payload []byte            `json:"payload"`
		Tags    map[string]string `json:"tags"`
	}
	type token struct {
		ID    string `json:"id"`
		Color color  `json:"color"`
	}
	raw := []token{{
		ID: "tok-2",
		Color: color{
			WorkID:  "work-from-typed-slice",
			Payload: []byte(`payload-bytes`),
			Tags:    map[string]string{"k": "v"},
		},
	}}

	if got := support.FirstInputWorkID(raw); got != "work-from-typed-slice" {
		t.Fatalf("FirstInputWorkID(typed slice) = %q, want work-from-typed-slice", got)
	}
	if got := string(support.FirstInputPayload(raw)); got != "payload-bytes" {
		t.Fatalf("FirstInputPayload(typed slice) = %q, want payload-bytes", got)
	}
	if got := support.FirstInputTags(raw)["k"]; got != "v" {
		t.Fatalf("FirstInputTags(typed slice)[k] = %q, want v", got)
	}
}
