package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type commandServiceFake struct {
	list    func(ListConfig) error
	inspect func(InspectConfig) error
	invoke  func(InvokeConfig) error
	pull    func(PullConfig) error
	remove  func(RemoveConfig) error
}
type modelsPullDoer func(*http.Request) (*http.Response, error)

func (doer modelsPullDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type modelsPullRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper modelsPullRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func (fake commandServiceFake) List(cfg ListConfig) error       { return fake.list(cfg) }
func (fake commandServiceFake) Inspect(cfg InspectConfig) error { return fake.inspect(cfg) }
func (fake commandServiceFake) Invoke(cfg InvokeConfig) error   { return fake.invoke(cfg) }
func (fake commandServiceFake) Pull(cfg PullConfig) error       { return fake.pull(cfg) }
func (fake commandServiceFake) Remove(cfg RemoveConfig) error {
	if fake.remove != nil {
		return fake.remove(cfg)
	}
	return nil
}
func TestCommandHandlerTransformsInvokeCommandState(t *testing.T) {
	server := "http://127.0.0.1:7437"
	logger := zap.NewNop()
	var diagnostics bytes.Buffer

	handler := NewCommandHandler(
		commandServiceFake{invoke: func(cfg InvokeConfig) error {
			assertInvokeCommandConfig(t, cfg, server, logger, &diagnostics)
			if len(cfg.ParameterSpecs) != 1 || cfg.ParameterSpecs[0] != `{"name":"temperature","value":0.2}` {
				t.Fatalf("InvokeConfig parameter specs = %#v", cfg.ParameterSpecs)
			}
			return nil
		}},
		func(*cobra.Command) io.Writer { return &diagnostics },
		func() (string, error) { return "/home/tester", nil },
		func(_ *cobra.Command, homeDir string) (operatorconfig.ResolvedDefaults, error) {
			if homeDir != "/home/tester" {
				t.Fatalf("operator defaults home = %q", homeDir)
			}
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return logger, nil },
	)

	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	cmd.SetOut(io.Discard)
	invokeInputs, inherited := resolvedInvokeHandlerInputs(t, server)
	if err := handler.Invoke(cmd, invokeInputs, inherited); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
}

func resolvedInvokeHandlerInputs(
	t *testing.T,
	server string,
) (resolvedinput.Inputs, resolvedinput.Inputs) {
	t.Helper()
	local, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: modelsInvokeNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument}},
			{ID: modelsInvokeOperationID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeTextID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeInputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeParameterID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeOutputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
		},
		[]resolvedinput.Candidate{
			{InputID: modelsInvokeNameInputID, Source: resolvedinput.SourcePositionalArgument, Value: resolvedinput.StringValue("OMNIVOICE_Q4_K_M")},
			{InputID: modelsInvokeOperationID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("TTS")},
			{InputID: modelsInvokeTextID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("hello")},
			{InputID: modelsInvokeInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue([]string{`{"name":"prompt","modality":"TEXT","contentType":"text/plain","mediaType":"text/plain","content":"hello"}`})},
			{InputID: modelsInvokeParameterID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue([]string{`{"name":"temperature","value":0.2}`})},
			{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("speech.wav")},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, inherited := resolvedModelsHandlerInputs(t, server)
	return local, inherited
}
func TestCommandHandlerTransformsListInspectAndPullArguments(t *testing.T) {
	server := "http://127.0.0.1:7437"
	called := map[string]bool{}
	handler := NewCommandHandler(
		commandServiceFake{
			list: func(cfg ListConfig) error {
				called["list"] = true
				if cfg.Server != server || !cfg.JSON || !cfg.Verbose || !cfg.Debug {
					t.Fatalf("ListConfig = %#v", cfg)
				}
				return nil
			},
			inspect: func(cfg InspectConfig) error {
				called["inspect"] = true
				if cfg.ModelName != "model-a" || cfg.Server != server {
					t.Fatalf("InspectConfig = %#v", cfg)
				}
				return nil
			},
			pull: func(cfg PullConfig) error {
				called["pull"] = true
				if cfg.ModelName != "model-b" || cfg.Server != server || cfg.Context.Err() != context.Canceled {
					t.Fatalf("PullConfig = %#v", cfg)
				}
				return nil
			},
		},
		func(*cobra.Command) io.Writer { return io.Discard },
		nil,
		nil,
		nil,
	)
	cmd := &cobra.Command{Use: "models"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	inspectInputs, pullInputs, inherited := resolvedModelsHandlerInputs(t, server)
	if err := handler.List(cmd, resolvedinput.Inputs{}, inherited); err != nil {
		t.Fatal(err)
	}
	if err := handler.Inspect(cmd, inspectInputs, inherited); err != nil {
		t.Fatal(err)
	}
	if err := handler.Pull(cmd, pullInputs, inherited); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"list", "inspect", "pull"} {
		if !called[operation] {
			t.Fatalf("%s service operation was not called", operation)
		}
	}
}

