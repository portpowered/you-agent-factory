# API Reference Prose, Examples, And Localization Plan

## Status

Proposed.

## Problem statement

The OpenAPI contract lists the REST operations and data shapes, but much of the
reference text does not show customers how to complete a task. Most operations
have summaries and descriptions, but few operations have named request and
response examples. Some descriptions also expose internal terms or combine too
many rules in one paragraph.

The reference text has no complete stable identity model. This prevents a
translator from translating one text resource without depending on its source
file, line number, JSON pointer, or current English text.

## Customer ask

Make the API reference clear and useful. Add concise explanations, minimal
examples, expected results, and relevant recovery guidance. Align the prose
with the planned customer-writing standard and ASD-STE100 principles.

Prepare the reference for localization. Give each localizable text resource a
stable identity. Keep API names, payload fields, enum values, and other machine
contracts unchanged.

## Intended outcome

A customer can select an API operation, understand when to use it, send a
minimal valid request, interpret the response, and recover from common errors.
The customer does not need source-code knowledge or internal Petri-net terms.

An author can add or change English reference text in the canonical OpenAPI
source. A translator can translate that text by stable ID. A documentation
renderer can apply a locale catalog with explicit English fallback behavior.

## Planning basis

This plan follows these repository standards and active plans:

- `docs/internal/standards/code/planning-standards.md`
- `docs/internal/standards/code/code-review-standards.md`
- `docs/internal/standards/code/general-backend-standards.md`
- `docs/internal/standards/code/general-website-standards.md`, section 9
- `docs/internal/standards/templates/task-templates.md`
- `docs/architecture/data-model.md`
- `docs/internal/development/plans/customer-prose-clarity-program.md`
- `docs/internal/development/plans/customer-prose-standard-and-enforcement.md`
- `docs/internal/development/plans/cli-and-reference-prose-migration.md`
- `docs/internal/development/plans/public-documentation-prose-migration.md`
- `docs/internal/development/plans/api-component-domain-structure.md`

The customer-prose plans are proposed work in the current tree. This plan
depends on their accepted writing profile and terminology register. It extends
their scope to authored OpenAPI text and API examples.

## External writing reference

