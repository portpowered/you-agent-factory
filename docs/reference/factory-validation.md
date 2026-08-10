---
author: Agent Factory Team
last-modified: 2026-08-09
doc-id: agent-factory/factory-validation
---

# Factory Validation

`you docs factory-validation` prints this guide from the packaged CLI topic
surface. Use it when authoring or changing a Factory, and use `you docs config`
for the complete `factory.json` field reference.

## The required pre-run gate

`you factory config validate` is the required static gate between authoring a
Factory and its first run. Run it after creating a Factory and immediately
before the first execution. Do not treat a Factory as ready to run until this
command passes.

Validation reads the selected JSON, YAML, or split-directory source and checks
the current static Factory contract. It does not:

- start a Factory Session or scheduler;
- invoke a worker, model, or provider;
- create or update a persisted named Factory; or
- prove that a provider, model, external tool, resource, or runtime path will
  be available or succeed.

This is therefore a validate-only authoring check, not an execution or
persistence operation. A successful result means that the submitted source is
accepted by the current static contract. It does not guarantee provider
readiness, external-resource availability, runtime liveness, reachability of
every path, or successful execution.

## Exact file and directory commands

Pass one source path as the final argument. A portable file can use `.json`,
`.yaml`, or `.yml`. The following commands use checked-in sources so their
syntax and output are reproducible:

```bash
# Portable JSON file.
you factory config validate ./examples/basic/factory/factory.json

# Portable YAML file.
you factory config validate ./packages/packaged-factories/factories/classify/factory.yaml

# Split Factory directory.
you factory config validate ./examples/basic/factory
```

The current binary prints `Factory validation passed.` for each of those
sources. It also reports the runtime taxonomy it resolved. For example, the
portable JSON command currently returns:

```text
Factory validation passed.
Runtime taxonomy:
  worker processor: unspecified worker type
  workstation process: legacy agent-run default (worker=processor)
```

The portable YAML command currently returns:

```text
Factory validation passed.
Runtime taxonomy:
  worker complexity-classifier: AGENT_WORKER
  worker small-executor: AGENT_WORKER
  worker medium-executor: AGENT_WORKER
  worker large-executor: AGENT_WORKER
  workstation classify-request: CLASSIFIER_WORKSTATION (worker=complexity-classifier)
  workstation execute-small: AGENT_RUN (worker=small-executor)
  workstation execute-medium: AGENT_RUN (worker=medium-executor)
  workstation execute-large: AGENT_RUN (worker=large-executor)
```

The split-directory command returns the same `Factory validation passed.` and
runtime-taxonomy lines as the portable JSON source above. A failing source
returns blocking findings instead; correct every blocking finding and run the
same command again before execution.

For a directory source, the directory must contain exactly one regular Factory
root file: `factory.json`, `factory.yaml`, or `factory.yml`. Keeping more than
one of these roots is ambiguous and is rejected. A direct file path selects
that file, while a directory path resolves its one supported root and then
loads the split runtime definitions beside it.

## What the gate checks

The command combines source loading with the current static validation rules.
The important author-facing checks are:

### Document, schema, and definition validity

The selected file must parse as JSON or YAML and normalize to the Factory
definition shape. Validation then checks the authored definition and topology,
including duplicate identifiers, declared work types and states, worker and
workstation references, route targets, resource references, required outcome
routes, and other blocking definition invariants. A parse error and a
well-formed-but-invalid definition are both pre-run failures.

### Join shape and supported arity

`SAME_NAME` and `ALL_CHILDREN_COMPLETE` are input-level guards. Each can be
used independently, but the combined join shape currently supports at most two
inputs. A workstation that combines that pair across more than two inputs is a
static validation error; it must be reduced to a supported input set or split
into supported stages before runtime.

### Worked failure: three-input fan-in

The checked-in
[`unsupported-three-input-join.json`](../examples/factory-validation/unsupported-three-input-join.json)
is internally coherent except for its unsupported join arity. Its `fan-in`
workstation combines these three inputs:

```text
parent:ready
same:ready       guarded by SAME_NAME(matchInput=parent)
child:complete   guarded by ALL_CHILDREN_COMPLETE(parentInput=parent)
```

