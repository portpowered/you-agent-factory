//go:build functionallong

package runtime_api

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestOOTBExperience_APIPreseededSimplePipelineCompletes(t *testing.T) {
	support.SkipLongFunctional(t, "slow OOTB API simple pipeline sweep")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-ootb-001",
		Payload:    []byte(`{"title":"Hello World"}`),
	})

	host, stream := startOOTBExperienceHost(t, dir)

	initialStatus := getGeneratedJSON[factoryapi.StatusResponse](t, host.Endpoint()+"/status")
	if initialStatus.FactoryState == "" {
		t.Fatal("GET /status returned an empty factory_state during first-run smoke")
	}

	assertTerminalDispatchForTrace(t, stream, "trace-ootb-001")
	token := requireGeneratedWorkByTrace(
		t,
		getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(host.Endpoint(), "/work")),
		"trace-ootb-001",
	)
	if stringPointerValue(token.WorkTypeName) != "task" {
		t.Fatalf("GET /work completed work type = %q, want task", stringPointerValue(token.WorkTypeName))
	}
	if generatedWorkStateName(token.State) != "complete" {
		t.Fatalf("GET /work completed state = %#v, want complete", token.State)
	}

	status := getGeneratedJSON[factoryapi.StatusResponse](t, host.Endpoint()+"/status")
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

	dir := support.ScaffoldFactory(t, ootbTwoStagePipelineConfig())
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-ootb-multistage-001",
		Payload:    []byte(`{"title":"Multi-stage test"}`),
	})

	host, stream := startOOTBExperienceHost(t, dir)

	assertTerminalDispatchForTrace(t, stream, "trace-ootb-multistage-001")
	token := requireGeneratedWorkByTrace(
		t,
		getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(host.Endpoint(), "/work")),
		"trace-ootb-multistage-001",
	)
	if stringPointerValue(token.WorkTypeName) != "task" {
		t.Fatalf("GET /work completed work type = %q, want task", stringPointerValue(token.WorkTypeName))
	}
	if generatedWorkStateName(token.State) != "complete" {
		t.Fatalf("GET /work completed state = %#v, want complete", token.State)
	}

	status := getGeneratedJSON[factoryapi.StatusResponse](t, host.Endpoint()+"/status")
	if status.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens = %d, want 1", status.TotalTokens)
	}
	if status.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1", status.Categories.Terminal)
	}
}

func TestOOTBExperience_APIStatusStaysQueryableAcrossCompletion(t *testing.T) {
	support.SkipLongFunctional(t, "slow OOTB API status-across-completion sweep")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-ootb-status-001",
		Payload:    []byte(`{"title":"Status check"}`),
	})

	host, stream := startOOTBExperienceHost(t, dir)

	initialStatus := getGeneratedJSON[factoryapi.StatusResponse](t, host.Endpoint()+"/status")
	if initialStatus.FactoryState == "" {
		t.Fatal("GET /status returned an empty factory_state before completion")
	}

	assertTerminalDispatchForTrace(t, stream, "trace-ootb-status-001")

	status := getGeneratedJSON[factoryapi.StatusResponse](t, host.Endpoint()+"/status")
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

func startOOTBExperienceHost(t *testing.T, dir string) (*support.RootRunFunctionalHost, *factoryEventHTTPStream) {
	t.Helper()

	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: dir,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)
	return host, stream
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
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "step-two",
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}
