package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers/inference"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedTTSFakeAudioFixture = "RIFF....WAVEpayload"

// TestPackagedTTSRequiredInputProducesAudioArtifactMetadata proves that invoking
// the packaged @you/tts Factory with required text input completes under a fake
// model edge and returns public primary-result audio artifact metadata for
// the successful TTS outcome.
func TestPackagedTTSRequiredInputProducesAudioArtifactMetadata(t *testing.T) {
	text := fmt.Sprintf(
		"functional packaged tts required input %d",
		time.Now().UnixNano(),
	)

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	overwritePackagedTTSFactoryWithProviderFakeTopology(t, factoryDir)

	fakeProvider := newPackagedTTSFakeProvider([]byte(packagedTTSFakeAudioFixture))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env: []string{
			"HOME=" + homeDir,
			"USERPROFILE=" + homeDir,
		},
		Edges: serviceedges.Edges{
			ProviderOverride: fakeProvider,
		},
	})

	response := postPackagedTTSInvocation(t, server, text)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one metadata text part", response.PrimaryResult)
	}

	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}

	var metadata factorydefinitions.TTSInvocationMetadata
	if err := json.Unmarshal([]byte(part.Text), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v; text = %q", err, part.Text)
	}
	if strings.TrimSpace(metadata.ArtifactPath) == "" {
		t.Fatalf("artifactPath is empty, want non-empty audio artifact path")
	}
	if metadata.MediaType != "audio/wav" {
		t.Fatalf("mediaType = %q, want audio/wav", metadata.MediaType)
	}
	wantBackend := factorydefinitions.DefaultTTSModelName + "/" + factorydefinitions.DefaultTTSBackendName
	if metadata.Backend != wantBackend {
		t.Fatalf("backend = %q, want %q", metadata.Backend, wantBackend)
	}

	audioBytes, err := os.ReadFile(metadata.ArtifactPath)
	if err != nil {
		t.Fatalf("read artifact audio file %q: %v", metadata.ArtifactPath, err)
	}
	if string(audioBytes) != packagedTTSFakeAudioFixture {
		t.Fatalf("artifact audio = %q, want fake model fixture payload", audioBytes)
	}
	if fakeProvider.callCount() == 0 {
		t.Fatal("fake provider Infer was not called, want packaged factory inference to reach the fake model edge")
	}
}

// TestPackagedTTSOptionalVoiceAndFormatReachModel proves that optional voice and
// format options supplied through the public packaged-factory invocation
// surface reach the fake model edge on resolved TTS operation bindings.
func TestPackagedTTSOptionalVoiceAndFormatReachModel(t *testing.T) {
	text := fmt.Sprintf(
		"functional packaged tts optional voice format %d",
		time.Now().UnixNano(),
	)
	voice := "alloy"
	format := "mp3"

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	overwritePackagedTTSFactoryWithOptionalVoiceAndFormatTopology(t, factoryDir)

	fakeProvider := newPackagedTTSFakeProvider([]byte(packagedTTSFakeAudioFixture))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env: []string{
			"HOME=" + homeDir,
			"USERPROFILE=" + homeDir,
		},
		Edges: serviceedges.Edges{
			ProviderOverride: fakeProvider,
		},
	})

	response := postPackagedTTSInvocationWithArgs(t, server, map[string]any{
		"text":   text,
		"voice":  voice,
		"format": format,
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}

	request := fakeProvider.lastRequest()
	if request == nil {
		t.Fatal("fake provider Infer was not called, want packaged factory inference to reach the fake model edge")
	}
	if voiceBinding, ok := modelBindingJSON(request.ModelBindings, "voice"); !ok {
		t.Fatalf("model bindings = %#v, want voice slot binding", request.ModelBindings)
	} else if got := stringValueFromBindingJSON(voiceBinding, "name"); got != voice {
		t.Fatalf("voice binding name = %q, want %q; binding = %s", got, voice, voiceBinding)
	}
	if formatBinding, ok := modelBindingJSON(request.ModelBindings, "format"); !ok {
		t.Fatalf("model bindings = %#v, want format slot binding", request.ModelBindings)
	} else if got := stringValueFromBindingJSON(formatBinding, "name"); got != format {
		t.Fatalf("format binding name = %q, want %q; binding = %s", got, format, formatBinding)
	}
}

