package discovery_test

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestProvidersListThroughRootBuildProcess proves the customer-facing command
// reaches the injected Providers service through the canonical process graph.
// The command runner is deliberately instrumented: discovery must not invoke
// a provider process, even though the same edge is available to execution.
// Keep one immutable root-built process for the three public invocations below;
// the ACP projection cases are pure and intentionally acquire no process.
func TestProvidersListThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	runner := &countingCommandRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	support.CleanupProcess(t, process)

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
	functionalevidence.Covers(t, "cli/you.providers.list")
}

func TestPackagedACPProjectionRejectsInvalidRuntimeBindings(t *testing.T) {
	const base = `{
  "acp": [{
    "name": "cursor-acp",
    "transport": "stdio",
    "executable": "cursor-agent",
    "command": "cursor-agent acp",
    "arguments": ["acp"],
    "posture": "installed_executable",
    "implementation": {"kind": "acp_agent", "profile": "cursor-acp"}
  }]
}`
	tests := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{
			name: "unknown runtime profile",
			mutate: func(document string) string {
				return strings.Replace(document, `"profile": "cursor-acp"`, `"profile": "missing-profile"`, 1)
			},
			want: "unknown runtime profile",
		},
		{
			name: "unsupported transport",
			mutate: func(document string) string {
				return strings.Replace(document, `"transport": "stdio"`, `"transport": "tcp"`, 1)
			},
			want: "unsupported transport",
		},
		{
			name: "argument drift",
			mutate: func(document string) string {
				return strings.Replace(document, `"arguments": ["acp"]`, `"arguments": ["wrong"]`, 1)
			},
			want: "command arguments drift",
		},
		{
			name: "canonical alias",
			mutate: func(document string) string {
				return strings.Replace(document, `"transport": "stdio",`, "\"aliases\": [\"cursor-acp\"],\n    \"transport\": \"stdio\",", 1)
			},
			want: "duplicates its canonical identity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := providerswire.ACPIntegrationsFromRuntimeCatalog([]byte(test.mutate(base)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("packaged runtime validation error = %v, want containing %q", err, test.want)
			}
		})
	}
}

type countingCommandRunner struct {
	calls atomic.Int32
}

func (runner *countingCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{}, nil
}

func execute(t *testing.T, process support.Process, args []string) (string, string, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	stdinIsTTY := false
	stdoutIsTTY := false
	inputs.Input.Env = os.Environ()
	inputs.Input.Stdin = strings.NewReader("")
	inputs.Input.Context = t.Context()
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.StdoutIsTTY = &stdoutIsTTY
	err := process.Execute(inputs.Input)
	return inputs.Stdout(), inputs.Stderr(), err
}

func assertHumanFacts(t *testing.T, output string) {
	t.Helper()
	for _, providerID := range expectedProviderIDs() {
		if count := strings.Count(output, providerID+"\t"); count != 1 {
			t.Fatalf("human output provider %q count = %d, want exactly once\n%s", providerID, count, output)
		}
	}
	for _, want := range []string{
		"antigravity\tAntigravity\tselectable\tunverified",
		"claude\tClaude Code\tselectable\tunverified",
		"codex\tCodex\tselectable\tunverified",
		"authentication/account-authentication: required",
		"gpt-5.6",
		"gpt-5.6-luna",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"audio: unsupported (transport: none)",
		"video: unsupported (transport: none)",
		"referenced_image_paths [maximum, paths] maximum=5",
		"claude-opus-4-6-thinking",
		"claude-sonnet-5",
		"Efforts:\tnone",
		"audio: supported (transport: file_path)",
		"video: supported (transport: file_path)",
		"add_dir_workspace [behavior, flag] value=--add-dir",
		"effort_selection [behavior, model_id] value=model_id",
		"print_timeout [default, seconds] default=300",
		"droid-acp\tDroid ACP\tselectable\tunverified",
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
	ID                         string               `json:"id"`
	Aliases                    []string             `json:"aliases"`
	Availability               string               `json:"availability"`
	Readiness                  string               `json:"readiness"`
	ImplementationAvailability string               `json:"implementationAvailability"`
	Capabilities               []string             `json:"capabilities"`
	Models                     []modelOutput        `json:"models"`
	Prerequisites              []prerequisiteOutput `json:"prerequisites"`
	Tools                      []any                `json:"tools"`
	KnownLimits                []limitOutput        `json:"knownLimits"`
}

type prerequisiteOutput struct {
	Status string `json:"status"`
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
	decoded := decodeJSONOutput(t, output)
	byID := assertJSONProviderInventory(t, decoded)
	assertJSONCodexFacts(t, byID["codex"])
	assertJSONAntigravityFacts(t, byID["antigravity"])
	assertJSONClaudeFacts(t, byID["claude"])
}

func decodeJSONOutput(t *testing.T, output string) listOutput {
	t.Helper()
	var decoded listOutput
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("providers list JSON is invalid: %v\n%s", err, output)
	}
	return decoded
}

