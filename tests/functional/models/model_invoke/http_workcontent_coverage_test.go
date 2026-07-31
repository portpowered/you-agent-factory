package model_invoke_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers/inference"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestHTTPModelInvocationMapsAudioWorkContent proves model HTTP transport maps
// audio work content through the shared workcontent mapper for functional coverage.
func TestHTTPModelInvocationMapsAudioWorkContent(t *testing.T) {
	dir := support.ScaffoldFactory(t, httpWorkContentFactoryConfig())
	support.WriteAgentConfig(t, dir, "tts-worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "OMNIVOICE_Q4_K_M"))

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	if err := os.WriteFile(audioPath, []byte("RIFF....WAVEpayload"), 0o644); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	providerStub := &httpWorkContentProvider{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalHTTPWorkContentAudioResponse(t, audioPath),
		},
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: providerStub,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

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
		t.Fatalf("response content count = %d, want 1", len(response.Content))
	}
	audioPart, err := response.Content[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("decode invocation audio content: %v", err)
	}
	if stringPointerValue(audioPart.ContentType) != "audio/wav" || httpWorkContentAudioPath(audioPart) != audioPath {
		t.Fatalf("audio part = %#v, want audio/wav at %s", audioPart, audioPath)
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
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          interfaces.WorkerTypeModel,
			"model":         "OMNIVOICE_Q4_K_M",
			"modelProvider": "CODEX",
			"modelLocality": interfaces.ModelLocalityCloud,
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

func mustMarshalHTTPWorkContentAudioResponse(t *testing.T, audioPath string) string {
	t.Helper()
	body, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}})
	if err != nil {
		t.Fatalf("marshal audio response content: %v", err)
	}
	return string(body)
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

func httpWorkContentAudioPath(audio factoryapi.WorkAudioContentPart) string {
	if audio.File != nil && string(*audio.File) != "" {
		return string(*audio.File)
	}
	return string(audio.Url)
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

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

type httpWorkContentProvider struct {
	mu       sync.Mutex
	response workerexecution.InferenceResponse
}

func (provider *httpWorkContentProvider) Infer(
	_ context.Context,
	_ workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.response, nil
}

var _ providercontract.Provider = (*httpWorkContentProvider)(nil)
