# Packaged Goal (`@you/goal`)

Use this guide when you want the first-party packaged goal factory: invoke it by
name from the terminal, read the primary result on stdout, inspect the
materialized factory on disk, and customize it like any other named factory.

`you docs authoring-factories` owns the broader factory-authoring workflow.
`you docs config` and `you docs sessions` own the shared invocation input and
return-policy contract. This guide focuses on the `@you/goal` packaged factory
workflow.

The default `@you/goal` factory is a goal-oriented example with planner,
executor, checker, and reviewer workstations. Use it as a runnable starting point
before authoring a custom factory from scratch.

## Quick start

Run the packaged goal factory by its canonical name:

```bash
you run --named @you/goal
```

Pass one-shot invocation text when you want the factory to handle a single
prompt through the shared `handlingBehavior: ["DEFAULT"]` contract:

```bash
you run --named @you/goal "Ship the login bugfix"
```

Pipe stdin when no positional text is present:

```bash
echo "Ship the login bugfix" | you run --named @you/goal
```

Supplying both positional text and piped stdin for the same invocation is
rejected with `INVOCATION_INPUT_SOURCE_CONFLICT` before runtime work starts.

## Batch success without browser or dashboard interaction

Normal `you run --named @you/goal` uses the default batch run mode (no
`--continuously`). For the standard success path you do not need to open the
operator dashboard or interact with a browser session:

- The CLI submits the goal through the real named packaged-factory path.
- The run completes when the submitted work reaches terminal success or the
  factory goes idle.
- The process exits after that terminal completion or idle instead of staying
  open for later operator submissions.
- Successful stdout carries the configured `primaryResult` from the shared
  invocation-return contract.

Use `--quiet` for scripted or CI-oriented runs when you want to suppress
dashboard startup output. That flag affects operator chatter only; it does not
change invocation input resolution or primary-result selection.

Use `--output response-stream` on supported one-shot `@you/goal` invocations when
the CLI owns the live runtime and you want live progress fragments instead of
waiting silently for the final `primaryResult`. This mode streams progress from
the local runtime session; it does not tail provider-native stdout. Unsupported
run shapes such as `--continuously`, replay mode, or non-invocation `you run`
paths return `INVOCATION_OUTPUT_UNSUPPORTED`. When the runtime path cannot attach
to a live progress stream, the CLI falls back to primary-result-only stdout.

This guide documents the supported headless **operator-interaction** claim for
the normal batch success path. It does **not** promise that batch invocation
avoids binding a localhost API listener. Listener behavior belongs to the shared
`you run` service startup contract and may differ across run modes; see
`you docs sessions` for operator-oriented modes that keep a service alive for
later submissions.

## Default invocation result

Successful `@you/goal` invocations print the packaged factory's primary result
on stdout using the existing invocation-result contract. The built-in factory
sets an explicit `invocationReturn` policy that selects terminal `goal:complete`
work content as `primaryResult`.

On the CLI, that successful `primaryResult` is written directly to stdout. You
do not need to reconstruct the answer from logs, event history, or dashboard
state.

The equivalent API path is `POST /factory-sessions/{session_id}/invocations`
with the same text input and return-policy semantics. Transport code adapts its
carrier; it does not invent separate primary-output rules.

## What to expect after you run it

`you run --named @you/goal` resolves the named factory, materializes it on first
use if needed, then starts a normal factory session using the on-disk factory
layout.

Customers should expect:

- The CLI selects exactly one named-factory directory and loads the split
  `factory.json`, `workers/`, and `workstations/` layout from that directory.
- The default factory runs the `goal` work type through the multi-stage flow
  described below, ending at terminal `goal:complete` on success or at a
  non-success goal state when routing or workstation failure stops progress.
- Later invocations reuse the materialized on-disk copy instead of overwriting
  customer edits with pristine embedded content.
- Legacy materialized copies that still use broken top-level `{{ .WorkID }}`
  prompt templates are upgraded automatically on the next resolution when the
  current built-in catalog provides canonical `PromptData` templates.

## Goal flow topology

The packaged `@you/goal` factory models one `goal` work type that moves through
planning, execution, checking, review, and completion stages. Each stage is a
normal workstation transition on submitted work; there is no goal-specific public
route or endpoint for any stage.

| Stage | Customer-facing role | Typical work state | Workstation |
|-------|----------------------|--------------------|-------------|
| Planning | Turn submitted goal text into an executable plan | `goal:init` → `goal:plan` | `plan-goal` |
| Execution | Implement the plan | `goal:plan` → `goal:execute` | `execute-goal` |
| Checking | Verify execution output and choose review mode | `goal:execute` → review path | `check-goal` |
| Review | Evaluate results and decide whether to finish or loop | `goal:review` or `goal:structured-review` | `review-goal` or `structured-review-goal` |
| Completion | Terminal success for the invocation | `goal:complete` | — |