func TestCommandHandlerTransformsRemoveArguments(t *testing.T) {
	server := "http://127.0.0.1:7437"
	var received RemoveConfig
	handler := NewCommandHandler(
		commandServiceFake{remove: func(cfg RemoveConfig) error {
			received = cfg
			return nil
		}},
		func(*cobra.Command) io.Writer { return io.Discard },
		nil,
		nil,
		nil,
	)
	cmd := &cobra.Command{Use: "remove"}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	_, _, inherited := resolvedModelsHandlerInputs(t, server)
	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{
			ID: modelsRemoveNameInputID, Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
		}},
		[]resolvedinput.Candidate{{
			InputID: modelsRemoveNameInputID, Source: resolvedinput.SourcePositionalArgument,
			Value: resolvedinput.StringValue("model-cache"),
		}},
	)
	if err != nil {
		t.Fatalf("resolve remove inputs: %v", err)
	}
	if err := handler.Remove(cmd, inputs, inherited); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if received.ModelName != "model-cache" || received.Server != server || !received.JSON || !received.Verbose || !received.Debug {
		t.Fatalf("RemoveConfig = %#v, want resolved model and common flags", received)
	}
}

func TestRootService_RemoveProjectsInstalledAssetOutcome(t *testing.T) {
	rootScope, err := (modelinference.RuntimeScopeRef{}).Parse("models-cli:remove-scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	var received modelinference.RemoveModelAssetsRequest
	var closed bool
	service := NewService(Config{
		Models: ownedCoverageModelsRoot{
			removeModel: func(_ context.Context, request modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
				received = request
				return modelinference.RemoveModelAssetsResult{
					ModelName: request.Name, Revision: "rev-2026", CachePath: "/models/model-cache/rev-2026",
					BytesRemoved: 42, Outcome: modelinference.AssetRemovalRemoved,
				}, nil
			},
		},
		OpenCatalogScope: func(context.Context) (InvokeRuntimeScope, error) {
			return InvokeRuntimeScope{Scope: rootScope, Close: func(context.Context) error {
				closed = true
				return nil
			}}, nil
		},
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}

	var human bytes.Buffer
	if err := service.Remove(RemoveConfig{
		Context: context.Background(), ModelName: " model-cache ", Output: &human,
	}); err != nil {
		t.Fatalf("Remove() human error = %v", err)
	}
	if want := "MODEL\tREMOVE OUTCOME\tREVISION\tCACHE PATH\tBYTES REMOVED\nmodel-cache\tREMOVED\trev-2026\t/models/model-cache/rev-2026\t42 B (42 bytes)\n"; human.String() != want {
		t.Fatalf("Remove() human = %q, want %q", human.String(), want)
	}
	if received.Scope != rootScope || received.Name != "model-cache" || !closed {
		t.Fatalf("remove request = %#v, closed = %v, want scoped model-cache request and closed scope", received, closed)
	}
}

func TestRootService_RemoteCommandsPreservePublicProjections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/models":
			_, _ = io.WriteString(w, `{"results":[{"name":"remote-model","status":"READY","managedRuntime":{"readinessState":"READY","lifecycleState":"INSTALLED"}}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/models/remote-model":
			_, _ = io.WriteString(w, `{"name":"remote-model","status":"READY","operations":[{"name":"TTS"}],"managedRuntime":{"readinessState":"READY","lifecycleState":"INSTALLED"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/models/remote-model":
			_, _ = io.WriteString(w, `{"modelName":"remote-model","outcome":"REMOVED","revision":"rev-2026","cachePath":"/models/remote-model/rev-2026","bytesRemoved":42}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService(Config{Models: ownedCoverageModelsRoot{}, HTTP: testHTTPProtocol(t)})
	baseURL := strings.TrimSuffix(server.URL, "/")
	assertRemoteListProjection(t, service, baseURL)
	assertRemoteInspectProjection(t, service, baseURL)
	assertRemoteRemoveProjection(t, service, baseURL)
}

func assertRemoteListProjection(t *testing.T, service Service, baseURL string) {
	t.Helper()
	var listOutput bytes.Buffer
	if err := service.List(ListConfig{Context: context.Background(), Server: baseURL, JSON: true, Output: &listOutput}); err != nil {
		t.Fatalf("remote List() error = %v", err)
	}
	var listed factoryapi.ListModelsResponse
	if err := json.Unmarshal(listOutput.Bytes(), &listed); err != nil {
		t.Fatalf("decode remote List() output: %v\n%s", err, listOutput.String())
	}
	if len(listed.Results) != 1 || listed.Results[0].Name != "remote-model" || listed.Results[0].ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("remote list response = %#v, want ready remote-model", listed)
	}
}

func assertRemoteInspectProjection(t *testing.T, service Service, baseURL string) {
	t.Helper()
	var inspectOutput bytes.Buffer
	if err := service.Inspect(InspectConfig{Context: context.Background(), ModelName: "remote-model", Server: baseURL, Output: &inspectOutput}); err != nil {
		t.Fatalf("remote Inspect() error = %v", err)
	}
	if !strings.Contains(inspectOutput.String(), "Name:\tremote-model") || !strings.Contains(inspectOutput.String(), "Operations:\tTTS") {
		t.Fatalf("remote inspect output = %q, want model identity and operation", inspectOutput.String())
	}
}

func assertRemoteRemoveProjection(t *testing.T, service Service, baseURL string) {
	t.Helper()
	var removeOutput bytes.Buffer
	if err := service.Remove(RemoveConfig{Context: context.Background(), ModelName: "remote-model", Server: baseURL, JSON: true, Output: &removeOutput}); err != nil {
		t.Fatalf("remote Remove() error = %v", err)
	}
	var removed factoryapi.ModelRemoveResponse
	if err := json.Unmarshal(removeOutput.Bytes(), &removed); err != nil {
		t.Fatalf("decode remote Remove() output: %v\n%s", err, removeOutput.String())
	}
	if removed.ModelName != "remote-model" || removed.Outcome != factoryapi.REMOVED || removed.BytesRemoved != 42 {
		t.Fatalf("remote remove response = %#v, want removed remote-model/42 bytes", removed)
	}
}

func TestLegacyHTTPService_RemoveWritesRemoteJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/models/legacy-model" {
			t.Fatalf("remove request = %s %s, want DELETE /models/legacy-model", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"modelName":"legacy-model","outcome":"REMOVED","revision":"rev-legacy","bytesRemoved":7}`)
	}))
	defer server.Close()

	service := New(testHTTPProtocol(t), testModelInvocationBuilder)
	if service == nil {
		t.Fatal("New() = nil, want legacy HTTP service")
	}
	var output bytes.Buffer
	if err := service.Remove(RemoveConfig{
		Context: context.Background(), ModelName: "legacy-model", Server: strings.TrimSuffix(server.URL, "/"),
		JSON: true, Output: &output,
	}); err != nil {
		t.Fatalf("legacy Remove() error = %v", err)
	}
	var removed factoryapi.ModelRemoveResponse
	if err := json.Unmarshal(output.Bytes(), &removed); err != nil {
		t.Fatalf("decode legacy Remove() output: %v\n%s", err, output.String())
	}
	if removed.ModelName != "legacy-model" || removed.Revision != "rev-legacy" || removed.BytesRemoved != 7 {
		t.Fatalf("legacy remove response = %#v, want legacy-model/rev-legacy/7", removed)
	}
}

func TestLegacyHTTPService_RemoveMapsNotFoundResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/models/missing-model" {
			t.Fatalf("remove request = %s %s, want DELETE /models/missing-model", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"model cache not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
	}))
	defer server.Close()

	service := New(testHTTPProtocol(t), testModelInvocationBuilder)
	err := service.Remove(RemoveConfig{
		Context: context.Background(), ModelName: "missing-model", Server: strings.TrimSuffix(server.URL, "/"),
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("Remove() error = nil, want not-found failure")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("Remove() error = %v, want ErrModelNotFound", err)
	}
}

