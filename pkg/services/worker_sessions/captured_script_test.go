package workersessions_test

import (
	"encoding/json"
	"reflect"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPublishScriptChunksReachCaptureAndDownstream(t *testing.T) {
	t.Parallel()
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			t.Parallel()
			spy := &workerRecordSpy{}
			var forwarded []workers.ProgressFragment
			publisher := workersessions.NewProviderSessionObservationPublisher(func(fragment workers.ProgressFragment) {
				forwarded = append(forwarded, fragment)
			})
			publisher.Bind(spy)
			fragment := workers.ProgressFragment{DispatchID: "script-worker", Kind: workers.ProgressFragmentKind,
				Type: stream, Payload: " exact chunk\n", Metadata: map[string]string{"stream": stream}}
			publisher.Publish(fragment)
			if len(spy.published) != 1 || len(forwarded) != 1 || !reflect.DeepEqual(forwarded[0], fragment) {
				t.Fatal("script output did not reach both observers once without alteration")
			}
			draft := spy.published[0].Draft
			if draft.Kind != workers.KindProgress || draft.Phase != workers.PhaseUpdated || len(spy.bindings) != 0 {
				t.Fatalf("script chunk lost its progress identity: %+v", draft)
			}
			if err := workers.ValidateDraft(draft); err != nil {
				t.Fatal(err)
			}
			var payload workers.ProgressPayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Label != stream || payload.Message != fragment.Payload {
				t.Fatalf("script chunk changed during capture: %+v", payload)
			}
		})
	}
}

func TestPublishBareStreamTypeDoesNotFabricateCapturedOutput(t *testing.T) {
	t.Parallel()
	spy := &workerRecordSpy{}
	publisher := workersessions.NewProviderSessionObservationPublisher(nil)
	publisher.Bind(spy)
	publisher.Publish(workers.ProgressFragment{DispatchID: "script-worker", Kind: workers.ProgressFragmentKind,
		Type: "stdout", Payload: "not a declared command stream"})
	if len(spy.published) != 0 {
		t.Fatal("bare event type fabricated a command observation")
	}
}