After checking, the factory routes to either plain review (`review-goal`) or
structured review (`structured-review-goal`). Accepted review outcomes advance
work to `goal:complete`. Review can also loop back to planning when changes or
test failures require another pass.

Non-success goal states are part of the same topology rather than separate
public routes:

- `goal:blocked` and `goal:needs-human` — authored routed states when review
  decides the goal cannot proceed without operator action.
- `goal:interrupted` — routed when review or dispatch interruption metadata
  indicates an interrupted stop.
- `goal:failed` — workstation failure, guard exhaustion, or another terminal
  failure path before `goal:complete`.

When a batch invocation stops before `goal:complete`, use the shared recovery
codes and inspect-first flow in the sections below. Those surfaces explain
whether the stop came from a routed goal state, a paused session, or an
interrupted dispatch.

## Non-success outcomes and recovery

`@you/goal` uses the shared invocation path that powers both
`you run --named @you/goal` and `POST /factory-sessions/{session_id}/invocations`.
When no primary result exists yet, the returned non-success code tells you why
the goal stopped:

| Outcome | Stable code | Meaning | What to do next |
|---------|-------------|---------|-----------------|
| Blocked authored goal state | `INVOCATION_BLOCKED` | Routed goal work reached `goal:blocked` before a complete primary result existed. | Inspect the session and blocked work, then unblock it with the existing session/work surfaces. |
| Human input required authored goal state | `INVOCATION_NEEDS_HUMAN` | Routed goal work reached `goal:needs-human` before a complete primary result existed. | Inspect the session and relevant work, then provide the needed operator input through the existing workflow. |
| Session paused | `INVOCATION_PAUSED` | Waiting stopped because the live Factory Session was paused. | Resume the session with `you session resume <session-id>` and then re-check progress. |
| Dispatch or session interrupted | `INVOCATION_INTERRUPTED` | Existing interruption metadata explains the stop better than a generic failure. | Inspect the session and dispatch context to decide whether to resume, retry, or rerun. |
| Runtime failure | `INVOCATION_RUNTIME_FAILURE` | The invocation scope failed before producing the configured primary result. | Inspect failed work and session status to find the failing step. |
| Wait deadline expired | `INVOCATION_TIMED_OUT` | The invocation was still running when the wait window expired. | Check session status and work progress, then wait longer or rerun with a different operator workflow. |
| No primary result resolved | `INVOCATION_PRIMARY_RESULT_UNRESOLVED` | Work settled, but the configured `invocationReturn` target never produced a primary result. | Inspect the session/work state and the authored `invocationReturn` contract. |

Blocked and needs-human are authored routed goal states. Paused and interrupted
come from shared session lifecycle or dispatch interruption context. They are
reported distinctly so operators do not need a goal-specific endpoint or raw
provider payload inspection to understand what happened.

## Operator controls during active execution

`@you/goal` reuses the same public `FactorySession`, `Work`, and `Dispatch`
controls as other live factories. There are no goal-specific pause, resume, or
interrupt routes.

During an open live session (for example one started by `you run --named
@you/goal` or `you run --continuously`):

- **Pause** with `you session pause [session-id]` or
  `POST /factory-sessions/{session_id}/pause` to stop automatic progression
  while the session keeps accepting inbound work and worker results.
- **Resume** with `you session resume [session-id]` or
  `POST /factory-sessions/{session_id}/resume` to wake execution and drain
  buffered submissions and completed worker results in submission order.
- **Interrupt** an in-flight dispatch through the existing dispatch
  interruption surfaces; interrupted goals route to `goal:interrupted` and
  surface `INVOCATION_INTERRUPTED` when an invocation waits on the interrupted
  work.

Paused sessions report `INVOCATION_PAUSED` when a batch invocation stops
waiting. `SESSION_LIFECYCLE_CONTROL` events record pause and resume for replay
and inspection.

Use `you session show`, `you work show`, and `GET
/factory-sessions/{session_id}/events` to inspect buffered work, interruption
context, and lifecycle history. See `you docs sessions` for the full pause,
resume, buffering, and replay semantics.

The non-success response also includes shared recovery context such as
`sessionId`, `workId`, `workName`, and `workState` when one work item explains
the outcome. Use that context with the existing commands:

```bash
you session show <session-id>
you work show <work-id> --session <session-id>
you work list --session <session-id>
you session resume <session-id>
```

## Inspect-first recovery flow