func TestModelsCLI_RemoveValidatesRequiredInputs(t *testing.T) {
	services := []struct {
		name    string
		service Service
	}{
		{name: "owned", service: NewService(Config{Models: ownedCoverageModelsRoot{}})},
		{name: "legacy", service: New(testHTTPProtocol(t), testModelInvocationBuilder)},
	}
	for _, tc := range services {
		t.Run(tc.name, func(t *testing.T) {
			if tc.service == nil {
				t.Fatal("service = nil, want Models CLI service")
			}
			if err := tc.service.Remove(RemoveConfig{Output: io.Discard}); err == nil || err.Error() != "context is required" {
				t.Fatalf("Remove(nil context) error = %v, want context is required", err)
			}
			if err := tc.service.Remove(RemoveConfig{Context: context.Background()}); err == nil || err.Error() != "output writer is required" {
				t.Fatalf("Remove(nil output) error = %v, want output writer is required", err)
			}
			if err := tc.service.Remove(RemoveConfig{Context: context.Background(), Output: io.Discard}); err == nil || err.Error() != "model name is required" {
				t.Fatalf("Remove(empty model) error = %v, want model name is required", err)
			}
		})
	}
}

func TestCompositionService_RemoveDelegatesToOwnedModelsRoot(t *testing.T) {
	root := ownedCoverageModelsRoot{
		removeModel: func(_ context.Context, request modelinference.RemoveModelAssetsRequest) (modelinference.RemoveModelAssetsResult, error) {
			return modelinference.RemoveModelAssetsResult{
				ModelName: request.Name, Outcome: modelinference.AssetRemovalRemoved,
			}, nil
		},
	}
	service := New(ownedCoverageHTTPProtocol(t), ownedCoverageCompositionInvocation{root: root})
	if service == nil {
		t.Fatal("New() = nil, want composition service")
	}
	var output bytes.Buffer
	if err := service.Remove(RemoveConfig{
		Context: context.Background(), ModelName: "owned-model", Output: &output,
	}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if !strings.Contains(output.String(), "owned-model\tREMOVED") {
		t.Fatalf("Remove() output = %q, want owned model removal", output.String())
	}
}

func resolvedModelsHandlerInputs(
	t *testing.T,
	server string,
) (resolvedinput.Inputs, resolvedinput.Inputs, resolvedinput.Inputs) {
	t.Helper()
	inherited, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: serverInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: jsonInputID, Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: verboseInputID, Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: debugInputID, Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
		},
		[]resolvedinput.Candidate{
			{InputID: serverInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue(server)},
			{InputID: jsonInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)},
			{InputID: verboseInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)},
			{InputID: debugInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	inspectInputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{
			ID: modelsInspectNameInputID, Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
		}},
		[]resolvedinput.Candidate{{
			InputID: modelsInspectNameInputID, Source: resolvedinput.SourcePositionalArgument,
			Value: resolvedinput.StringValue("model-a"),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	pullInputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{
			ID: modelsPullNameInputID, Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
		}},
		[]resolvedinput.Candidate{{
			InputID: modelsPullNameInputID, Source: resolvedinput.SourcePositionalArgument,
			Value: resolvedinput.StringValue("model-b"),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return inspectInputs, pullInputs, inherited
}

func TestRenderList_WritesDiscoveredModelsTable(t *testing.T) {
	cacheBytes := int64(1234)
	var out bytes.Buffer
	err := renderList(factoryapi.ListModelsResponse{
		Results: []factoryapi.ModelSummary{{
			Name:             "OMNIVOICE_Q4_K_M",
			ProviderLocality: factoryapi.WorkerModelLocalityLocal,
			Status:           factoryapi.ModelStatusREADY,
			LoadState:        factoryapi.UNLOADED,
			ManagedRuntime: factoryapi.ManagedRuntime{
				Identity:       "OMNIVOICE_Q4_K_M",
				ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
				LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
				Locality:       factoryapi.WorkerModelLocalityLocal,
				CacheBytes:     &cacheBytes,
			},
			Operations: []factoryapi.ModelInvocationOperation{{Name: "TTS"}},
			Modalities: []factoryapi.ModelInvocationContentType{factoryapi.ModelInvocationContentTypeAudio, factoryapi.ModelInvocationContentTypeText},
			Resources:  []factoryapi.ModelResourceSummary{{Name: "voice-cache", Type: factoryapi.ResourceTypeModel, Capacity: 1}},
		}},
	}, &out)
	if err != nil {
		t.Fatalf("RenderList: %v", err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "READINESS", "LIFECYCLE", "CACHE SIZE", "OMNIVOICE_Q4_K_M", "LOCAL", "READY", "INSTALLED", "TTS", "AUDIO,TEXT", "1.21 KiB (1234 bytes)"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("rendered table missing %q:\n%s", want, got)
		}
	}
}

func TestManagedRuntimeMappingPreservesCacheFacts(t *testing.T) {
	revision := "rev-1"
	cachePath := "/tmp/models/OMNIVOICE_Q4_K_M/rev-1"
	cacheBytes := int64(1234)
	got := managedRuntimeToGenerated(modelinference.Runtime{
		Identity: "OMNIVOICE_Q4_K_M",
		Revision: &revision, CachePath: &cachePath, CacheBytes: &cacheBytes,
	})
	if got.Revision == nil || *got.Revision != revision ||
		got.CachePath == nil || *got.CachePath != cachePath ||
		got.CacheBytes == nil || *got.CacheBytes != cacheBytes {
		t.Fatalf("managed runtime cache facts = revision=%v path=%v bytes=%v, want rev-1/path/1234", got.Revision, got.CachePath, got.CacheBytes)
	}
}

func TestNewRequiresHTTPAndInvocationDependencies(t *testing.T) {
	if service := New(nil, testModelInvocationBuilder); service != nil {
		t.Fatalf("New(nil, invocation) = %T, want nil", service)
	}
	if service := New(testHTTPProtocol(t), nil); service != nil {
		t.Fatalf("New(protocol, nil) = %T, want nil", service)
	}
	if service := New(testHTTPProtocol(t), testModelInvocationBuilder); service == nil {
		t.Fatal("New(protocol, invocation) = nil, want Models CLI service")
	}
}

func TestRenderModel_WritesManagedRuntimeInspectFields(t *testing.T) {
	diagnostics := factoryapi.StringMap{
		"readinessState": "MISSING",
		"missingAssets":  "weights.bin",
	}
	var out bytes.Buffer
	err := renderModel(factoryapi.ModelDetail{
		Name: "SECOND_RUNTIME",
		ManagedRuntime: factoryapi.ManagedRuntime{
			Identity:       "SECOND_RUNTIME",
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			Locality:       factoryapi.WorkerModelLocalityLocal,
			Diagnostics:    &diagnostics,
		},
		ProviderLocality: factoryapi.WorkerModelLocalityLocal,
		Operations:       []factoryapi.ModelInvocationOperation{{Name: "EMBED"}},
	}, &out)
	if err != nil {
		t.Fatalf("RenderModel: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Readiness:\tMISSING", "Lifecycle:\tNOT_INSTALLED",
		"Revision:\tNOT_INSTALLED", "Cache Size:\tNOT_INSTALLED", "Cache Path:\tNOT_INSTALLED",
		"missingAssets=weights.bin",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered inspect output missing %q:\n%s", want, got)
		}
	}
}

func TestQueryModel_NotFoundUsesFriendlyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"model not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
	}))
	defer server.Close()

	serverBase := strings.TrimSuffix(server.URL, "/")
	_, err := queryModelWithProtocol(context.Background(), testHTTPProtocol(t), serverBase, "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
	var apiErr *clihttp.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T, want decoded APIError", err)
	}
	if apiErr.CLIErrorCode() != "NOT_FOUND" || apiErr.CLIErrorFamily() != factoryapi.ErrorFamilyNotFound || apiErr.CLIErrorMessage() != "model not found" {
		t.Fatalf("APIError fields = code %q family %q message %q", apiErr.CLIErrorCode(), apiErr.CLIErrorFamily(), apiErr.CLIErrorMessage())
	}
}

