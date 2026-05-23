# PRD: Work Content

## Introduction

Add a canonical `content` model for work items so infinite-you can represent plain text, text plus image attachments, and future media types without forcing every worker and graph edge to reinterpret opaque `payload` blobs differently. The first implementation slice should support `text` and `image` input content while preserving existing `payload` compatibility and current text-only workflows. The runtime should normalize new and legacy inputs into one internal representation, expose the right worker capabilities, and prove the behavior through contract, integration, and functional tests that reach the Codex runner boundary, including `-i` image arguments.

## Goals

- Define a first-class `content` contract for submitted and returned work items.
- Support text-only and text-plus-image inputs in a stable, ordered format.
- Preserve legacy `payload` submission compatibility while preferring `content` in new surfaces.
- Normalize work content once at runtime so downstream workers do not invent their own parsing rules.
- Allow Codex-backed execution to translate image content into runner `-i` arguments.
- Add regression coverage from API contract through runtime dispatch and functional runner-edge validation.

## User Stories

### US-001: Accept canonical content on work submission
**Description:** As an API user, I want to submit work with a first-class `content` field so mixed text and image inputs are represented explicitly instead of being hidden in opaque payload conventions.

**Acceptance Criteria:**
- [ ] The public work schema accepts an optional `content` field for submitted and returned work items.
- [ ] The first supported content part types are `text` and `image`.
- [ ] Text content can be submitted as a single part or multiple ordered parts.
- [ ] Image content can be submitted alongside text in a deterministic part order.
- [ ] Invalid `content` shapes are rejected with `400` validation errors that identify the bad field or part.
- [ ] OpenAPI examples include at least one text-only submission and one text-plus-image submission.
- [ ] Contract and generated-artifact checks confirm the new schema is reflected in generated surfaces.
- [ ] Typecheck/lint passes.

### US-002: Preserve legacy payload compatibility through normalization
**Description:** As an existing customer, I want current `payload` submissions to keep working so the content upgrade does not break text-only factories or clients.

**Acceptance Criteria:**
- [ ] Work submission still accepts legacy `payload` without requiring `content`.
- [ ] New runtime normalization prefers explicit `content` when present.
- [ ] Legacy string payloads normalize into one canonical text content part.
- [ ] Legacy payload-only workflows continue to render the same effective prompt text they produced before this change.
- [ ] Requests that send both `content` and `payload` follow a documented precedence rule and do not silently merge contradictory values.
- [ ] Unit or package integration tests cover legacy payload-only submission, explicit content-only submission, and mixed-field precedence behavior.
- [ ] Typecheck/lint passes.

### US-003: Preserve multi-input graph behavior with content-aware dispatch
**Description:** As a workflow author, I want multi-input workstations to preserve input identity and ordering when consuming content so fan-in and mixed-input workflows remain predictable.

**Acceptance Criteria:**
- [ ] A workstation consuming multiple work inputs retains each source input as a distinct input instead of flattening all content into one opaque blob before dispatch.
- [ ] Canonical content part ordering is preserved within each input work item.
- [ ] Existing input ordering rules remain deterministic across multi-input dispatches.
- [ ] Input work identity, trace lineage, and relations remain attributable after content normalization.
- [ ] Integration tests cover a workstation consuming separate text and image-bearing work items in one dispatch.
- [ ] Typecheck/lint passes.

### US-004: Translate image content into Codex runner arguments
**Description:** As an operator using Codex-backed workers, I want image content to reach the runner through the expected CLI boundary so image-aware executions behave the same in tests and production.

**Acceptance Criteria:**
- [ ] Codex-backed execution detects image content parts and translates them into runner `-i <FILE>` arguments in dispatch order.
- [ ] Text-only inputs continue to invoke the runner without `-i` arguments.
- [ ] Unsupported content types are not passed through to the runner in this first slice.
- [ ] Missing or unreadable image references fail with a clear worker-visible error before a malformed runner call is treated as successful.
- [ ] Functional tests validate the exact runner-edge behavior using mocks in the current functional-test style, including one case that asserts `-i` arguments are present and ordered correctly.
- [ ] Functional tests also validate the no-image case does not add unexpected `-i` arguments.
- [ ] Typecheck/lint passes.

