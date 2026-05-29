# Mock Workers

Use mock workers when you want to verify routing, rejection loops, failure
paths, and script side effects without making live provider calls.

`you docs mock-workers` is the canonical guide for `--with-mock-workers` and
the mock-workers JSON contract. See [Authoring Factories](authoring-factories.md)
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
Each array entry selects a worker dispatch and declares the outcome to apply
when it matches.

| Field | Required | Description |
|-------|----------|-------------|
| `id` | No | Stable label for diagnostics and test fixtures. |
| `workerName` | No | Matches `workers[].name` from `factory.json`. |
| `workstationName` | No | Matches the workstation currently executing. |
| `workInputs` | No | Filters on consumed work input fields (see below). |
| `runType` | Yes | One of `accept`, `reject`, or `script`. |
| `scriptConfig` | When `runType` is `script` | Command, args, env, and related script fields. |
| `rejectConfig` | When `runType` is `reject` | Observable stdout, stderr, and exit code. |

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

### Default When Nothing Matches

If no `mockWorkers` entry matches a dispatch, mock-worker mode returns the
default accepted result. An empty `mockWorkers` array therefore accepts every
dispatch.

## Example Commands

Simple validation run without a config file:

```bash
you run --dir ./factory --with-mock-workers
```

Targeted rejection using the checked-in example config:

```bash
you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json
```

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

## Related

- [Config](config.md) — brief run-flag summary with pointers to this topic
- [Authoring Factories](authoring-factories.md) — full factory authoring workflow
- [Workers](workers.md) — worker types and configuration
