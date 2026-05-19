---
author: Agent Factory Team
last-modified: 2026-04-21
doc-id: agent-factory/workstations
---

# Workstations And Workers

Workstations are dispatch steps in `factory.json`. Workers are runtime
executors that those steps invoke. Use this page when you need to understand
how the two concepts cooperate in a workflow; use the canonical guides when
you need exact fields, validation details, or runtime behavior.

Canonical owners:

- [Workstations](workstations.md) owns workstation kinds, route fields, runtime
  step behavior, prompt fields, guards, cron, and workstation-scoped execution
  settings.
- [Workers](workers.md) owns worker types, worker-scoped runtime fields,
  model/script backend fields, and `workers/<name>/AGENTS.md` placement.
- [Factory JSON And Work Configuration](work.md) owns work types, work states,
  top-level `factory.json`, routing behavior, resources, and portability
  context.

## Recommended Layout

Keep topology in `factory.json`, worker system instructions in
`workers/<name>/AGENTS.md`, and workstation prompts in
`workstations/<name>/AGENTS.md`:

```text
factory/
  factory.json
  workers/
    executor/AGENTS.md
    reviewer/AGENTS.md
  workstations/
    execute-story/AGENTS.md
    review-story/AGENTS.md
  inputs/story/default/
```

Inline runtime fields are also supported in `factory.json` for single-file or
recorded configs. When a config embeds runtime definitions inline, keep the
bundle complete: every referenced worker and workstation must either have
inline runtime fields or a matching split `AGENTS.md` file on disk.

## How They Compose

`factory.json` declares the workflow topology. Each model or script
workstation names a worker through `worker`, consumes one or more input places,
and routes outcomes through `outputs`, `onContinue`, `onRejection`, or
`onFailure`.

The bound worker supplies the execution backend and shared system
instructions. The workstation supplies the step-specific prompt template,
execution limits, output schema, working directory, worktree path, environment,
and routing.

Use the split this way:

| Question | Go to |
|----------|-------|
| Which states, routes, and resources exist in `factory.json`? | [Factory JSON And Work Configuration](work.md) |
| When does a step run, where does it route, and what prompt does it render? | [Workstations](workstations.md) |
| Which model or script backend executes the step? | [Workers](workers.md) |
| Which template variables are available in prompts or script args? | [Templates](templates.md) |
| How do parent-aware joins or loop-breakers work? | [Workstation Guards And Guarded Loop Breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md) and [Parent-Aware Fan-In](../internal/development/parent-aware-fan-in.md) |

## Minimal Workflow Shape

This abbreviated topology shows the relationship without restating each
contract field:

```json
{
  "workTypes": [
    {
      "name": "story",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "in-review", "type": "PROCESSING" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "executor" },
    { "name": "reviewer" }
  ],
  "workstations": [
    {
      "name": "execute-story",
      "behavior": "REPEATER",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "in-review" }],
      "onFailure": { "workType": "story", "state": "failed" }
    },
    {
      "name": "review-story",
      "worker": "reviewer",
      "inputs": [{ "workType": "story", "state": "in-review" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onRejection": { "workType": "story", "state": "init" },
      "onFailure": { "workType": "story", "state": "failed" }
    }
  ]
}
```

With a split layout, `workers/executor/AGENTS.md` owns the executor backend and
system prompt, while `workstations/execute-story/AGENTS.md` owns the
step-specific prompt and execution settings. Use [Workers](workers.md) and
[Workstations](workstations.md) for those exact frontmatter fields.

## Authoring Checklist

- Declare work types, states, resources, workers, and workstations in
  `factory.json`; use [Factory JSON And Work Configuration](work.md) for the
  canonical top-level contract.
- Put execution backend details in worker definitions; use [Workers](workers.md)
  for worker types, provider fields, script fields, and worker placement.
- Put step routing, prompt runtime fields, cron, guards, and loop breakers in
  workstation definitions; use [Workstations](workstations.md) for the
  canonical workstation contract.
- Keep workflow examples focused on sequencing. Link to the owner guide instead
  of copying field-by-field rules into overview pages.
- Avoid retired fields such as `runtime_type`, `cron.interval`, `join`, and
  `worktree_cleanup`; the owner guides list the current supported fields.

## Related

- [Factory JSON And Work Configuration](work.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Batch Inputs](batch-inputs.md)
- [Templates](templates.md)
- [Author AGENTS.md](authoring-agents-md.md)
- [Workstation Guards And Guarded Loop Breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md)
- [Parent-Aware Fan-In](../internal/development/parent-aware-fan-in.md)
