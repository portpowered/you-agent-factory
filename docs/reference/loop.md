---
author: Agent Factory Team
last-modified: 2026-07-16
doc-id: agent-factory/guides/loop
---

# Recurring Loop Factory

`@you/loop` is the supported packaged named Factory for repeating one request.
It runs the request immediately, then keeps the resulting Factory Session
available for later cron-triggered iterations. Use this guide to install,
invoke, inspect, and recover a loop; use `you docs sessions` for the shared
Factory Session controls and recovery details.

## Install and start a loop

Initialize operator configuration once to materialize the editable packaged
Factories, including `@you/loop`:

```bash
you config init
```

Start the default hourly loop with one required request:

```bash
you run --named @you/loop "Check the release dashboard"
```

The first eligible iteration runs immediately. Successful command output is
that iteration's normal result; it does not mean the loop has completed. The
Factory Session stays active so the same request can run again at each eligible
cadence boundary.

## Configure cadence and isolation

The default `--period` is `1h`. The only supported period values are `1h` and
`24h`; an explicit valid value replaces the default and never silently falls
back to hourly execution.

```bash
you run --named @you/loop \
  --period 24h \
  "Check the release dashboard"
```

Use `--worktree` to run every iteration in one existing worker-isolation path:

```bash
you run --named @you/loop \
  --period 24h \
  --worktree release-dashboard \
  "Check the release dashboard"
```

`--worktree` must be a nonempty relative worktree name. Absolute paths,
traversal such as `../outside`, and unsupported values are rejected before a
worker dispatch. The option does not create a shared-worktree exception or
change the normal review behavior.

Missing, malformed, non-positive, or unsupported `--period` values are also
rejected before partial execution with a diagnostic that identifies `period`
and its supplied value. Run `you run --named @you/loop --help` to inspect the
live invocation signature.

## Select a provider or model

`@you/loop` leaves its worker model configuration open for the normal operator
defaults. Pass the existing global flags when starting the loop:

```bash
you --default-worker-model-provider codex \
  --default-worker-model gpt-5-codex \
  run --named @you/loop --period 24h \
  "Check the release dashboard"
```

The usual precedence is `file < env < flag`; authored worker model values, if
you edit the materialized Factory, still take precedence. Unsupported or
conflicting provider/model choices fail before an incorrect worker dispatch.
Selecting a provider or model does not change the period, request lineage,
worktree isolation, or review behavior. See `you docs config` for the full
operator-default contract and `you docs workers` for worker configuration.

## Inspect, stop, and recover

Use the existing Factory Session commands; `@you/loop` does not add a separate
control plane:

```bash
you session list
you session show <session-id>
you session delete <session-id>
```

`you session list` is the primary liveness check. Use the displayed session id
with `you session show` to inspect lifecycle, dispatches, and iteration results.
Use `you session delete` when the repeating session should no longer run.

Validation failures, such as an invalid period or worktree, occur before the
Factory Session starts and should be corrected before retrying. A worker
execution failure belongs to the affected iteration after the session has
started; inspect its session and Work/Dispatch context first, then use the
existing recovery guidance in `you docs sessions`. Do not invent a package-
specific retry or stop mechanism.

## Maintainer verification

After editing this reference topic, run `make docs-reference-smoke` from the
repository root.

## Related

- `you docs config` — packaged Factory materialization and model-default precedence
- `you docs run` — named Factory invocation and output modes
- `you docs sessions` — Factory Session inspection, lifecycle, and recovery
- `you docs workers` — worker configuration and provider behavior
