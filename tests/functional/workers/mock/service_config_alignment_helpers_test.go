package mock

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func twoStageServicePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "processing", "type": "PROCESSING"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
			{"name": "worker-b"},
		},
		"workstations": []map[string]any{
			{
				"name":      "step-one",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "step-two",
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func scaffoldSharedCommandRunnerFactory(t *testing.T) string {
	t.Helper()

	cfg := twoStageServicePipelineConfig()
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["workingDirectory"] = "/tmp/script-command-smoke"
	workstations[0]["env"] = map[string]any{"SCRIPT_ENV": "script-value"}
	workstations[1]["workingDirectory"] = "/tmp/provider-command-smoke"
	workstations[1]["env"] = map[string]any{"PROVIDER_ENV": "provider-value"}

	dir := support.ScaffoldFactory(t, cfg)
	support.WriteWorkstationConfig(t, dir, "step-two", `---
type: MODEL_WORKSTATION
---
Provider received {{ (index .Inputs 0).Payload }}.
`)
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: SCRIPT_WORKER
command: script-tool
args:
  - "{{ (index .Inputs 0).WorkID }}"
  - "{{ (index .Inputs 0).Payload }}"
---
`)
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "mixed-command-smoke-work",
		WorkTypeID: "task",
		TraceID:    "trace-mixed-command-smoke",
		Payload:    []byte("script-input"),
	})
	return dir
}

func assertSharedCommandRunnerScriptRequest(t *testing.T, dir string, scriptReq platformprocess.CommandRequest) {
	t.Helper()

	if scriptReq.Command != "script-tool" {
		t.Fatalf("script command = %q, want %q", scriptReq.Command, "script-tool")
	}
	if !reflect.DeepEqual(scriptReq.Args, []string{"mixed-command-smoke-work", "script-input"}) {
		t.Fatalf("script args = %v, want rendered work ID and payload args", scriptReq.Args)
	}
	if canonicalRuntimePath(scriptReq.WorkDir) != canonicalRuntimePath(support.ResolvedRuntimePath(dir, "/tmp/script-command-smoke")) {
		t.Fatalf("script work dir = %q, want %q", scriptReq.WorkDir, support.ResolvedRuntimePath(dir, "/tmp/script-command-smoke"))
	}
	if !containsEnv(scriptReq.Env, "SCRIPT_ENV=script-value") {
		t.Fatalf("script env missing SCRIPT_ENV in %v", scriptReq.Env)
	}
	if len(scriptReq.Stdin) != 0 {
		t.Fatalf("script stdin = %q, want empty stdin", string(scriptReq.Stdin))
	}
}

func assertSharedCommandRunnerProviderRequest(t *testing.T, dir string, providerReq platformprocess.CommandRequest) {
	t.Helper()

	if providerReq.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("provider command = %q, want %q", providerReq.Command, modelprovider.ProviderCodex)
	}
	support.AssertArgsContainSequence(t, providerReq.Args, []string{"exec"})
	support.AssertArgsContainSequence(t, providerReq.Args, []string{"--model", "gpt-5-codex"})
	if providerReq.Args[len(providerReq.Args)-1] != "-" {
		t.Fatalf("provider prompt placeholder = %q, want -", providerReq.Args[len(providerReq.Args)-1])
	}
	if !strings.Contains(string(providerReq.Stdin), "script-output") {
		t.Fatalf("provider stdin = %q, want it to include script output", string(providerReq.Stdin))
	}
	if canonicalRuntimePath(providerReq.WorkDir) != canonicalRuntimePath(support.ResolvedRuntimePath(dir, "/tmp/provider-command-smoke")) {
		t.Fatalf("provider work dir = %q, want %q", providerReq.WorkDir, support.ResolvedRuntimePath(dir, "/tmp/provider-command-smoke"))
	}
	if !containsEnv(providerReq.Env, "PROVIDER_ENV=provider-value") {
		t.Fatalf("provider env missing PROVIDER_ENV in %v", providerReq.Env)
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func canonicalRuntimePath(value string) string {
	if value == "" {
		return ""
	}

	cleaned := filepath.Clean(value)
	current := cleaned
	var suffix []string
	for {
		if _, err := os.Stat(current); err == nil {
			if resolved, err := filepath.EvalSymlinks(current); err == nil && resolved != "" {
				parts := append([]string{resolved}, suffix...)
				return filepath.Join(parts...)
			}
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
	return cleaned
}
