# Mock Workers

Use mock workers when you want to verify routing, rejection loops, failure
paths, and script side effects without making live provider calls.

`you docs mock-workers` is the canonical guide for `--with-mock-workers` and
the mock-workers JSON contract. See `you docs authoring-factories`
for the full factory setup workflow.

## Enable Mock-Worker Mode

Pass `--with-mock-workers` on `you run` to replace live worker dispatch with
deterministic mock outcomes:

```bash
you run --dir <factory> --with-mock-workers
```

The optional path argument selects a JSON config file:

```bash
you run --dir <factory> --with-mock-workers ./path/to/mock-workers.json
```

When you omit the path, the CLI enables mock mode with an empty config. That
is equivalent to:

```json
{
  "mockWorkers": []
}
```

With no matching entries, every dispatch returns the default accepted result.

## JSON Contract

The config file is a single JSON object with a top-level `mockWorkers` array.
Each array element selects a worker dispatch and declares the outcome to apply
when it matches.

| Field | Required | Description |
|-------|----------|-------------|
| `unmatchedDispatchPolicy` | No | Controls unmatched dispatches: `accept` (default when omitted) or `passthrough` for real-worker fallback. |
| `mockWorkers` | Yes | Array of mock-worker entries. |
| `id` | No | Stable label for diagnostics and test fixtures. |
| `workerName` | No | Matches `workers[].name` from `factory.json`. |
| `workstationName` | No | Matches the workstation currently executing. |
| `workInputs` | No | Filters on consumed work input fields (see below). |
| `runType` | Yes | One of `accept`, `reject`, or `script`. |
| `scriptConfig` | When `runType` is `script` | Command, args, env, and related script fields. |
| `rejectConfig` | When `runType` is `reject` | Observable stdout, stderr, and exit code. |
| `gateConfig` | No | Signals dispatch arrival, waits for an explicit release file, then applies the configured run type. |
| `usage` | No | Provider/model identity and token counts for a matched dispatch. |

Unknown JSON fields are rejected at load time.

### Work Input Selectors

Each `workInputs` entry narrows the match using any combination of:

| Field | Matches |
|-------|---------|
| `workId` | Work item identifier |
| `workType` | Work type name such as `story` |
| `state` | Current work state such as `in-review` |
| `inputName` | Consumed input token name such as `work` |
| `traceId` | Trace identifier when present |
| `channel` | Input channel when present |
| `payloadHash` | Payload hash when present |

All specified selector fields on an entry must match for that entry to apply.
Omit a selector field to leave it unconstrained.

### Deterministic Dispatch Gates

Use `gateConfig` when a test or local harness must observe a dispatch in
progress before allowing its configured `accept`, `reject`, or `script`
behavior to complete. The gate is orthogonal to `runType`: it first creates
the arrival signal, waits for the release signal, and only then applies the
selected outcome.

| Field | Required | Description |
|-------|----------|-------------|
| `arrivedFile` | Yes | Absolute path created when the matching dispatch reaches the mock-worker boundary. |
| `releaseFile` | Yes | Different absolute path whose creation releases the dispatch. |
| `timeout` | Yes | Positive duration such as `15s` bounding the wait for release. |

```json
{
  "mockWorkers": [
    {
      "id": "hold-prerequisite-finish",
      "workstationName": "finish",
      "workInputs": [{"workId": "work-prerequisite"}],
      "runType": "accept",
      "gateConfig": {
        "arrivedFile": "/tmp/finish-gate/arrived",
        "releaseFile": "/tmp/finish-gate/release",
        "timeout": "15s"
      }
    }
  ]
}
```

Gate paths are synchronization signals, not arbitrary delay controls. A
missing release fails at the configured timeout instead of leaving a dispatch
blocked indefinitely. The same mock-worker configuration is retained when a
service-mode process opens additional Factory Sessions.

### Run Types

**`accept`** — Return a successful accepted result with no extra config.

**`reject`** — Return a rejected result. Optional `rejectConfig` fields:

| Field | Description |
|-------|-------------|
| `stdout` | Text surfaced as command stdout |
| `stderr` | Text surfaced as command stderr |
| `exitCode` | Integer exit code between 1 and 255 when set |

**`script`** — Execute a local command through the shared command-runner
boundary. Requires `scriptConfig` with at least `command`. Optional fields:

