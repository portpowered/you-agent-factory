package initcmd

import "github.com/portpowered/infinite-you/pkg/interfaces"

func ralphScaffoldDefinition() scaffoldDefinition {
	return scaffoldDefinition{
		inputWorkType: RalphFactoryInputType,
		files: map[string]string{
			"README.md":                                                            ralphReadme(),
			interfaces.FactoryConfigFile:                                           ralphFactoryConfig(),
			factoryWorkersDirName + "/README.md":                                   ralphWorkersReadme(),
			factoryWorkstationsDirName + "/README.md":                              ralphWorkstationsReadme(),
			factoryInputsDirName + "/README.md":                                    ralphInputsReadme(),
			factoryWorkersDirName + "/planner/" + factoryAgentsFileName:            ralphPlannerWorkerAgents(),
			factoryWorkersDirName + "/executor/" + factoryAgentsFileName:           ralphExecutorWorkerAgents(),
			factoryWorkstationsDirName + "/plan-request/" + factoryAgentsFileName:  ralphPlanRequestWorkstationAgents(),
			factoryWorkstationsDirName + "/execute-story/" + factoryAgentsFileName: ralphExecuteStoryWorkstationAgents(),
		},
	}
}

func ralphReadme() string {
	return `# Ralph Scaffold

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
you init --type ralph --dir ralph-factory
` + "```" + `

Run it from your project root:

` + "```bash" + `
you run --dir ralph-factory
` + "```" + `

Seed an initial request without moving any generated files:

` + "```bash" + `
printf "Create a minimal release-planning loop for a document processing service.\nGenerate a human-readable PRD, a matching Ralph JSON plan, and an execution loop that completes one story per iteration until the work is done.\nKeep the plan product-neutral unless the customer request names a specific product.\n" > ralph-factory/inputs/request/default/release-planning-loop.md
` + "```" + `

The planner writes ` + "`prd.md`" + `, ` + "`prd.json`" + `, and ` + "`progress.txt`" + ` in ` + "`ralph-factory/`" + `.
The executor keeps those artifacts aligned and returns ` + "`<COMPLETE>`" + ` only when every story passes.
`
}

func ralphFactoryConfig() string {
	return `{
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
      "onFailure": [{ "workType": "request", "state": "failed" }]
    },
    {
      "name": "execute-story",
      "behavior": "REPEATER",
      "worker": "executor",
      "workingDirectory": ".",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onContinue": [{ "workType": "story", "state": "init" }],
      "onFailure": [{ "workType": "story", "state": "failed" }]
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
`
}

func ralphWorkersReadme() string {
	return `# Workers

The Ralph scaffold starts with two workers:
- planner creates aligned prd.md, prd.json, and progress.txt artifacts from an incoming request.
- executor advances one incomplete story per iteration until the plan is complete.

Edit each worker's AGENTS.md to match your provider, model, and execution policy.
The scaffold intentionally omits reviewer, ideation, and cron workers.
`
}

func ralphWorkstationsReadme() string {
	return `# Workstations

The Ralph scaffold keeps two customer-facing stages:
1. plan-request turns an incoming request into aligned prd.md, prd.json, and progress.txt artifacts.
2. execute-story reads those artifacts and completes one incomplete story iteration at a time.

An internal guarded loop-breaker routes exhausted story work to failed after repeated execution passes.
The scaffold intentionally excludes reviewer, ideation, and cron stages.
`
}

func ralphInputsReadme() string {
	return `# Inputs

The Ralph scaffold starts with one canonical request inbox:
  inputs/request/default/              - drop each starter request here as a Markdown file

Seed your first Ralph run by writing a request into that directory, then run the scaffold.
The generated planner and executor already route the rest of the loop from that canonical inbox contract.

Example request payload to drop into inputs/request/default/ as Markdown:

Create a minimal release-planning loop for a document processing service.
Generate a human-readable PRD, a matching Ralph JSON plan, and an execution loop
that completes one story per iteration until the work is done.
Keep the plan product-neutral unless the customer request names a specific product.
`
}

func ralphPlannerWorkerAgents() string {
	return `---
type: MODEL_WORKER
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
stopToken: "<COMPLETE>"
resources: ["agent-slot"]
timeout: 1h
skipPermissions: true
---

You are the planning worker for a minimal PRD-to-execution loop.
Produce clear, product-neutral planning artifacts that the executor can apply directly.
`
}

func ralphExecutorWorkerAgents() string {
	return `---
type: MODEL_WORKER
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
`
}

func ralphPlanRequestWorkstationAgents() string {
	return `---
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
`
}

func ralphExecuteStoryWorkstationAgents() string {
	return `---
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
`
}