// TestPackagedTTSModelFailureReturnsNoFalseArtifact proves that a forced model
// failure during packaged @you/tts invocation returns a failed public terminal
// outcome without success-shaped TTS audio artifact metadata in the primary
// result for that run.
func TestPackagedTTSModelFailureReturnsNoFalseArtifact(t *testing.T) {
	text := fmt.Sprintf(
		"functional packaged tts model failure %d",
		time.Now().UnixNano(),
	)

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	overwritePackagedTTSFactoryWithProviderFakeTopology(t, factoryDir)

	fakeProvider := newPackagedTTSFailingFakeProvider("omnivoice invoke failed: exit status 1")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env: []string{
			"HOME=" + homeDir,
			"USERPROFILE=" + homeDir,
		},
		Edges: serviceedges.Edges{
			ProviderOverride: fakeProvider,
		},
	})

	response := postPackagedTTSInvocation(t, server, text)
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONTTSGENERATIONFAILED {
		t.Fatalf(
			"invocation errorCode = %#v, want INVOCATION_TTS_GENERATION_FAILED",
			response.ErrorCode,
		)
	}
	if primaryResultContainsTTSArtifactMetadata(t, response.PrimaryResult) {
		t.Fatalf(
			"primary result = %#v, want no success-shaped TTS audio artifact metadata on model failure",
			response.PrimaryResult,
		)
	}
	if fakeProvider.callCount() == 0 {
		t.Fatal("fake provider Infer was not called, want packaged factory inference to reach the fake model edge")
	}
}

type packagedTTSFakeProvider struct {
	mu    sync.Mutex
	audio []byte
	calls int
	last  *workerexecution.ProviderInferenceRequest
}

func newPackagedTTSFakeProvider(audio []byte) *packagedTTSFakeProvider {
	return &packagedTTSFakeProvider{audio: append([]byte(nil), audio...)}
}

func (provider *packagedTTSFakeProvider) callCount() int {
	if provider == nil {
		return 0
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *packagedTTSFakeProvider) lastRequest() *workerexecution.ProviderInferenceRequest {
	if provider == nil {
		return nil
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.last == nil {
		return nil
	}
	cloned := workerexecution.CloneProviderInferenceRequest(*provider.last)
	return &cloned
}

func (provider *packagedTTSFakeProvider) Infer(
	_ context.Context,
	request workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	provider.mu.Lock()
	provider.calls++
	cloned := workerexecution.CloneProviderInferenceRequest(request)
	provider.last = &cloned
	provider.mu.Unlock()
	if strings.TrimSpace(request.ModelOperation) != "TTS" {
		return workerexecution.InferenceResponse{}, fmt.Errorf(
			"packaged tts fake provider unexpected operation %q",
			request.ModelOperation,
		)
	}

	outputFile := filepath.Join(os.TempDir(), fmt.Sprintf("packaged-tts-fake-%d.wav", time.Now().UnixNano()))
	if err := os.WriteFile(outputFile, provider.audio, 0o644); err != nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf("write fake tts audio artifact: %w", err)
	}

	encoded, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        outputFile,
		ContentType: "audio/wav",
		Slot:        "audio",
	}})
	if err != nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf("marshal fake tts audio content: %w", err)
	}
	return workerexecution.InferenceResponse{Content: string(encoded)}, nil
}

var _ providercontract.Provider = (*packagedTTSFakeProvider)(nil)

type packagedTTSFailingFakeProvider struct {
	mu          sync.Mutex
	calls       int
	last        *workerexecution.ProviderInferenceRequest
	failMessage string
}

func newPackagedTTSFailingFakeProvider(message string) *packagedTTSFailingFakeProvider {
	return &packagedTTSFailingFakeProvider{failMessage: message}
}