Use [ASD-STE100 Simplified Technical English, Issue 9](https://www.asd-ste100.org/assets/files/ASD-STE100_ISSUE9.pdf)
as the controlled-language reference. Issue 9 was released on January 15,
2025. The official site identifies it as the current issue.

The implementation must also use the
[official ASD-STE100 frequently asked questions](https://www.asd-ste100.org/STE_faq.html).
The FAQ explains that software can assist a writer but cannot certify the text
or replace technical and language review.

The repository will claim alignment with the You customer technical-writing
standard. It will not claim ASD-STE100 certification. The repository will not
copy the ASD-STE100 controlled dictionary into source control.

## Current-state snapshot

The snapshot below describes the tree on August 10, 2026. It is planning
evidence, not a permanent inventory assertion.

| Surface | Current state | Behavior gap |
| --- | ---: | --- |
| REST operations | 48 | All have a summary and description, but many descriptions are dense contract notes. |
| Operations with request media | 20 | Most lack a named, task-focused request example. |
| Declared operation responses | 184 | Most success responses lack a representative named payload. |
| Authored component fragments | 515 | Many descriptions explain shape but not customer purpose or field relationships. |
| `example:` keys in authored OpenAPI | 7 | This is too small to teach the complete public surface. |
| `examples:` keys in authored OpenAPI | 14 | Several are error examples. Coverage is not measured per operation or use case. |
| Operations with `x-doc-id` | 1 | The field currently links one operation to a longer guide. It is not a localization identity. |

The existing REST operation inventory records operation identity, summary
presence, description presence, media types, responses, and `x-doc-id`. It
does not measure example quality, text identity, or localization coverage.

The repository already owns
`contracts/common/documentation.schema.json`. That family-neutral contract
defines stable documentation IDs and canonical English text. The OpenAPI plan
must reuse its identity rules and suffix conventions where they fit. It must
not create an unrelated key format.

`api/openapi-main.yaml` and `api/components/` are the authored OpenAPI sources.
`api/openapi.yaml`, generated clients, and files below package `generated/`
directories are projections. Authors must not edit those projections.

## Scope

### In scope

- OpenAPI `info`, tag, server, operation, parameter, request, response, schema,
  property, enum-description, and example prose.
- Stable text identity for each localizable OpenAPI object.
- Named request, success-response, and relevant failure-response examples.
- An OpenAPI prose and example coverage inventory.
- Deterministic extraction of canonical English resources.
- Locale catalogs and localized documentation projections.
- Explicit locale fallback and translation coverage reports.
- Prose checks for authored OpenAPI text.
- Example validation against the bundled contract.
- Focused API-owned behavioral tests for high-value runnable examples.
- Publication of approved locale artifacts through the supported API package.

### Out of scope

- Changing REST paths, methods, operation IDs, payload fields, status codes, or
  runtime behavior to improve the prose.
- Localizing JSON keys, enum literals, error codes, identifiers, media types,
  code samples, paths, or other machine text.
- Adding runtime `Accept-Language` behavior to the product API.
- Translating runtime error payloads unless a separate product contract
  approves localized runtime messages.
- Hand-editing generated OpenAPI, Go, TypeScript, or package artifacts.
- Rewriting CLI help or packaged Markdown in this plan.
- Publishing a machine translation as an approved customer locale.
- Claiming that automated checks prove full ASD-STE100 conformance.
- Moving OpenAPI components only to satisfy a directory layout.

## API reference writing standard

The accepted customer technical-writing standard remains the normative prose
owner. The rules below specialize that standard for API reference text.

### Operation summary

An operation summary must state the action and primary resource. Use one short
phrase. Start with an imperative verb when the renderer presents the summary as
an action.

Good example:

```text
Submit Work to one Factory Session
```

Weak example:

```text
Work endpoint
```

### Operation description

An operation description must answer these questions when they apply:

1. When does the customer use this operation?
2. Which resource does the operation read or change?
3. What prerequisite or selection rule applies?
4. Is the operation idempotent?
5. What result confirms success?
6. Which common failure requires a different next action?

Use short descriptive paragraphs. Put one topic in each paragraph. Put a
necessary condition before the action or result that depends on it.

Do not repeat every schema field in the operation description. Link the
operation to reusable parameter and schema descriptions through the OpenAPI
contract. Use `x-doc-id` only when a longer task guide owns necessary detail.

### Parameters and fields

A parameter or property description must state its meaning and relevant
constraint. It must not only repeat the field name or type.

Describe units, bounds, default behavior, omission behavior, and identity
scope when they affect correct use. Keep validation language consistent with
the runtime and schema.

### Responses

A response description must state the observable outcome. It must not only
repeat the HTTP status phrase.

Failure descriptions must state the relevant cause class and safe next action.
Do not promise a retry when the runtime does not support one.

### Streams and asynchronous operations

Stream descriptions must state ordering, replay scope, cursor behavior,
termination behavior, and recovery behavior. Use separate short paragraphs or
a compact list. Do not compress the complete stream contract into one long
sentence.

Asynchronous operation descriptions must identify the accepted result, the
poll or stream path, terminal states, and safe request-ID reuse behavior.

### Protected machine text

The prose checker and translators must preserve these values exactly:

- HTTP methods and paths;
- operation IDs and component names;
- parameter and property names;
- JSON and YAML keys;
- enum values, status values, and error codes;
- media types, event names, and schema versions;
- shell commands, URLs, file paths, and identifiers; and
- literal request and response payloads.

Natural-language comments, example titles, and example explanations remain
localizable. A literal runtime message inside an example remains protected.

### Review duties

Every migrated family requires two reviews:

- A language reviewer checks the accepted writing profile and terminology.
- A subject-matter reviewer checks API meaning and example behavior.

Automated checks support these reviews. They do not replace them.

## Example standard

### Required operation examples

Each operation must have the smallest useful example set for its behavior.
The default requirement is:

- one minimal request example when the operation accepts a request;
- one representative success response example when the response has content;
- one relevant failure example when recovery is not obvious; and
- one stream-frame sequence for each public stream protocol.

An operation can omit an example only with a recorded reason. Valid reasons
include an empty response body or a binary payload that has a documented
metadata example. The inventory must report every omission.

### Example shape

Use named OpenAPI `examples` entries for operation use cases. Do not use array
position as example identity. Each example must have:

- a stable machine name;
- a concise localizable title;
- a localizable explanation when the title is not sufficient;
- the smallest payload that demonstrates the behavior;
- values that satisfy the referenced schema; and
- safe placeholder data with no credentials or host-specific paths.

Use a schema-level `example` only for a context-independent representative
value. Use operation-level named examples for customer tasks, edge cases, and
request-response pairs.

### Runnable samples

The first renderer should show a `curl` sample for HTTP operations. Generate
the command from the method, path, parameter values, media type, and canonical
request example. Do not maintain a second handwritten payload inside the code
sample.

Later renderers can add language samples from the same structured example.
Generated samples must use `http://localhost:7437` unless the operation needs a
documented different server.

### Example validation

Structural validation must prove that each example matches its referenced
schema. Behavioral validation must cover the highest-value customer journeys
through the public HTTP boundary.

Behavioral tests must not assert that every example value is a fixed runtime
result. Tests must distinguish deterministic contract fields from generated
identifiers, timestamps, ordering data, and environment-dependent values.

## Stable text identity and localization contract

### Separate link identity from text identity

Keep `x-doc-id` for a link to a longer canonical guide. Do not use it as a
translation key. Its current slash-based value also does not match the common
documentation ID format.

Add `x-text-id` as the stable item identity for one localizable OpenAPI object.
Use the lowercase item-ID grammar from
`contracts/common/documentation.schema.json`.

Example:

```yaml
operationId: submitWorkBySessionId
x-text-id: api.operation.submit-work-by-session-id
summary: Submit Work to one Factory Session
description: Submit one Work item to the selected live Factory Session.
```

The extraction tool derives field IDs from the item identity:

```text
api.operation.submit-work-by-session-id.title
api.operation.submit-work-by-session-id.description
```

OpenAPI `summary` maps to the common documentation `title` role. OpenAPI
`description` maps to the `description` role. This preserves the existing
`.title` and `.description` suffix rules.

### Identity rules

Every text ID must meet these rules:

- Use a public domain owner and resource meaning.
- Do not include a locale, source path, line number, array position, or English
  wording.
- Do not depend only on the current HTTP route.
- Remain stable when prose changes or a component file moves.
- Remain stable when a renderer changes.
- Never reuse a removed ID for a different meaning.
- Record a successor when a public text resource is replaced.
- Use one canonical owner for reused components.

Recommended item-ID shapes are:

```text
api.info
api.tag.factory-sessions
api.operation.submit-work-by-session-id
api.parameter.session-id
api.response.bad-request
api.schema.submit-work-request
api.schema.submit-work-request.property.name
api.schema.factory-session-status.value.running
api.operation.submit-work-by-session-id.example.minimal-request
```

The examples show the grammar. The accepted identity registry must define the
complete domain names before broad tagging starts.

### Enum text

OpenAPI 3.0 represents enum values as literals. The repository currently uses
positional `x-enum-descriptions` lists. Add an ID map keyed by the exact enum
literal. Do not identify enum text by list position.

Example:

```yaml
x-enum-text-ids:
  RUNNING: api.schema.factory-session-status.value.running.description
```

The validator must require exact agreement between enum literals, enum
descriptions, and enum text IDs. A changed enum order must not change identity.

### Example text

Each named Example Object receives its own `x-text-id`. The extractor derives
`.title` and `.description` IDs when those fields exist. The example payload
does not become a translation resource.

### Canonical English

Canonical English stays in the authored OpenAPI YAML. Locale catalogs must not
duplicate canonical English. The extractor creates a deterministic English
resource projection from the bundled OpenAPI for tooling and package users.

The extraction result must record:

- text ID;
- canonical English value;
- content role;
- OpenAPI item identity;
- source location in the authored tree when available; and
- a source digest for stale-translation detection.

### Locale catalogs

Approved translations live in domain-owned catalogs under
`api/locales/<bcp47>/`. Use canonical BCP 47 tags. Split large catalogs by the
same public domains used for text IDs.

Each translation record contains the text ID and translated value. It may also
contain the reviewed canonical source digest. It must not contain a second
copy of the English source text.

The locale registry must state:

- locale tag and customer-facing name;
- fallback locale;
- review status;
- included domain catalogs;
- translated, stale, and missing counts; and
- text direction when a renderer needs it.

### Fallback and stale translations

Canonical English is the required fallback. Apply fallback per complete text
resource. Do not concatenate translated and English fragments.

When canonical English changes, the source digest marks the translation stale.
The default development check reports stale translations. A release policy
decides whether an incomplete locale can ship. The renderer must identify
fallback coverage in its build report.

### Localized OpenAPI projection

The canonical `api/openapi.yaml` remains the English machine contract and the
source for Go and TypeScript generation. Client generation must never consume
a localized projection.

A focused documentation generator applies one approved locale catalog to the
canonical bundle. It changes only localizable prose fields. It preserves every
path, method, operation ID, schema, example payload, extension, and contract
value.

The API package can publish the canonical English resource catalog, the locale
registry, approved translation catalogs, and localized reference bundles. Add
public exports only after package contract and consumer tests define them.

## Canonical ownership

| Concern | Canonical owner | Notes |
| --- | --- | --- |
| English REST reference text | `api/openapi-main.yaml` and `api/components/` | Authored source. |
| Stable documentation grammar | `contracts/common/documentation.schema.json` | Extend its reusable definitions when needed. |
| OpenAPI text extension contract | A new contract under `contracts/api/` | Defines `x-text-id`, enum ID maps, catalogs, and compatibility rules. |
| Translation catalogs | `api/locales/<bcp47>/` | Approved human translation only. |
| English resource projection | Generated from bundled OpenAPI | Not an authoring source. |
| Localized OpenAPI bundles | Generated from English plus locale catalogs | Documentation only. |
| REST operation coverage inventory | `internal/contractinventory` and its command | Extend the existing deterministic inventory. |
| Published API artifacts | `packages/api/generated/` | Generated projection. |
| Product terminology | `docs/architecture/data-model.md` and the planned term register | Public vocabulary authority. |

Repository tooling owns extraction, validation, and generation. This work does
not add a product service or a `pkg/wire` dependency.

## Coverage contract

Extend the REST inventory or add one closely related documentation inventory.
The inventory must report these facts for every operation:

- stable operation text ID;
- summary and description IDs;
- request example names and IDs;
- success response example names and IDs;
- failure response example names and IDs;
- missing example reasons;
- linked guide ID when present;
- unresolved or duplicate text IDs;
- prose-check findings; and
- locale coverage by approved locale.

It must also report component-level text identity for reusable parameters,
responses, schemas, properties, enum values, and examples.

The report must be deterministic. Required CI must reject duplicate IDs,
missing IDs in the accepted scope, stale translations, invalid examples, and
new prose violations.

Use an exact temporary baseline during migration. The baseline must permit only
deletion during ordinary work. It must reach zero before this program closes.

## Implementation planning standard

Each implementation story must deliver one customer-visible API use case or
one tightly bounded enabling contract. Do not split ordinary work into separate
prose, ID, example, and test stories for the same operation family.

### Migration unit

One content migration task covers one of these units:

- one operation family with approximately three to eight related operations;
- one complete stream contract;
- one reusable component family with no more than approximately 2,000 prose
  words; or
- one localization-tooling behavior with direct fixtures.

Each operation-family task must update its descriptions, text IDs, examples,
coverage record, and focused tests together.

### Required task facts

Before editing a family, the implementer must record:

- the customer task;
- the canonical service and data-model owner;
- current request, response, status, and failure behavior;
- existing examples and tests;
- protected machine values;
- linked long-form guide, if any; and
- required language and subject-matter reviewers.

### Required acceptance criteria

Each content story must prove:

- a customer can identify when to use the operation;
- a minimal valid request is available when applicable;
- a representative success result is available;
- relevant recovery guidance is available;
- every localizable text has a stable unique ID;
- example payloads validate against the bundled schema;
- high-value runnable examples match HTTP behavior;
- public terminology matches the data model and term register;
- generated artifacts are current; and
- language and subject-matter review are recorded.

### Delivery boundary

Every story continues through required generation, focused tests, terminal
green CI, resolution of blocking feedback, conflict resolution, and actual PR
merge. Opening a PR or reaching green CI without merge is not completion.

## Work stories

### Story 1: Authors can identify and check every API text resource

#### Observable behavior

An author can add OpenAPI prose with one documented stable identity format.
The repository reports duplicate, malformed, missing, or orphaned IDs at the
authored source location.

#### Acceptance criteria

- The OpenAPI text extension and locale catalog contracts are documented and
  schema-validated.
- The contract reuses the common documentation item-ID grammar and title and
  description suffixes.
- `x-doc-id` remains a separate guide-link identity.
- The checker covers operations, parameters, responses, schemas, properties,
  enums, and named examples.
- Generated and external-reference text is not treated as an authored source.
- Duplicate IDs, source-derived IDs, malformed enum maps, and missing required
  IDs have direct fixtures.
- The initial exact baseline is reproducible and deletion-only.

### Story 2: Documentation builders can render one locale safely

#### Observable behavior

A documentation builder can select an approved locale and receive a localized
OpenAPI reference with field-level English fallback and a coverage report.

#### Acceptance criteria

- Canonical English is extracted deterministically from authored OpenAPI.
- Locale catalogs use canonical BCP 47 tags and domain-owned files.
- A test-only non-default locale fixture proves replacement, fallback, stale
  source detection, and right-to-left metadata handling.
- Localized projection changes only localizable prose fields.
- Client generation continues to use only canonical `api/openapi.yaml`.
- Repeated generation is byte-identical.
- Missing and stale translation counts appear in a deterministic report.
- No machine-translated locale is published as approved test evidence.

### Story 3: Customers can submit, list, inspect, and move Work

#### Observable behavior

A customer can use the Work operation family to submit one item, upsert a Work
Request, list Work, read one item, stage a file, and request a move.

#### Acceptance criteria

- Each Work operation states selection, identity, mutation, and idempotency
  behavior where applicable.
- Minimal request and success examples use one coherent Factory Session and
  Work scenario.
- The upsert example explains safe `request_id` reuse.
- List examples explain filters, ordering, counts, and pagination without
  internal token vocabulary.
- Move examples identify conflict behavior and a safe next action.
- Every localizable Work text and named example has a stable ID.
- Examples validate and one submit-to-read HTTP scenario passes.

### Story 4: Customers can start and operate a Factory Session

#### Observable behavior

A customer can choose open, synchronous, or asynchronous execution, then read
results or apply a supported lifecycle control.

#### Acceptance criteria

- The family explains when to use open, synchronous, and asynchronous paths.
- Examples show accepted, running, partial, and terminal result relationships.
- Lifecycle examples explain valid preconditions and conflict outcomes.
- Request-ID examples preserve the current retry and conflict contract.
- Artifact and dispatch examples use stable identities from one coherent
  scenario.
- Every localizable Factory Session text has a stable ID.
- Examples validate and focused session HTTP contract tests pass.

### Story 5: Customers can consume event and response streams

#### Observable behavior

A customer can connect, process retained and live records, reconnect with a
cursor, detect a retention gap, and stop at the documented terminal condition.

#### Acceptance criteria

- Factory Events, response events, and Worker Session events remain distinct.
- Each stream uses short sections for ordering, cursor, retention, terminal,
  and recovery behavior.
- Named frame-sequence examples preserve exact event and payload literals.
- Reconnect and stale-cursor examples identify the next safe request.
- `x-doc-id` links only to the longer canonical stream guide.
- Every localizable stream text and frame explanation has a stable ID.
- Stream examples validate and focused replay/reconnect tests pass.

### Story 6: Customers can validate and inspect Factory definitions

#### Observable behavior

A customer can preview or validate a Factory, read or save the Current
Factory, and inspect packaged Factory entries with clear examples.

#### Acceptance criteria

- Preview, validation, read, and save descriptions use public Factory terms.
- Examples distinguish a Factory definition from a live Factory Session.
- Validation examples pair one invalid input with its actionable result.
- Save examples state replacement and conflict behavior.
- Packaged Factory examples preserve localized product metadata as data. They
  do not confuse that runtime data with reference-page localization.
- Every localizable definition text has a stable ID.
- Examples validate and focused Factory definition API checks pass.

### Story 7: Customers can inspect Workers, Providers, and Models

#### Observable behavior

A customer can inspect Worker Sessions and Provider Sessions, list or read
Models, invoke a Model, and understand managed-runtime readiness and pull
results.

#### Acceptance criteria

- Worker Session and Provider Session remain separate customer concepts.
- Model examples state capability and readiness prerequisites.
- Binary Model results include metadata guidance without embedding unsafe
  binary values in YAML.
- Provider and Worker Session examples protect provider-issued identifiers.
- Every localizable family text has a stable ID.
- Examples validate and focused inspection and model contract tests pass.

### Story 8: Every public component explains its role

#### Observable behavior

A customer following an operation into a schema, property, enum, parameter, or
response receives concise and consistent reference text.

#### Acceptance criteria

- Every public reusable component has a stable text identity.
- Every required property description explains meaning or constraint.
- Enum descriptions use exact literal-keyed identities.
- Shared errors state cause classes and safe recovery without promising
  unsupported behavior.
- Internal implementation terms do not define public resources.
- The component prose baseline reaches zero.
- Generated Go and TypeScript contracts remain behaviorally unchanged.

### Story 9: Package consumers can load localized reference artifacts

#### Observable behavior

A package consumer can resolve the canonical English resources, locale
registry, approved catalogs, and approved localized OpenAPI bundles through
documented package exports.

#### Acceptance criteria

- Package exports use stable subpaths and data-only artifacts.
- The manifest records locale, digest, source version, and fallback metadata.
- Tarball tests prove the exact artifact allowlist.
- An isolated Node consumer resolves and reads every declared locale artifact.
- English and localized bundles have identical machine contracts.
- Unsupported internal package paths remain undocumented and unexported.
- API package candidate and publication checks pass.

### Story 10: The API reference baseline reaches zero

#### Observable behavior

Required CI rejects missing reference text, missing examples, invalid examples,
missing IDs, stale translations, and new writing violations across the complete
accepted public API scope.

#### Acceptance criteria

- Every operation and public component is migrated or has an approved bounded
  exclusion.
- Every exclusion has a reason, owner, and expiry.
- The temporary documentation baseline is empty.
- The English resource extraction and all approved locale projections are
  deterministic.
- The operation inventory has no undocumented example omission.
- Required full-scope checks are blocking in CI.
- A customer task study confirms that representative users can select, call,
  and verify the high-value operations.

## Dependency-aware delivery order

1. Accept the customer-writing standard and product terminology register.
2. Accept the OpenAPI text-ID, catalog, fallback, and compatibility contracts.
3. Extend the inventory and add the exact migration baseline.
4. Add canonical English extraction and localized projection fixtures.
5. Migrate the Work operation family.
6. Migrate Factory Session execution and lifecycle operations.
7. Migrate stream operations as one separate high-risk family.
8. Migrate Factory definition and validation operations.
9. Migrate Worker Session, Provider Session, and Model operations.
10. Migrate reusable component families in domain-sized slices.
11. Publish approved locale artifacts after package contracts exist.
12. Remove the baseline and enable full-scope blocking.

The domain-first component structure plan and this plan both affect OpenAPI
source references. Do not perform broad source moves and broad prose migration
in parallel. Stable text IDs must survive any accepted file move.

## Verification

Use the narrowest checks for each story, then run the public-contract gates.

Focused checks must include:

- schema validation for text metadata and locale catalogs;
- unit tests for ID parsing, uniqueness, extraction, fallback, and stale
  translation detection;
- fixture tests for every localizable OpenAPI object kind;
- contract validation for every request and response example;
- deterministic generation and drift tests;
- prose checking against canonical English fields;
- at least one non-default test locale path;
- focused HTTP behavior tests for high-value runnable examples; and
- package consumer tests when public exports change.

Repository gates must include, as applicable:

```text
node scripts/run-quiet-api-command.js validate:main ./api/openapi-main.yaml
make generate-api
make api-smoke
make interfaces-all
make api-package-verify
make verify-fast
make verify-pr
make lint
```

Run generation through supported Make targets. Review all generated diffs. Do
not edit generated files directly.

## Project-level acceptance criteria

- Every public operation explains purpose, prerequisites, success, and relevant
  recovery behavior.
- Every applicable operation has a minimal request, representative success
  response, and relevant failure example.
- Every public localizable text has one stable unique ID.
- Text IDs remain stable across wording changes, renderer changes, and source
  file moves.
- `x-doc-id` remains the identity for a linked long-form guide only.
- Canonical English remains in authored OpenAPI source.
- Locale catalogs contain translations by stable ID and do not duplicate the
  English source corpus.
- Field-level fallback, stale translation detection, and coverage reporting are
  deterministic.
- Localized projections do not change any machine contract.
- Public prose follows the accepted customer-writing profile and data-model
  vocabulary.
- Every example validates against the bundled OpenAPI schema.
- High-value examples have direct public HTTP behavior evidence.
- The temporary API documentation baseline is empty.
- Generated contracts and package artifacts are current.
- Required CI is terminal and passing, blocking feedback and conflicts are
  resolved, and every implementation PR is merged.

## Risks and controls

| Risk | Control |
| --- | --- |
| Simplified prose changes API meaning | Require subject-matter review and preserve contract tests. |
| Text IDs follow source layout | Require explicit semantic IDs and move-stability fixtures. |
| `x-doc-id` gains two incompatible meanings | Keep guide-link and text-resource identities separate. |
| English exists in two canonical places | Keep English in OpenAPI and generate the English resource catalog. |
| Translations become stale silently | Record source digests and report stale resources. |
| Localized specs feed client generation | Keep localized output in a documentation-only generation path. |
| Example payloads drift from schemas | Validate every example during API smoke. |
| Example payloads expose secrets | Use reviewed safe fixtures and a secret-pattern check. |
| Large prose changes create review fatigue | Limit each task to one operation or component family. |
| File moves conflict with content migration | Sequence domain moves and prose work. Preserve semantic IDs. |
| A checker is treated as STE certification | Keep the conformance claim bounded and require human review. |

## Work-story task packets

### Task packet: OpenAPI text identity and coverage contract

# problem statement

OpenAPI text resources have no complete stable identity or coverage contract,
so authors cannot prepare the reference for localization safely.

## customer ask

Give every localizable API reference resource a stable identity and report
missing, duplicate, malformed, or orphaned resources.

## solution

Define the OpenAPI text extension and locale contracts. Extend the REST
documentation inventory and add an exact migration baseline.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\api-reference-prose-examples-localization.md`

# changes

## package changes

- Add the focused API documentation contract under `contracts/api/`.
- Extend `internal/contractinventory` and its command with documentation
  coverage.
- Add the exact deletion-only baseline under `contracts/testdata/baseline/` or
  the accepted baseline owner.

## contracts

- Define `x-text-id`, enum text-ID maps, example text identity, locale catalog,
  fallback, source digest, and compatibility rules.

## services

- None. This is deterministic repository tooling.

## API changes

- Add documentation-only OpenAPI extensions. Do not change runtime contracts.

## tests

- Add schema, extraction, uniqueness, malformed-ID, duplicate-ID, orphan,
  enum-map, baseline, and deterministic-order fixtures.

### Task packet: localized OpenAPI documentation projection

# problem statement

Stable text IDs alone do not let a renderer apply translations or report
fallback and stale resources.

## customer ask

Build a localized API reference from an approved locale catalog without
changing the machine contract or generated clients.

## solution

Extract canonical English resources and generate a documentation-only localized
OpenAPI projection with field-level fallback and coverage reporting.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\api-reference-prose-examples-localization.md`

# changes

## package changes

- Add a focused extraction and projection command under `cmd/` with pure logic
  in a small repository-local package.
- Add locale registry and test-only locale fixtures.
- Add supported Make targets for generation and drift checking.

## contracts

- Preserve every non-prose OpenAPI value exactly.
- Define locale selection, fallback, source digest, and coverage output.

## services

- None. The generator is build-time tooling.

## API changes

- None to runtime REST behavior or canonical generated clients.

## tests

- Add projection equality, fallback, stale-resource, right-to-left metadata,
  deterministic generation, and machine-contract equivalence tests.

### Task packet: one operation-family prose and example migration

# problem statement

One public operation family does not give customers clear purpose, minimal
examples, expected results, recovery guidance, or stable text identities.

## customer ask

Use the selected operation family without guessing request shape, result
meaning, or safe recovery behavior.

## solution

Rewrite and tag one family as a vertical customer-behavior slice. Add named
request and response examples and validate them through the public contract.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\api-reference-prose-examples-localization.md`

# changes

## package changes

- Update the selected operations in `api/openapi-main.yaml`.
- Update only the reusable component fragments owned by that family.
- Regenerate all affected API and package projections.

## contracts

- Preserve paths, methods, operation IDs, parameters, status codes, media
  types, schemas, payload fields, and runtime behavior.
- Add stable text IDs and validated named examples.

## services

- None unless a separate approved behavior defect requires a service change.

## API changes

- Improve documentation metadata and examples only.

## tests

- Run prose, ID, example-schema, OpenAPI, generation, and focused public HTTP
  behavior tests for the family.

### Task packet: one reusable component-family migration

# problem statement

One reusable schema, parameter, enum, or response family lacks clear prose or
stable localization identities.

## customer ask

Follow an operation into its reusable components and understand field meaning,
constraints, literals, and recovery behavior.

## solution

Migrate one domain-owned component family. Add stable identities and concise
descriptions without changing the machine schema.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\api-reference-prose-examples-localization.md`

# changes

## package changes

- Update one bounded component family under `api/components/`.
- Regenerate affected OpenAPI, client, schema, and package projections.

## contracts

- Preserve component keys, property names, types, requirements, enum literals,
  constraints, and discriminator behavior.
- Add stable item and enum-value text identities.

## services

- None.

## API changes

- Improve component documentation only.

## tests

- Run prose, ID, enum-map, example, bundled-contract, generated-client, and
  focused domain contract checks.

### Task packet: publish approved locale artifacts

# problem statement

Package consumers cannot discover or load approved localized API reference
artifacts through supported exports.

## customer ask

Install the API package and resolve the locale registry, approved catalogs, and
localized reference bundles without inspecting package internals.

## solution

Add reviewed data-only exports and deterministic manifest metadata for approved
locale artifacts.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\api-reference-prose-examples-localization.md`

# changes

## package changes

- Update `packages/api` authored package metadata and README.
- Generate approved locale artifacts and manifest records.
- Update candidate and publication helpers when required.

## contracts

- Define stable package subpaths, locale metadata, artifact digests, fallback,
  and compatibility behavior.

## services

- None. The package remains data-only.

## API changes

- Add package artifact exports only. Do not change runtime REST behavior.

## tests

- Add package contract, tarball allowlist, installed-consumer, digest,
  candidate, and publication reconciliation tests.