func TestInvoke_JSONWritesValidationOnlyResponse(t *testing.T) {
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{
			ModelName:        modelName,
			Worker:           "tts-worker",
			Operation:        request.Operation,
			ProviderLocality: string(factoryapi.WorkerModelLocalityLocal),
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeAudio,
				File: "artifacts/output.wav",
			}},
		}, nil
	}))

	var out bytes.Buffer
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		JSON:       true,
		Output:     &out,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", `"operation":"TTS"`, `"mode":"VALIDATION_ONLY"`, `"validationOnly":true`, `"inferenceExecuted":false`} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	for _, forbidden := range []string{`"content"`, `"worker"`, `"artifact"`} {
		if bytes.Contains(out.Bytes(), []byte(forbidden)) {
			t.Fatalf("validation output unexpectedly contains %q:\n%s", forbidden, out.String())
		}
	}
}

func TestInvoke_AudioWritesOutputFile(t *testing.T) {
	audioBytes := []byte("RIFF....WAVE")
	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, audioBytes, 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		if request.Options == nil || request.Options.ResponseMode == nil || *request.Options.ResponseMode != factoryapi.AUDIOSTREAM {
			t.Fatalf("request options = %#v, want AUDIO_STREAM mode", request.Options)
		}
		return modelinference.Result{
			ModelName:  modelName,
			Operation:  request.Operation,
			StreamFile: streamFile,
		}, nil
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		Output:     io.Discard,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !bytes.Equal(got, audioBytes) {
		t.Fatalf("output bytes = %q, want %q", got, audioBytes)
	}
}

