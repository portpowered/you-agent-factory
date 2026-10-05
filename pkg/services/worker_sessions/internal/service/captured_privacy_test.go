package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestAppendDraftDeclaredSecretTextPreservesWorkerPayload(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	draft := workers.Draft{
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Payload:                    json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"declared-token"},{"kind":"TEXT","text":"visible"}]}`),
		DeclaredSecretJSONPointers: []string{"/contentBlocks/0/text"},
	}
	result, err := r.appendDraft(t.Context(), workersessions.Topic("privacy-worker"), events.AppendIdentity{
		SourceType: "worker_provider", SourceID: "privacy-worker", SourceSequence: 1, SourceEventID: "message",
	}, workerDraftSchemaID, draft)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result.Record.Payload), "declared-token") || strings.Contains(string(result.Record.Payload), "DeclaredSecret") {
		t.Fatal("published event exposed classified text or provenance")
	}
	var published workers.Draft
	if err := json.Unmarshal(result.Record.Payload, &published); err != nil {
		t.Fatal(err)
	}
	if err := workers.ValidateDraft(published); err != nil {
		t.Fatalf("safe published payload lost its Worker type: %v", err)
	}
	var message workers.MessagePayload
	if err := json.Unmarshal(published.Payload, &message); err != nil {
		t.Fatal(err)
	}
	if message.ContentBlocks[0].Text != "<redacted>" || message.ContentBlocks[1].Text != "visible" || !strings.Contains(string(draft.Payload), "declared-token") {
		t.Fatal("redaction changed adjacent text or adapter-owned payload")
	}
}

func TestAppendDraftInvalidDeclaredSecretDoesNotAppend(t *testing.T) {
	t.Parallel()
	for _, pointer := range []string{"/absent-secret-token", "/contentBlocks/7/text", "/contentBlocks/~2/text"} {
		r := newTestRegistry(t)
		identity := events.AppendIdentity{SourceType: "worker_provider", SourceID: "privacy-worker", SourceSequence: 1, SourceEventID: "message"}
		draft := workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
			Payload:                    json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"classified"}]}`),
			DeclaredSecretJSONPointers: []string{pointer},
		}
		_, err := r.appendDraft(t.Context(), workersessions.Topic("privacy-worker"), identity, workerDraftSchemaID, draft)
		if !errors.Is(err, recordings.ErrInvalidRecordingRedactionRequest) || strings.Contains(err.Error(), pointer) {
			t.Fatalf("invalid provenance error = %v", err)
		}
		draft.DeclaredSecretJSONPointers = []string{"/contentBlocks/0/text"}
		result, err := r.appendDraft(t.Context(), workersessions.Topic("privacy-worker"), identity, workerDraftSchemaID, draft)
		if err != nil || result.Record.ID.Position != 1 || result.Outcome != events.AppendOutcomeAccepted {
			t.Fatalf("rejected publication consumed a position or identity: %+v %v", result, err)
		}
	}
}
