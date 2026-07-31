# you-agent-factory CLI Reference

This directory is the **single source of truth** for packaged CLI reference
topics. Markdown here is embedded into the `you` binary (`docs/reference/embed.go`
→ `you docs <topic>`). There is no second tree to edit or copy under
`pkg/transports/cli/docs/`.

## Maintainer workflow

1. Edit the topic file in this directory (for example `docs/reference/config.md`).
2. Run `make docs-reference-smoke` from the repository root.
3. Ship the change. Rebuild or release the CLI when operators need the updated
   embedded output; do not mirror markdown into another directory.

Use the fixed CLI topic names for quick terminal help, then use the canonical
concept owners below when you need the complete customer-facing contract.

## Packaged CLI Topics

> **Terminal / agent readers:** cross-references between packaged CLI topics are runnable as `you docs <topic>` (for example `you docs config`). The `.md` links in maintainer tables below point at source files in this directory for editing only; they do not resolve in `you docs` terminal output.

`you docs <topic>` accepts these topics:

| Topic | Packaged scope | Canonical or broader customer guide |
|-------|----------------|--------------------------------------|
| `authoring-factories` | Practical factory authoring workflow, runnable examples, and cross-links to run-mode guides | [Author factories](authoring-factories.md) |
| `run` | Supported local, one-shot, batch, continuous, and mock-worker run shapes | [Run](run.md) |
| `config` | Operator initialization plus Factory validation, transformation, and minimum authoring contract | [Config](config.md) and [Author factories](authoring-factories.md) |
| `mock-workers` | `--with-mock-workers` and the `mockWorkers` JSON contract | [Mock workers](mock-workers.md) |
| `record-replay` | Default recording, `--record`, `--replay`, and `--no-record` | [Record and replay](record-replay.md) |
| `work` | Submitted work: session-scoped work routes, tags, and batch cross-links | [Submitted work](work.md) |
| `sessions` | Session list, session show, pause and resume, factory query, status API, dashboard, and run modes | [Sessions](sessions.md) |
| `orchestrators` | Factory orchestrator identity, FactorySession runtime nouns, dispatch/artifact/event aliases | [Orchestrators](orchestrators.md) |
| `javascript-workflows` | Supported JavaScript authoring, equivalent CLI/API/MCP execution, worker presets, host boundaries, and runnable examples | [JavaScript workflows](javascript-workflows.md) |
| `mcp` | `you mcp serve` host setup, backing modes, first-use smoke, and troubleshooting | [MCP host setup](mcp.md) |
| `workstations` | Workstation kinds, routes, runtime fields, and scoped execution settings | [Workstations](workstations.md) |
| `workers` | Worker quick reference | [Workers](workers.md) |
| `providers` | ACP presets, custom integrations, Factory selection, validation, removal, and JavaScript usage | [Providers and ACP agents](providers.md) |
| `resources` | Bounded-concurrency quick reference | [Resources](resources.md) and [Config](config.md) |
| `models` | Model discovery, readiness, pull, invocation, and Factory execution boundaries | [Models](models.md) |
| `batch-inputs` | Batch-request quick reference | [Batch inputs](batch-inputs.md) |
| `templates` | Template authoring guide | [Templates](templates.md) |

`batch-work` remains accepted by the installed CLI as a compatibility alias for
the canonical `batch-inputs` topic.
`workstation` remains accepted as a compatibility alias for the canonical
`workstations` topic.
`acp` is accepted as a short alias for the canonical `providers` topic.

## CLI Output And Diagnostics

Default command output is the customer-facing command result. Commands that emit
JSON must keep parseable JSON on stdout, and any troubleshooting diagnostics
from `--verbose` or `--debug` must use stderr unless an existing command-owned
diagnostics stream is explicitly documented.

`--verbose` is for concise operational context that helps diagnose a command
without changing its result. Verbose diagnostics may include paths, endpoint
URLs or paths, request or trace IDs, status codes, counts, durations, byte
sizes, output paths, and selected option summaries. `--debug` is for
lower-level development diagnostics where a command supports that mode, and
debug mode implies verbose command diagnostics.

Verbose and debug diagnostics must not include full prompts, full work payloads,
access tokens, full model input text, full successful response bodies,
sensitive generated content, or full command stdout or stderr unless an
existing explicit failure policy already permits a bounded preview.