func TestInvoke_AudioVerboseLogsOutputPath(t *testing.T) {
	audioBytes := []byte("RIFF....WAVE")
	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, audioBytes, 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{
			ModelName:  modelName,
			Operation:  request.Operation,
			StreamFile: streamFile,
		}, nil
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	var diagnostics bytes.Buffer
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:   "OMNIVOICE_Q4_K_M",
		Operation:   "TTS",
		Text:        "hello world",
		OutputPath:  outputPath,
		FactoryDir:  t.TempDir(),
		Logger:      zap.NewNop(),
		Output:      io.Discard,
		Verbose:     true,
		Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	assertDiagnosticsContains(t, diagnostics.String(), []string{
		"models invoke bootstrap request",
		"outputPath=" + outputPath,
		"models invoke bootstrap response",
		"outputPath=" + outputPath,
	})
}

func TestInvoke_AudioNotFoundUsesFriendlyError(t *testing.T) {
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, _ string, _ factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{}, fmt.Errorf("%w: model not found", modelinference.ErrNotFound)
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "missing",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		Output:     io.Discard,
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
}

func TestInvoke_JSONSurfacesClassifiedLoadingFailureFromBootstrap(t *testing.T) {
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, _ string, _ factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{}, fmt.Errorf("%w: wait for the managed runtime to finish loading and retry the invocation", modelinference.ErrLoading)
	}))

	err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		OutputPath: filepath.Join(t.TempDir(), "speech.wav"),
		Output:     io.Discard,
	})
	if err == nil {
		t.Fatal("expected loading failure")
	}
	if !strings.Contains(err.Error(), "wait for the managed runtime to finish loading") {
		t.Fatalf("error = %q, want loading guidance", err.Error())
	}
}

