# Domain-First OpenAPI Component Structure Plan

## Status

Proposed.

## Problem Statement

The authored OpenAPI component tree is organized primarily by component kind
and broad technical categories instead of the customer contracts and product
domains those components describe. Customers and contributors who start from a
Factory configuration, operator configuration, Factory Session, Work request,
or another public concept must search large mixed directories and reconstruct
ownership from filenames and references.

## Customer Ask

Make the authored API schema layout traceable from customer-facing concepts.
In particular, make the Factory configuration schema and the global/operator
configuration schema independently discoverable, then apply the same ownership
model to the remaining OpenAPI components without changing public API behavior.

## Intended Outcome

A reader can start from a customer concept and find, in one domain-owned
directory:

- the root schema or operation family;
- the schemas, parameters, and responses owned by that domain;
- the canonical source of truth when a standalone schema also exists;
- the published OpenAPI component or package export; and
- the production parser, validator, or service boundary that consumes it.

The migration is an authoring-layout change. OpenAPI component names, REST
operations, generated Go and TypeScript names, standalone JSON Schema
identities, package exports, and runtime validation behavior remain stable.

## Original Documents And Normative Inputs

- `factory/docs/standards/planning-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\internal\standards\code\code-review-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\internal\standards\code\general-backend-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\internal\development\development.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\data-model.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\packaged-structure.md`

## Current State

`api/components/` currently contains 515 component files:

| Current directory | Files | Current role |
| --- | ---: | --- |
| `parameters/` | 27 | Parameters from several unrelated operations |
| `responses/` | 14 | Shared and domain-specific HTTP responses |
| `schemas/api/` | 192 | Requests, responses, status types, sessions, models, Work, and operator configuration |
| `schemas/data-models/` | 132 | Factory authoring concepts mixed with runtime records |
| `schemas/events/` and `schemas/events/payloads/` | 65 | Canonical Factory Event contracts |
| `schemas/factory-world/` | 24 | Compatibility Factory visualization read models |
| `schemas/model-providers/` | 23 | Provider catalog contracts |
| `schemas/response-events/` and descendants | 32 | Ephemeral Factory response stream contracts |
| `schemas/shared/` | 6 | Small cross-surface helpers |

The two largest generic directories contain 324 of the 474 schema files. The
Factory root currently has a transitive authored-fragment closure of roughly
105 schema files, while the operator configuration root has eleven
`GlobalConfig*` fragments. A new pair of folders inside the existing generic
taxonomy would improve those roots locally but would not resolve the broader
ownership ambiguity.

There is also an important canonical-ownership distinction:

- `api/openapi.yaml#/components/schemas/Factory`, authored from
  `api/components/schemas/data-models/Factory.yaml`, is the canonical owner for
  the standalone Factory configuration schema generated into
  `contracts/config/factory.schema.json` and package projections.
- `contracts/config/you-config.schema.json` is the canonical standalone
  operator configuration schema. The `GlobalConfig*` OpenAPI fragments provide
  generated transport types and must remain behaviorally aligned with it.
- `internal/configcontractsmoke/family.go` records these separate ownership
  paths and their production parsers and package projections.

The plan must make this distinction explicit rather than implying that every
file under `api/components/` is the canonical source for every published schema.

## Goals

- Organize authored components by public product domain before component kind.
- Make Factory and operator configuration roots immediately discoverable.
- Keep schemas close to the parameters and responses owned by the same API
  domain.
- Give every component one canonical authored owner and allow cross-domain
  references without copying schemas.
- Preserve all public component names, route contracts, schema identities,
  package exports, and generated type names.
- Migrate in dependency-aware, independently reviewable slices.
- Replace the old `schemas/api` and `schemas/data-models` policy after all
  components have an explicit owner.

## Non-Goals

- Renaming OpenAPI components, REST operations, fields, enums, or generated
  types.
