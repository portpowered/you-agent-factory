package inference_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// These routes are fixed before each profile starts. The standalone Models
// CLI binds ~default, so its smallest local cohort runs sequentially on the
// parent's reusable process; unrelated package journeys remain parallel.
type terminalModelHostLauncher struct {
	endpoint string
	mu       sync.Mutex
	failures map[string]bool
}

func (launcher *terminalModelHostLauncher) Start(_ context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	for home, available := range launcher.failures {
		if strings.HasPrefix(spec.ModelPath, home+string(os.PathSeparator)) {
			return &terminalModelHostProcess{
				endpoint: launcher.endpoint + "/failed", available: available,
				invalidDigest: strings.Contains(home, "invalid-digest"),
			}, nil
		}
	}
	return &functionalModelHostProcess{endpoint: launcher.endpoint, stopped: make(chan struct{})}, nil
}

type terminalModelHostProcess struct {
	endpoint                 string
	available, invalidDigest bool
}

func (process *terminalModelHostProcess) HealthEndpoint() string { return process.endpoint }
func (*terminalModelHostProcess) Wait() error                    { return errors.New("HF_TOKEN=private-host-tail") }
func (*terminalModelHostProcess) Stop(context.Context) error     { return nil }
func (process *terminalModelHostProcess) DiagnosticSnapshot() (serviceedges.HostProcessDiagnosticSnapshot, bool) {
	digest := strings.Repeat("a", 64)
	if process.invalidDigest {
		digest = "HF_TOKEN=invalid-private-digest"
	}
	return serviceedges.HostProcessDiagnosticSnapshot{
		ExitClass: "NONZERO_EXIT", ExitCode: 23, ExitCodeKnown: true,
		Stdout:    serviceedges.HostProcessStreamDiagnostic{Bytes: 41, SHA256: digest, Truncated: true},
		CauseCode: "PROCESS_EXITED", CauseMessage: "HF_TOKEN=private-host-tail", CauseMessageRedacted: true,
	}, process.available
}

type terminalModelProtocol struct{}

func (terminalModelProtocol) Negotiate(ctx context.Context, endpoint string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	if strings.HasSuffix(endpoint, "/failed") {
		// Readiness remains false while the supervisor observes the child's
		// terminal signal. The probe must return so it can select that signal.
		return serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: request.ProtocolVersion, Backend: request.Backend, Ready: false}, nil
	}
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: request.ProtocolVersion, Backend: request.Backend, Ready: true,
	}, nil
}

type fileInputProtocolClient struct {
	mu      sync.Mutex
	prompts map[string]int
}

func (client *fileInputProtocolClient) Predict(ctx context.Context, request models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error) {
	prompt := request.Prompt
	if prompt == "" && len(request.Inputs) > 0 {
		prompt = request.Inputs[0].Content
	}
	client.mu.Lock()
	client.prompts[prompt]++
	client.mu.Unlock()
	return (genericCLIProtocolClient{}).Predict(ctx, request)
}

func (client *fileInputProtocolClient) callsFor(prompt string) int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.prompts[prompt]
}

func runTerminalModelHostScenario(t *testing.T, process support.Process, launcher *terminalModelHostLauncher, diagnostic string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), strings.ReplaceAll(diagnostic, " ", "-"))
	seedTerminalModelProfile(t, home)
	launcher.mu.Lock()
	launcher.failures[home] = diagnostic != "absent diagnostic"
	launcher.mu.Unlock()
	dir := functionalScaffoldFactory(t, multiOutputModelFactoryConfig(launcher.endpoint))
	output := filepath.Join(t.TempDir(), "answer.txt")
	if err := os.WriteFile(output, []byte("prior result"), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := terminalModelInputs(t, home, dir, "failed input", output)
	err := process.Execute(inputs.Input)
	if !errors.Is(err, models.ErrHostProcessCrash) {
		t.Fatalf("managed child exit = %v; want typed process crash; stderr=%s", err, inputs.Stderr())
	}
	if inputs.Stdout() != "" {
		t.Fatal("crashed child published a successful result")
	}
	assertFunctionalFile(t, output, "prior result")
	public := err.Error() + inputs.Stderr()
	for _, private := range []string{"HF_TOKEN", "private-host-tail", "invalid-private-digest"} {
		if strings.Contains(public, private) {
			t.Fatalf("public host failure leaked %s", private)
		}
	}
	// The same application remains usable with an independently owned healthy
	// profile after the failed child's supervision has completed.
	peer := t.TempDir()
	seedTerminalModelProfile(t, peer)
	healthy := terminalModelInputs(t, peer, dir, "healthy peer", output)
	if err := process.Execute(healthy.Input); err != nil {
		t.Fatalf("healthy peer after child exit: %v", err)
	}
	assertFunctionalFile(t, output, "healthy peer")
	inspected, err := executeStandaloneModelsCommand(t, process, dir, peer, "--json", "models", "inspect", "llm")
	var model factoryapi.ModelDetail
	if err != nil {
		t.Fatalf("healthy peer inspection: %v", err)
	}
	if err := json.Unmarshal([]byte(inspected.Stdout()), &model); err != nil {
		t.Fatal(err)
	}
	if model.Status != factoryapi.ModelStatusREADY {
		t.Fatalf("healthy peer status = %s", model.Status)
	}
}

func seedTerminalModelProfile(t *testing.T, home string) {
	t.Helper()
	writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	writeGenericBackendCache(t, home, "localai-llamacpp", genericLlamaBackendSelection(), []byte("localai-llamacpp/linux-amd64"))
}

func terminalModelInputs(t *testing.T, home, directory, text, output string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "models", "invoke", "llm", "--operation", "OMNI", "--text", text,
		"--output-map", "text=" + output, "--output-map", "usage=" + filepath.Join(filepath.Dir(output), "usage.json")})
	inputs.Input.Env = functionalHomeEnvironment(home)
	inputs.Input.WorkingDirectory = directory
	return inputs
}
