---
author: Agent Factory Team
last-modified: 2026-05-21
doc-id: agent-factory/reference/workstation
---

# Workstation

`you docs workstation` stays available as the stable packaged workstation quick
reference. Use
[`docs/reference/workstations.md`](../../../docs/reference/workstations.md) for
the maintained workstation guide.

Workstations are the dispatch steps in `factory.json`. A workstation consumes
input places, optionally dispatches to a worker, and routes the result to its
configured output, continue, rejection, or failure place. Classifier
workstations route accepted success through authored `classificationRoutes`
instead of normal success `outputs`.

## Minimal Workstation

```json
{
  "name": "review-story",
  "worker": "reviewer",
  "inputs": [{ "workType": "story", "state": "in-review" }],
  "outputs": [{ "workType": "story", "state": "complete" }],
  "onRejection": [{ "workType": "story", "state": "init" }],
  "onFailure": [{ "workType": "story", "state": "failed" }]
}
```

## Topology Fields

| Field | Description |
|-------|-------------|
| `name` | Stable workstation and transition name. |
| `behavior` | Scheduling behavior: `STANDARD`, `REPEATER`, `CRON`, or `POLLER`. |
| `type` | Runtime implementation: `MODEL_WORKSTATION`, `CLASSIFIER_WORKSTATION`, or `LOGICAL_MOVE`. |
| `worker` | Worker name to dispatch when the workstation executes. |
| `inputs` | Places that must be present before the workstation can fire. |
| `outputs` | Places produced on accepted completion. |
| `classificationRoutes` | Labeled success routes for `CLASSIFIER_WORKSTATION`; each label maps to one or more destination outputs. |
| `onContinue` | Places produced on ordinary partial-progress completion. |
| `onRejection` | Places produced on rejected completion. |
| `onFailure` | Places produced on failure or timeout. |
| `resources` | Resource capacity held while the dispatch is in flight. |
| `guards` | Workstation-level `VISIT_COUNT` guards. |
| `cron` | Schedule configuration for `behavior: "CRON"`. |

## Runtime Fields

These can live inline in `factory.json` or in
`workstations/<name>/AGENTS.md`:

| Field | Description |
|-------|-------------|
| `type` | Runtime implementation: `MODEL_WORKSTATION`, `CLASSIFIER_WORKSTATION`, or `LOGICAL_MOVE`. |
| `runner` | Stable runner override for this workstation: `codex`, `gemini`, `kiro`, `cursor-cli`, or `opencode`. |
| `promptFile` | Prompt template file relative to the workstation directory. |
| `promptTemplate` | Inline prompt template string. |
| `limits.maxExecutionTime` | Per-dispatch timeout. |
| `limits.maxRetries` | Retry budget before the circuit breaker treats the work as failed. |
| `stopWords` | Ordered markers used for accept-or-fail output handling. |
| `workingDirectory` | Rendered execution working directory. |
| `worktree` | Rendered worktree path passed to supported executors. |
| `env` | Rendered environment variables passed into execution. |

## Scheduling Kinds

- `STANDARD` fires once when its inputs are ready.
- `REPEATER` fires again after continue results and is the normal fit for
  iterative agent loops that keep rejection reserved for true send-back or
  negative outcomes.
- `CRON` submits internal time work on a schedule while the runtime stays in
  service mode.

Use a guarded `LOGICAL_MOVE` workstation to cap repeater or review loops.

## Classifier Routing

Use `CLASSIFIER_WORKSTATION` when accepted success is "return exactly one plain
string label and route to that authored branch only." This is the right fit
for flows like `approved`, `needs_changes`, or `spam`.

- `classificationRoutes` is required for classifiers and each route needs one
  unique non-empty `label` plus one or more destination `outputs`. Labels must
  be authored as plain text, not JSON literal text such as `"approved"`,
  `123`, `true`, `null`, `{...}`, or `[...]`, and must not include surrounding
  whitespace.
- The runtime trims surrounding whitespace before matching, then applies exact
  case-sensitive label matching.
- Classifier workstations must not also declare normal success `outputs`,
  `onContinue`, or `onRejection`.
- Empty labels, unknown labels, non-string outputs, parse failures, execution
  errors, and timeouts all fail through the ordinary `FAILED` path and use
  `onFailure` when it is configured.

Runner precedence is explicit: workstation `runner`, then factory `runner`,
then legacy worker `modelProvider`, then the default `codex` runner. Built-in
runner selection expects the matching local CLI and auth/setup to already be
present before execution starts.

## Related

- `you docs config`
- `you docs workers`
- `you docs resources`
- `you docs templates`