- Redesigning Factory, operator configuration, session, event, Work, provider,
  or model behavior.
- Changing `@you-agent-factory/api` export specifiers.
- Changing standalone JSON Schema `$id` values.
- Making `GlobalConfig` the canonical standalone operator schema as part of
  this structural migration.
- Introducing duplicate compatibility copies at old component paths.
- Adding a source-inventory or directory-topology test that proves only where
  files live.
- Reformatting or rewriting schema content unrelated to the move.

## Structural Principles

### 1. Domain First, Component Kind Second

The first directory below `api/components/` names the owning customer or
product domain. A domain may then contain `schemas/`, `parameters/`, and
`responses/` as needed.

This makes `factory-sessions/schemas/FactorySession.yaml` more informative than
`schemas/api/FactorySession.yaml`, and keeps session parameters and error
responses near the schemas used by the same operations.

### 2. Configuration Roots Are First-Class Domains

Factory configuration and operator configuration are separately loadable
customer contracts. They receive separate top-level domains even when a schema
is also registered in the bundled OpenAPI document for generation or HTTP use.

Use `operator-config` rather than `global-config` for the directory name. The
Operator Settings service and public documentation use operator configuration
as the durable ownership term; `GlobalConfig` remains the stable OpenAPI and
generated type name.

### 3. Ownership Follows Meaning, Not Every Consumer

A schema has one authored owner. Other domains reference it.

- A value primarily authored inside `factory.json` belongs to
  `factory-config`.
- Runtime Work content and Work records belong to `work`, even when a Factory
  configuration field references them.
- Factory Session read models belong to `factory-sessions`, even when an event
  carries one.
- Neutral primitives used by unrelated domains may live under `shared`.

`shared` is not a fallback for uncertain ownership. A component stays with the
domain that defines its semantics whenever such an owner exists.

### 4. Source Paths May Change; Public Identities Must Not

`api/openapi-main.yaml` continues to register the same component keys. Moving a
fragment changes its file `$ref`, not its public
`#/components/schemas/<ComponentName>` identity. Relative references inside
fragments are updated to their new canonical owners.

### 5. Moves Must Not Leave Compatibility Copies

The repository layout is not a supported API package surface. Files move to
their new owner and old copies are deleted in the same slice. Package consumers
continue to use only the declared exports documented by `packages/api`.

## Target Component Tree

```text
api/components/
  factory-config/
    README.md
    schemas/
      Factory.yaml
      common/
      invocation/
      orchestration/
      inputs/
      work-types/
      resources/
      workers/
      workstations/
      layout/
      portability/

  operator-config/
    README.md
    schemas/
      GlobalConfig.yaml
      defaults/
      runtime/
      workers/

  factory-definitions/
    schemas/
    parameters/
    responses/

  factory-sessions/
    schemas/
    parameters/
    responses/

  work/
    schemas/
    parameters/
    responses/

  worker-sessions/
    schemas/
    parameters/
    responses/

  provider-sessions/
    schemas/
    parameters/
    responses/

  providers/
    schemas/

  models/
    schemas/
    parameters/
    responses/

  events/
    schemas/
      payloads/

  response-events/
    schemas/
      content-blocks/
      payloads/
    parameters/
    responses/

  factory-visualization/
    schemas/

  shared/
    schemas/
    parameters/
    responses/
```

Directories that have no owned component of a given kind are omitted. The
tree is a target ownership model, not a requirement to create empty folders.

## Ownership Map