### US-005: Surface content capability boundaries clearly
**Description:** As a developer, I want workers to fail predictably when a work item includes content they cannot consume so factories do not silently drop media.

**Acceptance Criteria:**
- [ ] Text-only execution paths reject unsupported image-bearing content with a clear, stable error outcome when no image-capable runner path is configured.
- [ ] Image-capable execution paths accept mixed text and image content without requiring workstation-specific ad hoc parsing.
- [ ] Error surfaces do not stringify or discard unsupported content silently.
- [ ] Unit or integration tests cover one accepted mixed-content path and one rejected unsupported-content path.
- [ ] Typecheck/lint passes.

### US-006: Return and persist content consistently
**Description:** As an API and replay user, I want work reads and runtime history to preserve canonical content metadata so submitted inputs can be inspected and replayed accurately.

**Acceptance Criteria:**
- [ ] `GET /work` and `GET /work/{id}` return canonical `content` for work items that were submitted with explicit content.
- [ ] Returned work items preserve part order and stable part fields needed for downstream inspection.
- [ ] Replay or event-history surfaces that persist work records keep enough content shape to reconstruct submitted text and image inputs without collapsing them into ambiguous strings.
- [ ] Regression tests cover a mixed-content work item surviving submit, storage, and readback without part loss or reordering.
- [ ] Contract or replay tests cover backward compatibility for older payload-only artifacts where relevant.
- [ ] Typecheck/lint passes.

## Functional Requirements

- FR-1: The system must define a first-class `content` field on work items that can represent ordered content parts.
- FR-2: The initial content part types must include `text` and `image`.
- FR-3: The system must preserve deterministic ordering of content parts inside each work item.
- FR-4: The system must continue accepting legacy `payload` submissions.
- FR-5: The system must normalize legacy `payload` and explicit `content` into one canonical internal representation before worker dispatch.
- FR-6: The system must document and enforce precedence rules when both `payload` and `content` are present.
- FR-7: Multi-input dispatch must preserve per-input work identity and deterministic input ordering after normalization.
- FR-8: Codex-backed runner execution must translate image content into `-i` CLI arguments in the same order the images appear in canonical content.
- FR-9: Text-only Codex submissions must not receive synthetic `-i` arguments.
- FR-10: The system must fail clearly when referenced image input cannot be materialized for runner execution.
- FR-11: Work read APIs must surface canonical `content` for content-aware submissions.
- FR-12: Generated contracts, handwritten runtime behavior, and replay/history surfaces must remain aligned.

## Non-Goals

- No first-slice support for audio input or audio output contracts.
- No first-slice support for arbitrary binary inline blobs as the main transport format.
- No requirement that every worker type immediately becomes image-capable.
- No redesign of all prompt template variables in the same milestone beyond the minimum compatibility needed for normalized content.
- No OCR, image understanding quality guarantees, or provider-specific vision benchmarking in this slice.

## Design Considerations

- `content` should be named generically and not as a modality-specific feature so the contract stays open to future text, image, audio, and file forms.
- Ordered parts matter. A user instruction followed by two screenshots is not interchangeable with two screenshots followed by a later note.
- The first screen of support should feel boring and explicit: a small number of clearly typed parts with predictable validation beats a permissive shape that every worker interprets differently.
- Existing text-only users should not need to learn the new field on day one.

## Technical Considerations

### Codebase Findings

- The public work contract in [api/components/schemas/data-models/Work.yaml](/Users/abdifamily/infinite-you/api/components/schemas/data-models/Work.yaml) currently exposes only opaque `payload`, which is flexible but does not declare content type, ordering, or media boundaries.
- Unary submission in [api/components/schemas/api/SubmitWorkRequest.yaml](/Users/abdifamily/infinite-you/api/components/schemas/api/SubmitWorkRequest.yaml) and batched submission through `WorkRequest` use the same work model, so adding `content` at the work-item layer can cover both ingress paths.
- The internal normalized submission surface in [pkg/interfaces/factory_runtime.go](/Users/abdifamily/infinite-you/pkg/interfaces/factory_runtime.go) still stores `SubmitRequest.Payload` as `[]byte`, while public `Work.Payload` is `any`. That split likely needs a canonical internal content representation or an adjacent normalized field.
- Prompt rendering in [pkg/workers/prompt.go](/Users/abdifamily/infinite-you/pkg/workers/prompt.go) currently converts token payload bytes directly to strings and joins them for default prompts, so normalized content must preserve current text behavior while avoiding accidental stringification of image-bearing inputs.
- Prompt-template documentation in [docs/reference/prompt-variables.md](/Users/abdifamily/infinite-you/docs/reference/prompt-variables.md) currently describes `.Inputs[N].Payload` as `string`, which will need compatibility guidance or adjacent content-aware fields once normalized content reaches worker prompts.
- Existing functional tests already use runner-edge mocks for command validation in `tests/functional/bootstrap_portability/...`, which is the right pattern for proving Codex runner argument translation without relying on live model providers.

