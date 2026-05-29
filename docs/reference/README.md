# you-agent-factory CLI Reference

This directory is the package-owned reference surface for the customer docs and
the future `you docs <topic>` command. Use the fixed CLI topic names for quick
terminal help, then use the canonical concept owners below when you need the
complete customer-facing contract.

## Packaged CLI Topics

`you docs <topic>` accepts these topics:

| Topic | Packaged scope | Canonical or broader customer guide |
|-------|----------------|--------------------------------------|
| `authoring-factories` | Practical factory authoring workflow, runnable examples, mock workers, and replay | [Author factories](authoring-factories.md) |
| `config` | Split layout and `factory.json` placement | [Factory JSON and work configuration](work.md) and [Author factories](authoring-factories.md) |
| `work` | Work types, states, routing, resources, and portability fields | [Factory JSON and work configuration](work.md) |
| `workstations` | Workstation kinds, routes, runtime fields, and scoped execution settings | [Workstations](workstations.md) |
| `workers` | Worker quick reference | [Workers](workers.md) |
| `resources` | Bounded-concurrency quick reference | [Resources](resources.md) and [Factory JSON and work configuration](work.md) |
| `models` | Model discovery, invocation, and contract quick reference | [Models and model operations](models.md) |
| `batch-inputs` | Batch-request quick reference | [Batch inputs](batch-inputs.md) |
| `templates` | Template authoring guide | [Templates](templates.md) |

`batch-work` remains accepted by the installed CLI as a compatibility alias for
the canonical `batch-inputs` topic.
`workstation` remains accepted as a compatibility alias for the canonical
`workstations` topic.

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

CLI verbose diagnostics are separate from service runtime logs controlled by
`you run --runtime-log-*`. Runtime logs are structured service-owned logs;
command diagnostics explain the CLI invocation and transport or filesystem work
around that invocation.

## Canonical Concept Owners

- [Factory JSON and work configuration](work.md) owns work types, work states,
  top-level `factory.json`, routing behavior, runtime resources, and
  portability fields.
- [Workstations](workstations.md) owns workstation kinds, route fields, runtime
  step behavior, prompt/runtime fields, and workstation-scoped execution
  settings.
- [Workers](workers.md) owns worker types, worker-scoped runtime fields,
  model/script backend fields, and split `workers/<name>/AGENTS.md` placement.

Use these canonical concept owners when you need the current contract.

## Customer Guide Structure

- [Config](config.md) explains the canonical split factory layout around
  `factory.json`, `workers/`, `workstations/`, and `inputs/`.
- [Resources](resources.md) explains top-level resource pools and the
  `{name, capacity}` requirements consumed by workers or workstations.
- [Models and model operations](models.md) explains `MODEL_INVOKE`,
  `MODEL_WORKER` capabilities, typed model resources, `/models`, and local or
  cloud TTS authoring patterns.
- [Batch inputs](batch-inputs.md) explains the `FACTORY_REQUEST_BATCH` request
  shape, watched-file placement, and supported relation types.
- [Templates](templates.md) explains the supported Go-template surfaces, the
  full variable inventory, and the JSON-versus-Markdown quoting rules.
- [Author factories](authoring-factories.md) keeps factory sequencing,
  mock-worker checks, replay recording guidance, reusable
  [`docs/examples/`](../examples/README.md) inputs, and run commands.
- [Author AGENTS.md](authoring-agents-md.md) keeps split-file shape, prompt
  placement, and prompt-authoring examples.

## Related

- [Package docs index](../README.md)
- [Factory JSON and work configuration](work.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Resources](resources.md)
- [Models and model operations](models.md)
- [Author AGENTS.md](authoring-agents-md.md)
- [Batch inputs](batch-inputs.md)
- [Templates](templates.md)
