package runtime_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/deepresearch"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSessionInvocationAPI_PackagedDeepResearchUsesMaterializedFactorySource(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), "@you/deep-research", deepresearch.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	server := startFunctionalServerWithConfig(t, factoryDir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
	})

	args := map[string]any{
		"topic":         "event sourcing for workflow orchestration",
		"researchDepth": "3",
		"maxSubagents":  "1",
	}
	response := postInvocation(t, server.URL(), factoryapi.InvocationRequest{Args: &args})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one synthesized result", response.PrimaryResult)
	}
	primary, err := json.Marshal((*response.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	if got := string(primary); !strings.Contains(got, "event sourcing for workflow orchestration") || !strings.Contains(got, `"researchDepth":3`) || !strings.Contains(got, `"maxSubagents":1`) || !strings.Contains(got, "research-specialist-technical") {
		t.Fatalf("primary result = %s, want configured lead synthesis", got)
	}
	if response.SessionId == nil || strings.TrimSpace(*response.SessionId) == "" {
		t.Fatalf("invocation sessionId = %#v, want durable JavaScript session ID", response.SessionId)
	}
	dispatches := listFactorySessionDispatches(t, server.URL(), *response.SessionId)
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