func TestInvoke_RequiresInjectedInvocationOperation(t *testing.T) {
	if service := New(testHTTPProtocol(t), nil); service != nil {
		t.Fatalf("New(protocol, nil) = %T, want nil", service)
	}
}

func TestInvoke_AudioUnreachableUsesBootstrapInsteadOfTransportMessage(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				return modelinference.Result{
					ModelName: modelName,
					Operation: request.Operation,
				}, nil
			},
		}, nil
	}

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		FactoryDir: t.TempDir(),
		Server:     "http://127.0.0.1:1",
		Output:     io.Discard,
		Logger:     zap.NewNop(),
	})
	if err == nil {
		t.Fatal("expected bootstrap audio invoke without stream file to fail")
	}
	if strings.Contains(err.Error(), "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M/invocations") {
		t.Fatalf("error = %q, want bootstrap failure instead of unreachable transport message", err.Error())
	}
}

func TestPull_ClassifiedFailureReturnsManagedRuntimeOutcome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"","revision":"","downloadedFiles":[],"managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"SOURCE_FETCH_FAILED","readinessState":"FAILED"}}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    strings.TrimSuffix(server.URL, "/"),
		JSON:      true,
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected classified pull failure error")
	}
	if !strings.Contains(err.Error(), "SOURCE_FETCH_FAILED") {
		t.Fatalf("error = %q, want classified pull outcome", err.Error())
	}
	var response factoryapi.ModelPullResponse
	if decodeErr := json.Unmarshal(out.Bytes(), &response); decodeErr != nil {
		t.Fatalf("json output is invalid: %v\n%s", decodeErr, out.String())
	}
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED {
		t.Fatalf("pull outcome = %s, want SOURCE_FETCH_FAILED", response.ManagedRuntimePull.PullOutcome)
	}
	if response.Outcome != factoryapi.ModelPullOutcomeFAILED {
		t.Fatalf("pull JSON = %q, want top-level outcome FAILED", strings.TrimSpace(out.String()))
	}
}

