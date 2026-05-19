# Agent Factory CLI Reference

This directory is the package-owned reference surface for a future
`you docs <topic>` command. Each page stays short, focuses on the
current supported contract, and links to the deeper package guide for the same
topic.

## Canonical Concept Owners

- [Factory JSON and work configuration](work.md) owns work types, work states,
  top-level `factory.json`, routing behavior, runtime resources, and
  portability fields.
- [Workstations](workstations.md) owns workstation kinds, route fields, runtime
  step behavior, prompt/runtime fields, and workstation-scoped execution
  settings.
- [Workers](workers.md) owns worker types, worker-scoped runtime fields,
  model/script backend fields, and split `workers/<name>/AGENTS.md` placement.

Use overview and workflow pages for sequencing and examples. Use these
canonical concept owners when you need the current contract.

## Topics

- [Config](config.md) explains the canonical split factory layout around
  `factory.json`, `workers/`, `workstations/`, and `inputs/`.
- [Workstations](workstations.md) explains workstation kinds, route fields, and
  the worker-binding contract for runtime steps.
- [Workers](workers.md) explains worker types, worker-owned runtime fields, and
  where worker `AGENTS.md` files fit in the split layout.
- [Resources](resources.md) explains top-level resource pools and the
  `{name, capacity}` requirements consumed by workers or workstations.
- [Batch work](batch-work.md) explains the `FACTORY_REQUEST_BATCH` request
  shape, watched-file placement, and supported relation types.
- [Templates](templates.md) explains the supported Go-template surfaces and the
  JSON-versus-Markdown quoting rules.

## Related

- [Package docs index](../README.md)
- [Factory JSON and work configuration](work.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Workstations and workers](workstations-and-workers.md)
- [Author AGENTS.md](authoring-agents-md.md)
- [Batch inputs](batch-inputs.md)
- [Prompt variables](prompt-variables.md)