| Domain | Representative current contents | Target |
| --- | --- | --- |
| Factory configuration | `Factory`, `FactoryOrchestrator*`, `FactoryInvocation*`, authored guards, Work types/states, workers, workstations, layout, portability | `factory-config/` |
| Operator configuration | `GlobalConfig*` | `operator-config/` |
| Factory Definitions API | preview, validation, save, packaged catalog contracts and definition-owned errors | `factory-definitions/` |
| Factory Sessions | `FactorySession*`, lifecycle controls, dispatch/artifact session reads, session parameters | `factory-sessions/` |
| Work | `Work`, `WorkContent*`, invocation/submission/move/list request and response contracts, Work parameters | `work/` |
| Worker Sessions | `WorkerSession*` and list/transcript contracts | `worker-sessions/` |
| Provider Sessions | `ProviderSession*`, `LoadableProviderSession*` | `provider-sessions/` |
| Providers | Provider catalog, capability, modality, tool, documentation, and deprecation contracts | `providers/` |
| Models | `Model*`, `ManagedRuntime*`, resolved model bindings, model operations | `models/` |
| Factory Events | Factory Event envelope, enums, diagnostics, payloads, and recording | `events/` |
| Response Events | Factory response stream envelope, provenance, content blocks, payloads, stream parameters and errors | `response-events/` |
| Factory visualization | Existing `FactoryWorld*` compatibility read models | `factory-visualization/` |
| Shared transport primitives | general error, pagination, status, timestamps, maps, neutral resource usage/requirements | `shared/` when no domain owns the meaning |

## Configuration Directory Index Contract

Each configuration directory receives a concise `README.md`. This is customer
and contributor navigation, not a generated artifact or a duplicate schema
reference. Each index must state:

- the customer configuration file and root schema;
- the canonical schema owner;
- the bundled OpenAPI component identity;
- the standalone JSON Schema `$id`;
- the supported `@you-agent-factory/api` export;
- the production parser or mapping boundary;
- the command customers use to validate the configuration; and
- where the configuration's subfamilies are located.

For Factory configuration, the index points to the OpenAPI `Factory` component
as canonical and to the generated standalone Factory schema as its projection.
For operator configuration, the index points to
`contracts/config/you-config.schema.json` as canonical and explains that the
OpenAPI `GlobalConfig*` fragments provide generated boundary types.

## API And Contract Impact

### Authored OpenAPI

- Update file references in `api/openapi-main.yaml` without changing component
  keys or component order.
- Update relative fragment references to the new owner paths.
- Preserve the requirement that every reusable component registered in
  `openapi-main.yaml` is a single file `$ref` under `./components/`.

### Bundled OpenAPI And Generated Clients

The following generated artifacts must have no semantic change from source
movement alone:

- `api/openapi.yaml`
- `pkg/transports/http/generated/server.gen.go`
- `pkg/transports/http/client/client.gen.go`
- `ui/src/api/generated/openapi.ts`

If regeneration changes one of these artifacts beyond path-insensitive
formatting, the slice must stop and explain the contract difference instead of
accepting it as incidental reorganization.

### Standalone Configuration Schemas

Preserve:

- `https://schemas.portpowered.com/you/config/factory.schema.json`;
- `https://schemas.portpowered.com/you/config/you-config.schema.json`;
- `@you-agent-factory/api/schemas/factory`;
- `@you-agent-factory/api/schemas/you-config`; and
- the packaged-factories Factory schema projections.

The Factory standalone schema remains generated from the bundled OpenAPI
`Factory` component graph. The operator schema remains canonically authored
under `contracts/config/` during this plan.

### Runtime And Services

No service API or runtime behavior changes are planned. Production parsers and
owners remain:

- Factory configuration:
  `pkg/transports/mapping/factoryconfig.FactoryConfigMapper.Expand` and the
  Factory Definitions service boundary.
- Operator configuration:
  `pkg/services/operator_settings/transports/globalconfig.Decode` and the
  Operator Settings service boundary.

## Implementation Stories

Each story below is intended to be a separate PR-sized slice unless measured
reference churn shows that two adjacent, non-conflicting stories are safer to
review together. Stories move source and update all references in the same PR;
there is no intermediate compatibility-copy state.

### 1. Establish The Domain-First Authoring Contract

