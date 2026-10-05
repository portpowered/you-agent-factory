package inference_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Current provider-backed model discovery and named invocation retain their
// REST contract independently of retired OmniVoice fixtures.
func TestModelRESTDiscoversAndInvokesConfiguredCloudModel(t *testing.T) {
	t.Parallel()
	const modelName = "customer-cloud"
	dir := support.ScaffoldFactory(t, cloudModelRESTFactory(modelName))
	support.WriteAgentConfig(t, dir, "cloud-worker", support.BuildModelWorkerConfig(models.ProviderCodex, modelName))
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte(`[{"type":"TEXT","text":"customer model result"}]`)})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
	detail := support.GetJSON[factoryapi.ModelDetail](t, server.URL()+"/models/"+modelName)
	if detail.Name != modelName || len(detail.Capabilities) != 1 || detail.Capabilities[0].Worker != "cloud-worker" {
		t.Fatalf("model detail = %#v, want configured cloud model and worker", detail)
	}
	listed := support.GetJSON[factoryapi.ListModelsResponse](t, server.URL()+"/models")
	if _, found := findModelSummary(listed.Results, modelName); !found {
		t.Fatalf("configured model omitted from catalog: %#v", listed)
	}
	pull, err := http.Post(server.URL()+"/models/"+modelName+"/pull", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pull.Body.Close()
	var pullError factoryapi.ErrorResponse
	if err := json.NewDecoder(pull.Body).Decode(&pullError); err != nil {
		t.Fatal(err)
	}
	if pull.StatusCode != http.StatusBadRequest || !strings.Contains(pullError.Message, "not a local model") {
		t.Fatalf("cloud model pull = HTTP %d: %#v, want actionable unsupported-local-pull error", pull.StatusCode, pullError)
	}
	contentType := factoryapi.ModelOperationContentTypeText
	mode := factoryapi.METADATA
	content := factoryapi.WorkContent{mustFunctionalTextPart(t, "customer prompt")}
	result := postFunctionalJSON[factoryapi.ModelInvocationResponse](t, server.URL()+"/models/"+modelName+"/invocations", factoryapi.ModelInvocationRequest{
		Operation: "OMNI", Content: &content, Options: &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
		Bindings: &[]factoryapi.WorkstationOperationBinding{{Slot: "prompt", Selector: &factoryapi.WorkstationOperationBindingSelector{Type: &contentType}}},
	}, "invoke configured cloud model")
	assertCloudModelInvocation(t, modelName, result)
}

func assertCloudModelInvocation(t *testing.T, modelName string, result factoryapi.ModelInvocationResponse) {
	t.Helper()
	if result.ModelName != modelName || result.Worker != "cloud-worker" || result.ProviderLocality != factoryapi.WorkerModelLocalityCloud || len(result.Content) != 1 || len(result.Bindings) != 1 || result.Bindings[0].Slot != "prompt" {
		t.Fatalf("named invocation = %#v, want configured cloud result and prompt binding", result)
	}
	var text factoryapi.WorkTextContentPart
	payload, err := json.Marshal(result.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &text); err != nil {
		t.Fatal(err)
	}
	if text.Text != "customer model result" {
		t.Fatalf("result text = %q", text.Text)
	}
}

func cloudModelRESTFactory(modelName string) map[string]any {
	return map[string]any{
		"name": "cloud-model-rest", "workers": []map[string]any{{
			"name": "cloud-worker", "type": definitions.WorkerTypeModel,
			"model": modelName, "modelProvider": "CODEX", "modelLocality": definitions.ModelLocalityCloud,
			"operations": []map[string]any{{"name": "OMNI",
				"inputs":  []map[string]any{{"name": "prompt", "contentTypes": []string{"TEXT"}, "required": true}},
				"outputs": []map[string]any{{"name": "completion", "contentTypes": []string{"TEXT"}}},
			}},
		}},
	}
}
