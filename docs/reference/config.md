# Config

`you docs config` is the canonical packaged guide for initializing operator
settings and validating or transforming an authored Factory. For ways to start
or invoke a Factory, use `you docs run`.

## Initialize Operator And System Configuration

Run the initializer once for a new user home:

```bash
you config init
```

It creates `~/.you-agent-factory/config.json` and the named-factory root used by
packaged defaults. Re-running it preserves an existing configuration and
already-materialized packaged factories. Add global `--json` when automation
needs the paths and per-file outcomes.

The operator file can supply defaults for model-backed workers that omit their
own provider or model:

```json
{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}
```

New configurations also include the three operator presets used by
`@you/classifier`. They are explicit CODEX model selections: `small` uses
`gpt-5-mini`, `medium` uses `gpt-5`, and `large` uses `gpt-5.4`. The classifier
uses these presets for small, medium, and large
complexity routes respectively. The initializer never adds or changes presets
in an existing configuration; define equivalent preset ids yourself when
migrating an existing home before using the packaged classifier.

`YOU_DEFAULT_WORKER_MODEL_PROVIDER` and `YOU_DEFAULT_WORKER_MODEL` override the
file independently. Global `--default-worker-model-provider` and
`--default-worker-model` flags override both. Precedence is `file < env < flag`.
Authored worker values still win over operator defaults. Use `you docs workers`
for worker fields and `you docs javascript-workflows` for reusable child-agent
presets.

## Validate Or Transform A Factory

Factory configuration commands live under `you factory config`, not the
operator-level `you config` group. Every command requires a concrete Factory
file or directory path.

Validate a portable file or split directory without persisting it:

```bash
you factory config validate ./factory/factory.json
you factory config validate ./factory
```

Add global `--json` for structured validation output. Validation uses the same
validate-only Factory contract as `POST /factory-validations`.

Flatten a split Factory directory into canonical, camelCase JSON on stdout:

```bash
you factory config flatten ./factory > ./dist/factory.json
```

Expand a portable file into `factory.json`, `workers/`, and `workstations/`
beside that input file:

```bash
you factory config expand ./dist/factory.json
```

Flatten before sharing or versioning a single portable file. Expand when
runtime instructions should be edited as separate `AGENTS.md` files. Neither
command starts a Factory Session.

## Minimum Factory Authoring Contract

A Factory directory uses this layout:

```text
factory/
  factory.json
  workers/<worker-name>/AGENTS.md
  workstations/<workstation-name>/AGENTS.md
  inputs/<work-type-or-BATCH>/<channel>/
```

`factory.json` owns topology. Worker and workstation `AGENTS.md` files own
runtime instructions. Watched Work and Work Request batches belong under
`inputs/`.

A minimal invokable Factory declares a Work type and its states, a worker, and
a workstation that routes Work from an initial state to a terminal or failed
state:

```json
{
  "id": "review-factory",
  "workTypes": [
    {
      "name": "task",
      "handlingBehavior": ["DEFAULT"],
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "reviewer" }
  ],
  "workstations": [
    {
      "name": "review",
      "worker": "reviewer",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}
```

The core authored fields are:

| Field | Purpose |
|-------|---------|
| `id` | Stable Factory identifier. |
| `workTypes` | Work categories, lifecycle states, and optional handling behavior. Use exactly one `DEFAULT` for text-first one-shot invocation. |
| `workers` | Worker identities and execution settings. Workstations refer to workers by name. |
| `workstations` | Steps that consume `{workType, state}` inputs and route accepted, continued, rejected, or failed Work. |
| `resources` | Named capacity pools used to bound concurrent dispatches. |
| `invocationSignature` | Optional named, positional, repeated, or stdin argument schema shared by CLI and API callers. |
| `invocationReturn` | Optional primary-result selection policy; omission uses the submitted Work's first terminal result. |
| `supportingFiles` | Portability-only declarations for required tools and bundled helpers or docs. |

Keep these boundaries explicit:

- Work types name each supported state as `INITIAL`, `PROCESSING`, `TERMINAL`,
  or `FAILED`. Submitted Work uses that work type name.
- Workstation routes refer only to declared Work types and states. See
  `you docs workstations` and `you docs guards` for routing and guard fields.
- Workers define execution behavior. See `you docs workers` for worker kinds
  and `you docs models` for model discovery and readiness.
- Runtime `resources` are capacity pools. `supportingFiles` is a portable-file
  manifest; it does not provide runtime capacity. See `you docs resources`.
- `invocationSignature` defines accepted caller inputs and
  `invocationReturn` selects the primary result. See `you docs run` for
  complete invocation examples and `you docs sessions` for the API surface.

## Portability Notes

Inline worker bodies and workstation prompt templates remain valid in a
portable `factory.json`. The split layout is preferable for routine authoring.
Flatten collects the supported split runtime files into one portable document;
expand materializes them again.

Declare external portability requirements under `supportingFiles.requiredTools`
and bundled content under `supportingFiles.bundledFiles`. Bundled target paths
must be relative, use forward slashes, and contain no `.` or `..` segments.
Use `you docs authoring-factories` for the authoring sequence and
`you docs templates` for prompt templates.

## Troubleshooting

- If `validate`, `flatten`, or `expand` reports a missing argument, pass a
  `factory.json` path or Factory directory as the final argument.
- If validation rejects a route, confirm that its worker, Work type, and state
  names are declared in the same Factory.
- If a one-shot text invocation has no target, add `handlingBehavior:
  ["DEFAULT"]` to exactly one Work type.
- If operator configuration fails before startup, validate
  `~/.you-agent-factory/config.json` as JSON and check provider names. A missing
  operator file is valid; malformed JSON is not.

## Related

- `you docs run` for starting and invoking Factories
- `you docs authoring-factories` for the end-to-end authoring sequence
- `you docs work`, `you docs workers`, and `you docs workstations` for owned
  field detail
- `you docs resources`, `you docs templates`, and `you docs sessions` for
  capacity, prompts, and live Factory Session operations
