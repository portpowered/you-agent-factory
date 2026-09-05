---
author: Agent Factory Team
last-modified: 2026-08-23
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
This page owns the operational lifetime and restart boundary. When a Factory
Session has a configured Recordings record path, startup restores its retained
current-board state through the public Recordings history contract before the
session is ready; this does not make arbitrary in-memory queues durable.

## Use continuous hosting for real pipelines

For a real pipeline that accepts or retains Work across idle periods, use the
server-enabled continuous shape:

```bash
you run --dir ./factory --with-server --continuously
you run --dir ./factory --with-server --continuously --listen 127.0.0.1:7437
you run --dir ./factory --with-server --continuously --record ./recordings/factory.json
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

## Stop a local server gracefully on Windows

Use this procedure when a Windows terminal cannot deliver a usable console
cancellation event. The command uses the selected loopback server's
administrative control. It does not use forced process termination.

Choose one target in PowerShell terminal 1:

```powershell
# Serve the Current Factory continuously.
you server --listen 127.0.0.1:7437

# Or keep a server-enabled run alive while its queue is idle.
you run --dir ./factory --with-server --continuously --listen 127.0.0.1:7437
```

Run the stop command in PowerShell terminal 2:

```powershell
you --server http://127.0.0.1:7437 server stop
```

The same stop command applies to both target commands. It sends one
`POST /shutdown` request and waits for the selected listener to stop. A
successful command prints:

```text
Server stopped: http://127.0.0.1:7437
```

The stop request enters normal invocation cancellation. Application close
keeps the existing five-second bound. This path does not guarantee
synchronous Recording flush.

Confirm that the target PID is absent after the command returns. Replace the
sample PID with the PID from your target process.

The measured `you server` sample was:

```powershell
PS> tasklist /FI "PID eq 50632" /FO CSV /NH
"you.exe","50632","Console","2","38,116 K"
PS> you --server http://127.0.0.1:12090 server stop
Server stopped: http://127.0.0.1:12090
PS> tasklist /FI "PID eq 50632" /FO CSV /NH
INFO: No tasks are running which match the specified criteria.
```

The measured continuous-run sample was:

```powershell
PS> tasklist /FI "PID eq 16928" /FO CSV /NH
"you.exe","16928","Console","2","39,804 K"
PS> you --server http://127.0.0.1:25317 server stop
Server stopped: http://127.0.0.1:25317
PS> tasklist /FI "PID eq 16928" /FO CSV /NH
INFO: No tasks are running which match the specified criteria.
```

The process-boundary check ran each target ten consecutive times. The server
target passed `10/10` iterations in `416ms` to `502ms`. The continuous run
target passed `10/10` iterations in `375ms` to `451ms`.

If the command reports `SERVER_STOP_UNREACHABLE`, confirm the selected server
URI and its loopback listener. If it reports
`SERVER_STOP_OBSERVATION_TIMEOUT`, inspect the target process before retrying.

Keep forceful termination as a last-resort fallback:

```powershell
taskkill /PID 33724 /F
```

`taskkill /F` terminates the target without the orderly application-close
path. The measured fallback output was:

```text
SUCCESS: The process with PID 33724 has been terminated.
INFO: No tasks are running which match the specified criteria.
```

`you server` is continuous, non-resumable hosting for the exact Current Factory.
It does not accept recording, resume, or replay flags. Use
`you run --with-server --continuously --record <path>` when the host must keep
an explicit recording for restart recovery with `you run --resume <recording>`.

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

For a process and review loop, add `logicalRoundTrip` to the paired loop-breaker
guards. Its `maxVisits` value counts process/review pairs as logical cycles.
Its `maxRawVisits` value sums both workstation counts and stops imbalanced or
unchanged routes. Omit the field when the legacy per-workstation count is
required. See `you docs guards` for the complete configuration shape.

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
Worker Session through its stable identity. Use the unscoped fleet list or a
Work-scoped list to discover the identity, then use the direct inspection
commands:

```bash
you --server http://localhost:7437 worker-sessions list --output json
you --server http://localhost:7437 worker-sessions list --work-id <work-id>
you --server http://localhost:7437 worker-sessions show --worker-session-id <worker-session-id>
you --server http://localhost:7437 worker-sessions stream --worker-session-id <worker-session-id>
you --server http://localhost:7437 worker-sessions read --worker-session-id <worker-session-id>
```

The unscoped top-level list is the fleet-wide view: it includes direct and
Factory-originated observations across the process. Use `--scope direct`,
`--scope factory`, or `--scope all` when an origin-specific view is needed.
Repeat `--state` to select multiple lifecycle states; the values are combined
with OR. `--limit` is a positive result bound applied after scope and state
filters; `--next-token` resumes from the opaque cursor returned in JSON
`paginationContext`. The legacy `--max-results` flag remains accepted for
compatibility when `--limit` is omitted. These filters are fleet-wide only.
When `--work-id` is supplied, the Work-scoped endpoint preserves its
established unfiltered behavior and returns a typed error if a fleet-wide
filter is supplied. Omit `--work-id` to filter the fleet-wide view.

The human fleet table includes Work name and ID, the stable Worker Session ID,
provider and provider-session kind, provider-session ID when available, state,
start time, duration, and exit/failure kind. A `-` means that the observation
does not expose that fact. The Worker Session ID is the canonical identity for
`show`, `stream`, and `read`; a provider-session ID is a separate
provider-issued correlation value. JSON output preserves these identities and
includes `workId` and `workName` when Work attribution can be resolved.

Worker-ID inspection is backed by the Recordings-owned history under the
configured home root (`.you-agent-factory/worker-sessions`). A completed
Worker's `show`, `stream --replay-only`, and `read` results remain available
after a normal process restart without resolving a Provider Session. Such
responses may set `providerSession` to `null`; `recordingHealth` is then the
authoritative history state: `COMPLETE`, `DEGRADED`, or `INCOMPLETE`, with an
optional `recordingHealthReason` explaining a readable prefix or retention
condition. Provider-keyed commands remain the compatibility path when a
provider-native identity is available.

Direct Worker Session stdin is limited to 1,048,576 bytes, inclusive. This
limit applies to `--execution -` and to non-terminal stdin used for direct
Worker messages, continuation input, or replacement input.

Input at the limit is accepted. Larger input fails before Worker Session
admission. Use `--execution FILE`, `--user-message`, or
`--replacement-message` when the selected command supports those alternatives.

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
observed session actually runs. Only a server that answers replaces the local
result: when no factory server is reachable at that address, the control still
reports the unknown session rather than a transport failure, so a direct
invocation that owns its own sessions keeps reporting an unknown identity as an
unknown session. `--remote` sends the selected
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

The Work-scoped list returns observations correlated with one Work item.
Supply `--work-id` for this view. It returns a typed error when fleet-wide
filters are supplied.

The human Work-scoped table includes Work name and ID, the stable Worker Session
ID, provider, and provider-session kind. It also includes provider-session ID,
state, start time, duration, and exit/failure kind. A `-` means that the observation
does not expose that fact. The Worker Session ID is the canonical identity for
`show`, `stream`, and `read`; a provider-session ID is a separate
provider-issued correlation value. JSON output preserves these identities and
includes `workId` and `workName` when Work attribution can be resolved.

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

### Turn context usage

When supported provider usage evidence exists, `show --output json` and the
Worker Session API return `turnUsage` beside `tokenUsage`:

```json
{
  "tokenUsage": {"inputTokens": 700, "outputTokens": 12},
  "turnUsage": {
    "turnCount": 3,
    "finalContextTokens": 450,
    "peakContextTokens": 450
  }
}
```

`turnCount` counts supported provider usage turns. `finalContextTokens` is the
final turn's derived input. `peakContextTokens` is the largest derived input.
Provider counters are cumulative. Worker Sessions subtracts each counter from
the previous value, starting from zero. These fields describe context shape,
not pricing or cost. The optional `turnUsage` block is omitted when the
transcript has no supported usage evidence.

## Reload prompts at dispatch time

Supported worker and workstation prompt files are read when a dispatch starts.
This includes the worker and workstation `AGENTS.md` files in the split
layout, plus a workstation `promptFile` when one is configured. Save an edit
before the next dispatch and that next dispatch uses the new prompt. An
already-running dispatch keeps the prompt snapshot it already received;
editing a file does not mutate an in-flight dispatch or by itself restore a
stopped session. Use the record/replay/resume path for recorded Work after a
restart. The dispatch-time reload behavior is covered by the functional prompt
hot-reload check.

## Recover after a process restart

The live Factory Session queue is process-local and in memory, but its admitted
Work and dispatch lifecycle are represented in the Factory Event recording.
Restarting the process loses the running queue and provider process.
It does not make recorded Work unrecoverable. A retained recording can
reconstruct the recorded Factory state in a new live Factory Session with
`you run --resume <recording>`. Resume reads the source without overwriting it
and writes a successor recording by default. Use `--record <successor-path>` to
choose that path.

### Read confirmation state before recovery

Work, dispatch, and Worker Session reads include `confirmationState`. `CONFIRMED`
means the reported state or outcome is covered by completed recording storage.
`UNCONFIRMED` means the process has reported the fact, but the completed flush
watermark does not cover its producing event yet. The value predicts whether a
hard kill can lose or revise that fact. It does not change scheduling or
reconciliation rules, and it does not guarantee a provider-side exactly-once
effect.

The recorder keeps its existing 250 ms cadence. A read can conservatively stay
unconfirmed during a concurrent flush. The following real probe used an
isolated factory on 2026-08-23. It captured the same `you work show --json`
read before and after a completed flush interval:

```text
PS> 2026-08-23T15:35:28.1201791-07:00 you --server http://127.0.0.1:17437 --json work show batch-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e-durability-probe-work
{"chainingTraceDepth":1,"confirmationState":"UNCONFIRMED","currentChainingTraceId":"trace-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","name":"durability-probe-work","requestId":"request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","state":{"name":"processing","type":"PROCESSING"},"tags":{"_work_name":"durability-probe-work","_work_type":"task"},"traceId":"trace-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","workId":"batch-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e-durability-probe-work","workTypeName":"task"}
PS> 2026-08-23T15:35:35.6719323-07:00 you --server http://127.0.0.1:17437 --json work show batch-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e-durability-probe-work
{"chainingTraceDepth":1,"confirmationState":"CONFIRMED","currentChainingTraceId":"trace-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","name":"durability-probe-work","requestId":"request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","state":{"name":"processing","type":"PROCESSING"},"tags":{"_work_name":"durability-probe-work","_work_type":"task"},"traceId":"trace-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","workId":"batch-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e-durability-probe-work","workTypeName":"task"}
```

The probe then hard-killed the process at `2026-08-23T15:35:51.3326794-07:00`.
After restart, the same read returned the last durable state:

```text
PS> 2026-08-23T15:35:52.5535148-07:00 you --server http://127.0.0.1:17437 --json work show batch-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e-durability-probe-work
{"chainingTraceDepth":1,"confirmationState":"CONFIRMED","currentChainingTraceId":"trace-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","name":"durability-probe-work","requestId":"request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","state":{"name":"processing","type":"PROCESSING"},"tags":{"_work_name":"durability-probe-work","_work_type":"task"},"traceId":"trace-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e","workId":"batch-request-b8c4cecb-5b5e-4358-96ad-25136dc9fc3e-durability-probe-work","workTypeName":"task"}
```

Use `UNCONFIRMED` as a prompt to wait for confirmation or inspect the
reconciliation result after restart. Do not treat `CONFIRMED` as proof that a
provider completed an external side effect exactly once. Providers can act
before a process stops, so idempotency remains the recovery control for
duplicate effects.

Resume re-admits recorded non-terminal Work, including Work that was queued or
in flight at the stop boundary. Terminal Work represented by the recording is
not dispatched again, and a completed dispatch remains one completed dispatch
in the successor. A dispatch without a recorded completion can run again after
resume. This is not an exactly-once provider-effect guarantee.
A provider may have performed an effect before the process stopped.
Make provider-side operations idempotent when duplicate attempts matter.

Work that never reached a durable Factory Event is not recoverable from that
recording. Portable JavaScript recordings remain replay and inspection
artifacts. They do not contain a JavaScript VM or provider process and cannot be
passed to `--resume`. Unfinalized recordings with a valid complete event prefix
are supported. A truncated final event-stream block can be skipped after
earlier complete events. Mid-stream corruption and recordings without a valid
complete event are rejected.
When the Factory Session has a configured Recordings record path, startup
reconstructs the latest retained current-board state before accepting Work or
starting normal scheduling. Work identity, request identity, payload,
lineage, relations, and logical state are restored for the current session, so
`you work list` and `you work show` can read the board without an explicit
`--resume` invocation.

An attempt whose dispatch or Worker Session was active when the daemon stopped
is not resumed as a live process. Startup records a daemon-restart interruption
for that attempt, preserves its dispatch and Worker Session history, and
reports the old Worker Session as terminal process-gone rather than `RUNNING`.
Associated non-terminal Work is re-armed at its last durable logical state;
guards, dependencies, retry policy, and capacity decide whether and when a new
dispatch becomes eligible. Work with no in-flight attempt and terminal Work
are not changed by this reconciliation. No duplicate live attempt is restored.

Verify recovery with the same session target used for submission:

1. Run `you work list` and `you work show <work-id>` to confirm Work identity,
   state, payload, and relations.
2. Run `you worker-sessions list --work-id <work-id>` to inspect the Worker
   Session attempts attributed to that Work.
3. Inspect `GET /factory-sessions/<session-id>/dispatches` for exact
   session-level dispatch records, or call `you.factory_session.list_dispatches`
   through MCP. The Work-scoped Worker Session list does not replace these
   Factory Session dispatch reads.
4. Read the Factory Session events or Worker Session observation when the
   interruption reason and original attempt identity are needed.

If no current-board Recording was configured, the live Factory Session queue
remains process-local and in memory. Inspect durable artifacts first, then
resubmit through the normal Work ingress while preserving each Work's
authored `name`; a resubmission is a new attempt and is not automatic replay
of the old queue.

Use this recovery sequence:

1. Inspect any durable artifacts first: the existing worktree, generated
   outputs, runtime logs, and any recording that was explicitly retained.
2. If the recording contains recoverable Factory Event history, run
   `you run --resume <recording> --record <successor-path>` and inspect both
   recording paths. Confirm that recorded terminal Work was not dispatched
   again and that recorded non-terminal Work was re-admitted.
3. If Work was never admitted into the recording, start a new Factory Session.
   Use the continuous server shape above.
   Resubmit the Work through the normal Work ingress with its authored `name`.
   Do not invent a new name because the old process stopped.
   This is the same-name resubmit fallback. It applies only to Work absent from
   the recoverable recording.
4. Use the same name in the Workstation worktree template.
   For `.claude/worktrees/{{ (index .Inputs 0).Name }}`, runtime rules can
   target the existing named artifact directory.
5. Verify resumed or resubmitted Work reaches an explicit terminal or failed
   state. A successful recovery proves only that this recovery worked for that
   Work. Provider-side exactly-once effects still require idempotency.

The six production recoveries recorded on 2026-08-08/09 remain historical
operational evidence for the same-name fallback. They are not a durability
guarantee or a substitute for inspecting the source and successor recordings.

For a copyable record → kill → resume journey with a fresh binary and isolated
temporary directory, use the procedure in `you docs record-replay`.

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
