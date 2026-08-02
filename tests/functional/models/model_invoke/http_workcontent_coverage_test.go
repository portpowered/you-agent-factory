package model_invoke_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestHTTPModelInvocationPreservesStructuredWorkContent proves the HTTP
// transport carries structured Work content and bindings through the Models
// root invocation input without a Workers/provider execution seam.
func TestHTTPModelInvocationPreservesStructuredWorkContent(t *testing.T) {
	dir := support.ScaffoldFactory(t, httpWorkContentFactoryConfig())
	home := t.TempDir()
	writeReadyOmniVoiceCache(t, home)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       homeEnvironment(home),
		Edges: serviceedges.Edges{
			ModelAssetResolveHomeDirectory: func() (string, error) { return home, nil },
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	catalogResponse, err := http.Get(server.URL() + "/models/OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("get model catalog: %v", err)
	}
	catalogBody, _ := io.ReadAll(catalogResponse.Body)
	_ = catalogResponse.Body.Close()
	if catalogResponse.StatusCode != http.StatusOK {
		t.Fatalf("catalog status = %d, want 200 body = %s", catalogResponse.StatusCode, catalogBody)
	}

	responseMode := factoryapi.METADATA
	response := postHTTPWorkContentJSON[factoryapi.ModelInvocationResponse](
		t,
		server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations",
		factoryapi.ModelInvocationRequest{
			Operation: "TTS",
			Bindings:  httpWorkContentModelBindings(),
			Content: &factoryapi.WorkContent{
				mustHTTPWorkContentTextPart(t, "hello world"),
				mustHTTPWorkContentImagePart(t, "fixtures/review.png"),
				mustHTTPWorkContentJSONPart(t, map[string]any{"note": "coverage"}),
			},
			Options: &factoryapi.ModelInvocationOptions{ResponseMode: &responseMode},
		},
		"model HTTP workcontent coverage invocation",
	)
	if len(response.Content) != 1 {
		t.Fatalf("response content count = %d, want one Models-root JSON result", len(response.Content))
	}
	jsonPart, err := response.Content[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode invocation JSON content: %v", err)
	}
	encoded, err := json.Marshal(jsonPart.Json)
	if err != nil {
		t.Fatalf("marshal echoed JSON content: %v", err)
	}
	for _, want := range []string{"hello world", "fixtures/review.png", "coverage"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("echoed JSON content = %s, want %q", encoded, want)
		}
	}
	if len(response.Bindings) != 1 || response.Bindings[0].Slot != "text" {
		t.Fatalf("response bindings = %#v, want text input lineage", response.Bindings)
	}
}

func httpWorkContentFactoryConfig() map[string]any {
	return map[string]any{
		"name": "http-workcontent-coverage",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{
			"name": "omnivoice-cache", "type": interfaces.ResourceTypeModel,
			"capacity": 1, "model": "OMNIVOICE_Q4_K_M", "backend": "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          interfaces.WorkerTypeModel,
			"model":         "OMNIVOICE_Q4_K_M",
			"modelProvider": "CODEX",
			"modelLocality": interfaces.ModelLocalityLocal,
			"command":       "omnivoice-llamacpp",
			"resources":     []map[string]any{{"name": "omnivoice-cache", "capacity": 1}},
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name": "text", "contentTypes": []string{interfaces.ModelOperationContentTypeText}, "required": true,
				}},
				"outputs": []map[string]any{{
					"name": "audio", "contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		}},
	}
}

func httpWorkContentModelBindings() *[]factoryapi.WorkstationOperationBinding {
	return &[]factoryapi.WorkstationOperationBinding{{
		Slot: "text",
		Selector: &factoryapi.WorkstationOperationBindingSelector{
			Type: func() *factoryapi.ModelOperationContentType {
				value := factoryapi.ModelOperationContentTypeText
				return &value
			}(),
		},
	}}
}

func mustHTTPWorkContentTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
	}); err != nil {
		t.Fatalf("encode text part: %v", err)
	}
	return part
}

func mustHTTPWorkContentImagePart(t *testing.T, path string) factoryapi.WorkContentPart {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
		Type: factoryapi.WorkContentPartTypeImage,
		Url:  factoryapi.WorkContentURLProperty("file://" + path),
	}); err != nil {
		t.Fatalf("encode image part: %v", err)
	}
	return part
}

func mustHTTPWorkContentJSONPart(t *testing.T, value map[string]any) factoryapi.WorkContentPart {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkJsonContentPart(factoryapi.WorkJsonContentPart{
		Type: factoryapi.WorkContentPartTypeJSON,
		Json: value,
	}); err != nil {
		t.Fatalf("encode json part: %v", err)
	}
	return part
}

func postHTTPWorkContentJSON[T any](t *testing.T, endpoint string, payload any, failurePrefix string) T {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", failurePrefix, err)
	}
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, resp.StatusCode, string(payload))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return out
}
