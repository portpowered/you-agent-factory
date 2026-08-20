# Provider Harness Capability Catalog Plan

## Status

Proposed.

## Problem statement

Customers cannot reliably compare model and execution-harness pairings because
the Providers catalog richly describes only the three native integrations,
while packaged ACP integrations have launch commands but no authored model,
modality, tool, evidence, or protocol-support facts.

## Customer ask

Publish a complete, machine-readable inventory that helps customers choose a
provider harness and model based on:

- whether the harness uses ACP or a native adapter;
- the input and output modalities the harness can carry;
- the modalities supported by each known model;
- the tools the harness provides out of the box or through configuration;
- the difference between static documented support, an unknown fact, and a
  capability observed from a live installed ACP agent; and
- the complete set of ACP harnesses the product intentionally supports, with each
  harness owning enough implementation detail to differ from every other ACP
  harness.

The initial must-prove examples supplied with the request are:

- Antigravity/Gemini can accept video and audio through MP4 files and can
  generate images.
- Codex accepts image input and can generate images.
- Grok CLI supports video and image generation.
- ACP support differs by harness and must be explicit.
- Qwen Code and applicable Qwen models support video input.

These examples are acceptance fixtures to verify against primary documentation
and executable conformance evidence; the catalog must not turn an unverified
claim into `supported` merely because it appeared in a plan.

## Current repository findings

The repository already contains much of the required foundation:

- `ProviderManifest` supports exact model IDs, directional text/image/audio/
  video facts, transports, named tools, known limits, support posture, and
  discovery prerequisites.
- `packages/model-providers/providers/` authors rich manifests for
  `antigravity`, `claude`, and `codex` and generates the public catalog and
  schemas.
- `you providers list --json` exposes models, modalities, tools, and limits for
  descriptors that contain those facts.
- `pkg/services/providers/internal/services/builtins/wire/catalog.json`
  contains 20 packaged ACP launch entries.
