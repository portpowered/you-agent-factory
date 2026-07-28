package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const compositionCoverageUnreachableServer = "http://127.0.0.1:1"

func TestNewCompositionFacadeDelegatesListAndPullThroughOwnedRoot(t *testing.T) {
	t.Parallel()

	var pulled string
	root := compositionModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			return modelinference.List{
				Results: []modelinference.Summary{{Name: "OMNIVOICE_Q4_K_M"}},
			}, nil
		},
		pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
			pulled = name
			return modelinference.PullResult{ModelName: name}, nil
		},
	}
	service := modelscli.New(
		compositionHTTPProtocol(t),
		compositionInvocation{root: root},
	)
	if service == nil {
		t.Fatal("New() = nil, want composition facade")
	}

	var listOut bytes.Buffer
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		Output:  &listOut,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !strings.Contains(listOut.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("List() output = %q, want model name", listOut.String())
	}

	var pullOut bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &pullOut,
	}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if pulled != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("pulled model = %q, want OMNIVOICE_Q4_K_M", pulled)
	}
}

func TestNewCompositionFacadeInvokeFallsBackToLegacyWhenServerSet(t *testing.T) {
	t.Parallel()

	err := modelscli.New(
		compositionHTTPProtocol(t),
		compositionInvocation{root: compositionModelsRoot{}},
	).Invoke(modelscli.InvokeConfig{
		Context:    context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello",
		Server:     compositionCoverageUnreachableServer,
		FactoryDir: t.TempDir(),
		Output:     io.Discard,
	})
	if err == nil {
		t.Fatal("Invoke() error = nil, want legacy remote/bootstrap failure")
	}
}

type exportingCompositionInvocation struct {
	compositionInvocation
}

func (exportingCompositionInvocation) ExportModelInvocationArtifact(sourcePath, destinationPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(destinationPath, data, 0o600)
}

func TestConfigFromCompositionExportsInvocationArtifacts(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(source, []byte("artifact"), 0o600); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "copied.txt")

	cfg := modelscli.ConfigFromComposition(
		compositionHTTPProtocol(t),
		exportingCompositionInvocation{compositionInvocation{root: compositionModelsRoot{}}},
	)
	if cfg.Artifacts == nil {
		t.Fatal("ConfigFromComposition().Artifacts = nil, want artifact exporter")
	}
	if err := cfg.Artifacts.ExportInvocationArtifact(source, destination); err != nil {
		t.Fatalf("ExportInvocationArtifact() error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read copied artifact: %v", err)
	}
	if string(got) != "artifact" {
		t.Fatalf("copied artifact = %q, want %q", string(got), "artifact")
	}
}

func TestConstructedService_RemoteListReportsUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		HTTP:   compositionHTTPProtocol(t),
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}
	err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		Server:  compositionCoverageUnreachableServer,
		Output:  io.Discard,
	})
	if err == nil {
		t.Fatal("List() error = nil, want unreachable endpoint failure")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("List() error = %q, want %q", err.Error(), want)
	}
}

func TestConstructedService_RemoteInspectReportsUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		HTTP:   compositionHTTPProtocol(t),
	})
	err := service.Inspect(modelscli.InspectConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    compositionCoverageUnreachableServer,
		Output:    io.Discard,
	})
	if err == nil {
		t.Fatal("Inspect() error = nil, want unreachable endpoint failure")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Inspect() error = %q, want %q", err.Error(), want)
	}
}

func TestConstructedService_RemotePullReportsUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		HTTP:   compositionHTTPProtocol(t),
	})
	err := service.Pull(modelscli.PullConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    compositionCoverageUnreachableServer,
		Output:    io.Discard,
	})
	if err == nil {
		t.Fatal("Pull() error = nil, want unreachable endpoint failure")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M/pull"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Pull() error = %q, want %q", err.Error(), want)
	}
}

func TestConstructedService_InspectMapsOperationSlots(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getModel: func(context.Context, string) (modelinference.Detail, error) {
				return modelinference.Detail{
					Summary: modelinference.Summary{
						Name: "OMNIVOICE_Q4_K_M",
						Operations: []modelinference.Operation{{
							Name: "TTS",
							Inputs: []modelinference.OperationSlot{{
								Name: "text", ContentTypes: []string{"text"}, Required: boolPtr(true),
							}},
							Outputs: []modelinference.OperationSlot{{
								Name: "audio", ContentTypes: []string{"audio"},
							}},
						}},
					},
					Capabilities: []modelinference.Capability{{
						Worker: "tts-worker",
						Operations: []modelinference.Operation{{
							Name: "TTS",
							Inputs: []modelinference.OperationSlot{{
								Name: "text", ContentTypes: []string{"text"},
							}},
						}},
						ResourceNames: []string{"gpu"},
					}},
				}, nil
			},
		},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			scope, err := (modelinference.RuntimeScopeRef{}).Parse("composition-coverage:catalog-scope")
			if err != nil {
				return modelscli.InvokeRuntimeScope{}, err
			}
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})
	var out bytes.Buffer
	if err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &out,
	}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if !strings.Contains(out.String(), `"text"`) || !strings.Contains(out.String(), `"audio"`) {
		t.Fatalf("Inspect() JSON = %q, want mapped operation slots", out.String())
	}
	if !strings.Contains(out.String(), `"tts-worker"`) || !strings.Contains(out.String(), `"gpu"`) {
		t.Fatalf("Inspect() JSON = %q, want mapped capabilities", out.String())
	}
}

func TestConstructedService_RemotePullJSONEncodesSuccessResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M/pull" {
			t.Fatalf("path = %q, want pull path", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(factoryapi.ModelPullResponse{ModelName: "OMNIVOICE_Q4_K_M"})
	}))
	defer server.Close()

	var out bytes.Buffer
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		HTTP:   compositionHTTPProtocol(t),
	})
	if err := service.Pull(modelscli.PullConfig{
		Context:   context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    strings.TrimSuffix(server.URL, "/"),
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !strings.Contains(out.String(), "OMNIVOICE_Q4_K_M") {
		t.Fatalf("Pull() JSON = %q, want model name", out.String())
	}
}

func boolPtr(value bool) *bool {
	return &value
}
