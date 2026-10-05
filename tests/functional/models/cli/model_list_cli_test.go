package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var modelListProcess support.ApplicationProcess

func TestMain(m *testing.M) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build models CLI process: %v\n", err)
		os.Exit(1)
	}
	modelListProcess = process
	exitCode := m.Run()
	closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Close(closeContext); err != nil {
		fmt.Fprintf(os.Stderr, "close models CLI process: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

// TestProcessModelsList_UsesServerFlagAndReturnsCatalogJSON proves list routing and JSON catalog output.
func TestProcessModelsList_UsesServerFlagAndReturnsCatalogJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"results":[{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","revision":"rev1","cacheBytes":1234,"readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[]}}]}`)
	}))
	t.Cleanup(server.Close)

	process := modelListProcess
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", strings.TrimSuffix(server.URL, "/"), "models", "list",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models list) error = %v\nstderr=%s", err, inputs.Stderr())
	}

	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode models list output: %v\n%s", err, inputs.Stdout())
	}
	if len(response.Results) != 1 || response.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("list response = %#v, want OMNIVOICE_Q4_K_M", response)
	}
	if response.Results[0].ManagedRuntime.Revision == nil ||
		*response.Results[0].ManagedRuntime.Revision != "rev1" ||
		response.Results[0].ManagedRuntime.CacheBytes == nil ||
		*response.Results[0].ManagedRuntime.CacheBytes != 1234 {
		t.Fatalf("managed runtime cache facts = revision=%v bytes=%v, want rev1/1234", response.Results[0].ManagedRuntime.Revision, response.Results[0].ManagedRuntime.CacheBytes)
	}
}

func TestProcessModelsCatalogDiscoversCustomOperatorModel(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	configDir := filepath.Join(home, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := `{"models":{"index-tts2.5":{"source":"file:///models/index-tts2_5-orig.gguf","backend":"localai-audio-cpp","loadPolicy":"ON_DEMAND","operations":["TTS"]}}}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	workingDirectory := t.TempDir()
	invoke := func(args ...string) string {
		t.Helper()
		inputs := support.FakeInputs(t.Context(), args)
		inputs.Input.WorkingDirectory = workingDirectory
		inputs.Input.Env = append(inputs.Input.Env, "USERPROFILE="+home, "HOME="+home)
		if err := modelListProcess.Execute(inputs.Input); err != nil {
			t.Fatalf("Process.Execute(%v) error = %v\nstderr=%s", args, err, inputs.Stderr())
		}
		return inputs.Stdout()
	}
	var listed factoryapi.ListModelsResponse
	if err := json.Unmarshal([]byte(invoke("you", "--json", "models", "list")), &listed); err != nil {
		t.Fatalf("decode models list: %v", err)
	}
	found := false
	for _, model := range listed.Results {
		if model.Name == "index-tts2.5" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("models list omitted index-tts2.5: %#v", listed.Results)
	}
	var inspected factoryapi.ModelDetail
	if err := json.Unmarshal([]byte(invoke("you", "--json", "models", "inspect", "index-tts2.5")), &inspected); err != nil {
		t.Fatalf("decode models inspect: %v", err)
	}
	if inspected.Name != "index-tts2.5" {
		t.Fatalf("models inspect name = %q, want index-tts2.5", inspected.Name)
	}
}

// TestProcessModelsInspect_UsesResolvedModelNameArgument proves inspect forwards the resolved model identity.
func TestProcessModelsInspect_UsesResolvedModelNameArgument(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M" {
			t.Fatalf("path = %q, want /models/OMNIVOICE_Q4_K_M", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M/rev1","revision":"rev1","cacheBytes":1234,"readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[]},"capabilities":[],"diagnostics":{}}`)
	}))
	t.Cleanup(server.Close)

	process := modelListProcess
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", strings.TrimSuffix(server.URL, "/"),
		"models", "inspect", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models inspect) error = %v\nstderr=%s", err, inputs.Stderr())
	}

	var response factoryapi.ModelDetail
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode models inspect output: %v\n%s", err, inputs.Stdout())
	}
	if response.Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("inspect response = %#v, want OMNIVOICE_Q4_K_M", response)
	}
	if response.ManagedRuntime.CachePath == nil ||
		*response.ManagedRuntime.CachePath != "/tmp/models/OMNIVOICE_Q4_K_M/rev1" ||
		response.ManagedRuntime.Revision == nil || *response.ManagedRuntime.Revision != "rev1" ||
		response.ManagedRuntime.CacheBytes == nil || *response.ManagedRuntime.CacheBytes != 1234 {
		t.Fatalf("inspect cache facts = path=%v revision=%v bytes=%v, want resolved path/rev1/1234", response.ManagedRuntime.CachePath, response.ManagedRuntime.Revision, response.ManagedRuntime.CacheBytes)
	}
}

// TestProcessModelsPull_UsesServerFlagAndReturnsPullJSON proves pull routing and JSON pull output.
func TestProcessModelsPull_UsesServerFlagAndReturnsPullJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M/pull" {
			t.Fatalf("path = %q, want /models/OMNIVOICE_Q4_K_M/pull", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M","revision":"rev1","downloadedFiles":[{"path":"weights.gguf","bytes":42}],"managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`)
	}))
	t.Cleanup(server.Close)

	process := modelListProcess
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", strings.TrimSuffix(server.URL, "/"),
		"models", "pull", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models pull) error = %v\nstderr=%s", err, inputs.Stderr())
	}

	var response factoryapi.ModelPullResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode models pull output: %v\n%s", err, inputs.Stdout())
	}
	if response.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("pull response = %#v, want OMNIVOICE_Q4_K_M", response)
	}
	if response.Outcome != factoryapi.ModelPullOutcomePULLED ||
		response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY ||
		response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("pull response = %#v, want PULLED/INSTALLED_SUCCESSFULLY/READY", response)
	}
}

// TestProcessModelsList_ReturnsHumanReadableCatalog proves list human presentation.
func TestProcessModelsList_ReturnsHumanReadableCatalog(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"results":[{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","revision":"rev1","cacheBytes":1234,"readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[]}}]}`)
	}))
	t.Cleanup(server.Close)

	process := modelListProcess
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", strings.TrimSuffix(server.URL, "/"), "models", "list",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models list) error = %v\nstderr=%s", err, inputs.Stderr())
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", "TTS", "NAME", "CACHE SIZE", "1.21 KiB (1234 bytes)"} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("list output missing %q:\n%s", want, inputs.Stdout())
		}
	}
}

// TestProcessModelsInspect_ReturnsHumanReadableDetail proves inspect human presentation.
func TestProcessModelsInspect_ReturnsHumanReadableDetail(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M" {
			t.Fatalf("path = %q, want /models/OMNIVOICE_Q4_K_M", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M/rev1","revision":"rev1","cacheBytes":1234,"readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[]},"capabilities":[],"diagnostics":{}}`)
	}))
	t.Cleanup(server.Close)

	process := modelListProcess
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", strings.TrimSuffix(server.URL, "/"),
		"models", "inspect", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models inspect) error = %v\nstderr=%s", err, inputs.Stderr())
	}
	for _, want := range []string{
		"Name:\tOMNIVOICE_Q4_K_M", "Revision:\trev1",
		"Cache Size:\t1.21 KiB (1234 bytes)",
		"Cache Path:\t/tmp/models/OMNIVOICE_Q4_K_M/rev1", "TTS",
	} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("inspect output missing %q:\n%s", want, inputs.Stdout())
		}
	}
}
