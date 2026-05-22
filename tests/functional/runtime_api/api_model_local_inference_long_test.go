//go:build functionallong

package runtime_api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	realOmniVoiceLongTestEnabledEnv  = "INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS"
	realOmniVoiceLongTestCommandEnv  = "INFINITE_YOU_OMNIVOICE_COMMAND"
	realOmniVoiceLongTestCacheDirEnv = "INFINITE_YOU_OMNIVOICE_CACHE_DIR"
	realOmniVoiceDefaultCommand      = "omnivoice-llamacpp"
)

func TestRealLocalInference_OMNIVOICEModelInvokeAndDirectAPIProduceAudio(t *testing.T) {
	support.SkipLongFunctional(t, "slow real OMNIVOICE local inference sweep")
	if os.Getenv(realOmniVoiceLongTestEnabledEnv) != "1" {
		t.Skip("set INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1 to run real OMNIVOICE local inference coverage")
	}

	command := resolveRealOmniVoiceCommand(t)
	cacheDir := stringsTrimSpaceOrDefault(os.Getenv(realOmniVoiceLongTestCacheDirEnv), filepath.Join(t.TempDir(), "managed-model-cache"))
	dir := support.ScaffoldFactory(t, realLocalInferenceFactoryConfig())
	writeRealLocalInferenceWorkerConfig(t, dir, command)
	writeRealLocalInferenceWorkstationConfig(t, dir)

	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.ModelCacheDir = cacheDir
	}, factory.WithServiceMode())

	pull := postJSON[factoryapi.ModelPullResponse](t, server.URL()+"/models/OMNIVOICE_Q4_K_M/pull", nil, "asset pull failure")
	if pull.ModelName != "OMNIVOICE_Q4_K_M" || pull.CachePath == "" || pull.Revision == "" || len(pull.DownloadedFiles) == 0 {
		t.Fatalf("asset pull failure: response = %#v, want model identity, cache path, revision, and files", pull)
	}

	jsonInvocation := postJSON[factoryapi.ModelInvocationResponse](t, server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Bindings:  realLocalInferenceDirectBindings(),
		Content: &factoryapi.WorkContent{
			mustGeneratedFunctionalTextPart(t, "hello from direct local model invocation"),
		},
	}, "invocation failure")
	if jsonInvocation.ModelName != "OMNIVOICE_Q4_K_M" || jsonInvocation.Operation != "TTS" || jsonInvocation.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
		t.Fatalf("output validation failure: invocation identity = %#v, want OMNIVOICE local TTS metadata", jsonInvocation)
	}
	if len(jsonInvocation.Content) != 1 {
		t.Fatalf("output validation failure: invocation content count = %d, want 1", len(jsonInvocation.Content))
	}
	audioPart, err := jsonInvocation.Content[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("output validation failure: decode invocation audio content: %v", err)
	}
	if stringPointerValue(audioPart.ContentType) != "audio/wav" || stringsTrimSpaceOrDefault(audioPart.File, "") == "" {
		t.Fatalf("output validation failure: invocation audio part = %#v, want audio/wav file output", audioPart)
	}
	assertWAVFile(t, audioPart.File, "output validation failure")

	streamBytes, streamType := postAudioInvocation(t, server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Bindings:  realLocalInferenceDirectBindings(),
		Content: &factoryapi.WorkContent{
			mustGeneratedFunctionalTextPart(t, "hello from streamed local model invocation"),
		},
		Options: &factoryapi.ModelInvocationOptions{
			ResponseMode: func() *factoryapi.ModelInvocationResponseMode {
				mode := factoryapi.AUDIOSTREAM
				return &mode
			}(),
		},
	})
	if streamType != "audio/wav" {
		t.Fatalf("output validation failure: stream content-type = %q, want audio/wav", streamType)
	}
	assertWAVBytes(t, streamBytes, "output validation failure")

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         "real-local-model-inference",
		WorkTypeName: "speech",
		Content: &factoryapi.WorkContent{
			mustGeneratedFunctionalTextPart(t, "hello from factory-level model invoke"),
		},
	})
	work := waitForGeneratedWorkAtPlace(t, server.URL(), traceID, "speech:complete", 60*time.Second)
	complete := findGeneratedWorkByTraceIDAndPlace(t, work.Results, traceID, "speech:complete")
	if complete.Content == nil || len(*complete.Content) != 1 {
		t.Fatalf("output validation failure: completed work content = %#v, want one audio content part", complete.Content)
	}
	completedAudio, err := (*complete.Content)[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("output validation failure: decode completed work audio content: %v", err)
	}
	if stringPointerValue(completedAudio.ContentType) != "audio/wav" || stringsTrimSpaceOrDefault(completedAudio.File, "") == "" {
		t.Fatalf("output validation failure: completed work audio = %#v, want audio/wav file output", completedAudio)
	}
	assertWAVFile(t, completedAudio.File, "output validation failure")
	assertRecordedRealLocalModelEvents(t, server.GetFactoryEvents(t))
}

