package script_test

import (
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	scriptWorkerConfig        = "---\nargs:\n    - default-output\ncommand: echo\ntype: SCRIPT_WORKER\n---\n"
	scriptWorkstationPrompt   = "---\ntype: MODEL_WORKSTATION\n---\nExecute the script.\n"
	worktreeWorkerConfig      = "---\ntype: MODEL_WORKER\nmodel: test-model\nmodelProvider: claude\nstopToken: COMPLETE\n---\nProcess the input task.\n"
	worktreeWorkstationPrompt = "---\ntype: MODEL_WORKSTATION\n---\nProcess the task.\n"
)

func newScriptFactoryDir(t *testing.T, name string) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": name,
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "done", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "script-worker"}},
		"workstations": []map[string]any{{
			"name":      "run-script",
			"worker":    "script-worker",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "done"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, "script-worker", scriptWorkerConfig)
	support.WriteWorkstationConfig(t, dir, "run-script", scriptWorkstationPrompt)
	return dir
}

func newScriptWorktreeFactoryDir(t *testing.T, name string) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": name,
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
			"worktree":  "{{ (index .Inputs 0).Name }}",
		}},
	})
	support.WriteAgentConfig(t, dir, "worker-a", worktreeWorkerConfig)
	support.WriteWorkstationConfig(t, dir, "process", worktreeWorkstationPrompt)
	return dir
}
