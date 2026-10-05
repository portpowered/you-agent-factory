package workersessions_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPublishDeclaredSecretsReachCaptureAndDownstreamRedacted(t *testing.T) {
	t.Parallel()
	for _, pointer := range []bool{false, true} {
		t.Run(map[bool]string{false: "value", true: "pointer"}[pointer], func(t *testing.T) {
			t.Parallel()
			spy := &workerRecordSpy{}
			var forwarded []workers.ProgressFragment
			publisher := workersessions.NewProviderSessionObservationPublisher(func(fragment workers.ProgressFragment) {
				forwarded = append(forwarded, fragment)
			})
			publisher.Bind(spy)
			draft := workers.Draft{
				Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
				Provenance:                 workers.Provenance{Provider: "codex", NativeEventType: "message.completed"},
				Payload:                    json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"declared-token"},{"kind":"TEXT","text":"visible"}]}`),
				DeclaredSecretJSONPointers: []string{"/contentBlocks/0/text"},
			}
			original := workers.CloneDraft(draft)
			var value any = draft
			if pointer {
				value = &draft
			}
			publisher.Publish(workers.CanonicalDraftFragment("worker-1", value))
			if len(spy.published) != 1 || len(forwarded) != 1 {
				t.Fatal("redacted canonical output did not reach both observers exactly once")
			}
			safe, ok := forwarded[0].CanonicalDraft.(workers.Draft)
			if !ok || !reflect.DeepEqual(safe.Payload, spy.published[0].Draft.Payload) {
				t.Fatal("capture and downstream received different drafts")
			}
			if strings.Contains(string(safe.Payload), "declared-token") || len(safe.DeclaredSecretJSONPointers) != 0 {
				t.Fatal("declared secret or provenance survived publication")
			}
			var message workers.MessagePayload
			if err := json.Unmarshal(safe.Payload, &message); err != nil {
				t.Fatal(err)
			}
			if message.ContentBlocks[0].Text != "<redacted>" || message.ContentBlocks[1].Text != "visible" {
				t.Fatal("redaction lost typed text or adjacent customer content")
			}
			if err := workers.ValidateDraft(safe); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(draft, original) {
				t.Fatal("publication mutated producer-owned draft")
			}
		})
	}
}

func TestPublishInvalidSecretProvenanceReachesNeitherObserver(t *testing.T) {
	t.Parallel()
	spy := &workerRecordSpy{}
	forwarded := 0
	publisher := workersessions.NewProviderSessionObservationPublisher(func(workers.ProgressFragment) { forwarded++ })
	publisher.Bind(spy)
	publisher.Publish(workers.CanonicalDraftFragment("worker-1", workers.Draft{
		Kind: workers.KindProgress, Phase: workers.PhaseUpdated,
		Payload:                    json.RawMessage(`{"label":"progress","message":"declared-token"}`),
		DeclaredSecretJSONPointers: []string{"/missing-declared-token"},
	}))
	if len(spy.bindings) != 0 || len(spy.published) != 0 || forwarded != 0 {
		t.Fatal("invalid provenance had publication or association effects")
	}
}