Use the same inspect-first sequence for every stopped `@you/goal` run. The
public nouns stay the same across CLI and API: `FactorySession`, `Work`, and
`Dispatch`.

1. Start from the shared recovery context on the non-success invocation
   response: `sessionId`, `workId`, `workName`, and `workState` when present.
2. Inspect the live `FactorySession` with `you session show <session-id>` or
   `GET /factory-sessions/{session_id}` to confirm whether automation is paused,
   blocked on work state, needs human input, or interrupted during a dispatch.
3. Inspect the affected `Work` with `you work show <work-id> --session <session-id>`
   or `GET /factory-sessions/{session_id}/work/{work_id}` to read the stop
   summary, latest dispatch or result summary, and suggested recovery surface.
4. Apply the existing session or work control that matches that stop reason.
   Do not look for `@you/goal`-specific resume or inspect routes.
5. Re-run `you session show` and `you work show` to confirm the same
   `FactorySession` and `Work` moved forward through the normal goal flow.

### Recovery by stop reason

| Stop reason | What inspect should tell you | Existing control to use next |
|-------------|------------------------------|------------------------------|
| Paused `FactorySession` | The session lifecycle is paused while buffered work stays attached to the same session. | Resume with `you session resume <session-id>`, then re-check the same session and work. |
| Blocked `Work` state such as `goal:blocked` | The blocked work item, its current state, and the latest relevant dispatch or result summary. | Use existing work repair, work move, or follow-up submission controls for that work item. |
| Needs-human `Work` state such as `goal:needs-human` | The work item that needs operator input, approval, or artifact review before progress can continue. | Provide the required human input or approval through the existing workflow, then re-inspect the same work item. |
| Interrupted `Dispatch` or session | The interrupted dispatch/result summary and the affected session/work context. | Use existing dispatch retry, work repair, or session workflow controls after inspecting the interruption context. |

This flow intentionally reuses the shared session and work inspection surfaces.
`@you/goal` does not add `/goal/inspect`, `/goal/resume`, or another goal-only
public recovery route.

For broader session discovery, status inspection, and work submission after the
factory is running, use `you docs sessions`, `you docs work`, and
`you docs batch-inputs`.

## Where the factory materializes

`you run --named @you/goal` resolves named factories in this order:

1. Project-local `./factory`
2. Global shared root `~/.you-agent-factory/you-agent-factories`
3. Built-in catalog materialization on first use

On first invocation, `@you/goal` materializes into the global root using the
normal named-factory persist pipeline. Scoped names are URL-encoded on disk:

```text
~/.you-agent-factory/you-agent-factories/@you%2Fgoal/
```

Inspect the materialized factory:

```bash
you factory list --dir ~/.you-agent-factory/you-agent-factories
```

The materialized directory uses the standard split factory layout customers edit
after first use:

```text
factory.json
workers/
  goal-planner/AGENTS.md
  goal-executor/AGENTS.md
  goal-checker/AGENTS.md
  goal-reviewer/AGENTS.md
workstations/
  plan-goal/AGENTS.md
  execute-goal/AGENTS.md
  check-goal/AGENTS.md
  review-goal/AGENTS.md
  structured-review-goal/AGENTS.md
```

On first materialization, the CLI expands the built-in catalog into that layout:
`factory.json` carries topology, routing, and `invocationReturn`, while worker
and workstation prompt bodies land in the split `AGENTS.md` files. Later
`you run --named @you/goal` invocations load this on-disk copy as-is, so edits
to prompts or factory settings persist without reinstalling the packaged factory.

## Customer edits affect the next run

Packaged factories stay editable after materialization. The CLI reuses the
on-disk directory on later invocations instead of overwriting customer changes
with pristine embedded content.

Edit distinguishing fields such as:

- `workers/goal-executor/AGENTS.md` prompt body
- `workstations/execute-goal/AGENTS.md` workstation prompt
- `factory.json` worker model or resource settings

The next `you run --named @you/goal` invocation loads the edited on-disk factory
immediately. No reinstall, cache clear, or special reload step is required.

If the materialized factory becomes invalid or incomplete, invocation fails
with a clear packaged-factory load error instead of silently falling back to
embedded behavior.

### Maintainer verification

After editing this reference topic, run `make docs-reference-smoke` from the
repository root.

## Related topics

- `you docs authoring-factories` — named-factory resolution, factory layout,
  and when to start from a packaged example instead of authoring from scratch
- `you docs config` — invocation input sources and `invocationReturn` policy
- `you docs sessions` — session-scoped invocation API, dashboard URL, and run modes
- `you docs workstations` — workstation kinds and authoring fields
- `you docs workers` — worker kinds and authoring fields
- `you docs mock-workers` — deterministic local testing with `--with-mock-workers`
