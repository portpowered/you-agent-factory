---
author: Agent Factory Team
last-modified: 2026-08-12
doc-id: agent-factory/guides/operations
---

# Operations

This is the canonical runbook for keeping a real pipeline alive, recognizing
an incomplete run, and recovering after the process that owns a Factory
Session stops. Use it when Work can arrive after startup or when an operator
must distinguish an idle queue from completed Work.

For the complete command and output-mode reference, use `you docs run`. For
the submitted-Work event schema, use `you docs work`; for worker and
workstation prompt fields, use `you docs workers` and `you docs workstations`.
This page owns the operational lifetime and restart boundary; it does not add
a storage engine or change runtime behavior.

## Use a continuous server for real pipelines

For a real pipeline that accepts or retains Work across idle periods, use the
server-enabled continuous shape:

```bash
you run --dir ./factory --with-server --continuously
you run --dir ./factory --with-server --continuously --listen 127.0.0.1:7437
```

`--with-server` keeps the Factory Session API available for submitters and
inspection commands. `--continuously` keeps the runtime alive while the queue
is idle. The process ends when it is cancelled normally, so an idle period is
not presented as a completed pipeline. Add `--with-site` when the same run
should also serve the dashboard; it follows the same lifetime and finite-drain
policy described below. Use `--listen <host:port>` for an exact loopback bind;
an explicit local `--server` is retained only for legacy scripts and emits a
deprecation warning.

The continuous shape is the production-oriented default for long-running
pipelines. Submit initial or later Work through the existing submitted-Work
surfaces described by `you docs work`; use `you docs sessions` to confirm that
the addressed Factory Session is still live.

## Read the queue state correctly

These states answer different questions:

- **Terminal Work** has reached a `TERMINAL` or `FAILED` state. It is finished
  for runtime scheduling purposes.
- **Idle or drained** means the scheduler currently has no dispatch to start.
  It does not, by itself, say that every Work item is terminal.
- **Non-terminal Work** is still in progress from the customer's point of
  view. A `PROCESSING` state can be held by an active dispatch, or it can be
  stranded because no next transition is enabled.

Do not use an empty dispatch queue, a quiet log, or a process exit as a
completion claim without checking the Work state. Continuous mode intentionally
does not apply finite-drain classification while it is idle.

## Finite runs and the A7 drained-incomplete diagnostic

Finite runs are useful for a bounded batch. A finite server-enabled run returns
success when no Work was admitted or every admitted Work item is terminal. If
the runtime truly drains while customer Work remains non-terminal, the current
binary returns exit status `1`, joins its owned runtime and listener, and writes
this diagnostic to stderr:

```text
Error: factory session drained with N non-terminal work items; run is incomplete
```

There is no success or completion claim on stdout for that failure. The same
finite classification applies to `--with-site`; the site adds the dashboard
but does not turn non-terminal Work into success. This is the current A7
drained-incomplete diagnostic, not a provider or storage failure.

The checked-in `operations-stranded-work.json` input deliberately places one
`task` Work item in `in-review` without the matching review input. It is a
diagnostic example, not a production batch:

```bash
you run --dir ./factory --with-server --work ./docs/examples/operations-stranded-work.json
```

For that input, the observed stderr and exit status are:

```text
Error: factory session drained with 1 non-terminal work items; run is incomplete
exit status: 1
```

An empty finite server-enabled run was also exercised with:

```bash
you run --dir ./factory --with-server
```

It initialized the Factory Session and listener and exited with status `0`.
The normal stdout includes the runtime log, metrics, dashboard URL, and
recording paths; those paths are environment-specific and are not completion
proof by themselves.

Plain batch mode has no API listener for later submit or inspection. If an
active provider dispatch keeps Work in `PROCESSING`, the command can continue
waiting for that dispatch. If no dispatch remains and no transition is
enabled, the current binary classifies the drained non-terminal Work and
returns the same exit-1 diagnostic rather than reporting success. Use the
continuous server shape when an operator may need to submit recovery Work
while the process remains alive.

## Bound retry loops with an explicit route

`VISIT_COUNT` is an inclusive threshold. A guard with `"maxVisits": 3` becomes
eligible when the watched workstation's visit count is greater than or equal to
(`>=`) `3`; it is not limited to counts strictly greater than `3`.

