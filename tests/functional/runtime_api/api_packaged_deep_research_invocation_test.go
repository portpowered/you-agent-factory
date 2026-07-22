package runtime_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_PackagedDeepResearchUsesMaterializedFactorySource(t *testing.T) {
	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	server := startFunctionalServerWithArgs(t, factoryDir, true, nil)

	args := map[string]any{
		"topic":         "event sourcing for workflow orchestration",
		"researchDepth": 3,
		"maxSubagents":  1,
	}
	factory := support.GetJSON[factoryapi.Factory](t, server.URL()+"/factory-sessions/~default/factory")
	workflowFile := filepath.Join(factoryDir, "scripts", "deep-research.workflow.js")
	response := postJSON[factoryapi.FactorySessionSyncExecutionResponse](
		t,
		server.URL()+"/factory-sessions/sync",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: "deep-research-materialized-source",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
				WorkflowFile: &workflowFile,
			},
			Args:         &args,
			Orchestrator: factory.Orchestrator,
		},
		"start durable deep-research session",
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", response.Status)
	}
	if response.Result == nil || response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one synthesized result", response.Result)
	}
	primary, err := json.Marshal((*response.Result.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	if got := string(primary); !strings.Contains(got, "event sourcing for workflow orchestration") || !strings.Contains(got, `"researchDepth":3`) || !strings.Contains(got, `"maxSubagents":1`) || !strings.Contains(got, "research-specialist-technical") {
		t.Fatalf("primary result = %s, want configured lead synthesis", got)
	}
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable JavaScript session ID")
	}
	dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want one bounded specialist dispatch and one lead synthesis", len(dispatches.Dispatches))
	}
	labels := map[string]bool{}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Label != nil {
			labels[*dispatch.Label] = true
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
		}
		if dispatch.ModelProvider == nil || *dispatch.ModelProvider != "CODEX" || dispatch.Model == nil || *dispatch.Model != "gpt-5" || dispatch.ReasoningEffort == nil || *dispatch.ReasoningEffort != "medium" {
			t.Fatalf("dispatch execution selection = provider=%#v model=%#v reasoning=%#v, want approved package defaults", dispatch.ModelProvider, dispatch.Model, dispatch.ReasoningEffort)
		}
	}
	if !labels["research-specialist-technical"] || !labels["lead-research-synthesis"] {
		t.Fatalf("dispatch labels = %#v, want technical specialist and lead synthesis", labels)
	}
}

func listFactorySessionDispatches(t *testing.T, serverURL, sessionID string) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	response, err := http.Get(strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + sessionID + "/dispatches")
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s/dispatches: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var payload bytes.Buffer
		_, _ = payload.ReadFrom(response.Body)
		t.Fatalf("GET /factory-sessions/%s/dispatches status = %d, want 200: %s", sessionID, response.StatusCode, payload.String())
	}
	var decoded factoryapi.ListFactorySessionDispatchesResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode dispatch list: %v", err)
	}
	return decoded
}
