//go:build functionallong

package runtime_api

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestOOTBExperience_APIPreseededSimplePipelineCompletes(t *testing.T) {
	support.SkipLongFunctional(t, "slow OOTB API simple pipeline sweep")

	dir := scaffoldFactory(t, simplePipelineConfig())
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-ootb-001",
		Payload:    []byte(`{"title":"Hello World"}`),
	})

	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	initialStatus := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if initialStatus.FactoryState == "" {
		t.Fatal("GET /status returned an empty factory_state during first-run smoke")
	}

	token := waitForGeneratedWorkTypeComplete(t, server.URL(), "task", 10*time.Second)
	if token.WorkType != "task" {
		t.Fatalf("GET /work completed work_type = %q, want task", token.WorkType)
	}
	if token.PlaceId != "task:complete" {
		t.Fatalf("GET /work completed place_id = %q, want task:complete", token.PlaceId)
	}

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens = %d, want 1", status.TotalTokens)
	}
	if status.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1", status.Categories.Terminal)
	}
	if status.Categories.Failed != 0 {
		t.Fatalf("GET /status failed count = %d, want 0", status.Categories.Failed)
	}
}

func TestOOTBExperience_APIPreseededTwoStagePipelineCompletes(t *testing.T) {
	support.SkipLongFunctional(t, "slow OOTB API two-stage pipeline sweep")

	dir := scaffoldFactory(t, ootbTwoStagePipelineConfig())
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-ootb-multistage-001",
		Payload:    []byte(`{"title":"Multi-stage test"}`),
	})

	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	token := waitForGeneratedWorkTypeComplete(t, server.URL(), "task", 10*time.Second)
	if token.WorkType != "task" {
		t.Fatalf("GET /work completed work_type = %q, want task", token.WorkType)
	}
	if token.PlaceId != "task:complete" {
		t.Fatalf("GET /work completed place_id = %q, want task:complete", token.PlaceId)
	}

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens = %d, want 1", status.TotalTokens)
	}
	if status.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1", status.Categories.Terminal)
	}
}

func TestOOTBExperience_APIStatusStaysQueryableAcrossCompletion(t *testing.T) {
	support.SkipLongFunctional(t, "slow OOTB API status-across-completion sweep")

	dir := scaffoldFactory(t, simplePipelineConfig())
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-ootb-status-001",
		Payload:    []byte(`{"title":"Status check"}`),
	})

	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	initialStatus := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if initialStatus.FactoryState == "" {
		t.Fatal("GET /status returned an empty factory_state before completion")
	}

	waitForGeneratedWorkTypeComplete(t, server.URL(), "task", 10*time.Second)

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.FactoryState != "RUNNING" && status.FactoryState != "COMPLETED" {
		t.Fatalf("GET /status factory_state = %q, want RUNNING or COMPLETED", status.FactoryState)
	}
	if status.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens = %d, want 1", status.TotalTokens)
	}
	if status.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1", status.Categories.Terminal)
	}
}

func ootbTwoStagePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{
			{"name": "worker-a"},
			{"name": "worker-b"},
		},
		"workstations": []map[string]any{
			{
				"name":      "step-one",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
			{
				"name":      "step-two",
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
}