For example, if rejected review Work returns to `story:init`, a loop breaker
watching `review-story` with `maxVisits: 3` sees this sequence:

1. Visits `1` and `2` are below the threshold, so the breaker remains disabled
   and `review-story` may receive the Work again.
2. Visit `3` satisfies the guard. When that rejection returns the Work to
   `story:init`, the breaker is eligible on the same Work history.
3. The breaker moves the Work to an explicit failed state instead of allowing
   another review pass.

Use a guarded `LOGICAL_MOVE` to make that route explicit:

```json
{
  "name": "review-loop-breaker",
  "type": "LOGICAL_MOVE",
  "inputs": [{ "workType": "story", "state": "init" }],
  "outputs": [{ "workType": "story", "state": "failed" }],
  "guards": [
    {
      "type": "VISIT_COUNT",
      "workstation": "review-story",
      "maxVisits": 3
    }
  ]
}
```

A `VISIT_COUNT` guard on the normal worker workstation only gates that
workstation; it does not create a failed or terminal destination. Pair the
guard with a `LOGICAL_MOVE` whose input is the loop's return state and whose
output is the deliberate failed or terminal state. This is the loop-breaker
pattern. Do not put a visit guard only on the worker path and assume it will
stop and classify the Work for you.

## Bound shared resources before dispatch

A top-level resource pool is a finite capacity boundary. A workstation
requirement is checked when its dispatch is eligible: the dispatch acquires
the requested number of available units, holds them while it runs, and returns
them when the dispatch ends, including failure or timeout. With a pool of
capacity `2` and a workstation requirement of `1`, at most two matching
dispatches can hold the pool at once. Work that is waiting for an enabled
transition but cannot acquire a unit remains queued; queued Work does not
reserve future capacity.

The normal shape is a named pool plus a requirement on the stage that should
be throttled:

```json
{
  "resources": [{ "name": "pipeline-slot", "capacity": 2 }],
  "workstations": [
    {
      "name": "execute-story",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "resources": [{ "name": "pipeline-slot", "capacity": 1 }]
    }
  ]
}
```

### Avoid priority inversion between stages

Sharing one small pool across stages can produce a priority inversion. A
downstream stage (or a downstream Work item from another trace) can already be
eligible and acquire the only `pipeline-slot` while upstream Work needed to
unlock another path is queued. The downstream dispatch then holds the slot,
delaying the upstream stage that the operator expected to make progress.

The runtime has deterministic internal candidate ordering, but the resource
contract does not provide an author-controlled stage priority, reservation, or
fairness guarantee that makes upstream Work win this race. Do not rely on Work
names, submission order, or an assumed scheduler priority to reserve a shared
unit for an upstream stage.

Mitigate the hazard in the factory design:

- Give stages that must make independent progress separate pools, such as
  `upstream-slot` and `downstream-slot`, and size each pool for its stage's
  concurrency budget.
- If a single external quota must remain globally bounded, reserve capacity in
  the topology instead: make upstream acquire a dedicated reservation before
  creating or advancing downstream Work, or gate downstream routing until the
  upstream stage reaches a safe state.
- Treat a zero-available pool plus queued upstream Work as a scheduling hazard,
  not proof that the upstream Work is invalid. Revisit which stages share the
  pool and which stage is allowed to hold it for the longest time.

## Watch Work and inspect Worker Sessions

Use the Work event stream as the normal live observation path. It emits state
transitions as they happen, so an operator does not need a hand-written
`sleep`/`work list` polling loop:

```bash
you --server http://localhost:7437 work watch --follow
you --server http://localhost:7437 work watch --session session-beta --follow
```

Without `--session`, the command targets the default compatibility session
(`~default`). `--session <session>` selects one live Factory Session. The
finite form ends after the observed Work cohort reaches terminal states;
`--follow` stays attached after terminal transitions until Ctrl-C or parent
cancellation. Stdout is one NDJSON Work transition per line, and diagnostics
remain on stderr. The stream observes the live session; it does not create a
durable watch cursor or replay lost session state. See `you docs work` for the
transition schema and reconnect boundary.

When a model-backed dispatch needs provider-level diagnosis, inspect its
Worker Session through its stable identity. Direct Worker Sessions do not need
a Factory Session or Work correlation:

