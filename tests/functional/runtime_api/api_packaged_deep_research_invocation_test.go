package runtime_api

import (
	"encoding/json"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/deepresearch"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
	if got := string(primary); !strings.Contains(got, "event sourcing for workflow orchestration") || !strings.Contains(got, `"researchDepth":3`) || !strings.Contains(got, `"maxSubagents":1`) {
		t.Fatalf("primary result = %s, want configured lead synthesis", got)
	}
}
