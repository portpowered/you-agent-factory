package service

// This file independently verifies the shared ACP L1 V0 conformance corpus
// (internal/testutil/acpconformance) against the real Providers-owned
// inbound ACP mapper. It feeds every inbound-supported (round_trip or
// inbound_only) session_update corpus case through the same client callback
// the real ACP SDK connection invokes, so this direction is proven
// independently of the pkg/transports/acp outbound compatibility boundary,
// per ACP-L1-V0-protocol-conformance-004.

import (
	"context"
	"encoding/json"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/testutil/acpconformance"
)

func TestInboundSupportedSessionUpdateCasesMatchProviderMapper(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	cases := corpus.ByRole(acpconformance.RoleSessionUpdate)
	if len(cases) == 0 {
		t.Fatal("expected at least one session_update case")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			if c.Directionality != acpconformance.DirectionRoundTrip && c.Directionality != acpconformance.DirectionInboundOnly {
				t.Fatalf("case %q directionality = %q, want round_trip or inbound_only for an inbound-supported session_update case", c.ID, c.Directionality)
			}
			var update acpsdk.SessionUpdate
			if err := json.Unmarshal(c.Payload, &update); err != nil {
				t.Fatalf("payload does not parse as acpsdk.SessionUpdate: %v", err)
			}

			mapperClient := &client{}
			if err := mapperClient.SessionUpdate(context.Background(), acpsdk.SessionNotification{Update: update}); err != nil {
				t.Fatalf("SessionUpdate() unexpected error: %v", err)
			}
			progress := mapperClient.progressFacts()
			if len(progress) == 0 {
				t.Fatalf("provider mapper produced no progress for case %q", c.ID)
			}
			got := progress[0]
			if got.Metadata["kind"] != c.Facts.Kind {
				t.Errorf("progress kind = %q, want %q", got.Metadata["kind"], c.Facts.Kind)
			}
			if c.Facts.ItemID != "" && got.Metadata["item_id"] != c.Facts.ItemID {
				t.Errorf("progress item_id = %q, want %q", got.Metadata["item_id"], c.Facts.ItemID)
			}
			if c.Facts.Phase != "" && got.Phase != c.Facts.Phase {
				t.Errorf("progress phase = %q, want %q", got.Phase, c.Facts.Phase)
			}
			if c.Facts.Text != "" && got.Detail != c.Facts.Text {
				t.Errorf("progress detail = %q, want %q", got.Detail, c.Facts.Text)
			}
			for key, want := range c.Facts.Metadata {
				if got.Metadata[key] != want {
					t.Errorf("progress metadata[%q] = %q, want %q", key, got.Metadata[key], want)
				}
			}

			text := mapperClient.content()
			if c.Facts.Kind == "message" {
				if text != c.Facts.Text {
					t.Errorf("accumulated text = %q, want %q", text, c.Facts.Text)
				}
			} else if text != "" {
				t.Errorf("accumulated text = %q, want empty for non-message kind %q", text, c.Facts.Kind)
			}
		})
	}
}
