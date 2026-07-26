# Config

`you docs config` is the canonical packaged guide for configuring operator
settings and validating or transforming an authored Factory. For ways to start
or invoke a Factory, use `you docs run`.

## Configure Provider And Model Defaults

Use the single setup command to configure the default provider and optional
free-form model used by model-backed workers:

```bash
you init --provider codex
you init --provider claude --model claude-sonnet-4-5
```

The provider must be registered. The model, when supplied, may be any non-empty
identifier and is not restricted to a local model catalog. A successful command
reports the selected defaults and operator-config path. `you init` updates only
the provider/model defaults and preserves all other operator settings. It does
not support global `--json`; passing that flag fails without changing the file.

Run `you init` without flags in a terminal for guided setup. The provider and
model prompts show current defaults in brackets; press Enter to retain a
displayed value. Enter `/cancel`, send EOF, or interrupt the command to abandon
setup without changing the operator config. Outside a terminal, `--provider` is
required.

Normal runtime initialization creates the operator-config document when needed
and materializes packaged/default Factories under
`~/.you-agent-factory/factories`. This lifecycle is initializer-owned and does
not require a separate setup or Factory-scaffolding command.

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

`YOU_DEFAULT_WORKER_MODEL_PROVIDER` and `YOU_DEFAULT_WORKER_MODEL` override the
file independently. Global `--default-worker-model-provider` and
`--default-worker-model` flags override both. Precedence is `file < env < flag`.
Authored worker values still win over operator defaults. Use `you docs workers`
for worker fields and `you docs javascript-workflows` for reusable child-agent
presets.

The same document can set the rolling-file policy for runtime logs and metrics:

```json
{
  "runtime": {
    "logging": {
      "directory": "/var/log/you/runtime",
      "maxSizeMB": 100,
      "maxBackups": 20,
      "maxAgeDays": 30,
      "compress": false
    },
    "metrics": {
      "directory": "/var/log/you/metrics",
      "maxSizeMB": 100,
      "maxBackups": 20,
      "maxAgeDays": 30,
      "compress": false
    }
  }
}
```

Both sections independently default to 100 MB files, 20 backups, 30 days of
retention, and no compression. Omitting `directory` uses the runtime-owned
location below `~/.you-agent-factory`.

## Validate Or Transform A Factory

Factory configuration commands live under `you factory config`, not the
operator-level `you config` group. Every command requires a concrete Factory
file or directory path.

Validate a portable file or split directory without persisting it:

```bash
you factory config validate ./factory/factory.json
you factory config validate ./factory/factory.yaml
you factory config validate ./factory
```

Add global `--json` for structured validation output. Validation uses the same
validate-only Factory contract as `POST /factory-validations`. A directory must
contain exactly one of `factory.json`, `factory.yaml`, or `factory.yml`; missing
and ambiguous roots are rejected instead of choosing a precedence.

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
