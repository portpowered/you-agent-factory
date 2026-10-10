package inference_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Standalone Models commands select ~default rather than a Factory Session.
// Keep this local cohort serial while independent groups remain parallel. Each
// command selects its own profile and uses one reusable production graph.
func TestModelsStandaloneCatalogAndCacheRemainProfileScopedAfterFailures(t *testing.T) {
	t.Parallel()
	process, _, _ := buildGenericCLIProcess(t, singleOutputModelFactoryConfig, nil, nil, nil, nil, nil)
	healthyHome := t.TempDir()
	writeGenericBuiltinModelCache(t, healthyHome, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	writeGenericBackendCache(t, healthyHome, "localai-llamacpp", genericLlamaBackendSelection(), []byte("localai-llamacpp/linux-amd64"))
	config := fmt.Sprintf(`{"models":{"llm":{"source":%q},"profile-only":{"source":%q,"backend":"localai-llamacpp","loadPolicy":"ON_DEMAND","operations":["OMNI"]}}}`, genericLLMFixtureSource, genericLLMFixtureSource)
	if err := os.WriteFile(filepath.Join(healthyHome, ".you-agent-factory", "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	workingDirectory := t.TempDir() // No Factory: use the documented standalone catalog.
	for _, scenario := range []string{"cached success", "malformed configuration", "unknown model", "missing asset"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario != "cached success" {
				runStandaloneModelsProfileFailure(t, process, workingDirectory, scenario)
			}
			assertStandaloneModelsHealthyProfile(t, process, workingDirectory, healthyHome)
		})
	}
}

func executeStandaloneModelsCommand(t *testing.T, process support.Process, directory, home string, args ...string) (*support.CapturedInputs, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you"}, args...))
	inputs.Input.Env = isolatedModelEnvironment(home, filepath.Join(home, ".agent-factory", "models"))
	inputs.Input.WorkingDirectory = directory
	return inputs, process.Execute(inputs.Input)
}

func assertStandaloneModelsHealthyProfile(t *testing.T, process support.Process, directory, home string) {
	t.Helper()
	listed, err := executeStandaloneModelsCommand(t, process, directory, home, "--json", "models", "list")
	if err != nil {
		t.Fatalf("healthy standalone catalog: %v; stderr=%s", err, listed.Stderr())
	}
	var catalog factoryapi.ListModelsResponse
	if err := json.Unmarshal([]byte(listed.Stdout()), &catalog); err != nil {
		t.Fatalf("decode standalone catalog: %v; stdout=%s", err, listed.Stdout())
	}
	found := false
	for _, model := range catalog.Results {
		if model.Name == "profile-only" {
			found = true
		}
	}
	if !found {
		t.Fatalf("standalone catalog lost configured model: %s", listed.Stdout())
	}
	for _, payload := range []string{"first cached result", "second cached result"} {
		invoked, err := executeStandaloneModelsCommand(t, process, directory, home,
			"models", "invoke", "llm", "--offline", "--input", "prompt="+payload)
		if err != nil || invoked.Stdout() != payload {
			t.Fatalf("cached standalone invocation: error=%v stdout=%q stderr=%q", err, invoked.Stdout(), invoked.Stderr())
		}
	}
	inspected, err := executeStandaloneModelsCommand(t, process, directory, home, "--json", "models", "inspect", "llm")
	if err != nil {
		t.Fatalf("healthy cached model readback: %v; stderr=%s", err, inspected.Stderr())
	}
	var model factoryapi.ModelDetail
	if err := json.Unmarshal([]byte(inspected.Stdout()), &model); err != nil {
		t.Fatal(err)
	}
	if model.Name != "llm" || model.Status != factoryapi.ModelStatusREADY || model.ManagedRuntime.CachePath == nil ||
		!pathWithinRoot(*model.ManagedRuntime.CachePath, filepath.Join(home, ".agent-factory", "models")) {
		t.Fatalf("healthy selected cache readback: %#v", model)
	}
}

func runStandaloneModelsProfileFailure(t *testing.T, process support.Process, directory, scenario string) {
	t.Helper()
	home := t.TempDir()
	var inputs *support.CapturedInputs
	var err error
	if scenario == "malformed configuration" {
		inputs, err = executeMalformedStandaloneModelsCatalog(t, process, directory, home)
	} else {
		listed, listErr := executeStandaloneModelsCommand(t, process, directory, home, "--json", "models", "list")
		if listErr != nil {
			t.Fatalf("isolated empty-home catalog: %v; stderr=%s", listErr, listed.Stderr())
		}
		var catalog factoryapi.ListModelsResponse
		if err := json.Unmarshal([]byte(listed.Stdout()), &catalog); err != nil {
			t.Fatal(err)
		}
		for _, model := range catalog.Results {
			if model.Name == "profile-only" {
				t.Fatal("empty-home catalog leaked peer's configured model")
			}
		}
		name := "llm"
		if scenario == "unknown model" {
			name = "not-configured-model"
		}
		inputs, err = executeStandaloneModelsCommand(t, process, directory, home,
			"models", "invoke", name, "--operation", "OMNI", "--offline", "--input", "prompt=must not publish")
		if scenario == "unknown model" && !errors.Is(err, models.ErrNotFound) {
			t.Fatalf("unknown standalone model: %v; stderr=%s", err, inputs.Stderr())
		}
		if scenario == "missing asset" {
			var failure *models.AssetOfflineError
			if !errors.As(err, &failure) {
				t.Fatalf("missing standalone asset: %v; stderr=%s", err, inputs.Stderr())
			}
		}
	}
	if err == nil || inputs.Stdout() != "" {
		t.Fatalf("%s: error=%v stdout=%q, want failure without successful output", scenario, err, inputs.Stdout())
	}
}

func executeMalformedStandaloneModelsCatalog(t *testing.T, process support.Process, directory, home string) (*support.CapturedInputs, error) {
	t.Helper()
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"models":`), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs, err := executeStandaloneModelsCommand(t, process, directory, home, "--json", "models", "list")
	if !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(err.Error(), "parse operator config") {
		t.Fatalf("malformed standalone configuration: error=%v stderr=%q, want truncated operator JSON failure", err, inputs.Stderr())
	}
	assertFunctionalFile(t, configPath, `{"models":`)
	return inputs, err
}

func cleanModelsEnvironment(home string) []string {
	environment := functionalHomeEnvironment(home)
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, runcli.ModelCacheDirEnvironment) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func (launcher *recordingModelHostLauncher) Active() bool {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.active
}

// Standalone Models commands bind ~default, so this smallest local cohort is
// serial. One graph routes immutable host failures by each profile's asset
// path; independent test groups remain parallel.
func TestModelsCLIHostFailuresPreserveOutputAndHealthyPeer(t *testing.T) {
	t.Parallel()
	launcher := &selectedFailureHostLauncher{failures: map[string]string{}}
	process, peerDirectory, peerEnvironment := buildGenericCLIProcess(t,
		multiOutputModelFactoryConfig, nil, launcher, selectedFailureHostProtocol{}, nil, nil)
	names := []string{"launch rejection", "protocol rejection", "protocol mismatch"}
	homes := map[string]string{}
	for _, name := range names {
		home := t.TempDir()
		homes[name] = home
		launcher.failures[home] = name
		writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
		writeGenericBackendCache(t, home, "localai-llamacpp", genericLlamaBackendSelection(), []byte("localai-llamacpp/linux-amd64"))
	}
	// Finish the immutable route table before any host operation can start.
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			home := homes[name]
			want := models.ErrHostProtocolIncompatible
			if name == "launch rejection" {
				want = models.ErrHostProcessCrash
			}
			directory := functionalScaffoldFactory(t, multiOutputModelFactoryConfig("http://127.0.0.1:1"))
			output := filepath.Join(t.TempDir(), "answer.txt")
			if err := os.WriteFile(output, []byte("prior answer"), 0o600); err != nil {
				t.Fatal(err)
			}
			inputs := support.FakeInputs(t.Context(), []string{"you", "models", "invoke", "llm",
				"--operation", "OMNI", "--text", "rejected result", "--output-map", "text=" + output,
				"--output-map", "usage=" + filepath.Join(filepath.Dir(output), "usage.json")})
			inputs.Input.Env = isolatedModelEnvironment(home, filepath.Join(home, ".agent-factory", "models"))
			inputs.Input.WorkingDirectory = directory
			err := process.Execute(inputs.Input)
			if !errors.Is(err, want) || inputs.Stdout() != "" {
				t.Fatalf("%s: error=%v stdout=%q, want %v without successful output", name, err, inputs.Stdout(), want)
			}
			assertPullToReadySafeOutput(t, map[string]string{"error": err.Error(), "stderr": inputs.Stderr()}, "private-host-secret", "private.invalid")
			assertFunctionalFile(t, output, "prior answer")
			entries, readErr := os.ReadDir(filepath.Dir(output))
			if readErr != nil || len(entries) != 1 || entries[0].Name() != "answer.txt" {
				t.Fatalf("failed invocation left partial outputs: %v, %v", entries, readErr)
			}
			peerOutput := filepath.Join(t.TempDir(), "peer-answer.txt")
			peer := support.FakeInputs(t.Context(), []string{"you", "models", "invoke", "llm",
				"--operation", "OMNI", "--text", "healthy peer after " + name, "--output-map", "text=" + peerOutput,
				"--output-map", "usage=" + filepath.Join(filepath.Dir(peerOutput), "usage.json")})
			peer.Input.Env, peer.Input.WorkingDirectory = peerEnvironment, peerDirectory
			if err := process.Execute(peer.Input); err != nil {
				t.Fatalf("healthy peer after %s: error=%v stdout=%q", name, err, peer.Stdout())
			}
			assertFunctionalFile(t, peerOutput, "healthy peer after "+name)
		})
	}
}

type selectedFailureHostLauncher struct {
	failures map[string]string
}

func (launcher *selectedFailureHostLauncher) Start(_ context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	endpoint := "healthy"
	for home, name := range launcher.failures {
		if pathWithinRoot(spec.ModelPath, home) {
			if name == "launch rejection" {
				return nil, errors.New("private-host-secret https://private.invalid")
			}
			endpoint = name
		}
	}
	return &functionalModelHostProcess{endpoint: endpoint, stopped: make(chan struct{})}, nil
}

type selectedFailureHostProtocol struct{}

func (selectedFailureHostProtocol) Negotiate(_ context.Context, endpoint string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	if endpoint == "protocol rejection" {
		return serviceedges.ModelHostProtocolNegotiationResult{}, models.ErrHostProtocolIncompatible
	}
	version := request.ProtocolVersion
	if endpoint == "protocol mismatch" {
		version = "unsupported-version"
	}
	return serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: version, Backend: request.Backend, Ready: true}, nil
}
