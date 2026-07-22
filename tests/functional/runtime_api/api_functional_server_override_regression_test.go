package runtime_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type functionalStateCategories struct {
	Failed     int
	Initial    int
	Processing int
	Terminal   int
}

func TestFunctionalServerOverrideCompatibilityRegression_MockWorkersAndProviderOverride(t *testing.T) {
	support.SkipLongFunctional(t, "slow functional-server override sweep")
	t.Run("StartFunctionalServerMockWorkersCompletes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"mock worker compatibility"}`))

		server := startFunctionalServer(t, dir, true)
		status := waitForFunctionalServerCompletion(t, server, 10*time.Second)
		categories := functionalStateCategoriesFromStatus(status)

		if categories.Terminal != 1 {
			t.Fatalf("terminal token count = %d, want 1", categories.Terminal)
		}
		if categories.Failed != 0 {
			t.Fatalf("failed token count = %d, want 0", categories.Failed)
		}
	})

	t.Run("ProviderOverrideIsAppliedBeforeServiceBuildForHTTPRuntime", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
		support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

		runner := testutil.NewProviderCommandRunner(
			platformprocess.CommandResult{Stdout: []byte("first runtime step complete. COMPLETE")},
			platformprocess.CommandResult{Stdout: []byte("second runtime step complete. COMPLETE")},
		)
		server := startFunctionalServerWithArgs(
			t,
			dir,
			false,
			nil,
			withWorkerCommands(runner, nil),
		)

		traceID := submitFunctionalServerWork(t, server, "task", []byte(`{"title":"provider override regression"}`))
		if traceID == "" {
			t.Fatal("expected POST /work to return a trace ID")
		}

		status := waitForFunctionalServerIdleTerminal(t, server, 10*time.Second)
		categories := functionalStateCategoriesFromStatus(status)
		if categories.Failed != 0 {
			t.Fatalf("failed token count = %d, want 0", categories.Failed)
		}

		if got := runner.CallCount(); got != 2 {
			t.Fatalf("provider command runner calls = %d, want 2", got)
		}
		for i, req := range runner.Requests() {
			if req.Command != string(modelprovider.ProviderCodex) {
				t.Fatalf("provider request %d command = %q, want %q", i, req.Command, modelprovider.ProviderCodex)
			}
		}
	})
}

func submitFunctionalServerWork(t *testing.T, server *functionalAPIServer, workTypeID string, payload []byte) string {
	t.Helper()

	reqBody, err := json.Marshal(factoryapi.SubmitWorkRequest{
		Name:         stringPtr("override-regression-submit"),
		WorkTypeName: workTypeID,
		Payload:      payload,
	})
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}

	resp, err := http.Post(support.DefaultSessionWorkURL(server.URL(), "/work"), "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work: expected 201 Created, got %d", resp.StatusCode)
	}

	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	return result.TraceId
}

func waitForFunctionalServerCompletion(
	t *testing.T,
	server *functionalAPIServer,
	timeout time.Duration,
) factoryapi.StatusResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
		if status.FactoryState == string(interfaces.FactoryStateCompleted) ||
			(status.RuntimeStatus == string(interfaces.RuntimeStatusIdle) &&
				status.Categories.Processing == 0 &&
				status.Categories.Terminal+status.Categories.Failed > 0) {
			return status
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("factory did not reach COMPLETED within %s", timeout)
	return factoryapi.StatusResponse{}
}

func waitForFunctionalServerIdleTerminal(
	t *testing.T,
	server *functionalAPIServer,
	timeout time.Duration,
) factoryapi.StatusResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
		if status.FactoryState == string(interfaces.FactoryStateRunning) &&
			status.RuntimeStatus == string(interfaces.RuntimeStatusIdle) &&
			status.Categories.Terminal == 1 {
			return status
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("factory did not reach running idle terminal state within %s", timeout)
	return factoryapi.StatusResponse{}
}

func functionalStateCategoriesFromStatus(status factoryapi.StatusResponse) functionalStateCategories {
	return functionalStateCategories{
		Failed:     status.Categories.Failed,
		Initial:    status.Categories.Initial,
		Processing: status.Categories.Processing,
		Terminal:   status.Categories.Terminal,
	}
}
