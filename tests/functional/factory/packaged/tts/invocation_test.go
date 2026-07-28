package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
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

type packagedTTSFakeProvider struct {
	audio []byte
	calls int
}

func newPackagedTTSFakeProvider(audio []byte) *packagedTTSFakeProvider {
	return &packagedTTSFakeProvider{audio: append([]byte(nil), audio...)}
}

func (provider *packagedTTSFakeProvider) callCount() int {
	if provider == nil {
		return 0
	}
	return provider.calls
}

func (provider *packagedTTSFakeProvider) Infer(
	_ context.Context,
	request workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	provider.calls++
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

// overwritePackagedTTSFactoryWithProviderFakeTopology keeps the installed
// @you/tts layout but replaces authored topology with a cloud-backed inference
// worker that reaches the provider override fake edge.
func overwritePackagedTTSFactoryWithProviderFakeTopology(t *testing.T, factoryDir string) {
	t.Helper()

	scaffoldDir := scaffoldPackagedTTSLikeFactory(t)
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