func TestModelPullCompatibilityOutcomeProjectsManagedOutcomeTotally(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		managed factoryapi.ManagedRuntimePullOutcome
		want    factoryapi.ModelPullOutcome
	}{
		{name: "installed", managed: factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY, want: factoryapi.ModelPullOutcomePULLED},
		{name: "already present", managed: factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT, want: factoryapi.ModelPullOutcomeALREADYPRESENT},
		{name: "already ready", managed: factoryapi.ManagedRuntimePullOutcomeALREADYREADY, want: factoryapi.ModelPullOutcomeALREADYPRESENT},
		{name: "still loading", managed: factoryapi.ManagedRuntimePullOutcomeSTILLLOADING, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "timed out", managed: factoryapi.ManagedRuntimePullOutcomeTIMEDOUT, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "cancelled", managed: factoryapi.ManagedRuntimePullOutcomeCANCELLED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "source fetch failed", managed: factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "source resolution failed", managed: factoryapi.ManagedRuntimePullOutcomeSOURCERESOLUTIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "integrity verification failed", managed: factoryapi.ManagedRuntimePullOutcomeINTEGRITYVERIFICATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "assembly failed", managed: factoryapi.ManagedRuntimePullOutcomeASSEMBLYFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "cache installation failed", managed: factoryapi.ManagedRuntimePullOutcomeCACHEINSTALLATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "readiness evaluation failed", managed: factoryapi.ManagedRuntimePullOutcomeREADINESSEVALUATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "asset preparation failed", managed: factoryapi.ManagedRuntimePullOutcomeASSETPREPARATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "unsupported", managed: factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "future outcome", managed: factoryapi.ManagedRuntimePullOutcome("FUTURE_OUTCOME"), want: factoryapi.ModelPullOutcomeFAILED},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := pullResultToGenerated(modelinference.PullResult{
				ModelName: "voice", Outcome: "PULLED", ManagedPullOutcome: string(tc.managed),
			})
			if result.Outcome != tc.want {
				t.Fatalf("managed outcome %q projected as %q, want %q", tc.managed, result.Outcome, tc.want)
			}
		})
	}
}

