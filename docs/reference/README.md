# Agent Factory CLI Reference

This directory is the package-owned reference surface for the customer docs and
the future `you docs <topic>` command. Use the fixed CLI topic names for quick
terminal help, then use the canonical concept owners below when you need the
complete customer-facing contract.

## Packaged CLI Topics

`you docs <topic>` accepts these topics:

| Topic | Packaged scope | Canonical or broader customer guide |
|-------|----------------|--------------------------------------|
| `config` | Split layout and `factory.json` placement | [Factory JSON and work configuration](work.md) |
| `workstation` | Workstation quick reference | [Workstations](workstations.md) |
| `workers` | Worker quick reference | [Workers](workers.md) |
| `resources` | Bounded-concurrency quick reference | [Resources](resources.md) and [Factory JSON and work configuration](work.md) |
| `models` | Model discovery, invocation, and contract quick reference | [Models and model operations](models.md) |
| `batch-work` | Batch-request quick reference | [Batch inputs](batch-inputs.md) |
| `templates` | Template authoring guide | [Templates](templates.md) |

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
  examples, and run commands.
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
