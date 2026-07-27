package smoke

import (
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestEndToEndDispatch_CompletesThroughCustomerProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "E2E test"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "E2E done. COMPLETE"},
	)
	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want one terminal work item", status.Categories)
	}

	if provider.CallCount() != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount())
	}

	call := provider.LastCall()
	if call.Model != "test-model" {
		t.Errorf("expected model test-model, got %q", call.Model)
	}
}

func TestEndToEndDispatch_MultipleWorkItemsCompleteIndependently(t *testing.T) {
	dir := support.ScaffoldFactory(t, simpleEndToEndPipelineConfig())
	for i := 0; i < 3; i++ {
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    fmt.Sprintf("trace-e2e-batch-%d", i),
			Payload:    []byte(`{"title":"batch item"}`),
		})
	}

	status := runFactoryThroughCustomerProcess(t, dir, nil)
	if status.Categories.Terminal != 3 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want three terminal work items", status.Categories)
	}
}

func runFactoryThroughCustomerProcess(
	t *testing.T,
	dir string,
	provider providercontract.Provider,
) factoryapi.StatusResponse {
	t.Helper()
	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: server.Start,
		ProviderOverride: provider,
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	status := support.WaitForTerminalStatus(t, server.WaitForURL(t), 15*time.Second)
	daemon.Stop(t)
	return status
}

func simpleEndToEndPipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}