```bash
you --server http://localhost:7437 worker-sessions list
you --server http://localhost:7437 worker-sessions list --scope direct --state COMPLETED --max-results 25
you --server http://localhost:7437 worker-sessions show --worker-session-id <worker-session-id>
you --server http://localhost:7437 worker-sessions stream --worker-session-id <worker-session-id>
you --server http://localhost:7437 worker-sessions read --worker-session-id <worker-session-id>
```

To resume a terminal direct Worker Session, continue it through the server-owned
Provider Session association. The command reserves a distinct successor and
returns its lineage after admission; use `--async` to return before terminal
output, or omit it to wait on the successor event stream:

```bash
you worker-sessions continue <source-worker-session-id> \
  --request-id <continuation-request-id> \
  --successor-worker-session-id <successor-worker-session-id> \
  --user-message "Continue the work"
you --server http://localhost:7437 worker-sessions continue <source-worker-session-id> \
  --remote --request-id <continuation-request-id> \
  --successor-worker-session-id <successor-worker-session-id> \
  --async --output json "Review the result"
```

To replace an active direct Worker Session, interrupt its admitted dispatch and
provide a distinct successor identity and replacement input. The server first
records the source as canceled, then admits the successor against the same
Provider Session association. Use `--async` to return after those admission
barriers, or omit it to wait for the successor terminal output:

```bash
you worker-sessions interrupt <source-worker-session-id> \
  --request-id <interrupt-request-id> \
  --successor-worker-session-id <successor-worker-session-id> \
  --replacement-message "Take a different approach"
you --server http://localhost:7437 worker-sessions interrupt <source-worker-session-id> \
  --remote --request-id <interrupt-request-id> \
  --successor-worker-session-id <successor-worker-session-id> \
  --async --output json "Stop and revise the plan"
```

Interrupt failures include a stable phase: `VALIDATION`,
`SOURCE_CANCELLATION`, or `SUCCESSOR_ADMISSION`. Local placement is the
default. `--remote` sends the complete request only to the configured
`--server`; it never falls back to local state.

Use the direct Worker Session controls when the same admitted session should
be paused, resumed, canceled, or terminated. Each command accepts one stable
Worker Session identity and returns a JSON or human-readable control result;
repeated terminal controls are safe no-ops. Pause returns only after the
authoritative `PAUSED` snapshot, resume uses the exact recorded Provider
Session, and terminate joins the in-flight dispatch before returning:

```bash
you worker-sessions pause <worker-session-id>
you worker-sessions resume <worker-session-id>
you worker-sessions cancel <worker-session-id> --output json
you worker-sessions terminate <worker-session-id>
you --server http://localhost:7437 worker-sessions terminate <worker-session-id> --remote
```

Local placement is the default for all four controls, and a control is always
resolved by the same stable Worker Session identity that `list`, `show`, `read`,
and `stream` report. When the local placement does not own the addressed
session, the control continues to the configured `--server`, which is where an
observed session actually runs; without a reachable server it still reports the
unknown session rather than a fabricated result. `--remote` sends the selected
action only to the configured `--server`; a transport or control failure never
falls back to local state. Outcomes are `APPLIED`, `NOOP`, `UNSUPPORTED`, or
`FAILED`, with stable error classifications for invalid identity, unknown
session, invalid state, transport failure, and an already terminal session.

Cancel and terminate do not require the session to have published a Provider
Session yet: a `RUNNING` session whose `providerSessionAvailable` is still
`false` is cancelled by its `workerSessionId` like any other.

Every ended Worker Session names why it ended. `show` reports the reason on its
`Failure` line and `list` reports it in the failure column; the API returns the
same value as `failure.kind` on the observation. The reasons that distinguish
the common endings are:

| Reason | Meaning |
| --- | --- |
| `OPERATOR_CANCELED` | A `cancel` control ended the session. |
| `OPERATOR_TERMINATED` | A `terminate` control ended the session. |
| `PROCESS_GONE` | The worker process exited before the attempt completed. |
| `TIMEOUT` | The attempt exceeded its hard execution deadline. |

A session that has not ended reports no reason. Use this instead of re-deriving
the cause from process forensics: an ended session never reports its reason as
`unavailable`.

Local placement is the default. `--remote` selects exactly the configured
`--server`; a failed remote continuation never falls back to a new local
request. The JSON response includes the source, successor, predecessor, event
topic, and observation guidance needed for later `show`, `read`, or `stream`
operations. A source must be terminal and have a valid server-recorded Provider
Session that supports continuation.