func resolveRealOmniVoiceCommand(t *testing.T) string {
	t.Helper()
	command := stringsTrimSpaceOrDefault(os.Getenv(realOmniVoiceLongTestCommandEnv), realOmniVoiceDefaultCommand)
	resolved, err := exec.LookPath(command)
	if err != nil {
		t.Fatalf("platform or backend failure: OMNIVOICE runtime command %q not found on PATH: %v", command, err)
	}
	return resolved
}

func realLocalInferenceFactoryConfig() map[string]any {
	return map[string]any{
		"name": "real-local-model-inference",
		"workTypes": []map[string]any{{
			"name": "speech",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       "MODEL",
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name": "tts-worker",
		}},
		"workstations": []map[string]any{{
			"name":      "speak",
			"type":      "MODEL_INVOKE",
			"operation": "TTS",
			"worker":    "tts-worker",
			"operationBindings": []map[string]any{{
				"slot": "text",
				"selector": map[string]any{
					"type": "TEXT",
				},
			}},
			"inputs":    []map[string]string{{"workType": "speech", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "speech", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "speech", "state": "failed"}},
		}},
	}
}

func writeRealLocalInferenceWorkerConfig(t *testing.T, dir string, command string) {
	t.Helper()
	support.WriteAgentConfig(t, dir, "tts-worker", `---
type: MODEL_WORKER
model: OMNIVOICE_Q4_K_M
modelProvider: CODEX
modelLocality: LOCAL
command: `+command+`
resources:
  - name: omnivoice-cache
    capacity: 1
operations:
  - name: TTS
    inputs:
      - name: text
        required: true
        contentTypes:
          - TEXT
    outputs:
      - name: audio
        contentTypes:
          - AUDIO
---
Synthesize speech from the resolved text content.
`)
}

func writeRealLocalInferenceWorkstationConfig(t *testing.T, dir string) {
	t.Helper()
	support.WriteWorkstationConfig(t, dir, "speak", `---
type: MODEL_INVOKE
---
Generate speech output.
`)
}

func realLocalInferenceDirectBindings() *[]factoryapi.WorkstationOperationBinding {
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

func mustGeneratedFunctionalTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
	}); err != nil {
		t.Fatalf("encode generated text part: %v", err)
	}
	return part
}

func postAudioInvocation(t *testing.T, endpoint string, request factoryapi.ModelInvocationRequest) ([]byte, string) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal audio invocation request: %v", err)
	}
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("invocation failure: POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("invocation failure: POST %s status = %d, want 200: %s", endpoint, resp.StatusCode, string(payload))
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("output validation failure: read streamed audio body: %v", err)
	}
	return audio, resp.Header.Get("Content-Type")
}

func postJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("%s: marshal request: %v", failurePrefix, err)
		}
		body = bytes.NewReader(encoded)
	}
	resp, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: POST %s status = %d, want 200: %s", failurePrefix, endpoint, resp.StatusCode, string(payload))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return out
}

func assertWAVFile(t *testing.T, path string, failurePrefix string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: read audio file %q: %v", failurePrefix, path, err)
	}
	assertWAVBytes(t, data, failurePrefix)
}

func assertWAVBytes(t *testing.T, data []byte, failurePrefix string) {
	t.Helper()
	if len(data) <= 44 {
		t.Fatalf("%s: audio byte length = %d, want header plus non-empty sample data", failurePrefix, len(data))
	}
	if !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WAVE")) {
		t.Fatalf("%s: audio header = %q/%q, want RIFF/WAVE", failurePrefix, string(data[:4]), string(data[8:12]))
	}
}

func findGeneratedWorkByTraceIDAndPlace(t *testing.T, works []factoryapi.Work, traceID string, placeID string) factoryapi.Work {
	t.Helper()
	for _, item := range works {
		if stringPointerValue(item.TraceId) == traceID && generatedWorkPlaceID(item) == placeID {
			return item
		}
	}
	t.Fatalf("output validation failure: no work item found for trace %q in %s", traceID, placeID)
	return factoryapi.Work{}
}

func assertRecordedRealLocalModelEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	var sawRequest bool
	var sawResponse bool
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeModelRequest:
			payload, err := event.Payload.AsModelRequestEventPayload()
			if err == nil && payload.Operation == "TTS" {
				sawRequest = true
			}
		case factoryapi.FactoryEventTypeModelResponse:
			payload, err := event.Payload.AsModelResponseEventPayload()
			if err == nil && payload.Operation == "TTS" && payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
				sawResponse = true
			}
		}
	}
	if !sawRequest || !sawResponse {
		t.Fatalf("output validation failure: model events missing TTS request/response; sawRequest=%v sawResponse=%v", sawRequest, sawResponse)
	}
}

func stringsTrimSpaceOrDefault(value string, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
