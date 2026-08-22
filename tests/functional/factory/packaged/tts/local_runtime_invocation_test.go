package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedTTSLocalRuntimePayloadPreservesExactBoundText proves the
// installed local TTS route carries the complete customer value through the
// operation-binding boundary. The command-runner edge observes the payload
// consumed by the real local-runtime adapter, rather than a provider fake.
func TestPackagedTTSLocalRuntimePayloadPreservesExactBoundText(t *testing.T) {
	text := "The release is ready, with every submitted word preserved exactly."
	homeDir := t.TempDir()
	support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	cacheDir := t.TempDir()
	writePackagedTTSReadyModelCache(t, cacheDir)
	runner := newPackagedTTSLocalRuntimeCommandRunner([]byte(packagedTTSFakeAudioFixture))
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--named", factorydefinitions.PackagedTTSFactoryName,
		"--no-record",
		"--output", "primary",
		"--to", text,
	})
	inputs.Input.Env = append(
		os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		run.ModelCacheDirEnvironment+"="+cacheDir,
	)
	inputs.Input.WorkingDirectory = t.TempDir()

	process := support.BuildProcess(t, serviceedges.Edges{ModelRuntimeCommandRunner: runner})
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(packaged local TTS) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}

	payload := runner.LastPayload(t)
	if payload.Operation != "TTS" || payload.ModelName != factorydefinitions.DefaultTTSModelName {
		t.Fatalf("local runtime payload identity = %#v, want TTS/%s", payload, factorydefinitions.DefaultTTSModelName)
	}
	if payload.Text != text {
		t.Fatalf("local runtime payload text = %q, want complete bound text %q", payload.Text, text)
	}
	assertPackagedTTSTextBinding(t, payload.Bindings, text)
	if !primaryResultContainsTTSArtifactMetadata(t, response.PrimaryResult) {
		t.Fatalf("primary result = %#v, want local-runtime audio metadata", response.PrimaryResult)
	}
	if got, err := os.ReadFile(payload.OutputFile); err != nil || string(got) != packagedTTSFakeAudioFixture {
		t.Fatalf("local-runtime audio artifact = %q, %v; want fixture", got, err)
	}
}

type packagedTTSLocalRuntimePayload struct {
	Operation  string                                          `json:"operation"`
	ModelName  string                                          `json:"modelName"`
	OutputFile string                                          `json:"outputFile"`
	Text       string                                          `json:"text"`
	Bindings   []workerexecution.ResolvedModelOperationBinding `json:"bindings"`
}

type packagedTTSLocalRuntimeCommandRunner struct {
	mu      sync.Mutex
	audio   []byte
	payload *packagedTTSLocalRuntimePayload
}

func newPackagedTTSLocalRuntimeCommandRunner(audio []byte) *packagedTTSLocalRuntimeCommandRunner {
	return &packagedTTSLocalRuntimeCommandRunner{audio: append([]byte(nil), audio...)}
}

func (runner *packagedTTSLocalRuntimeCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	var payload packagedTTSLocalRuntimePayload
	if err := json.Unmarshal(request.Stdin, &payload); err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("decode local TTS payload: %w", err)
	}
	if payload.OutputFile == "" {
		return platformprocess.CommandResult{}, fmt.Errorf("local TTS payload output file is empty")
	}
	if err := os.WriteFile(payload.OutputFile, runner.audio, 0o644); err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("write local TTS audio fixture: %w", err)
	}
	runner.mu.Lock()
	runner.payload = &payload
	runner.mu.Unlock()
	return platformprocess.CommandResult{}, nil
}

func (runner *packagedTTSLocalRuntimeCommandRunner) LastPayload(t testing.TB) packagedTTSLocalRuntimePayload {
	t.Helper()
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.payload == nil {
		t.Fatal("local TTS runtime command was not called")
	}
	payload := *runner.payload
	payload.Bindings = append([]workerexecution.ResolvedModelOperationBinding(nil), runner.payload.Bindings...)
	return payload
}

func writePackagedTTSReadyModelCache(t testing.TB, cacheDir string) {
	t.Helper()
	modelRoot := filepath.Join(cacheDir, factorydefinitions.DefaultTTSModelName)
	revisionDir := filepath.Join(modelRoot, "review-fixture")
	if err := os.MkdirAll(revisionDir, 0o755); err != nil {
		t.Fatalf("create packaged TTS model cache: %v", err)
	}
	files := []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(revisionDir, name), []byte("fixture"), 0o644); err != nil {
			t.Fatalf("write packaged TTS model asset %s: %v", name, err)
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"modelName": factorydefinitions.DefaultTTSModelName,
		"revision":  "review-fixture",
		"files":     []map[string]string{{"path": files[0]}, {"path": files[1]}},
	})
	if err != nil {
		t.Fatalf("marshal packaged TTS model cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelRoot, ".managed-cache.json"), metadata, 0o644); err != nil {
		t.Fatalf("write packaged TTS model cache metadata: %v", err)
	}
}

func assertPackagedTTSTextBinding(
	t testing.TB,
	bindings []workerexecution.ResolvedModelOperationBinding,
	want string,
) {
	t.Helper()
	for _, binding := range bindings {
		if binding.Slot != "text" {
			continue
		}
		if len(binding.Content) == 1 && binding.Content[0].Text == want {
			return
		}
		t.Fatalf("local TTS text binding = %#v, want one exact text part %q", binding.Content, want)
	}
	t.Fatalf("local TTS bindings = %#v, want text slot with exact bound text %q", bindings, want)
}

var _ platformprocess.CommandRunner = (*packagedTTSLocalRuntimeCommandRunner)(nil)