func (provider *packagedTTSFailingFakeProvider) callCount() int {
	if provider == nil {
		return 0
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *packagedTTSFailingFakeProvider) Infer(
	_ context.Context,
	request workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	provider.mu.Lock()
	provider.calls++
	cloned := workerexecution.CloneProviderInferenceRequest(request)
	provider.last = &cloned
	provider.mu.Unlock()
	if strings.TrimSpace(request.ModelOperation) != "TTS" {
		return workerexecution.InferenceResponse{}, fmt.Errorf(
			"packaged tts failing fake provider unexpected operation %q",
			request.ModelOperation,
		)
	}
	return workerexecution.InferenceResponse{}, errors.New(provider.failMessage)
}

var _ providercontract.Provider = (*packagedTTSFailingFakeProvider)(nil)

// overwritePackagedTTSFactoryWithProviderFakeTopology keeps the installed
// @you/tts layout but replaces authored topology with a cloud-backed inference
// worker that reaches the provider override fake edge.
func overwritePackagedTTSFactoryWithProviderFakeTopology(t *testing.T, factoryDir string) {
	overwritePackagedTTSFactoryTopology(t, factoryDir, scaffoldPackagedTTSLikeFactory)
}

func overwritePackagedTTSFactoryWithOptionalVoiceAndFormatTopology(t *testing.T, factoryDir string) {
	overwritePackagedTTSFactoryTopology(t, factoryDir, scaffoldPackagedTTSLikeFactoryWithOptionalVoiceAndFormat)
}

func overwritePackagedTTSFactoryTopology(
	t *testing.T,
	factoryDir string,
	scaffoldFactory func(*testing.T) string,
) {
	t.Helper()

	scaffoldDir := scaffoldFactory(t)
	scaffoldConfig, err := os.ReadFile(filepath.Join(scaffoldDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read scaffold factory.json: %v", err)
	}

	var scaffold map[string]any
	if err := json.Unmarshal(scaffoldConfig, &scaffold); err != nil {
		t.Fatalf("unmarshal scaffold factory.json: %v", err)
	}
	scaffold["id"] = factorydefinitions.PackagedTTSFactoryProject
	scaffold["name"] = "tts"

	updated, err := json.MarshalIndent(scaffold, "", "  ")
	if err != nil {
		t.Fatalf("marshal scaffold factory.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), updated, 0o644); err != nil {
		t.Fatalf("write installed factory.json: %v", err)
	}

	for _, name := range []string{"factory.yaml", "factory.yml"} {
		path := filepath.Join(factoryDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", name, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(factoryDir, "workers")); err != nil {
		t.Fatalf("remove installed worker sidecars: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(factoryDir, "workstations")); err != nil {
		t.Fatalf("remove installed workstation sidecars: %v", err)
	}
}

func scaffoldPackagedTTSLikeFactory(t *testing.T) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "tts",
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       "MODEL",
			"capacity":   1,
			"model":      factorydefinitions.DefaultTTSModelName,
			"backend":    factorydefinitions.DefaultTTSBackendName,
			"loadPolicy": "ON_DEMAND",
		}},
		"workTypes": []map[string]any{{
			"name": "task",
			"handlingBehavior": []string{
				factorydefinitions.WorkTypeHandlingBehaviorDefault,
			},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "tts-executor",
			"type":          factorydefinitions.WorkerTypeInference,
			"model":         factorydefinitions.DefaultTTSModelName,
			"modelLocality": factorydefinitions.ModelLocalityCloud,
			"modelProvider": "CODEX",
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name":         "text",
					"contentTypes": []string{"TEXT"},
					"required":     true,
				}},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{"AUDIO"},
				}},
			}},
		}},
		"workstations": []map[string]any{{
			"name":      "execute-tts",
			"type":      "INFERENCE_RUN",
			"operation": "TTS",
			"worker":    "tts-executor",
			"operationBindings": []map[string]any{{
				"slot": "text",
				"selector": map[string]any{
					"type": "TEXT",
				},
			}},
			"inputs": []map[string]string{{
				"workType": "task",
				"state":    "init",
			}},
			"outputs": []map[string]string{{
				"workType": "task",
				"state":    "complete",
			}},
			"onFailure": []map[string]string{{
				"workType": "task",
				"state":    "failed",
			}},
		}},
	})
}