Top-level list defaults to `direct`; use `--scope factory` or `--scope all` to
include Factory-originated observations explicitly. Repeat `--state` to filter
by lifecycle state. `--max-results` bounds one page and `--next-token` resumes
from the opaque cursor returned in JSON `paginationContext`.

The existing Work-oriented entry point remains available when a Work
correlation is what you need:

```bash
you --server http://localhost:7437 worker-sessions list --work-id <work-id>
```

The Work-scoped `list` returns a stable table. It can
return `No worker sessions found.` with exit status `0` when the Work has no
matching attempts. If a listed attempt exposes a provider, kind, and provider
session ID, the Factory compatibility routes remain available with the exact
tuple:

```bash
you --server http://localhost:7437 worker-sessions show --provider codex --kind session_id --id <provider-session-id>
you --server http://localhost:7437 worker-sessions stream --provider codex --kind session_id --id <provider-session-id>
you --server http://localhost:7437 worker-sessions read --provider codex --kind session_id --id <provider-session-id>
```

Use `show` for one session's origin, correlation, lifecycle state, timing, token
usage, transcript availability, failure, and parse diagnostics. Use `stream`
to follow retained and live canonical session events; it ends successfully on
a terminal event and reports source failures. Use `read` only after the
session is finished to read its ordered normalized transcript. The current
CLI returns exit status `1` with `WORKER_SESSION_NOT_FOUND` when the supplied
provider identity has no observation or transcript. A completed provider
session is covered end to end by the repository's functional CLI check,
including list, show, live and terminal stream frames, and read.

## Reload prompts at dispatch time

Supported worker and workstation prompt files are read when a dispatch starts.
This includes the worker and workstation `AGENTS.md` files in the split
layout, plus a workstation `promptFile` when one is configured. Save an edit
before the next dispatch and that next dispatch uses the new prompt. An
already-running dispatch keeps the prompt snapshot it already received;
editing a file does not mutate an in-flight dispatch, restore lost Work, or
restore the in-memory Factory Session queue after a restart. The dispatch-time
reload behavior is covered by the functional prompt hot-reload check.

## Recover after a process restart

The live Factory Session queue is process-local and in memory. Restarting the
process loses queued and in-flight live session state; there is no storage
engine available that reconstructs that queue. Recordings, runtime logs,
artifacts, and files already written to a worktree can still be useful for
inspection, but they are evidence for recovery, not a durable session queue or
automatic replay guarantee.

Use this recovery sequence:

1. Inspect any durable artifacts first: the existing worktree, generated
   outputs, runtime logs, and any recording that was explicitly retained.
2. Start a new Factory Session with the continuous server shape above.
3. Resubmit the intended Work through the normal Work ingress, preserving each
   Work's authored `name`. Do not invent a new name just because the old
   process stopped.
4. Let the workstation's worktree template render from that same name. For a
   template such as
   `.claude/worktrees/{{ (index .Inputs 0).Name }}`, the resumed dispatch
   targets the existing named artifact directory; valid existing worktrees are
   reusable when the runtime's worktree rules allow it.
5. Verify the resumed Work reaches an explicit terminal or failed state. A
   successful resubmission proves only that this recovery worked for that
   Work; it does not make the original in-memory queue durable.

The six production recoveries recorded on 2026-08-08/09 are operational
evidence that this inspect → restart → same-name resubmit procedure worked in
those cases. They are not a durability guarantee, restart replay contract, or
promise that every provider dispatch can be resumed automatically.

## Related topics

- `you docs run` — supported run shapes, server/site lifecycles, output, and
  exit behavior
- `you docs guards` — `VISIT_COUNT` attachment and guarded `LOGICAL_MOVE`
  loop-breaker topology
- `you docs resources` — pool declarations, requirements, and capacity rules
- `you docs workstations` — stage routing and workstation authoring fields
- `you docs workers` — worker backends and shared worker prompt fields
- `you docs sessions` — live Factory Session discovery and lifecycle controls
- `you docs work` — Work submission, listing, showing, and transition watches
- `you docs record-replay` — recording and replay artifact boundaries
- `you docs authoring-factories` — Factory layout and authoring workflow
