package service

// This file independently verifies the shared, sanitized L1 V0 ACP
// conformance corpus (internal/testutil/acpfixtures) against the real
// Providers-owned inbound ACP mapper. pkg/transports/acp's outbound
// compatibility boundary asserts its own session/update validation against
// the exact same committed corpus in its fixtures_test.go; neither package
// imports the other's production code, so both protocol directions are
// proven consistent with the same wire inputs independently.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/testutil/acpfixtures"
)

// TestSharedConformanceCorpusSessionUpdateMatchesInboundMapper feeds every
// accepted session/update case in the shared corpus whose update kind this
// inbound mapper actually recognizes (agent_message_chunk,
// agent_thought_chunk, usage_update, session_info_update) through the same
// client.SessionUpdate callback a real ACP SDK connection invokes, and
// asserts the mapper's own observable progress facts against the same raw
// wire input the outbound boundary validated -- not against the outbound
// boundary's TextUpdate shape, which this inbound mapper does not produce.
// user_message_chunk and config_option_update are intentionally out of
// scope: this mapper does not map them to any observable progress fact.
func TestSharedConformanceCorpusSessionUpdateMatchesInboundMapper(t *testing.T) {
	cases, err := acpfixtures.CasesByRole(acpfixtures.RoleSessionUpdate)
	if err != nil {
		t.Fatalf("CasesByRole() error = %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("expected at least one session/update case in the shared corpus")
	}

	var message struct {
		Params json.RawMessage `json:"params"`
	}
	var envelope struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	}
	var wire struct {
		SessionUpdate string `json:"sessionUpdate"`
		Content       *struct {
			Text string `json:"text"`
		} `json:"content"`
		Title *string `json:"title"`
		Size  *int    `json:"size"`
		Used  *int    `json:"used"`
	}

	tested := 0
	for _, c := range cases {
		if c.Classification != acpfixtures.ClassificationAccepted {
			continue
		}
		if err := json.Unmarshal(c.Input, &message); err != nil {
			t.Fatalf("%s: decode raw input JSON-RPC message: %v", c.Name, err)
		}
		if err := json.Unmarshal(message.Params, &envelope); err != nil {
			t.Fatalf("%s: decode raw input envelope: %v", c.Name, err)
		}
		if err := json.Unmarshal(envelope.Update, &wire); err != nil {
			t.Fatalf("%s: decode raw update: %v", c.Name, err)
		}
		switch wire.SessionUpdate {
		case "agent_message_chunk", "agent_thought_chunk", "usage_update", "session_info_update":
		default:
			continue
		}

		var update acpsdk.SessionUpdate
		if err := json.Unmarshal(envelope.Update, &update); err != nil {
			t.Fatalf("%s: decode as acpsdk.SessionUpdate: %v", c.Name, err)
		}
		kind, want := wire.SessionUpdate, wire

		t.Run(c.Name, func(t *testing.T) {
			tested++
			mapperClient := &client{}
			if err := mapperClient.SessionUpdate(context.Background(), acpsdk.SessionNotification{SessionId: acpsdk.SessionId(envelope.SessionID), Update: update}); err != nil {
				t.Fatalf("SessionUpdate() unexpected error: %v", err)
			}
			progress := mapperClient.progressFacts()
			if len(progress) != 1 {
				t.Fatalf("SessionUpdate() produced %d progress facts, want 1", len(progress))
			}
			got := progress[0]

			switch kind {
			case "agent_message_chunk", "agent_thought_chunk":
				if want.Content == nil {
					t.Fatalf("%s: fixture input has no content.text to compare against", c.Name)
				}
				if got.Detail != want.Content.Text {
					t.Errorf("progress detail = %q, want %q (the same text the outbound boundary validated)", got.Detail, want.Content.Text)
				}
				// Only agent_message_chunk accumulates into the assembled
				// response text; agent_thought_chunk is reasoning, which
				// surfaces solely as a progress fact.
				if kind == "agent_message_chunk" && mapperClient.content() != want.Content.Text {
					t.Errorf("accumulated content = %q, want %q", mapperClient.content(), want.Content.Text)
				}
			case "usage_update":
				if want.Used == nil || want.Size == nil {
					t.Fatalf("%s: fixture input is missing size/used", c.Name)
				}
				if got.Metadata["used_tokens"] != fmt.Sprint(*want.Used) {
					t.Errorf("progress used_tokens = %q, want %d", got.Metadata["used_tokens"], *want.Used)
				}
				if got.Metadata["max_tokens"] != fmt.Sprint(*want.Size) {
					t.Errorf("progress max_tokens = %q, want %d", got.Metadata["max_tokens"], *want.Size)
				}
			case "session_info_update":
				if want.Title == nil {
					t.Fatalf("%s: fixture input is missing title", c.Name)
				}
				if got.Detail != *want.Title {
					t.Errorf("progress detail = %q, want %q", got.Detail, *want.Title)
				}
			}
		})
	}

	if tested == 0 {
		t.Fatal("no shared session/update cases matched an inbound-mappable update kind")
	}
}
