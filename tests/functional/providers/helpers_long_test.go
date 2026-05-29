//go:build functionallong

package providers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func buildModelWorkerConfig(provider interfaces.ModelProvider, model string) string {
	return fmt.Sprintf(`---
type: MODEL_WORKER
model: %s
modelProvider: %s
stopToken: COMPLETE
---
Process the input task.
`, model, provider)
}

func writeNamedWorkerAgents(t *testing.T, dir, workerName, content string) {
	t.Helper()

	support.WriteAgentConfig(t, dir, workerName, content)
}

func writeExecutionTemplateWorkstationAgents(t *testing.T, dir, workstationName string) {
	t.Helper()

	agentsMD := strings.Join([]string{
		"---",
		"type: MODEL_WORKSTATION",
		`workingDirectory: '/workspace/{{ (index .Inputs 0).Name }}/{{ index (index .Inputs 0).Tags "branch" }}'`,
		`worktree: 'worktrees/{{ index (index .Inputs 0).Tags "branch" }}/{{ (index .Inputs 0).WorkID }}'`,
		"env:",
		`  TEMPLATE_BRANCH: '{{ index (index .Inputs 0).Tags "branch" }}'`,
		`  TEMPLATE_NAME: '{{ (index .Inputs 0).Name }}'`,
		`  TEMPLATE_PAYLOAD: '{{ (index .Inputs 0).Payload }}'`,
		`  TEMPLATE_WORKID: '{{ (index .Inputs 0).WorkID }}'`,
		"---",
		executionTemplatePrompt(),
	}, "\n") + "\n"
	writeFixtureFile(t, dir, []string{"workstations", workstationName, "AGENTS.md"}, agentsMD)
}

func configureResourceGatedTemplateWorkstation(t *testing.T, dir string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["resources"] = []any{
			map[string]any{"name": "aaa-slot", "capacity": 1},
			map[string]any{"name": "zzz-slot", "capacity": 1},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["resources"] = []any{
			map[string]any{"name": "aaa-slot", "capacity": 1},
			map[string]any{"name": "zzz-slot", "capacity": 1},
		}
	})
}

func configureExecutionTemplateWorkstation(t *testing.T, dir string) {
	t.Helper()

	workstationName := ""
	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["resources"] = []any{
			map[string]any{"name": "template-slot", "capacity": 1},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstationName = workstation["name"].(string)
		workstation["resources"] = []any{
			map[string]any{"name": "template-slot", "capacity": 1},
		}
	})
	writeExecutionTemplateWorkstationAgents(t, dir, workstationName)
}

func configureCursorExecutionTemplateWorkstation(t *testing.T, dir string) {
	t.Helper()

	workstationName := ""
	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["resources"] = []any{
			map[string]any{"name": "template-slot", "capacity": 1},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstationName = workstation["name"].(string)
		workstation["resources"] = []any{
			map[string]any{"name": "template-slot", "capacity": 1},
		}
	})
	writeCursorExecutionTemplateWorkstationAgents(t, dir, workstationName)
}

func writeCursorExecutionTemplateWorkstationAgents(t *testing.T, dir, workstationName string) {
	t.Helper()

	agentsMD := strings.Join([]string{
		"---",
		"type: MODEL_WORKSTATION",
		`workingDirectory: '/workspace/{{ (index .Inputs 0).Name }}/{{ index (index .Inputs 0).Tags "branch" }}'`,
		"env:",
		`  TEMPLATE_BRANCH: '{{ index (index .Inputs 0).Tags "branch" }}'`,
		`  TEMPLATE_NAME: '{{ (index .Inputs 0).Name }}'`,
		`  TEMPLATE_PAYLOAD: '{{ (index .Inputs 0).Payload }}'`,
		`  TEMPLATE_WORKID: '{{ (index .Inputs 0).WorkID }}'`,
		"---",
		executionTemplatePrompt(),
	}, "\n") + "\n"
	writeFixtureFile(t, dir, []string{"workstations", workstationName, "AGENTS.md"}, agentsMD)
}

