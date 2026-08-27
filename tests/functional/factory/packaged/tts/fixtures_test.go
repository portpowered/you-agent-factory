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

	"github.com/portpowered/infinite-you/internal/testutil"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type packagedTTSFakeProvider struct {
	testutil.ProviderServiceAdapter
	mu           sync.Mutex
	audio        []byte
	artifactRoot string
	audioPath    string
	calls        int
	last         *workerexecution.ProviderInferenceRequest
}

func newPackagedTTSFakeProvider(t testing.TB, audio []byte) *packagedTTSFakeProvider {
	t.Helper()
	provider := &packagedTTSFakeProvider{
		audio:        append([]byte(nil), audio...),
		artifactRoot: t.TempDir(),
	}
	provider.ProviderServiceAdapter.InferFunc = provider.Infer
	return provider
}

func (provider *packagedTTSFakeProvider) callCount() int {
	if provider == nil {
		return 0
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *packagedTTSFakeProvider) lastAudioPath() string {
	if provider == nil {
		return ""
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.audioPath
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
	callNumber := provider.calls
	cloned := workerexecution.CloneProviderInferenceRequest(request)
	provider.last = &cloned
	provider.mu.Unlock()
	if strings.TrimSpace(request.ModelOperation) != "TTS" {
		return workerexecution.InferenceResponse{}, fmt.Errorf(
			"packaged tts fake provider unexpected operation %q",
			request.ModelOperation,
		)
	}

	outputFile := filepath.Join(provider.artifactRoot, fmt.Sprintf("audio-%d.wav", callNumber))
	if err := os.WriteFile(outputFile, provider.audio, 0o644); err != nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf("write fake tts audio artifact: %w", err)
	}
	provider.mu.Lock()
	provider.audioPath = outputFile
	provider.mu.Unlock()

	encoded, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        outputFile,
		ContentType: "audio/wav",
		Slot:        "audio",
	}})
	if err != nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf("marshal fake tts audio content: %w", err)
	}
	return workerexecution.InferenceResponse{
		Content: string(encoded),
		Diagnostics: &workerexecution.WorkDiagnostics{
			Metadata: map[string]string{
				workerexecution.ProviderResponseMetadataCompletionEvidence: "provider_response",
			},
		},
	}, nil
}

type packagedTTSFailingFakeProvider struct {
	testutil.ProviderServiceAdapter
	mu          sync.Mutex
	calls       int
	last        *workerexecution.ProviderInferenceRequest
	failMessage string
}

func newPackagedTTSFailingFakeProvider(message string) *packagedTTSFailingFakeProvider {
	provider := &packagedTTSFailingFakeProvider{failMessage: message}
	provider.ProviderServiceAdapter.InferFunc = provider.Infer
	return provider
}

func (provider *packagedTTSFailingFakeProvider) callCount() int {
	if provider == nil {
		return 0
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *packagedTTSFailingFakeProvider) lastRequest() *workerexecution.ProviderInferenceRequest {
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

// overwritePackagedTTSFactoryWithProviderFakeTopology keeps the installed
// @you/tts layout but replaces authored topology with a cloud-backed inference
// worker that reaches the provider override fake edge.
func overwritePackagedTTSFactoryWithProviderFakeTopology(t *testing.T, factoryDir string) {
	overwritePackagedTTSFactoryTopology(t, factoryDir, scaffoldPackagedTTSLikeFactory)
}

func overwritePackagedTTSFactoryWithCommandRunnerTopology(t *testing.T, factoryDir string) {
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

	packagedConfig, err := os.ReadFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read installed packaged TTS factory.json: %v", err)
	}
	var packaged map[string]any
	if err := json.Unmarshal(packagedConfig, &packaged); err != nil {
		t.Fatalf("unmarshal installed packaged TTS factory.json: %v", err)
	}

	scaffoldDir := scaffoldFactory(t)
	scaffoldConfig, err := os.ReadFile(filepath.Join(scaffoldDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read scaffold factory.json: %v", err)
	}

	var scaffold map[string]any
	if err := json.Unmarshal(scaffoldConfig, &scaffold); err != nil {
		t.Fatalf("unmarshal scaffold factory.json: %v", err)
	}
	preservePackagedTTSWorkstationPrompt(t, factoryDir, packaged, scaffold)
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

func preservePackagedTTSWorkstationPrompt(
	t *testing.T,
	factoryDir string,
	packaged map[string]any,
	scaffold map[string]any,
) {
	t.Helper()

	packagedWorkstations, ok := packaged["workstations"].([]any)
	if !ok {
		t.Fatalf("installed packaged TTS workstations = %#v, want array", packaged["workstations"])
	}
	var prompt any
	for _, raw := range packagedWorkstations {
		workstation, ok := raw.(map[string]any)
		if !ok || workstation["name"] != factorydefinitions.PackagedTTSInvokeWorkstationName {
			continue
		}
		prompt = workstation["body"]
		break
	}
	if prompt == nil {
		agentsPath := filepath.Join(
			factoryDir,
			factorydefinitions.WorkstationsDir,
			factorydefinitions.PackagedTTSInvokeWorkstationName,
			factorydefinitions.FactoryAgentsFileName,
		)
		agents, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatalf("read installed packaged TTS workstation prompt: %v", err)
		}
		prompt = authoredPromptBody(string(agents))
	}

	scaffoldWorkstations, ok := scaffold["workstations"].([]any)
	if !ok {
		t.Fatalf("scaffold TTS workstations = %#v, want array", scaffold["workstations"])
	}
	for _, raw := range scaffoldWorkstations {
		workstation, ok := raw.(map[string]any)
		if !ok || workstation["name"] != factorydefinitions.PackagedTTSInvokeWorkstationName {
			continue
		}
		workstation["body"] = prompt
		return
	}
	t.Fatalf("scaffold TTS workstation %q is missing", factorydefinitions.PackagedTTSInvokeWorkstationName)
}

func authoredPromptBody(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	rest := content[len("---\n"):]
	if index := strings.Index(rest, "\n---\n"); index >= 0 {
		return strings.TrimSpace(rest[index+len("\n---\n"):])
	}
	return content
}

func scaffoldPackagedTTSLikeFactory(t *testing.T) string {
	return scaffoldTTSLikeFactory(t, factorydefinitions.PackagedTTSInvokeWorkstationName)
}

func scaffoldFactoryTTSAudioDispatch(t *testing.T) string {
	return scaffoldTTSLikeFactory(t, "tts-dispatch")
}

func scaffoldTTSLikeFactory(t *testing.T, workstationName string) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "tts",
		"invocationSignature": map[string]any{
			"parameters": []map[string]any{{
				"name":         "text",
				"externalName": "to",
				"required":     true,
				"bindings": []map[string]any{
					{"kind": "POSITIONAL", "position": 1},
					{"kind": "STDIN"},
					{"kind": "NAMED"},
				},
			}},
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
			"name":      workstationName,
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