func scaffoldPackagedTTSLikeFactoryWithOptionalVoiceAndFormat(t *testing.T) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "tts",
		"invocationSignature": map[string]any{
			"parameters": []map[string]any{
				{
					"name":     "text",
					"required": true,
					"bindings": []map[string]any{
						{"kind": "POSITIONAL", "position": 1},
						{"kind": "STDIN"},
					},
				},
				{
					"name":         "voice",
					"externalName": "voice",
					"typeHint":     "STRING",
					"bindings":     []map[string]any{{"kind": "NAMED"}},
				},
				{
					"name":         "format",
					"externalName": "format",
					"typeHint":     "STRING",
					"bindings":     []map[string]any{{"kind": "NAMED"}},
				},
			},
		},
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       "MODEL",
			"capacity":   1,
			"model":      factorydefinitions.DefaultTTSModelName,
			"backend":    factorydefinitions.DefaultTTSBackendName,
			"loadPolicy": "ON_DEMAND",
		}},
		"workTypes": []map[string]any{{
			"name": "task",
			"handlingBehavior": []string{
				factorydefinitions.WorkTypeHandlingBehaviorDefault,
			},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "tts-executor",
			"type":          factorydefinitions.WorkerTypeInference,
			"model":         factorydefinitions.DefaultTTSModelName,
			"modelLocality": factorydefinitions.ModelLocalityCloud,
			"modelProvider": "CODEX",
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{
					{
						"name":         "text",
						"contentTypes": []string{"TEXT"},
						"required":     true,
					},
					{
						"name":         "voice",
						"contentTypes": []string{"JSON"},
					},
					{
						"name":         "format",
						"contentTypes": []string{"JSON"},
					},
				},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{"AUDIO"},
				}},
			}},
		}},
		"workstations": []map[string]any{{
			"name":      "execute-tts",
			"type":      "INFERENCE_RUN",
			"operation": "TTS",
			"worker":    "tts-executor",
			"operationBindings": []map[string]any{
				{
					"slot": "text",
					"selector": map[string]any{
						"type": "TEXT",
					},
				},
				{
					"slot": "voice",
					"config": []map[string]any{{
						"type": "JSON",
						"role": "voice",
						"json": map[string]any{"name": "${voice}"},
					}},
				},
				{
					"slot": "format",
					"config": []map[string]any{{
						"type": "JSON",
						"role": "format",
						"json": map[string]any{"name": "${format}"},
					}},
				},
			},
			"inputs": []map[string]string{{
				"workType": "task",
				"state":    "init",
			}},
			"outputs": []map[string]string{{
				"workType": "task",
				"state":    "complete",
			}},
			"onFailure": []map[string]string{{
				"workType": "task",
				"state":    "failed",
			}},
		}},
	})
}

func postPackagedTTSInvocation(
	t *testing.T,
	server *support.FunctionalAPIServer,
	text string,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: invocationTextSourceKindPtr(),
		Content:    invocationTextContentPtr(text),
	})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}

	endpoint := strings.TrimSuffix(server.URL(), "/") +
		"/factory-sessions/" + factorysessions.DefaultSessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, string(payload))
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func postPackagedTTSInvocationWithArgs(
	t *testing.T,
	server *support.FunctionalAPIServer,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(factoryapi.InvocationRequest{Args: &args})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}

	endpoint := strings.TrimSuffix(server.URL(), "/") +
		"/factory-sessions/" + factorysessions.DefaultSessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, string(payload))
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func modelBindingJSON(
	bindings []workerexecution.ResolvedModelOperationBinding,
	slot string,
) (json.RawMessage, bool) {
	for _, binding := range bindings {
		if binding.Slot != slot || len(binding.Content) == 0 {
			continue
		}
		part := binding.Content[0]
		if len(part.JSON) > 0 {
			return part.JSON, true
		}
	}
	return nil, false
}

func stringValueFromBindingJSON(payload json.RawMessage, key string) string {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return ""
	}
	value, _ := decoded[key].(string)
	return value
}

func invocationTextSourceKindPtr() *factoryapi.InvocationInputSourceKind {
	sourceKind := factoryapi.InvocationInputSourceKindText
	return &sourceKind
}

func invocationTextContentPtr(text string) *factoryapi.WorkContent {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		panic(fmt.Sprintf("build invocation text content: %v", err))
	}
	content := factoryapi.WorkContent{part}
	return &content
}

func primaryResultContainsTTSArtifactMetadata(
	t *testing.T,
	primaryResult *factoryapi.WorkContent,
) bool {
	t.Helper()
	if primaryResult == nil || len(*primaryResult) == 0 {
		return false
	}
	for _, part := range *primaryResult {
		textPart, err := part.AsWorkTextContentPart()
		if err != nil {
			continue
		}
		var metadata factorydefinitions.TTSInvocationMetadata
		if err := json.Unmarshal([]byte(textPart.Text), &metadata); err != nil {
			continue
		}
		if strings.TrimSpace(metadata.ArtifactPath) != "" && metadata.MediaType != "" {
			return true
		}
	}
	return false
}