As a customer or contributor entering the API contract tree, I can identify the
target owner for each public contract family before following individual
schemas.

This is a narrowly justified enabling story: repository documentation must
define ownership before later file moves can be reviewed consistently.

Acceptance criteria:

- The target tree, placement rules, canonical ownership distinctions, and
  supported public identities are documented in the canonical API authoring
  guidance.
- The guidance uses public vocabulary from `docs/architecture/data-model.md`.
- The guidance says new schemas go directly to a domain owner rather than the
  legacy `schemas/api` or `schemas/data-models` buckets.
- No runtime, API, generated artifact, or package export changes in this story.

### 2. Make Factory Configuration Independently Traceable

As a Factory author, I can start at `factory-config/schemas/Factory.yaml` and
follow schemas grouped by the sections I author in `factory.json`.

Acceptance criteria:

- `Factory.yaml` and Factory-authored component schemas move under
  `factory-config/schemas/` and its customer-oriented subdirectories.
- Schemas with a distinct canonical domain owner, such as runtime Work records,
  are referenced from that owner rather than copied into Factory configuration.
- The Factory configuration index maps the root, component identity, standalone
  schema, package export, production mapper, validation command, and subfamilies.
- A representative valid Factory configuration accepted before the move remains
  accepted, and representative invalid enum, unknown-field, and malformed
  configurations remain rejected with the same public outcomes.
- Factory validation, flatten, expand, load, and save round trips remain
  behaviorally unchanged.
- The bundled `Factory` component and standalone Factory schema have no semantic
  drift.
- Generated Go and TypeScript API artifacts have no semantic drift.

### 3. Make Operator Configuration Independently Traceable

As an operator, I can start at the operator configuration directory and
understand the complete `.you-agent-factory/config.json` contract, its
canonical source, and its generated OpenAPI type projection.

Acceptance criteria:

- All `GlobalConfig*` OpenAPI fragments move under
  `operator-config/schemas/` and its focused subdirectories.
- The operator configuration index explicitly identifies
  `contracts/config/you-config.schema.json` as the canonical standalone owner.
- The OpenAPI component names and generated `GlobalConfig*` Go type names remain
  unchanged.
- Valid operator configuration remains accepted.
- Unknown top-level fields, unknown nested fields, malformed JSON, and trailing
  JSON values remain rejected by the production decoder.
- Operator defaults, runtime settings, ACP settings, and worker presets retain
  their schema parity and round-trip behavior.
- The standalone schema `$id` and package export have no semantic drift.

### 4. Group Factory Definition Operations With Their Contracts

As a customer using Factory definition APIs, I can find preview, validation,
save, catalog, and definition-specific failure contracts under one Factory
Definitions owner.

Acceptance criteria:

- Factory preview, validation, save, packaged catalog, prompt validation, and
  workflow-preview components move to `factory-definitions/` when they are
  owned by Factory Definitions.
- Definition-specific parameters and responses move with their schemas.
- Current Factory read/save/create API operations continue to reference the
  same public component identities and return the same status/schema pairs.
- Focused Factory definition API contract tests pass with no generated-client
  semantic drift.

### 5. Group Factory Session Contracts Vertically

As a customer operating Factory Sessions, I can find session open, execute,
read, result, lifecycle control, dispatch, artifact, and session-specific error
contracts under the Factory Sessions owner.

Acceptance criteria:

- `FactorySession*`, session lifecycle, durable execution, session dispatch and
  artifact read contracts move to `factory-sessions/`.
- Session parameters and session-specific responses move in the same slice.
- Runtime dispatch and artifact value types are assigned to Factory Sessions or
  another explicit domain owner and are not left in `data-models`.
- Session route request and response component identities, response codes, and
  generated client method signatures remain unchanged.
- Existing direct source-path reads in contract tests are updated or, where the
  bundled component already proves the behavior, replaced with bundled-contract
  assertions rather than new topology assertions.