### Suggested Implementation Shape

- Add optional `content` to the public work schema rather than overloading opaque `payload` with undocumented multimodal conventions.
- Represent `content` as an ordered list of typed parts. In the first slice, support `text` parts with inline text and `image` parts with a resolved file reference or other runtime-materializable reference.
- Introduce one canonical runtime content representation during submission normalization so worker adapters can consume stable part types instead of reparsing raw JSON.
- Keep legacy `payload` ingress behavior by normalizing string payloads into one text part and documenting how non-string legacy payloads are handled in this phase.
- Extend Codex runner argument shaping so image parts are materialized and passed as repeated `-i` arguments while text content continues to feed the main prompt path.
- Keep worker capability handling explicit. A worker path that only supports text should reject image-bearing content instead of silently dropping image parts.

### Testing Strategy

The change should be proven at the contract, normalization, integration, and functional layers.

#### Contract tests

- Validate text-only `content` examples.
- Validate mixed text-plus-image `content` examples.
- Reject invalid part shapes such as missing `type`, missing `text` on text parts, or missing image reference fields on image parts.
- Confirm generated server and client artifacts reflect the new schema.

#### Normalization and unit tests

- Legacy string `payload` normalizes to one text content part.
- Explicit `content` preserves part ordering exactly.
- Requests containing both `payload` and `content` follow the documented precedence rule.
- Unsupported or malformed image references fail with explicit normalization or dispatch preparation errors.

#### Integration tests

- Submit a text-only work item with `content` and verify readback preserves canonical content.
- Submit a mixed text-plus-image work item and verify stored work, dispatch preparation, and read APIs preserve ordered parts.
- Submit a multi-input workload where one input contains text content and another contains image content, then verify the workstation sees two distinct inputs with stable ordering and lineage.
- Validate one rejection case where a text-only execution path receives image-bearing content and fails clearly.

#### Functional tests

- Add a Codex-runner functional test that reaches the runner edge and asserts image content becomes ordered `-i <FILE>` arguments.
- Add a paired text-only functional test that asserts no `-i` arguments are added.
- Keep these tests mock-based at the current functional boundary, following the existing command-runner validation style used by current portability and provider-invocation tests.
- Validate the request still carries the expected prompt text while image arguments are attached separately.

#### Replay and persistence tests

- Verify mixed-content work survives submit, storage, and readback without part loss or reordering.
- Add compatibility coverage for payload-only historical behavior so record/replay and projections do not regress when older artifacts omit `content`.

## Success Metrics

- A customer can submit a text-only work item using `content` without breaking existing text-only behavior.
- A customer can submit text plus image work and have Codex-backed execution receive the expected `-i` runner arguments.
- Multi-input workflows continue to preserve distinct input identity and deterministic ordering after content normalization.
- Functional tests catch regressions at the runner edge when image arguments stop being passed correctly.
- Existing payload-only workflows continue to pass without requiring immediate migration.

## Open Questions

- What exact image reference form should first-slice `image` parts use: artifact path, workspace path, bundled-file reference, or a broader URI abstraction?
- Should non-string legacy JSON `payload` values normalize into text, structured JSON content, or remain payload-only until a later phase?
- Should explicit `content` fully override `payload`, or should certain payload-derived metadata still be retained for compatibility?
- How much of the worker prompt/template surface should expose content-aware fields in the first slice versus keeping that internal to runner adapters?
- When audio input and output arrive later, should they extend the same ordered part model directly or introduce a second higher-level abstraction for larger referenced media objects?