Run the exact file command before trying to run this Factory:

```bash
you factory config validate ./docs/examples/factory-validation/unsupported-three-input-join.json
```

The current binary rejects it before a Factory Session or provider execution
begins:

```text
Factory validation failed.
Runtime taxonomy:
  workstation fan-in: LOGICAL_MOVE (worker=)
Blocking targets:
  error same-name-all-children-complete-join-arity WORKSTATION(fan-in) INPUTS: workstation "fan-in" uses an unsupported SAME_NAME plus ALL_CHILDREN_COMPLETE join arity: observed arity is 3 inputs, and at most 2 inputs are supported for this join shape. Split the fan-in into supported two-input workstation stages or reduce the joined inputs.
Error: Factory Definition source ./docs/examples/factory-validation/unsupported-three-input-join.json (JSON): factory validation found blocking issues
```

The diagnostic is the current contract: the observed arity is `3`, the
supported maximum is `2`, and the affected workstation is `fan-in`. Reduce the
joined inputs or split the fan-in into stages where every join has at most two
inputs. Then validate the corrected file again before running it. The checked-in
[`supported-two-input-join.json`](../examples/factory-validation/supported-two-input-join.json)
shows the reduced form:

```bash
you factory config validate ./docs/examples/factory-validation/supported-two-input-join.json
```

Its current-binary result is:

```text
Factory validation passed.
Runtime taxonomy:
  workstation two-input-join: LOGICAL_MOVE (worker=)
```

### Guard attachment, cardinality, and references

Guard placement and the fields required at each level are part of the static
contract:

- A top-level `guards[]` entry supports `INFERENCE_THROTTLE_GUARD` and requires
  its provider and positive refresh-window fields.
- A workstation `guards[]` entry supports `VISIT_COUNT` and `MATCHES_FIELDS`.
  `VISIT_COUNT` requires a positive visit limit and a declared workstation;
  `MATCHES_FIELDS` requires a non-empty `matchConfig.inputKey`.
- Each workstation input has a `guards[]` array with at most one entry. Input
  guards such as `SAME_NAME`, `ALL_CHILDREN_COMPLETE`, and `ANY_CHILD_FAILED`
  must name the required peer `matchInput` or `parentInput`, cannot reference
  their own input, and must use a real `spawnedBy` workstation when one is
  supplied.

Typos in guard types, missing required guard fields, invalid cardinality, and
unknown or self-referencing peers are reported before dispatch.

### Split-layout runtime definitions

When the source is a split Factory directory, validation also loads the
referenced runtime files without repairing or materializing them. The supported
locations are:

```text
<factory-directory>/factory.json   # or factory.yaml / factory.yml
<factory-directory>/workers/<name>/AGENTS.md
<factory-directory>/workstations/<name>/AGENTS.md
```

A worker or workstation that relies on a split definition must have its
`AGENTS.md`; its frontmatter must parse, and a workstation's referenced
`promptFile` must be present. Inline definitions may provide the runtime fields
or prompt body, but an existing split entity directory with no required body
still fails validation when the inline definition leaves that body empty.

### Classifier labels and routes

A `CLASSIFIER_WORKSTATION` must declare one or more `classificationRoutes` and
must route through them rather than normal success `outputs`. Each route needs
one non-empty, unique label and at least one output. Labels may not have
leading or trailing whitespace or be JSON literal text. The route outputs must
also name declared work types and states. Classifier workstations cannot mix
these routes with `onContinue` or `onRejection`.

## After a pass

Only after validation passes should the author perform the first run. Run the
gate again after any topology, guard, route, schema, worker, workstation, or
split-layout prompt/configuration change that the next execution relies on.

For field-level topology and guard examples, run `you docs guards`,
`you docs workstations`, and `you docs workers`. For the end-to-end authoring
sequence, run `you docs authoring-factories`.

## Related

- `you docs config` — Factory fields, portability, and transformation commands.
- `you docs guards` — Guard types, attachment levels, and guarded moves.
- `you docs workstations` — Workstation kinds, inputs, outputs, and routes.
- `you docs workers` — Worker types and split `AGENTS.md` placement.
- `you docs authoring-factories` — The broader authoring and run workflow.