- Focused session API and generated-client contract tests pass.

### 6. Group Work And Worker Session Contracts

As a customer submitting and inspecting Work, I can trace Work content,
requests, relations, movement, listing, invocation, and Worker Session
observations through their respective owners.

Acceptance criteria:

- Runtime `Work`, Work content, submission, invocation, movement, listing, and
  Work-owned relation contracts move under `work/`.
- Worker Session list, event, observation, replay, failure, and transcript
  contracts move under `worker-sessions/`.
- Work and Worker Session parameters and responses move with their domains.
- Factory configuration references Work-owned schemas without duplicating them.
- Work submission, invocation, listing, movement, and Worker Session read API
  contracts remain unchanged in the bundled OpenAPI and generated clients.
- Focused Work and Worker Session contract and smoke tests pass.

### 7. Group Provider, Provider Session, And Model Contracts

As a customer configuring or inspecting inference capabilities, I can navigate
provider catalogs, provider sessions, managed runtimes, and models through
separate domain owners.

Acceptance criteria:

- Provider catalog/capability schemas move from `model-providers` to
  `providers/schemas/` without renaming public Provider components.
- Provider Session schemas move from the generic API bucket to
  `provider-sessions/`.
- Model, managed runtime, pull, invocation, and resolved binding contracts move
  to `models/`.
- Shared provider identities referenced by Factory configuration retain one
  explicit owner and are referenced rather than copied.
- Provider and model REST operations, schema identities, generated types, and
  package behavior remain unchanged.
- Provider parity, model contract, and generated API checks pass.

### 8. Complete Events, Response Events, Visualization, And Shared Ownership

As a customer inspecting event or visualization contracts, I can distinguish
canonical Factory Events, ephemeral Factory response events, compatibility
visualization read models, and genuinely shared primitives by their top-level
owners.

Acceptance criteria:

- Existing event families retain their envelope/payload separation while
  moving to the domain-first top-level layout.
- Factory Events and Factory response events remain separate public contract
  families with unchanged discriminator mappings and payload coverage.
- `factory-world` components move under `factory-visualization/` without
  renaming `FactoryWorld*` components in this plan.
- Remaining neutral schemas, parameters, and responses move under `shared/`;
  domain-specific components do not remain there for convenience.
- The legacy top-level `parameters/`, `responses/`, `schemas/api/`,
  `schemas/data-models/`, `schemas/factory-world/`, and
  `schemas/model-providers/` directories are empty and removed.
- Event standalone schemas, bundled OpenAPI discriminators, generated clients,
  and visualization contracts have no semantic drift.

### 9. Close Documentation And Published-Contract Alignment

As a package consumer or contributor, I can follow documentation from the
customer contract to its canonical source and supported package export without
depending on a retired repository path.

Acceptance criteria:

- Canonical development and architecture documentation describes the final
  domain-first tree.
- Maintained documentation and live test helpers no longer cite retired
  component paths.
- Historical plans or baselines are changed only when they function as current
  instructions or executable inputs; archival evidence is not rewritten merely
  for cosmetic consistency.
- `packages/api/README.md` continues to document only supported exports and
  warns consumers not to depend on package-internal or repository paths.
- README and packaged documentation links remain valid where changed.

## Verification Strategy

### Per-Story Focused Checks

Every component-move story runs:

```text
node scripts/run-quiet-api-command.js validate:main ./api/openapi-main.yaml
make api-smoke
```

Configuration stories additionally run:

```text
make contracts-generate
make contracts-check
make config-contract-smoke
make api-package-verify
```

Domain stories run the focused Go contract tests for the affected HTTP/service
area. `make generate-api` is invoked only through the supported generation
workflow; generated files are never hand-edited.

### Generated-Artifact Evidence

For a pure source-layout slice, review must confirm no semantic diff in:

```text
api/openapi.yaml
pkg/transports/http/generated/server.gen.go
pkg/transports/http/client/client.gen.go
ui/src/api/generated/openapi.ts
contracts/config/factory.schema.json
packages/api/generated/schemas/factory.schema.json
packages/api/generated/schemas/you-config.schema.json
```

Only artifacts affected by the slice need direct comparison, but Factory and
operator configuration moves require their complete configuration projections.

The plan does not add tests that scan source directories to enforce the target
tree. Behavioral contract tests, deterministic regeneration, package consumer
tests, and review of the authored source moves provide the evidence.

### Broader Gates

- Run `make verify-fast` on each PR-sized slice.
- Run `make verify-pr` before merging each shared/public-contract PR.
- Run `make interfaces-all` when a slice affects publishable contract or UI
  package generation beyond the direct Go/dashboard OpenAPI outputs.
- Run `make readme-check` when root README links change.

## Project-Level Acceptance Criteria

- A reader starting from Factory configuration can locate its root, authored
  subfamilies, canonical schema, production mapper, validation command, and
  supported package export without searching a generic schema bucket.
- A reader starting from operator configuration can locate its root,
  `GlobalConfig*` projection, canonical standalone schema, production decoder,
  validation command, and supported package export without searching a generic
  schema bucket.
- Every remaining component has one documented domain owner.
- No authored reusable schema remains under `schemas/api` or
  `schemas/data-models`.
- No old-path compatibility copies or duplicate schema owners remain.
- Public OpenAPI component keys, operation IDs, route contracts, generated type
  names, standalone schema `$id` values, and package exports remain stable.
- Valid and invalid Factory and operator configuration cases retain their
  production parser outcomes.
- Bundled OpenAPI, generated clients, standalone schemas, and package
  projections are deterministic and aligned with canonical sources.
- Required lint, contract, generation, unit, integration, and PR verification
  gates pass for every delivered slice.
- Delivery continues until required CI is terminal and passing, all blocking
  review feedback is explicitly addressed, merge conflicts are resolved, and
  every in-scope PR is actually merged. Opening a PR, receiving approval, or
  reaching green CI without merge is not completion.

## Risks And Mitigations

### Relative Reference Breakage

Moving nested YAML files changes relative `$ref` depth. Move one ownership
family at a time, update the root registrations and internal references in the
same commit, and validate `openapi-main.yaml` before regeneration.

### Generated Ordering Or Formatting Churn

Keep component keys and their order in `openapi-main.yaml` unchanged during
pure movement. Treat unexpected generated changes as a contract investigation,
not acceptable move noise.

### Canonical Owner Confusion

Factory and operator configuration currently use different standalone schema
ownership paths. Preserve that distinction in the two directory indexes and in
`configcontractsmoke`; defer source unification to a separately planned
behavior and tooling change.

### Merge Conflict Volume

`api/openapi-main.yaml` and shared fragment references are high-churn files.
Sequence stories rather than implementing them concurrently, rebase each slice
onto the latest merged predecessor, and regenerate after conflict resolution.

### Overuse Of Shared Components

Require reviewers to identify the semantic owner before accepting a component
under `shared`. Cross-domain reuse alone is insufficient when one product
domain defines the component's meaning.

### Historical Documentation Churn

Update normative guidance and active references. Do not rewrite historical
audits, merged-plan evidence, or baselines unless they are executed or presented
as current guidance.

## Delivery Order

1. Establish the documented ownership contract.
2. Move Factory configuration.
3. Move operator configuration.
4. Move Factory Definitions operations.
5. Move Factory Sessions.
6. Move Work and Worker Sessions.
7. Move Providers, Provider Sessions, and Models.
8. Move Events, Response Events, Factory visualization, and shared primitives;
   remove the retired generic directories.
9. Complete documentation and package-consumer alignment.

This order establishes the customer-requested configuration roots first, then
migrates adjacent public domains while keeping each review centered on one
observable navigation and contract-preservation outcome.
