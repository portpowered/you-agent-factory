package initcmd

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ScaffoldType names a supported init scaffold.
type ScaffoldType string

const (
	// DefaultScaffoldType is the existing single-step task-processing scaffold.
	DefaultScaffoldType ScaffoldType = "default"
	// RalphScaffoldType is the minimal PRD-to-execution scaffold.
	RalphScaffoldType ScaffoldType = "ralph"

	// DefaultFactoryInputType is the work type created by the default scaffold.
	DefaultFactoryInputType = "task"
	// RalphFactoryInputType is the request intake work type for the Ralph scaffold.
	RalphFactoryInputType = "request"
)

type scaffoldDefinition struct {
	inputWorkType string
	files         map[string]string
}

var supportedScaffoldTypes = []ScaffoldType{
	DefaultScaffoldType,
	RalphScaffoldType,
}

func resolveScaffoldDefinition(rawType string) (ScaffoldType, scaffoldDefinition, error) {
	scaffoldType := DefaultScaffoldType
	if rawType != "" {
		scaffoldType = ScaffoldType(rawType)
	}

	switch scaffoldType {
	case DefaultScaffoldType:
		return scaffoldType, defaultScaffoldDefinition(), nil
	case RalphScaffoldType:
		return scaffoldType, ralphScaffoldDefinition(), nil
	default:
		return "", scaffoldDefinition{}, fmt.Errorf(
			"unsupported scaffold type %q (supported: %s)",
			rawType,
			supportedScaffoldTypesString(),
		)
	}
}

func supportedScaffoldTypesString() string {
	parts := make([]string, 0, len(supportedScaffoldTypes))
	for _, scaffoldType := range supportedScaffoldTypes {
		parts = append(parts, string(scaffoldType))
	}
	return strings.Join(parts, ", ")
}

func defaultScaffoldDefinition() scaffoldDefinition {
	return scaffoldDefinition{
		inputWorkType: DefaultFactoryInputType,
		files: map[string]string{
			interfaces.FactoryConfigFile: `{
  "name": "factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "processor" }
  ],
  "workstations": [
    {
      "name": "process",
      "worker": "processor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}
`,
			factoryWorkersDirName + "/README.md": `# Workers

Worker configuration files go here.
Each subdirectory contains an AGENTS.md defining a worker type with its execution settings.
`,
			factoryWorkstationsDirName + "/README.md": `# Workstations

Workstation configuration files go here.
Each subdirectory contains an AGENTS.md defining the workstation prompt template.
`,
			factoryInputsDirName + "/README.md": `# Inputs

Multi-channel input directory for work submissions.

Default local task path:
  inputs/task/default/                 - Markdown or JSON task submissions

General layout:
  inputs/<work-type>/default/          - manual submissions
  inputs/<work-type>/<execution-id>/   - executor-generated work

The file watcher monitors this directory tree and automatically watches new subdirectories.
`,
			factoryWorkersDirName + "/processor/" + factoryAgentsFileName: `---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
resources: ["agent-slot"]
timeout: 1h
skipPermissions: true
---`,
			factoryWorkstationsDirName + "/process/" + factoryAgentsFileName: `---
type: MODEL_WORKSTATION
---

You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

The customer has asked you to perform the following request:

{{ (index .Inputs 0).Payload }}
`,
		},
	}
}