func assertJSONProviderInventory(t *testing.T, decoded listOutput) map[string]providerOutput {
	t.Helper()
	wantIDs := expectedProviderIDs()
	if len(decoded.Providers) != len(wantIDs) {
		t.Fatalf("provider count = %d, want %d", len(decoded.Providers), len(wantIDs))
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
	for _, providerID := range wantIDs {
		if _, ok := byID[providerID]; !ok {
			t.Fatalf("provider %q is missing from %v", providerID, ids)
		}
	}
	for _, providerID := range []string{"antigravity", "claude", "codex"} {
		provider, ok := byID[providerID]
		if !ok {
			t.Fatalf("first-party provider %q is missing from %v", providerID, ids)
		}
		if provider.Models == nil || provider.Prerequisites == nil || provider.Tools == nil || provider.KnownLimits == nil {
			t.Fatalf("provider %q omitted explicit capability arrays: %#v", providerID, provider)
		}
		if provider.Availability != "selectable" || provider.Readiness != "unverified" {
			t.Fatalf("provider %q availability/readiness = %q/%q, want selectable/unverified", providerID, provider.Availability, provider.Readiness)
		}
		for _, prerequisite := range provider.Prerequisites {
			if prerequisite.Status != "required" {
				t.Fatalf("provider %q prerequisite status = %q, want required", providerID, prerequisite.Status)
			}
		}
	}
	for _, providerID := range expectedACPProviderIDs() {
		provider := byID[providerID]
		if provider.Availability != "selectable" || provider.Readiness != "unverified" {
			t.Fatalf("ACP provider %q availability/readiness = %q/%q, want selectable/unverified", providerID, provider.Availability, provider.Readiness)
		}
		if provider.ImplementationAvailability != "externally-supplied" {
			t.Fatalf("ACP provider %q implementation availability = %q, want externally-supplied", providerID, provider.ImplementationAvailability)
		}
		if provider.Models == nil || len(provider.Models) != 0 || provider.Tools == nil || len(provider.Tools) != 0 || provider.KnownLimits == nil || len(provider.KnownLimits) != 0 {
			t.Fatalf("ACP provider %q published unverified model/tool/limit facts = %#v", providerID, provider)
		}
		wantCapabilities := []string{"prompt_submission"}
		if providerID == "cursor-acp" {
			wantCapabilities = []string{"image_input", "permission_bypass", "prompt_submission"}
		}
		if !sameStrings(provider.Capabilities, wantCapabilities) {
			t.Fatalf("ACP provider %q capabilities = %v, want %v", providerID, provider.Capabilities, wantCapabilities)
		}
		if len(provider.Prerequisites) != 4 {
			t.Fatalf("ACP provider %q prerequisites = %#v, want stdio, executable, authentication, and workspace", providerID, provider.Prerequisites)
		}
		for _, prerequisite := range provider.Prerequisites {
			if prerequisite.Status != "required" {
				t.Fatalf("ACP provider %q prerequisite status = %q, want required", providerID, prerequisite.Status)
			}
		}
	}
	if droid := byID["droid-acp"]; !sameStrings(droid.Aliases, []string{"factory-droid", "factorydroid"}) {
		t.Fatalf("Droid aliases = %v, want factory-droid and factorydroid", droid.Aliases)
	}
	return byID
}

func expectedProviderIDs() []string {
	return append([]string{"antigravity", "claude", "codex"}, expectedACPProviderIDs()...)
}

func expectedACPProviderIDs() []string {
	return []string{
		"copilot-acp", "cursor-acp", "droid-acp", "fast-agent-acp", "gemini-acp",
		"grok-build-acp", "iflow-acp", "kilocode-acp", "kimi-acp", "kiro-acp",
		"mux-acp", "openclaw-acp", "opencode-acp", "pi-acp", "pool-acp",
		"qoder-acp", "qwen-acp", "reasonix-acp", "trae-acp", "zeroclaw-acp",
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertJSONCodexFacts(t *testing.T, codex providerOutput) {
	t.Helper()
	for _, modelID := range []string{"gpt-5.6", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra"} {
		gpt := findModel(codex.Models, modelID)
		if gpt == nil {
			t.Fatalf("Codex is missing %s", modelID)
		}
		assertModality(t, gpt.Modalities, "input", "audio", "unsupported", "none")
		assertModality(t, gpt.Modalities, "input", "video", "unsupported", "none")
	}
	imageLimit := findLimit(codex.KnownLimits, "referenced_image_paths")
	if imageLimit == nil || imageLimit.Maximum == nil || *imageLimit.Maximum != 5 {
		t.Fatalf("Codex image-path limit = %#v, want maximum 5", imageLimit)
	}
}

func assertJSONAntigravityFacts(t *testing.T, antigravity providerOutput) {
	t.Helper()
	agyModel := findModel(antigravity.Models, "claude-opus-4-6-thinking")
	if agyModel == nil || len(agyModel.Efforts) != 0 {
		t.Fatalf("AGY model/efforts = %#v, want claude-opus-4-6-thinking with explicit empty efforts", agyModel)
	}
	assertModality(t, agyModel.Modalities, "input", "audio", "supported", "file_path")
	assertModality(t, agyModel.Modalities, "input", "video", "supported", "file_path")
	if limit := findLimit(antigravity.KnownLimits, "add_dir_workspace"); limit == nil || limit.Value != "--add-dir" {
		t.Fatalf("AGY workspace limit = %#v, want --add-dir", limit)
	}
	if limit := findLimit(antigravity.KnownLimits, "effort_selection"); limit == nil || limit.Value != "model_id" {
		t.Fatalf("AGY effort-selection limit = %#v, want model_id", limit)
	}
	if limit := findLimit(antigravity.KnownLimits, "print_timeout"); limit == nil || limit.Default == nil || *limit.Default != 300 {
		t.Fatalf("AGY timeout limit = %#v, want default 300", limit)
	}
}

func assertJSONClaudeFacts(t *testing.T, claude providerOutput) {
	t.Helper()
	wantClaudeModels := []string{"claude-opus-4-6-thinking", "claude-sonnet-4-20250514", "claude-sonnet-5"}
	if len(claude.Models) != len(wantClaudeModels) {
		t.Fatalf("Claude models = %#v, want exact IDs %v", claude.Models, wantClaudeModels)
	}
	for _, modelID := range wantClaudeModels {
		model := findModel(claude.Models, modelID)
		if model == nil {
			t.Fatalf("Claude is missing model %s", modelID)
		}
		if len(model.Efforts) != 5 || strings.Join(model.Efforts, ",") != "low,medium,high,xhigh,max" {
			t.Fatalf("Claude %s efforts = %v, want low through max", model.ID, model.Efforts)
		}
		assertModality(t, model.Modalities, "input", "text", "supported", "inline")
		assertModality(t, model.Modalities, "input", "audio", "unsupported", "none")
		assertModality(t, model.Modalities, "input", "video", "unsupported", "none")
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