func configureTwoInputResourceGatedTemplateWorkstation(t *testing.T, dir, workstationName, workerName string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workTypes"] = []any{
			map[string]any{
				"name": "zeta-resource",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
			map[string]any{
				"name": "alpha-resource",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		}
		cfg["resources"] = []any{
			map[string]any{"name": "repo-slot", "capacity": 1},
			map[string]any{"name": "gpu-slot", "capacity": 1},
		}
		cfg["workers"] = []any{map[string]any{"name": workerName}}
		cfg["workstations"] = []any{map[string]any{
			"name":   workstationName,
			"worker": workerName,
			"inputs": []any{
				map[string]any{"workType": "zeta-resource", "state": "init"},
				map[string]any{"workType": "alpha-resource", "state": "init"},
			},
			"outputs": []any{
				map[string]any{"workType": "zeta-resource", "state": "done"},
				map[string]any{"workType": "alpha-resource", "state": "done"},
			},
			"onFailure": []map[string]any{{"workType": "zeta-resource", "state": "failed"}},
			"resources": []any{map[string]any{"name": "repo-slot", "capacity": 1}, map[string]any{"name": "gpu-slot", "capacity": 1}},
		}}
	})
}

func writeTwoInputResourceSeeds(t *testing.T, dir string) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "zeta-input-name",
		WorkID:     "zeta-work",
		WorkTypeID: "zeta-resource",
		TraceID:    "trace-two-input-resources",
		Payload:    []byte("zeta-payload"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "alpha-input-name",
		WorkID:     "alpha-work",
		WorkTypeID: "alpha-resource",
		TraceID:    "trace-two-input-resources",
		Payload:    []byte("alpha-payload"),
	})
}

func writeExecutionTemplateSeed(t *testing.T, dir string) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "execution-template-name",
		WorkID:     "work-execution-template",
		WorkTypeID: "task",
		TraceID:    "trace-execution-template",
		Payload:    []byte("execution-template-payload"),
		Tags: map[string]string{
			"branch": "feature-token-branch",
		},
	})
}

func twoInputTemplateArgs() []string {
	return []string{
		`first_name={{ (index .Inputs 0).Name }}`,
		`first_payload={{ (index .Inputs 0).Payload }}`,
		`second_name={{ (index .Inputs 1).Name }}`,
		`second_payload={{ (index .Inputs 1).Payload }}`,
		`inputs={{ len .Inputs }}`,
	}
}

func executionTemplatePrompt() string {
	return strings.Join([]string{
		`name={{ (index .Inputs 0).Name }}`,
		`payload={{ (index .Inputs 0).Payload }}`,
		`context_workdir={{ .Context.WorkDir }}`,
		`env_branch={{ index .Context.Env "TEMPLATE_BRANCH" }}`,
		`env_workid={{ index .Context.Env "TEMPLATE_WORKID" }}`,
		`inputs={{ len .Inputs }}`,
	}, "\n")
}

func executionTemplateWantPrompt(dir string) string {
	return strings.Join([]string{
		"name=execution-template-name",
		"payload=execution-template-payload",
		"context_workdir=" + support.ResolvedRuntimePath(dir, "/workspace/execution-template-name/feature-token-branch"),
		"env_branch=feature-token-branch",
		"env_workid=work-execution-template",
		"inputs=1",
	}, "\n")
}

func cursorExecutionTemplateWantPrompt(dir string) string {
	return cursorMergedPrompt("Process the input task.", executionTemplateWantPrompt(dir))
}

func assertProviderExecutionFields(t *testing.T, dir string, req workers.CommandRequest) {
	t.Helper()

	if req.WorkDir != support.ResolvedRuntimePath(dir, "/workspace/execution-template-name/feature-token-branch") {
		t.Fatalf("provider work dir = %q, want resolved workstation working_directory", req.WorkDir)
	}
	for _, want := range []string{
		"TEMPLATE_BRANCH=feature-token-branch",
		"TEMPLATE_NAME=execution-template-name",
		"TEMPLATE_PAYLOAD=execution-template-payload",
		"TEMPLATE_WORKID=work-execution-template",
	} {
		if !containsEnv(req.Env, want) {
			t.Fatalf("provider env missing %s in %v", want, req.Env)
		}
	}
}