// portos:func-length-exception owner=agent-factory reason=ralph-init-scaffold-template review=2026-07-21 removal=extract-ralph-readme-factory-and-agents-templates-before-next-ralph-scaffold-expansion
func ralphScaffoldDefinition() scaffoldDefinition {
	return scaffoldDefinition{
		inputWorkType: RalphFactoryInputType,
		files: map[string]string{
			"README.md": `# Ralph Scaffold

This scaffold turns an incoming request into aligned planning artifacts and then
completes one story per execution iteration until the plan is done.

## Workflow

1. ` + "`plan-request`" + ` reads a request from ` + "`inputs/request/default/`" + ` and writes:
   - ` + "`prd.md`" + `
   - ` + "`prd.json`" + `
   - ` + "`progress.txt`" + `
2. ` + "`execute-story`" + ` reads those artifacts and completes one incomplete story per iteration.
3. ` + "`execute-story-loop-breaker`" + ` is an internal guarded failure path for repeated execution iterations.

This scaffold intentionally excludes reviewer, thoughts or ideation, and cron stages.

## Quickstart

Create the scaffold from your project root:

` + "```bash" + `
agent-factory init --type ralph --dir ralph-factory
` + "```" + `

Run it from your project root:

` + "```bash" + `
agent-factory run --dir ralph-factory
` + "```" + `

Seed an initial request without moving any generated files:

` + "```bash" + `
printf "Create a minimal release-planning loop for a document processing service.\nGenerate a human-readable PRD, a matching Ralph JSON plan, and an execution loop that completes one story per iteration until the work is done.\nKeep the plan product-neutral unless the customer request names a specific product.\n" > ralph-factory/inputs/request/default/release-planning-loop.md
` + "```" + `

The planner writes ` + "`prd.md`" + `, ` + "`prd.json`" + `, and ` + "`progress.txt`" + ` in ` + "`ralph-factory/`" + `.
The executor keeps those artifacts aligned and returns ` + "`<COMPLETE>`" + ` only when every story passes.
`,
			interfaces.FactoryConfigFile: `{
  "name": "factory",
  "workTypes": [
    {
      "name": "request",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "planned", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    },
    {
      "name": "story",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "planner" },
    { "name": "executor" }
  ],
  "workstations": [
    {
      "name": "plan-request",
      "worker": "planner",
      "workingDirectory": ".",
      "inputs": [{ "workType": "request", "state": "init" }],
      "outputs": [
        { "workType": "request", "state": "planned" },
        { "workType": "story", "state": "init" }
      ],
      "onFailure": { "workType": "request", "state": "failed" }
    },
    {
      "name": "execute-story",
      "behavior": "REPEATER",
      "worker": "executor",
      "workingDirectory": ".",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onContinue": { "workType": "story", "state": "init" },
      "onFailure": { "workType": "story", "state": "failed" }
    },
    {
      "name": "execute-story-loop-breaker",
      "type": "LOGICAL_MOVE",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "failed" }],
      "guards": [
        {
          "type": "VISIT_COUNT",
          "workstation": "execute-story",
          "maxVisits": 8
        }
      ]
    }
  ]
}
`,
			factoryWorkersDirName + "/README.md": `# Workers

The Ralph scaffold starts with two workers:
- planner creates aligned prd.md, prd.json, and progress.txt artifacts from an incoming request.
- executor advances one incomplete story per iteration until the plan is complete.

Edit each worker's AGENTS.md to match your provider, model, and execution policy.
The scaffold intentionally omits reviewer, ideation, and cron workers.
`,
			factoryWorkstationsDirName + "/README.md": `# Workstations

The Ralph scaffold keeps two customer-facing stages:
1. plan-request turns an incoming request into aligned prd.md, prd.json, and progress.txt artifacts.
2. execute-story reads those artifacts and completes one incomplete story iteration at a time.

An internal guarded loop-breaker routes exhausted story work to failed after repeated execution passes.
The scaffold intentionally excludes reviewer, ideation, and cron stages.
`,
			factoryInputsDirName + "/README.md": `# Inputs

The Ralph scaffold watches request intake here:
  inputs/request/default/              - customer requests to turn into a plan and execution loop

General layout:
  inputs/<work-type>/default/          - manual submissions
  inputs/<work-type>/<execution-id>/   - executor-generated work

The file watcher monitors this directory tree and automatically watches new subdirectories.

Example request payload to drop into inputs/request/default/ as Markdown:

Create a minimal release-planning loop for a document processing service.
Generate a human-readable PRD, a matching Ralph JSON plan, and an execution loop
that completes one story per iteration until the work is done.
Keep the plan product-neutral unless the customer request names a specific product.
`,
			factoryWorkersDirName + "/planner/" + factoryAgentsFileName: `---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
stopToken: "<COMPLETE>"
resources: ["agent-slot"]
timeout: 1h
skipPermissions: true
---

You are the planning worker for a minimal PRD-to-execution loop.
Produce clear, product-neutral planning artifacts that the executor can apply directly.
`,
			factoryWorkersDirName + "/executor/" + factoryAgentsFileName: `---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
stopToken: "<COMPLETE>"
resources: ["agent-slot"]
timeout: 1h
skipPermissions: true
---

You are the execution worker for a minimal PRD-driven implementation loop.
Complete one story at a time, keep the planning artifacts aligned with reality,
and leave reviewer, ideation, and cron concerns out of scope.
`,
			factoryWorkstationsDirName + "/plan-request/" + factoryAgentsFileName: `---
type: MODEL_WORKSTATION
---

You are planning work item {{ (index .Inputs 0).WorkID }}.

Create the planning artifacts in the current working directory:
1. "prd.md" — a human-readable PRD for the requested change.
2. "prd.json" — a matching Ralph JSON plan for the execution loop.
3. "progress.txt" — initialize it with a "## Codebase Patterns" section for future iterations.

Requirements for "prd.json":
- include "branchName" with a deterministic, branch-safe name for the planned work
- capture the project description, requested changes, and customer intent from the request
- include prioritized user stories with stable IDs, clear titles, acceptance criteria, notes, and "passes: false" until completed
- keep the structure aligned with "prd.md" so the execution loop can trust either artifact
- keep names and wording product-neutral unless the request explicitly names a product

Requirements for "prd.md":
- describe the same branch, scope, priorities, and acceptance criteria as "prd.json"
- explain the work in customer-facing language
- do not introduce reviewer, ideation, or cron stages that are outside this scaffold

{{ if .Context.WorkDir }}
Working directory: {{ .Context.WorkDir }}
{{ end }}
{{ if .Context.Project }}
Project context: {{ .Context.Project }}
{{ end }}
{{ if index (index .Inputs 0).Tags "branch" }}
Requested branch tag: {{ index (index .Inputs 0).Tags "branch" }}
{{ end }}

When "prd.md", "prd.json", and "progress.txt" are written and aligned, respond with "<COMPLETE>".

Customer request:

{{ (index .Inputs 0).Payload }}
`,
			factoryWorkstationsDirName + "/execute-story/" + factoryAgentsFileName: `---
type: MODEL_WORKSTATION
---

You are executing Ralph story work item {{ (index .Inputs 0).WorkID }}.

Read the generated planning artifacts from the current working directory before you change anything:
- "prd.json"
- "prd.md"
- "progress.txt" if it already exists

Execution loop rules:
1. Pick the highest-priority user story in "prd.json" where "passes" is "false".
2. Complete only that one story in this iteration.
3. Keep "prd.md", "prd.json", and "progress.txt" aligned with the work you finished.
4. Run the relevant validation before marking the story complete.
5. Mark the finished story "passes: true" in "prd.json".
6. Respond with "<COMPLETE>" only when every story in "prd.json" is complete. Otherwise respond with "<CONTINUE>" after finishing the current iteration.
7. Treat "<CONTINUE>" as ordinary partial progress for another execution pass. Reserve rejection semantics for a separate review step that sends work back.

Keep the workflow product-neutral and do not invent reviewer, ideation, or cron steps that are outside this scaffold.

{{ if .Context.WorkDir }}
Working directory: {{ .Context.WorkDir }}
{{ end }}
{{ if .Context.Project }}
Project context: {{ .Context.Project }}
{{ end }}
{{ if index (index .Inputs 0).Tags "branch" }}
Requested branch tag: {{ index (index .Inputs 0).Tags "branch" }}
{{ end }}

Story payload:

{{ (index .Inputs 0).Payload }}
`,
		},
	}
}