| Field | Description |
|-------|-------------|
| `args` | Argument list |
| `env` | Environment map |
| `workingDirectory` | Working directory for the command |
| `stdin` | Stdin payload |
| `timeout` | Duration string such as `30s` |

### Unmatched Dispatch Policy

When no `mockWorkers` entry matches a dispatch, mock-worker mode uses
`unmatchedDispatchPolicy`:

| Value | Behavior |
|-------|----------|
| omitted or `"accept"` | Return the default accepted mock result. This preserves the historical mock-worker default. |
| `"passthrough"` | Execute the dispatch through the normal worker runner and provider path instead of returning the synthetic accepted result. |

Use `"accept"` when you want every dispatch to stay deterministic. Use
`"passthrough"` for mixed mock/live runs where targeted `mockWorkers[]` entries
handle specific steps and everything else falls through to the real worker.

An empty `mockWorkers` array with the default policy therefore accepts every
dispatch. The same empty array with `"passthrough"` runs every dispatch through
the real worker path.

## Example Commands

Simple validation run without a config file:

```bash
you run --dir ./factory --with-mock-workers
```

Targeted rejection using the checked-in example config:

```bash
you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json
```

Script mock using the checked-in script example:

```bash
you run --dir ./examples/write-code-review \
  --with-mock-workers ./docs/examples/mock-workers-script.json \
  --work ./docs/examples/startup-work.json
```

Mixed mock/live run with reviewer rejection plus real-worker fallback for
unmatched dispatches:

```bash
you run --dir ./examples/write-code-review \
  --with-mock-workers ./docs/examples/mock-workers-mixed.json \
  --work ./docs/examples/startup-work.json
```

## Script Mock Example

The checked-in
[`docs/examples/mock-workers-script.json`](../examples/mock-workers-script.json)
targets the `executor` worker at the `execute-story` workstation and runs a local
command instead of returning a synthetic accept/reject result:

```json
{
  "mockWorkers": [
    {
      "id": "executor-script-side-effect",
      "workerName": "executor",
      "workstationName": "execute-story",
      "workInputs": [
        {
          "workType": "story",
          "state": "init",
          "inputName": "work"
        }
      ],
      "runType": "script",
      "scriptConfig": {
        "command": "printf",
        "args": ["mock script stdout\n"],
        "env": {
          "MOCK_WORKER": "1"
        },
        "workingDirectory": ".",
        "stdin": "optional script stdin payload",
        "timeout": "30s"
      }
    }
  ]
}
```

Set `runType` to `"script"`, provide `scriptConfig.command`, and use the
optional `args`, `env`, `workingDirectory`, `stdin`, and `timeout` fields to
mirror the command you want the mock boundary to execute.

## Usage Reporting

A matched mock-worker entry can declare provider usage with an optional
`usage` object. The configured `runType` outcome remains unchanged. Omit the
object when the dispatch should report no usage.

| Field | Required | Description |
|-------|----------|-------------|
| `provider` | When `usage` is present | Non-empty provider identity used by Worker Session inspection and Costs. |
| `model` | When `usage` is present | Non-empty model identity used by Worker Session inspection and Costs. |
| `inputTokens` | No | Non-negative input token count. An omitted class is absent; `0` remains an explicit value. |
| `outputTokens` | No | Non-negative output token count. An omitted class is absent; `0` remains an explicit value. |
| `cachedInputTokens` | No | Non-negative cached-input count. Requires `inputTokens` and cannot exceed it. Omitted and `0` retain the same missing-versus-zero distinction. |
| `reasoningOutputTokens` | No | Non-negative reasoning-output count. Requires `outputTokens` and cannot exceed it. Omitted and `0` retain the same missing-versus-zero distinction. |

Cached input is part of input. Reasoning output is part of output. Total tokens
are derived as `inputTokens + outputTokens`, so neither subclass is counted
again.

### Priceable Usage Example

The checked-in
[`docs/examples/mock-workers-usage.json`](../examples/mock-workers-usage.json)
configures one accepted `executor` dispatch with all four token classes:

```json
{
  "mockWorkers": [
    {
      "id": "executor-usage",
      "workerName": "executor",
      "workstationName": "execute-story",
      "runType": "accept",
      "usage": {
        "provider": "codex",
        "model": "gpt-5-codex",
        "inputTokens": 1000000,
        "cachedInputTokens": 400000,
        "outputTokens": 500000,
        "reasoningOutputTokens": 100000
      }
    }
  ]
}
```

Start `examples/simple-tasks` in one terminal and keep its API available:

```bash
you run --dir ./examples/simple-tasks --continuously --with-server \
  --with-mock-workers ./docs/examples/mock-workers-usage.json
```

In a second terminal, list the Worker Sessions and select the `executor`
Worker Session ID. Inspect its usage and the Factory cost report:

```bash
you --server http://localhost:7437 worker-sessions list --work-id <work-id>
you --server http://localhost:7437 worker-sessions show --worker-session-id <worker-session-id>
you --server http://localhost:7437 metrics costs
```

The `worker-sessions show` output reports the declared token classes and total
of `1500000`. With the shipped `codex/gpt-5-codex` rates, Costs reports
`PRICED` and `Cost (USD): $5.80`. This example uses only
`--with-mock-workers`; it does not require a recording or replay fixture.

## Mixed Mock and Real-Worker Fallback

The checked-in
[`docs/examples/mock-workers-mixed.json`](../examples/mock-workers-mixed.json)
keeps the review rejection mock from
[`docs/examples/mock-workers.json`](../examples/mock-workers.json) and opts
unmatched dispatches into real-worker passthrough:

```json
{
  "unmatchedDispatchPolicy": "passthrough",
  "mockWorkers": [
    {
      "id": "reviewer-rejects-first-pass",
      "workerName": "reviewer",
      "workstationName": "review-story",
      "workInputs": [
        {
          "workType": "story",
          "state": "in-review",
          "inputName": "work"
        }
      ],
      "runType": "reject",
      "rejectConfig": {
        "stdout": "needs changes",
        "stderr": "missing acceptance criteria",
        "exitCode": 42
      }
    }
  ]
}
```

Matched `accept`, `reject`, and `script` entries keep their configured outcomes.
Only dispatches that do not match any `mockWorkers[]` entry use the unmatched
policy.

## Rejection Example

The reusable example in
[`docs/examples/mock-workers.json`](../examples/mock-workers.json) matches a
review dispatch and returns a deterministic rejection:

```json
{
  "mockWorkers": [
    {
      "id": "reviewer-rejects-first-pass",
      "workerName": "reviewer",
      "workstationName": "review-story",
      "workInputs": [
        {
          "workType": "story",
          "state": "in-review",
          "inputName": "work"
        }
      ],
      "runType": "reject",
      "rejectConfig": {
        "stdout": "needs changes",
        "stderr": "missing acceptance criteria",
        "exitCode": 42
      }
    }
  ]
}
```

Combine mock workers with startup work for an end-to-end review-loop exercise:

```bash
you run --dir ./examples/write-code-review \
  --with-mock-workers ./docs/examples/mock-workers.json \
  --work ./docs/examples/startup-work.json
```

[`docs/examples/README.md`](../examples/README.md) documents how the example
files fit together. The checked-in
[`examples/write-code-review/factory.json`](../../examples/write-code-review/factory.json)
factory is a concrete starting point for adapting these commands.

## Reviewer Verification

When reviewing mock-worker docs or runtime changes, use this recipe:

1. Read the packaged topic: `you docs mock-workers`
2. Run one pure-mock scenario with the checked-in rejection example:

```bash
you run --dir ./examples/write-code-review \
  --with-mock-workers ./docs/examples/mock-workers.json \
  --work ./docs/examples/startup-work.json
```

3. Run one script-mock scenario with the checked-in script example:

```bash
you run --dir ./examples/write-code-review \
  --with-mock-workers ./docs/examples/mock-workers-script.json \
  --work ./docs/examples/startup-work.json
```

**Signoff note:** Do not rely on a live real-agent passthrough run for signoff
in this change. The `unmatchedDispatchPolicy: "passthrough"` mixed mock/live
path is covered by automated service and runner tests instead of manual
live-provider QA.

## Related

- `you docs config` — brief run-flag summary with pointers to this topic
- `you docs authoring-factories` — full factory authoring workflow
- `you docs workers` — worker types and configuration
