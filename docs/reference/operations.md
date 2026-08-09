---
author: Agent Factory Team
last-modified: 2026-08-09
doc-id: agent-factory/guides/operations
---

# Operations

This is the canonical runbook for keeping a real pipeline alive, recognizing
an incomplete run, and recovering after the process that owns a Factory
Session stops. Use it when Work can arrive after startup or when an operator
must distinguish an idle queue from completed Work.

For the complete command and output-mode reference, use `you docs run`. This
page owns the operational lifetime and restart boundary; it does not add a
storage engine or change runtime behavior.

## Use a continuous server for real pipelines

For a real pipeline that accepts or retains Work across idle periods, use the
server-enabled continuous shape:

```bash
you run --dir ./factory --with-server --continuously
```

`--with-server` keeps the Factory Session API available for submitters and
inspection commands. `--continuously` keeps the runtime alive while the queue
is idle. The process ends when it is cancelled normally, so an idle period is
not presented as a completed pipeline. Add `--with-site` when the same run
should also serve the dashboard; it follows the same lifetime and finite-drain
policy described below.

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
- `you docs sessions` — live Factory Session discovery and lifecycle controls
- `you docs work` — Work submission, listing, showing, and transition watches
- `you docs record-replay` — recording and replay artifact boundaries
- `you docs authoring-factories` — Factory layout and authoring workflow
