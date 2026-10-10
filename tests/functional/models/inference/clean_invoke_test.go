package inference_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
