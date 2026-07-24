package support_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBuildProcessWithEdgesOverrideObservesPublicWorkLocation proves the shared
// harness constructs through support.BuildProcess / root construction, replaces
// only an external effect through edges.Edges, and observes completion via the
// public Work listing helpers rather than Petri projections.
func TestBuildProcessWithEdgesOverrideObservesPublicWorkLocation(t *testing.T) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "public-process-observation",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker",
			"behavior":  "STANDARD",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"public process observation"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Public observation done. COMPLETE"},
	)
	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 15*time.Second)

	if provider.CallCount() != 1 {
		t.Fatalf("ProviderOverride call count = %d, want 1 (edges.Edges external-effect seam)", provider.CallCount())
	}
	location := support.WorkCustomerLocation("task", "complete")
	if got := support.CountWorkAtCustomerState(listed, location); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d, want 1; listed=%#v", location, got, listed)
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want one terminal and zero failed", session.Runtime.Progress.Categories)
	}
}

// TestPublicWorkLocationHelpersIgnoreContradictorySessionPetriMarking proves the
// converted work-location helpers follow public Work listings even when a session
// carries a contradictory Petri marking. Support does not expose Petri helpers,
// so authors cannot observe place occupancy through the harness marking path.
func TestPublicWorkLocationHelpersIgnoreContradictorySessionPetriMarking(t *testing.T) {
	workType := "task"
	complete := "complete"
	workID := "work-public-1"
	listed := factoryapi.ListWorkResponse{
		Results: []factoryapi.Work{{
			WorkId:       &workID,
			WorkTypeName: &workType,
			State:        &factoryapi.WorkState{Name: complete, Type: factoryapi.WorkStateTypeTERMINAL},
		}},
	}
	// Fixture only: OpenAPI still projects Petri on sessions, but shared support
	// must not teach authors to read it for work-location assertions.
	session := factoryapi.FactorySession{
		Runtime: factoryapi.FactorySessionRuntime{
			Petri: &factoryapi.FactorySessionPetriProjection{
				Marking: []factoryapi.TokenResponse{{
					Id:       "tok-misleading",
					PlaceId:  "task:init",
					WorkId:   workID,
					WorkType: workType,
					TraceId:  "trace-1",
				}},
			},
		},
	}
	if session.Runtime.Petri == nil || len(session.Runtime.Petri.Marking) != 1 {
		t.Fatal("test fixture must include a contradictory Petri marking")
	}
	if session.Runtime.Petri.Marking[0].PlaceId != "task:init" {
		t.Fatalf("fixture PlaceId = %q, want task:init", session.Runtime.Petri.Marking[0].PlaceId)
	}

	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 1 from Work listing", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 (must not mirror Petri PlaceId)", got)
	}
	if !support.HasWorkAtCustomerState(listed, workID, "task:complete") {
		t.Fatal("HasWorkAtCustomerState must follow Work listing state, not Petri PlaceId")
	}
	if support.HasWorkAtCustomerState(listed, workID, "task:init") {
		t.Fatal("HasWorkAtCustomerState(task:init) must stay false when Work listing says complete")
	}
}

// TestFirstInputExtractorsReturnOnlyPublicCustomerFields proves dispatch input
// helpers return customer field values (work id, payload, tags) rather than a
// private workers.Token / engine snapshot object.
func TestFirstInputExtractorsReturnOnlyPublicCustomerFields(t *testing.T) {
	raw := []any{
		map[string]any{
			"id": "tok-private-shape",
			"color": map[string]any{
				"work_id": "work-public-fields",
				"payload": json.RawMessage(`{"title":"chapter"}`),
				"tags":    map[string]string{"lane": "parser"},
			},
		},
	}

	workID := support.FirstInputWorkID(raw)
	payload := support.FirstInputPayload(raw)
	tags := support.FirstInputTags(raw)

	// Compile-time contract: these helpers return customer field types. Reintroducing
	// a workers.Token (or similar private snapshot) return type fails to compile here.
	assertPublicCustomerFields(t, workID, payload, tags)

	if workID != "work-public-fields" {
		t.Fatalf("FirstInputWorkID = %q, want work-public-fields", workID)
	}
	if string(payload) != `{"title":"chapter"}` {
		t.Fatalf("FirstInputPayload = %q, want chapter JSON", payload)
	}
	if tags["lane"] != "parser" {
		t.Fatalf("FirstInputTags = %#v, want lane=parser", tags)
	}
}

func assertPublicCustomerFields(t *testing.T, workID string, payload []byte, tags map[string]string) {
	t.Helper()
	if workID == "" {
		t.Fatal("work id must be a non-empty string customer field")
	}
	if len(payload) == 0 {
		t.Fatal("payload must be customer []byte content")
	}
	if len(tags) == 0 {
		t.Fatal("tags must be a customer string map")
	}
}