- The official [ACP Registry](https://github.com/agentclientprotocol/registry)
  can help discover candidate agents and basic distribution metadata, but its
  published format intentionally lacks the model, modality, tool, evidence,
  execution-policy, and adapter detail You needs. It cannot be the product
  source of truth.

The principal gaps are:

1. Native provider metadata and ACP launch metadata have separate authored
   sources.
2. ACP descriptors are synthesized with one generic optimistic capability set;
   their models, modalities, tools, limits, support posture, and evidence are
   empty.
3. `supported` and `unsupported` exist, but `unknown` and `conditional` do not.
   An empty model or tool list therefore cannot state whether the harness has
   none, uses runtime discovery, or has not been researched.
4. Model modalities currently represent an end-to-end provider/model fact but
   cannot independently express what the harness can transport and what the
   selected model can understand or emit.
5. `file_path` cannot explain relevant constraints such as accepted media
   types, containers, resource-link delivery, or tool-mediated output.
6. Tool support cannot distinguish built-in/default tools from optional,
   user-configured, external, or unknown tools.
7. The CLI `show` projection, HTTP mapping, and MCP schema expose less metadata
   than the domain descriptor and CLI list JSON.
8. ACP initialize negotiation is retained internally after a successful run,
   but it is not represented as a separate observed capability layer in
   customer discovery.

## Intended outcome

The Providers service remains the single identity, catalog, selection, and
execution authority. Each provider descriptor explicitly describes the
execution harness, its static capability evidence, its model catalog posture,
and any separately observed runtime facts. Generated packages, CLI output, HTTP
mapping, and MCP tools project the same information without launching a
provider during ordinary catalog discovery.

A customer can answer questions such as:

- “Which harness/model pairings can accept video input?”
- “Which of those also have a built-in image-generation tool?”
- “Is this harness ACP-based, natively adapted, or only cataloged?”
- “Is the claim documented, live-observed, conditional on the selected model,
  or still unknown?”
- “What installation command or prerequisite applies on my platform?”

The product will filter for compatible pairings and explain uncertainty; it
will not claim to rank subjective model quality.

## Scope

### In scope

- Extending the existing provider manifest and Providers-owned descriptor.
- Unifying native and packaged ACP metadata behind the generated provider
  catalog.
- Making each directory below `packages/model-providers/providers/` the
  complete authored source for one native or ACP harness, including its public
  facts and harness-specific runtime inputs.
- Migrating every ACP harness the product intentionally ships into that package
  structure and adding new ACP harnesses through the same contribution path.
- An optional maintainer tool that reads the ACP registry to scaffold partial
  candidate metadata; generated candidates have no authority until reviewed
  and completed as provider packages.
- Preserving all existing provider identities, aliases, and persisted worker
  configuration behavior.
- Full, deterministic CLI JSON and human projections, including a compatibility
  query for model/harness selection.
- Aligning provider package schemas/types, Providers domain projection, HTTP
  and MCP mappings, generated artifacts, docs, and focused tests.
- Optional live inspection of an installed ACP harness, with observations kept
  separate from static authored claims.

### Out of scope

- Automatically downloading or installing arbitrary binary ACP agents.
- Automatically trusting every capability statement in third-party marketing
  or the ACP registry.
- Ranking providers by quality, price, speed, or benchmark score.
- Treating a model-family name as proof that every model in that family has the
  same modalities.
- A dashboard picker in the first delivery lane. The API/types must permit a
  later UI without introducing UI-owned provider truth.
- Renaming Providers to Harnesses or adding a parallel harness registry.
- Treating ACP-registry membership as a requirement for inclusion, support, or
  removal.

## Canonical data and ownership

| Concern | Canonical owner | Notes |
| --- | --- | --- |
| Provider identity, aliases, selection, descriptor, and one-attempt execution | `pkg/services/providers` | No second harness catalog or registry. |
| Authored public manifest vocabulary | OpenAPI component fragments under `api/components/schemas/model-providers/` | `api/openapi.yaml` and language clients remain generated. |
| One harness's complete authored definition | `packages/model-providers/providers/<provider-id>/` | The directory owns public metadata, harness kind, runtime launch/adapter configuration, capabilities, evidence, and harness-local fixtures. |
| Supported provider inventory | The set of valid provider package directories | A provider exists because You authors and reviews its package, not because a remote registry lists it. |
| Optional external discovery metadata | Maintainer-time import output, initially from the ACP registry | Scaffolding input only; never read by runtime, normal generation, or CI as authority. |
| Public generated provider catalog | `packages/model-providers/generated/catalog.json` | Generated; never hand-edited. |
| Runtime native/ACP projection | Generated from provider package directories | Replaces separately authored command-only or adapter-registration inventories where practical. |
| Static customer discovery | Providers Catalog service | Side-effect-free and marked `unverified` for machine readiness. |
| Live ACP observations | Providers ACP service and a Providers-owned inspection result | Never overwrite the authored static maximum or become durable truth without a separately approved persistence design. |

### Provider package shape

Each provider directory is a small package boundary, not merely one row in a
global catalog. A package may contain:

```text
packages/model-providers/providers/<provider-id>/
  provider.yaml              # public identity, support, models, modalities, tools
  harness.yaml               # discriminated native_cli/acp execution definition
  evidence.yaml              # bounded evidence records referenced by public facts
  README.md                  # maintainer notes and primary-source links
  testdata/                  # sanitized handshake/output/capability fixtures
```

The exact file split may be simplified when a provider is small, but the
directory is the ownership boundary and generator input. A provider-specific
harness definition may express different commands, flags, environment rules,
session behavior, output fidelity, permission behavior, model discovery, and
ACP quirks without widening one global ACP record into a lowest-common-
denominator schema. Shared schema vocabulary describes comparable customer
facts; provider-local files preserve the implementation detail needed to
execute and verify that harness.

The generator discovers package directories, validates each package as a
cohesive unit, and produces detached public and runtime projections. No global
overlay is allowed to silently change an individual provider package's
capabilities or execution behavior.

### Harness-specific runtime ownership

The provider package also owns the declarative binding to the Providers
runtime implementation that executes it. The common contract should be a
discriminated envelope, not one flattened ACP configuration:

```yaml
implementation:
  kind: native_adapter | acp_agent
  profile: qwen-acp
  launch:
    command: qwen
    arguments: [--acp]
  promptContent:
    video: resource_link
    audio: resource_link
  modelDiscovery: session_config
  sessionResume: acp_load_session
  permissions: acp_request_permission
```

The profile name is a validated binding, not a service locator. Concrete
execution code remains owned by `pkg/services/providers` and constructed by
`pkg/wire`; the provider package supplies the data and selects an already
registered typed implementation. Construction must fail when an executable
provider package references no implementation or when an implementation has no
owning provider package.

Provider packages may differ in:

- executable/package-runner and platform-specific launch shapes;
- prompt attachment encoding, supported MIME types, and file/resource-link
  behavior;
- model discovery and model-selection mechanisms;
- session creation, resume, and reconnect behavior;
- permission/authentication handling;
- response-fidelity and tool-event normalization;
- default timeouts, working-directory behavior, and environment requirements;
  and
- harness-local conformance fixtures and known limits.

Reusable ACP protocol transport remains shared. Provider-specific behavior is
expressed by typed profiles or focused adapters, never by inferring behavior
from the provider name and never by a global map outside the provider package
generation path.

## Proposed contract

The exact field names should be finalized in the OpenAPI review, but the
contract must preserve the following semantics.

### 1. Harness integration facts

Add a provider-level harness object:

```yaml
harness:
  kind: native_cli | acp
  transports: [stdio]
  acp:
    support: supported | unsupported | unknown
    protocolVersions: [1]
    initialization: retained_daemon | per_attempt
  launch:
    posture: bundled | package_runner | installed_executable | catalog_only
    platformSupport: [darwin-aarch64, linux-x86_64, windows-x86_64]
  externalCatalogReferences:
    - kind: acp_registry
      id: qwen-code
```

Requirements:

- ACP support is a typed fact, never inferred from an `-acp` suffix.
- External catalog references are optional provenance/discovery links. They do
  not define the canonical ID, launch behavior, support level, or capability
  facts. Existing IDs such as `cursor-acp`, `gemini-acp`, and `droid-acp`
  remain accepted.
- A provider package can be present as `catalog_only` when its harness
  implementation is intentionally documented but cannot yet be launched by
  the current local execution contract.
- Static catalog readiness remains `unverified`; a packaged command is not
  evidence that its executable or authentication is ready.

### 2. Explicit support state and evidence

Extend modality, tool, and protocol facts to use:

```text
supported | unsupported | conditional | unknown
```

`conditional` must name the condition, normally the selected model, provider
account feature, platform, or operator configuration. `unknown` is a positive
data value and must never be collapsed into an absent array or `unsupported`.

Add bounded evidence records and references:

```yaml
evidence:
  - id: qwen-video-docs-2026-08
    kind: primary_documentation | protocol_probe | conformance_fixture | maintainer_assertion
    url: https://...
    verifiedAt: 2026-08-10
    harnessVersion: 1.2.3
```

Every `supported`, `unsupported`, or `conditional` modality and every supported
tool must cite at least one evidence record. `unknown` requires no evidence.
The generator validates evidence references, dates, and bounded descriptions;
it does not make network calls.

### 3. Harness and model modalities

Keep model modalities, and add the harness-level routes needed to determine
end-to-end compatibility:

```yaml
harnessModalities:
  - direction: input
    modality: video
    support: supported
    mechanisms: [file_path]
    mediaTypes: [video/mp4]
    fileExtensions: [.mp4]
    evidenceRefs: [agy-video-fixture]
```

The route vocabulary must distinguish at least:

- inline content;
- file path;
- ACP resource link or embedded content, if implemented;
- tool-mediated output; and
- none.

Model records retain directional modalities and gain the same support/evidence
semantics. Add a provider-level model-catalog posture:

```text
exact | runtime_discovered | operator_selected | unknown
```

An empty `models` array therefore has an explicit meaning. A model/harness
pairing supports a modality only when both layers support a compatible route.
If either layer is `unknown`, the effective result is `unknown`; if either is
`unsupported`, the effective result is `unsupported`; conditional facts retain
their explanation.

Image generation must not be misrepresented as native image output when it is
available only through a tool. In that case, the harness declares an
image-generation tool whose output modality is image and the model's direct
output modality remains truthful.

### 4. Tool availability

Extend named tools beyond `supported`/`unsupported`:

```yaml
tools:
  - name: image_generation
    support: supported
    availability: built_in | optional | operator_configured | external | unknown
    enabledByDefault: true | false | null
    outputModalities: [image]
    description: Generate an image through the harness-provided tool.
    evidenceRefs: [codex-imagegen-docs]
```

Use stable provider-neutral tool names for matching, with optional native names
as aliases. The initial vocabulary should include the existing `filesystem`,
`shell`, and `web_search` names and add only evidenced categories needed by the
seed providers, such as `image_generation`, `video_generation`, `browser`, and
`mcp`. Do not claim tools inherited only from the host Codex environment as
provider-harness defaults unless the shipped integration actually supplies
them.

### 5. Static versus observed ACP capabilities

Static manifest facts answer what a maintained provider version is evidenced
to support. A live inspection result answers what one installed process
negotiated at a point in time:

```yaml
observed:
  source: acp_initialize
  observedAt: 2026-08-10T12:34:56Z
  protocolVersion: 1
  harnessVersion: 1.2.3
  capabilities: {...}
```

Plain `you providers list` remains inert. A separate explicit inspection
operation may launch the provider and must report authentication, executable,
timeout, and protocol failures without mutating the static catalog. The
operation must advertise only ACP fields actually present in the protocol;
model modality and tool claims still require their own evidence when ACP does
not negotiate them.

### 6. Custom ACP integrations

Operator-configured ACP entries continue to work. Unless the operator also
selects a known catalog identity, their model catalog, modalities, and tools are
`unknown`. The runtime must remove the current generic ACP descriptor that
optimistically claims image input, streaming, reasoning, tools, file changes,
plans, and usage for every custom command.

## Provider package and optional ACP import policy

1. The set of valid directories below `packages/model-providers/providers/` is
   the complete supported inventory. Normal generation walks those directories
   and has no network dependency.
2. Move the current packaged ACP launch entries into individual provider
   packages. Their runtime commands, aliases, protocol posture, modality/tool
   facts, and tests must be owned by the same package.
3. A new provider is added by contributing a complete provider package and
   passing its schema, evidence, projection, and harness conformance checks.
   ACP-registry membership is neither necessary nor sufficient.
4. Provide an optional maintainer command such as `you-dev providers import-acp`
   that fetches or accepts an ACP-registry document and emits candidate package
   scaffolds or a comparison report into a temporary/output directory.
5. Imported fields are restricted to facts the external source actually owns,
   such as display name, website, repository, distribution, and suggested
   command. Capability, support, readiness, and implementation fields are
   emitted as unknown/TODO and require local review.
6. The import tool never edits existing provider packages, never makes a
   provider selectable, never updates generated artifacts, and is not invoked
   by ordinary builds or CI.
7. Existing canonical IDs and aliases remain resolvable in worker configs,
   Factory definitions, recordings, and provider-session references. An
   optional external catalog reference can document a related upstream entry
   without changing You's identity.
8. Local packages remain supported or deprecated according to You's evidence
   and maintenance policy. No package is added, changed, or removed solely
   because a remote registry changed.

## Customer surfaces

### Catalog listing and detail

- `you providers list` shows a compact deterministic summary suitable for the
  expanded inventory: identity, harness kind, support posture, readiness,
  model-catalog posture, and high-level known modality/tool counts.
- `you providers show <id>` becomes the complete human detail view and includes
  every fact present in JSON.
- `you providers list --json` and `you providers show <id> --json` emit the
  complete typed descriptor with explicit empty arrays and explicit unknown
  states.
- `you workers list` continues to focus on selectable execution integrations;
  catalog-only entries remain discoverable through `you providers` without
  pretending they can run.

If changing the current verbose human `list` output is judged too disruptive,
retain it for compatibility and add a compact flag first. JSON field additions
must follow the repository's public-contract compatibility policy.

### Pair compatibility query

Add a pure, side-effect-free Providers operation and CLI projection, for
example:

```text
you providers match --input video --output image --tool image_generation
you providers match --interface acp --input video --json
```

Matching uses AND semantics across requested constraints and returns each
candidate harness/model pair with:

- `compatible`, `conditional`, `unknown`, or `incompatible` outcome;
- the harness route and model fact used for each modality;
- tool availability and whether it is built in;
- evidence references; and
- reasons for conditional, unknown, or rejected candidates.

The operation does not rank model intelligence or silently exclude unknown
facts. Default human output may show compatible and conditional candidates;
JSON includes all evaluated candidates and reasons.

### Programmatic consumers

- Extend the published model-provider schemas and generated TypeScript types.
- Keep Providers root values detached and clone-safe.
- Bring the existing Providers HTTP list/get mapping and MCP list/get schemas
  to full descriptor parity.
- If REST provider discovery is promoted to the public OpenAPI paths, add it as
  a separately reviewed API behavior with generated Go/TypeScript clients and
  contract tests; do not expose an undocumented transport-only shape.
- A later dashboard consumes generated API types and projections. It must not
  import or recreate the catalog as UI-owned state.

## Implementation stories

### Story 1: Publish a truthful provider-harness capability contract

As a catalog consumer, I can distinguish harness capabilities, model
capabilities, built-in tools, ACP support, conditions, unknown facts, and
evidence so that an absent fact is never mistaken for support.

Acceptance criteria:

- OpenAPI component fragments define harness kind, ACP metadata, model-catalog
  posture, support states, modality routes/media types, tool availability, and
  evidence records.
- The generated Provider Manifest and Provider Catalog schemas reject duplicate
  facts, dangling evidence references, unsupported route/support combinations,
  unqualified conditional facts, and direct/tool-mediated output
  contradictions.
- `antigravity`, `claude`, and `codex` migrate without losing their current
  exact model IDs, efforts, limits, prerequisites, or response-fidelity facts.
- The five seed examples are represented only after a primary source or
  checked conformance fixture verifies them.
- Generated OpenAPI, Go, TypeScript, provider catalog, and package artifacts are
  current and are not hand-edited.

Evidence:

- Schema/generator unit tests for every enum and invalid combination.
- Golden catalog tests for supported, unsupported, conditional, and unknown
  modality/tool cases.
- Existing provider projection and CLI capability tests updated to prove no
  fact loss.

### Story 2: Package every supported ACP harness without breaking identities

As a customer, I can discover every ACP harness the product intentionally supports,
with its own implementation and capability definition, while only verified
launch mappings are selectable.

Acceptance criteria:

- Every current packaged ACP entry is migrated into exactly one provider
  package directory with its launch behavior, aliases, protocol posture,
  runtime implementation profile, capability facts, evidence, and
  harness-local fixtures.
- All 20 current packaged ACP identities continue to resolve, either as stable
  canonical IDs or documented aliases, and persisted worker configuration does
  not require migration.
- A newly supported ACP harness is added only through a reviewed provider
  package; an optional import scaffold is insufficient to publish it.
- A package whose execution integration is incomplete can be intentionally
  visible as `catalog-only` with unknown capability facts and actionable
  documentation/prerequisites.
- Selectable ACP entries derive their runtime command projection from the same
  provider package used for discovery.
- Build-time validation rejects selectable packages with missing runtime
  implementation bindings and registered provider implementations with no
  owning package.
- Ordinary list/get construction does not fetch an external catalog, launch an
  agent, inspect the filesystem, or open a network connection.

Evidence:

- Provider-package discovery, completeness, and generated-projection drift
  tests.
- Table-driven legacy ID and alias resolution tests.
- Catalog/runtime projection parity tests.
- Harness-profile registration and provider-local conformance fixtures for
  attachment mapping, model selection, session behavior, and response fidelity
  where those behaviors differ.
- A functional `root.BuildProcess` providers-list test that proves zero provider
  command executions.

### Story 3: Expose one complete descriptor across CLI, HTTP, MCP, and custom ACP

As a customer or automation author, I receive the same provider-harness facts
from every supported discovery surface.

Acceptance criteria:

- CLI list/show human and JSON outputs include harness, model-catalog,
  modality, tool, evidence, support-posture, readiness, and prerequisite facts
  with deterministic ordering.
- Providers HTTP mapping and MCP list/get result schemas include the complete
  descriptor rather than the current reduced identity/capability subset.
- A configured custom ACP command is selectable but reports unknown models,
  modalities, and tools instead of the generic optimistic ACP capabilities.
- Static discovery reports readiness `unverified` until an explicit probe or
  execution produces observed readiness facts.
- Unknown, conditional, unsupported, and supported values survive every
  projection unchanged.

Evidence:

- Domain-to-CLI, domain-to-HTTP, and domain-to-MCP contract tests over the same
  fixture.
- JSON-schema validation of CLI/MCP/API examples where applicable.
- Functional CLI discovery tests for native, packaged ACP, catalog-only, and
  custom ACP entries.

### Story 4: Match model and harness pairings by requested capabilities

As a customer choosing an execution configuration, I can ask which
harness/model pairs satisfy required modalities and tools and see why uncertain
or incompatible candidates do not fully match.

Acceptance criteria:

- A pure Providers operation evaluates harness routes, model facts, tool
  availability, and model-catalog posture without I/O.
- The CLI supports ACP/native, input modality, output modality, and tool
  constraints with documented AND semantics.
- A tool-mediated image output does not satisfy a request for direct model
  image output unless the request permits tool-mediated output.
- Dynamic or unknown model catalogs return conditional/unknown candidates
  rather than fabricated model IDs or false incompatibility.
- The seed cases produce the expected evidenced candidates after their facts
  are verified.

Evidence:

- Table-driven pure matching tests covering supported, conditional, unknown,
  unsupported, route mismatch, tool-mediated output, and dynamic models.
- CLI human/JSON contract tests with deterministic result and explanation
  ordering.
- One functional match invocation through `root.BuildProcess`.

### Story 5: Inspect live ACP negotiation without corrupting static truth

As an operator, I can explicitly inspect an installed ACP agent and compare its
negotiated protocol capabilities with the static catalog before assigning real
work.

Acceptance criteria:

- Inspection is an explicit operation; ordinary list, show, and match remain
  side-effect-free.
- The result records protocol version, observed timestamp, sanitized harness
  version when available, and negotiated ACP capabilities.
- Timeout, missing executable, authentication, malformed protocol, and version
  mismatch outcomes are typed and actionable without leaking command lines,
  credentials, or raw stderr.
- Observed facts are returned separately and never rewrite authored capability
  metadata.
- ACP fields not negotiated by the protocol remain static or unknown; the
  inspection does not infer video/model/tool support from unrelated flags.

Evidence:

- ACP command-runner edge fixtures for success and each failure class.
- Concurrency coverage proving inspection cannot race or corrupt an in-flight
  retained daemon/session.
- Structured safe-operation logging tests.

### Story 6: Make catalog maintenance and customer guidance durable

As a maintainer and customer, I can add or update provider packages
reproducibly and understand how to interpret support, unknown facts, tools, and
pair matching.

Acceptance criteria:

- An optional ACP-registry import command emits partial candidate scaffolds or
  a comparison report without modifying canonical provider packages.
- Offline package validation and generation checks prove that provider package
  definitions and all generated public/runtime projections agree.
- The model-provider package README/support guide and `you docs providers` /
  `you docs harnesses` describe the source/evidence model, catalog-only entries,
  live observations, pairing query, and custom ACP unknown behavior.
- Documentation includes verified Antigravity, Codex, Grok, and Qwen examples
  without claiming capabilities broader than the evidence.
- Generated catalog/schema/package files and runtime ACP projection remain in
  sync after a clean generation run.

Evidence:

- Import-tool unit tests over a frozen ACP-registry fixture prove that only
  basic discovery/distribution fields are copied and all unsupported facts
  remain unknown/TODO.
- Provider-package discovery and generation tests run without network access.
- `make docs-reference-smoke` and model-provider package verification.
- A manual review of human CLI output at the full inventory size on Windows and
  one Unix-like platform.

## Delivery sequencing

1. Story 1 establishes semantics and migrates the current rich native entries.
2. Story 2 moves ACP inventory into the canonical generation path and preserves
   compatibility.
3. Story 3 exposes the completed data consistently and removes optimistic
   custom-ACP defaults.
4. Story 4 adds side-effect-free compatibility matching once catalog truth is
   trustworthy.
5. Story 5 adds explicit live observation as a separate, higher-risk behavior.
6. Story 6 completes maintenance automation and customer guidance; its docs
   should evolve alongside earlier stories and be finalized after behavior is
   stable.

Stories 1-3 form the minimum useful release. Story 4 is the customer-selection
outcome. Story 5 may ship separately because it introduces process and
concurrency risk; it is not a prerequisite for truthful static discovery.

## API and generated-artifact changes

Expected authored contract changes include:

- `api/components/schemas/model-providers/ProviderManifest.yaml`
- `ProviderModel.yaml`, `ProviderModality*.yaml`, and `ProviderTool*.yaml`
- new harness, evidence, model-catalog-posture, route, availability, and
  pairing-result component fragments
- the matching component references in `api/openapi-main.yaml`

Expected generated or projected outputs include:

- `api/openapi.yaml`
- generated Go/TypeScript OpenAPI types when the components are consumed there
- `packages/model-providers/generated/*.json`
- `packages/model-providers/types/*.d.ts`
- `packages/model-providers/generated/catalog.json`
- the runtime packaged ACP launch projection
- CLI generated command manifests if new `match` or `inspect` commands are
  added

The implementation must use the repository generation targets and must not
hand-edit these outputs.

## Test and verification plan

Use the narrowest useful checks during each story, then the public/shared gates
before merge:

```text
go test ./internal/providercatalog/...
go test ./packages/model-providers/...
go test ./pkg/services/providers/...
go test ./pkg/transports/cli/...
go test ./tests/functional/providers/...
make generate-api
make provider-catalog-generate
make provider-catalog-check
make model-provider-package-generate
make model-provider-package-check
make docs-reference-smoke
make api-smoke                 # when a public REST surface changes
make verify-fast
make verify-pr
make build-all
```

Required behavioral coverage:

- deterministic offline generation and drift detection;
- complete modality matrix validation for text/image/audio/video in both
  directions, including unknown and conditional values;
- harness/model route intersection and tool-mediated outputs;
- duplicate/colliding provider package IDs, external references, and aliases;
- every valid provider package accounted for exactly once in public and runtime
  projections;
- all legacy ACP IDs still resolving;
- no process or network side effects from static discovery/matching;
- custom ACP facts remaining unknown;
- live inspection failure classification, cancellation, safe logging, and
  concurrency isolation;
- CLI human/JSON, HTTP, and MCP projection parity; and
- generated schema/package/runtime projection parity.

Functional tests must construct through `root.BuildProcess` and execute through
`Process.Execute`, prefer CLI entry for customer flows, replace provider
process effects through `edges.Edges` command-runner seams, and avoid sleeps in
favor of deterministic fixtures.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Capability facts age quickly | Require evidence date/version, make unknown explicit, and review provider packages independently. |
| A global schema collapses meaningful harness differences | Make the provider directory the package boundary and use discriminated harness definitions plus provider-local fixtures. |
| External registry churn leaks into the product | Treat imports as optional scaffolding only; normal generation reads provider packages and remains offline. |
| Provider IDs collide with external catalog IDs | Store optional external references separately and preserve local canonical IDs/aliases. |
| Harness support is confused with model support | Keep separate harness routes and model modalities; derive pairing results by intersection. |
| Image generation is confused with direct image output | Represent tool-mediated output on the tool and require match callers to allow it. |
| Large human output becomes unusable | Make `show` the complete detail view and provide filtering/matching while retaining compatible list behavior. |
| Generic custom ACP claims remain misleading | Default custom entries to unknown and add optional live inspection. |
| A remote import becomes a production dependency | Restrict network access to an explicit maintainer tool; generated/runtime code reads provider packages only. |

## Project-level acceptance criteria

- The Providers service remains the only canonical catalog/selection/execution
  owner.
- Every valid directory below `packages/model-providers/providers/` appears
  exactly once in the generated public catalog and, where executable, exactly
  once in the runtime registration projection.
- Provider package directories, not the ACP registry or a global overlay, own
  harness-specific launch, capability, model, tool, evidence, and conformance
  facts.
- Existing ACP identities and aliases continue to resolve from persisted
  configurations and Factory definitions.
- Customers can distinguish native versus ACP harnesses, harness versus model
  modality support, direct versus tool-mediated output, built-in versus
  configured tools, and static versus observed facts.
- Unsupported, conditional, and unknown are never collapsed into one another.
- Plain discovery and matching are side-effect-free.
- CLI JSON, published package types/catalog, HTTP mapping, and MCP schemas agree
  on the descriptor contract.
- Required focused checks, generated-artifact checks, `verify-pr`, and relevant
  package/API/docs gates are terminal and passing.
- Delivery continues through blocking review feedback, conflict resolution,
  terminal green CI, and actual PR merge. Opening a PR or reaching green CI
  without merge is not completion.

## Work-story task packets

Each implementation task submitted to You Agent Factory workers must use the
repository task template. The packets below are the minimum independently
reviewable units; an implementation PR may split a packet further but must not
combine unrelated cleanup.

### Task packet: capability contract

# problem statement

Provider manifests cannot distinguish harness support, model support,
conditional/unknown facts, tool availability, or evidence.

## customer ask

Publish truthful model/harness modalities, tools, and ACP support for automated
and human provider selection.

## solution

Extend the authored OpenAPI provider-manifest vocabulary, generator semantics,
and native manifests with separate harness/model facts and evidence.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\provider-harness-capability-catalog-plan.md`

# changes

## package changes

- Extend authored provider schemas and `internal/providercatalog` validation.
- Migrate current native provider manifests and regenerate package artifacts.

## contracts

- Add harness kind/ACP metadata, support states, modality routes, model-catalog
  posture, tool availability, and evidence records.

## services

- Extend the Providers descriptor/projection without adding a new registry.

## API changes

- Regenerate OpenAPI and generated consumers from component fragments.

## tests

- Add schema, semantic-validation, projection, and native golden tests.

### Task packet: provider-packaged ACP inventory

# problem statement

The command-only ACP catalog is incomplete and separate from public provider
capability truth.

## customer ask

Discover all ACP harnesses You intentionally packages without breaking current
provider names or pretending every entry is executable.

## solution

Migrate every shipped ACP harness into its own provider package and derive both
public descriptors and runtime launch mappings from those package directories.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\provider-harness-capability-catalog-plan.md`

# changes

## package changes

- Add harness/evidence files, typed runtime-profile bindings, and provider-local
  fixtures to the current ACP provider packages.
- Add provider-package discovery/generator checks and optional ACP import
  scaffolding.
- Replace the independently authored runtime ACP inventory with a generated
  projection.

## contracts

- Add discriminated ACP harness internals, optional external catalog
  references, distribution/platform, and catalog-only posture without changing
  Providers canonical identity ownership.

## services

- Project ACP descriptors, typed implementation-profile bindings, and launch
  registrations from the same provider package while keeping execution code in
  the Providers service.

## API changes

- Publish every provider-packaged ACP descriptor through the existing catalog
  contract.

## tests

- Add provider-package completeness, runtime-profile registration,
  harness-local conformance, drift, alias compatibility, no-side-effect,
  optional-import, and projection parity tests.

### Task packet: discovery parity and custom ACP truth

# problem statement

CLI show, HTTP, and MCP omit rich provider facts, and custom ACP integrations
receive unsupported optimistic claims.

## customer ask

Consume the same truthful capability metadata from every discovery surface.

## solution

Project the complete Providers descriptor everywhere and make unresearched
custom ACP capability fields explicitly unknown.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\provider-harness-capability-catalog-plan.md`

# changes

## package changes

- Extend CLI list/show renderers and JSON shapes.
- Extend HTTP mappings and MCP schemas/results.

## contracts

- Preserve every support/evidence state across projections with deterministic
  ordering and explicit empty arrays.

## services

- Replace generic custom-ACP capability synthesis with unknown facts.

## API changes

- Align existing provider discovery transports; add public OpenAPI paths only
  through a separately reviewed API contract if required.

## tests

- Add shared-fixture CLI/HTTP/MCP parity and functional custom-ACP discovery
  coverage.

### Task packet: provider pairing match

# problem statement

Raw metadata still forces customers to manually intersect harness routes,
model modalities, and tool availability.

## customer ask

Identify harness/model pairings that satisfy required modalities and tools.

## solution

Add a pure Providers pairing evaluator and a deterministic `you providers
match` CLI projection with explicit uncertainty and rejection reasons.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\provider-harness-capability-catalog-plan.md`

# changes

## package changes

- Add Providers match request/result values and pure evaluation logic.
- Add CLI manifest, handler, human output, and JSON output.

## contracts

- Define constraints, AND semantics, result status, evidence, and direct versus
  tool-mediated output behavior.

## services

- Expose matching through the Providers service root; perform no I/O.

## API changes

- Add MCP/HTTP match only if those transports are part of the accepted customer
  behavior for the story.

## tests

- Add table-driven evaluator, CLI contract, and one `root.BuildProcess`
  functional test.

### Task packet: live ACP inspection

# problem statement

Static metadata cannot prove what one installed ACP process negotiates, while
ordinary discovery must remain inert.

## customer ask

Explicitly inspect installed ACP protocol support before assigning work.

## solution

Add a separate Providers inspection operation that reports sanitized live ACP
observations without rewriting static catalog truth.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\provider-harness-capability-catalog-plan.md`

# changes

## package changes

- Add inspection request/result values, CLI surface, and safe failure mapping.

## contracts

- Separate static facts from timestamped ACP initialize observations.

## services

- Reuse the injected ACP service/command edges and retained negotiation state;
  do not add a second process manager.

## API changes

- Expose inspection only on explicitly accepted transports and document its
  process side effect.

## tests

- Add command-runner fixtures, failure classification, cancellation,
  concurrency, and safe logging tests.