CLI verbose diagnostics are separate from service runtime logs and runtime
metrics controlled by `you run --runtime-log-*` and
`you run --runtime-metrics-*`. Runtime logs are structured service-owned
diagnostic logs. Runtime metrics are a separate structured operational JSONL
channel. Command diagnostics explain the CLI invocation and transport or
filesystem work around that invocation.

## Canonical Concept Owners

- [Config](config.md) owns work types, work states, top-level `factory.json`,
  routing behavior, runtime resources, and portability fields.
- [Submitted work](work.md) owns `POST /factory-sessions/{session_id}/work`,
  submitted-work tags, and batch cross-links.
- [Sessions](sessions.md) owns live session discovery, session show, pause and
  resume, factory query, status API fields, dashboard URL, and `--server` /
  `--session` routing for HTTP client commands.
- [Orchestrators](orchestrators.md) owns `Factory`, `FactoryOrchestrator`,
  `FactorySession`, `Dispatch`, `FactoryArtifact`, `FactoryEvent`, and accepted
  dynamic workflow aliases.
- [JavaScript workflows](javascript-workflows.md) owns the supported JavaScript
  authoring surface, equivalent execution and inspection flows, child worker
  preset rules, host-capability boundary, and executable examples.
- [MCP host setup](mcp.md) owns the canonical `you mcp serve` host
  configuration, backing modes, first-use smoke, and troubleshooting.
- [Workstations](workstations.md) owns workstation kinds, route fields, runtime
  step behavior, prompt/runtime fields, and workstation-scoped execution
  settings.
- [Workers](workers.md) owns worker types, worker-scoped runtime fields,
  model/script/hosted backend fields, explicit `AGENT_WORKER` tool policy,
  agent-run failure classes, hosted `auth.secretRef` guidance, and split
  `workers/<name>/AGENTS.md` placement.
- [Providers and ACP agents](providers.md) owns ACP installation, built-in
  presets, operator-added integrations, `executorProvider` selection, and
  provider lifecycle commands.
- [Workstations](workstations.md) owns `AGENT_RUN` versus `INFERENCE_RUN`
  runtime behavior in addition to workstation kinds, route fields, and
  workstation-scoped execution settings.
- [Sessions](sessions.md) owns agent-run dispatch inspection in addition to
  live session discovery, status API fields, and dashboard routing.

Use these canonical concept owners when you need the current contract.

## Customer Guide Structure

- [Config](config.md) explains the canonical split factory layout around
  `factory.json`, `workers/`, `workstations/`, and `inputs/`.
- [Resources](resources.md) explains top-level resource pools and the
  `{name, capacity}` requirements consumed by workers or workstations.
- [Models](models.md) explains model discovery, readiness, pull, direct
  invocation, and the `INFERENCE_WORKER`/`INFERENCE_RUN` Factory boundary.
- [Batch inputs](batch-inputs.md) explains the `FACTORY_REQUEST_BATCH` request
  shape, watched-file placement, and supported relation types.
- [Templates](templates.md) explains the supported Go-template surfaces, the
  full variable inventory, and the JSON-versus-Markdown quoting rules.
- [Mock workers](mock-workers.md) owns `--with-mock-workers`, the
  `mockWorkers` JSON contract, selection fields, and `runType` outcomes.
- [Record and replay](record-replay.md) owns default recording, generated
  artifact paths, `--record`, `--replay`, `--no-record`, and incompatible flag
  combinations.
- [Author factories](authoring-factories.md) keeps factory sequencing, quick-start
  run commands, reusable [`docs/examples/`](../examples/README.md) inputs, and
  links to the dedicated mock-worker and record/replay guides.
- [Author AGENTS.md](authoring-agents-md.md) keeps split-file shape, prompt
  placement, and prompt-authoring examples.

## Related

- [Package docs index](../README.md)
- [Config](config.md)
- [Sessions](sessions.md)
- [Submitted work](work.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Providers and ACP agents](providers.md)
- [Resources](resources.md)
- [Models](models.md)
- [Author AGENTS.md](authoring-agents-md.md)
- [Batch inputs](batch-inputs.md)
- [Templates](templates.md)