func TestModelsPullUsesDedicatedProtocolAndPreservesCallerCancellation(t *testing.T) {
	t.Parallel()
	var standardCalls atomic.Int32
	standard, err := clihttp.NewProtocol(modelsPullDoer(func(*http.Request) (*http.Response, error) {
		standardCalls.Add(1)
		return nil, errors.New("ordinary CLI protocol was used for pull")
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("standard protocol: %v", err)
	}
	pullStarted := make(chan struct{})
	var startOnce atomic.Bool
	pull, err := clihttp.NewProtocol(modelsPullDoer(func(request *http.Request) (*http.Response, error) {
		if _, ok := request.Context().Deadline(); ok {
			return nil, errors.New("pull request inherited a fixed client deadline")
		}
		if startOnce.CompareAndSwap(false, true) {
			close(pullStarted)
		}
		<-request.Context().Done()
		return nil, request.Context().Err()
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("pull protocol: %v", err)
	}
	service := NewWithOutputFileSystemAndPullProtocol(
		standard, pull, testModelInvocationBuilder, nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.Pull(PullConfig{
			Context: ctx, ModelName: "OMNIVOICE_Q4_K_M", Server: "http://factory.test",
			Output: io.Discard,
		})
	}()
	select {
	case <-pullStarted:
	case <-time.After(time.Second):
		t.Fatal("dedicated pull protocol was not invoked")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pull error = %v, want caller cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pull did not terminate after caller cancellation")
	}
	if standardCalls.Load() != 0 {
		t.Fatalf("ordinary protocol calls = %d, want 0", standardCalls.Load())
	}
}

func TestModelsPullWaitsForDedicatedProtocolTerminalResponse(t *testing.T) {
	t.Parallel()
	standard, err := clihttp.NewProtocol(modelsPullDoer(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("ordinary CLI protocol was used for pull")
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("standard protocol: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	pull, err := clihttp.NewProtocol(modelsPullDoer(func(request *http.Request) (*http.Response, error) {
		if _, ok := request.Context().Deadline(); ok {
			return nil, errors.New("pull request inherited a fixed client deadline")
		}
		close(started)
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`,
			)),
		}, nil
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("pull protocol: %v", err)
	}
	service := NewWithOutputFileSystemAndPullProtocol(
		standard, pull, testModelInvocationBuilder, nil,
	)
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- service.Pull(PullConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M",
			Server: "http://factory.test", JSON: true, Output: &output,
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dedicated pull protocol was not invoked")
	}
	select {
	case err := <-done:
		t.Fatalf("pull completed before terminal response: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pull after terminal response: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pull did not finish after terminal response")
	}
	if !strings.Contains(output.String(), `"outcome":"PULLED"`) {
		t.Fatalf("pull output = %q, want terminal success", output.String())
	}
}
