package discovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestProvidersListThroughRootBuildProcess proves the customer-facing command
// reaches the injected Providers service through the canonical process graph.
// The command runner is deliberately instrumented: discovery must not invoke
// a provider process, even though the same edge is available to execution.
func TestProvidersListThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	runner := &countingCommandRunner{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	t.Cleanup(func() {
		if err := process.Close(context.Background()); err != nil {
			t.Errorf("close process: %v", err)
		}
	})

	humanStdout, humanStderr, err := execute(t, process, []string{"you", "providers", "list"})
	if err != nil {
		t.Fatalf("Process.Execute(providers list) error = %v\nstdout:\n%s\nstderr:\n%s", err, humanStdout, humanStderr)
	}
	if humanStderr != "" {
		t.Fatalf("providers list stderr = %q, want empty without verbose mode", humanStderr)
	}
	assertHumanFacts(t, humanStdout)

	jsonStdout, jsonStderr, err := execute(t, process, []string{"you", "providers", "list", "--json"})
	if err != nil {
		t.Fatalf("Process.Execute(providers list --json) error = %v\nstdout:\n%s\nstderr:\n%s", err, jsonStdout, jsonStderr)
	}
	if jsonStderr != "" {
		t.Fatalf("providers list --json stderr = %q, want empty without verbose mode", jsonStderr)
	}
	assertJSONFacts(t, jsonStdout)

	unsupportedStdout, _, err := execute(t, process, []string{"you", "providers", "list", "--unsupported"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("unsupported providers argument error = %v, want standard unknown-flag failure", err)
	}
	if unsupportedStdout != "" {
		t.Fatalf("unsupported providers argument stdout = %q, want no partial inventory", unsupportedStdout)
	}
	if calls := runner.calls.Load(); calls != 0 {
		t.Fatalf("provider command calls = %d, want 0 for discovery and usage failure", calls)
	}
}

type countingCommandRunner struct {
	calls atomic.Int32
}

func (runner *countingCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{}, nil
}

func execute(t *testing.T, process interface{ Execute(root.Input) error }, args []string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdinIsTTY := false
	stdoutIsTTY := false
	err := process.Execute(root.Input{
		Args:             args,
		Env:              os.Environ(),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: t.TempDir(),
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	return stdout.String(), stderr.String(), err
}

func assertHumanFacts(t *testing.T, output string) {
	t.Helper()
	for _, providerID := range []string{"antigravity", "claude", "codex"} {
		if count := strings.Count(output, providerID+"\t"); count != 1 {
			t.Fatalf("human output provider %q count = %d, want exactly once\n%s", providerID, count, output)
		}
	}
	for _, want := range []string{
		"gpt-5.6",
		"audio: unsupported (transport: none)",
		"video: unsupported (transport: none)",
		"referenced_image_paths [maximum, paths] maximum=5",
		"claude-opus-4-6-thinking",
		"Efforts:\tlow, medium, high",
		"audio: supported (transport: file_path)",
		"video: supported (transport: file_path)",
		"add_dir_workspace [behavior, flag] value=--add-dir",
		"print_timeout [default, seconds] default=300",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("human output missing %q\n%s", want, output)
		}
	}
}

type listOutput struct {
	Providers []providerOutput `json:"providers"`
}

type providerOutput struct {
	ID            string        `json:"id"`
	Models        []modelOutput `json:"models"`
	Prerequisites []any         `json:"prerequisites"`
	Tools         []any         `json:"tools"`
	KnownLimits   []limitOutput `json:"knownLimits"`
}

type modelOutput struct {
	ID         string           `json:"id"`
	Efforts    []string         `json:"efforts"`
	Modalities []modalityOutput `json:"modalities"`
}

type modalityOutput struct {
	Direction string `json:"direction"`
	Modality  string `json:"modality"`
	Support   string `json:"support"`
	Transport string `json:"transport"`
}

type limitOutput struct {
	Name    string `json:"name"`
	Maximum *int64 `json:"maximum"`
	Default *int64 `json:"default"`
	Value   string `json:"value"`
}

func assertJSONFacts(t *testing.T, output string) {
	t.Helper()
	var decoded listOutput
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("providers list JSON is invalid: %v\n%s", err, output)
	}
	if len(decoded.Providers) == 0 {
		t.Fatal("providers list JSON returned no providers")
	}
	ids := make([]string, 0, len(decoded.Providers))
	byID := make(map[string]providerOutput, len(decoded.Providers))
	for _, provider := range decoded.Providers {
		ids = append(ids, provider.ID)
		if _, exists := byID[provider.ID]; exists {
			t.Fatalf("provider %q was emitted more than once", provider.ID)
		}
		byID[provider.ID] = provider
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("provider IDs are not deterministic: %v", ids)
	}
	for _, providerID := range []string{"antigravity", "claude", "codex"} {
		provider, ok := byID[providerID]
		if !ok {
			t.Fatalf("first-party provider %q is missing from %v", providerID, ids)
		}
		if provider.Models == nil || provider.Prerequisites == nil || provider.Tools == nil || provider.KnownLimits == nil {
			t.Fatalf("provider %q omitted explicit capability arrays: %#v", providerID, provider)
		}
	}

	codex := byID["codex"]
	gpt := findModel(codex.Models, "gpt-5.6")
	if gpt == nil {
		t.Fatal("Codex is missing gpt-5.6")
	}
	assertModality(t, gpt.Modalities, "input", "audio", "unsupported", "none")
	assertModality(t, gpt.Modalities, "input", "video", "unsupported", "none")
	imageLimit := findLimit(codex.KnownLimits, "referenced_image_paths")
	if imageLimit == nil || imageLimit.Maximum == nil || *imageLimit.Maximum != 5 {
		t.Fatalf("Codex image-path limit = %#v, want maximum 5", imageLimit)
	}

	antigravity := byID["antigravity"]
	agyModel := findModel(antigravity.Models, "claude-opus-4-6-thinking")
	if agyModel == nil || !sameStrings(agyModel.Efforts, []string{"low", "medium", "high"}) {
		t.Fatalf("AGY model/efforts = %#v, want claude-opus-4-6-thinking with low, medium, high", agyModel)
	}
	assertModality(t, agyModel.Modalities, "input", "audio", "supported", "file_path")
	assertModality(t, agyModel.Modalities, "input", "video", "supported", "file_path")
	if limit := findLimit(antigravity.KnownLimits, "add_dir_workspace"); limit == nil || limit.Value != "--add-dir" {
		t.Fatalf("AGY workspace limit = %#v, want --add-dir", limit)
	}
	if limit := findLimit(antigravity.KnownLimits, "print_timeout"); limit == nil || limit.Default == nil || *limit.Default != 300 {
		t.Fatalf("AGY timeout limit = %#v, want default 300", limit)
	}
}

func findModel(models []modelOutput, id string) *modelOutput {
	for index := range models {
		if models[index].ID == id {
			return &models[index]
		}
	}
	return nil
}

func findLimit(limits []limitOutput, name string) *limitOutput {
	for index := range limits {
		if limits[index].Name == name {
			return &limits[index]
		}
	}
	return nil
}

func assertModality(t *testing.T, modalities []modalityOutput, direction, modality, support, transport string) {
	t.Helper()
	for _, candidate := range modalities {
		if candidate.Direction == direction && candidate.Modality == modality {
			if candidate.Support != support || candidate.Transport != transport {
				t.Fatalf("%s/%s modality = %#v, want support=%s transport=%s", direction, modality, candidate, support, transport)
			}
			return
		}
	}
	t.Fatalf("missing %s/%s modality", direction, modality)
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
