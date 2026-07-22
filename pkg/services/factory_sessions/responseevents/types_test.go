package responseevents_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
)

func TestIsAuthoritativeMessageSnapshot_RejectsPartialTimeoutSnapshots(t *testing.T) {
	partial := responseevents.MessagePayload{
		Role:    "assistant",
		Partial: true,
		ContentBlocks: []responseevents.ContentBlock{{
			Kind: responseevents.ContentBlockText,
			Text: "partial answer before timeout",
		}},
	}
	if responseevents.IsAuthoritativeMessageSnapshot(partial) {
		t.Fatal("partial=true snapshot must not be authoritative")
	}

	final := responseevents.MessagePayload{
		Role: "assistant",
		ContentBlocks: []responseevents.ContentBlock{{
			Kind: responseevents.ContentBlockText,
			Text: "final answer",
		}},
	}
	if !responseevents.IsAuthoritativeMessageSnapshot(final) {
		t.Fatal("non-partial snapshot should be authoritative")
	}
}

func TestCloneDraftCopiesPayload(t *testing.T) {
	source := responseevents.Draft{Payload: []byte(`{"message":"safe"}`)}
	cloned := responseevents.CloneDraft(source)

	cloned.Payload[0] = '['
	if source.Payload[0] != '{' {
		t.Fatalf("source payload mutated through clone: %q", source.Payload)
	}
}
