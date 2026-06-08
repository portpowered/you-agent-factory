# PRD: Contract Validation Policy Stub

## Introduction

Dynamic workflows need a first-pass contract for how JavaScript and TypeScript workflow sources are accepted, resolved, validated, bounded by policy, and projected into results before the full runtime is implemented. This work defines the observable validation and policy behavior that later API, CLI, MCP, website, fake-service, and runtime batches can rely on.

The concrete change is to establish the contract for workflow source validation, JavaScript/TypeScript loader expectations, safe read-only MVP policy defaults, source lookup order, artifact URI conventions, structured JSON result constraints, and policy-denied capability diagnostics. The intent is to let customers submit or reference a workflow source and receive consistent accept/reject, preview, and result-shape behavior without granting unrestricted filesystem, shell, network, connector, or binary-output access.

## Context

The customer ask references `docs/temp/dynamic-workflows-plan.md` for Dynamic Workflows Batch 001. This slice depends on the shared data model contract from `contract-data-model-orchestrators`; it does not build the full JavaScript runtime, dispatcher, UI workflow runner, or live provider integration.

Concrete problem:

- Later dynamic workflow work needs stable rules for what source forms are valid and how source names resolve.
- JavaScript workflows must fail before session creation when they use unsupported globals, invalid metadata, invalid schemas, or forbidden host access.
- TypeScript workflow files need explicit MVP loader behavior so customers know whether they can run `.ts` sources directly, through a placeholder transpile path, or through a clear not-supported diagnostic.
- Policy defaults must be safe and bounded before agents, artifacts, and preview flows fan out.
- Result and artifact contracts must prevent large or binary payloads from being embedded in JSON results.

High-level solution:

- Define validation behavior for inline and file-backed JavaScript workflow sources, including source-location-aware diagnostics.
- Define TypeScript loader expectations for `.ts` workflow files and unsupported TypeScript features in the MVP.
- Resolve requested workflow names and files through one ordered lookup contract across API, CLI, MCP, and website.
- Apply a read-only default policy with bounded agents, concurrency, depth, retries, budgets, runner/model/profile allowlists, denied network/connectors/danger-full-access, and artifact-root checks.
- Require structured-cloneable JSON-compatible workflow results and artifact URIs for non-text, binary, or large outputs.

## Project-Level Acceptance Criteria

- [ ] Workflow validation rejects invalid JavaScript source, invalid `meta`, unsupported globals, unsupported primitive calls, invalid schemas, forbidden host access, and incompatible policy requests before session creation or preview approval.
- [ ] `.js` and `.ts` workflow source handling has explicit MVP behavior, including successful JavaScript loading, bounded TypeScript loader/transpile expectations, and clear diagnostics for unsupported TypeScript or module features.
- [ ] API, CLI, MCP, and website normalization use the same source lookup order for workflow names and file refs: project `.claude/workflows`, user workflows, package-relative workflows, built-in/global JavaScript factories, then explicit factory lookup when requested.
- [ ] The effective default policy is read-only, bounded, stable-hashable, included in previews/session creation responses, and rejects denied capabilities before runtime execution.
- [ ] Workflow results are structured JSON-compatible values; large, non-text, image, audio, binary, and diagnostic outputs are represented through `WorkContent` artifact refs or `you-artifact://sessions/{session_id}/artifacts/{artifact_id}` URIs.
- [ ] Quality gate: generated contracts are refreshed where required, typecheck passes, lint passes, and focused backend/frontend/contract tests pass.

## Goals

- Give implementers a single contract for validating workflow source before execution.
- Make TypeScript workflow support explicit enough to avoid ambiguous customer behavior in the MVP.
- Keep all source resolution surfaces aligned across API, CLI, MCP, and website.
- Default workflow execution to safe local read-only behavior with bounded fan-out and denied host capabilities.
- Make artifact and result projections safe, structured, and reusable across session result APIs, events, CLI output, MCP responses, and UI viewers.
- Provide direct test evidence for validation, policy, source resolution, artifact URI, and structured-result behavior.

## User Stories

### contract-validation-policy-stub-001: Reject invalid or unsafe workflow source before session creation
**Description:** As a workflow author, I want invalid or unsafe workflow source rejected with actionable diagnostics before a run starts so that runtime failures and unsafe host access are prevented early.

**Acceptance Criteria:**
- [ ] Inline and file-backed JavaScript workflow validation rejects syntax errors before a factory session is created.
- [ ] Invalid `meta`, invalid `args` schema, unsupported globals, unsupported primitive call shapes, and forbidden direct filesystem/process/network access each produce path-aware or source-location-aware validation diagnostics.
- [ ] Validation accepts the supported MVP workflow globals and primitives: `meta`, global structured `args`, `phase`, `log`, `workflow.log`, `workflow.artifact`, `workflow.final`, `agent`, `agent.run`, `parallel`, and `pipeline`.
- [ ] Validation does not execute workflow source or invoke child agents while producing diagnostics.
- [ ] Focused backend or contract tests cover one valid source, one syntax error, one invalid metadata/schema case, and one forbidden host-access case.
- [ ] Typecheck passes.
- [ ] Tests pass.

### contract-validation-policy-stub-002: Define JavaScript and TypeScript loader expectations
**Description:** As a workflow author, I want `.js` and `.ts` workflow files to have explicit loading behavior so that Claude-compatible examples either run through supported syntax or fail with clear guidance.

**Acceptance Criteria:**
- [ ] `.js` workflow files load as the MVP executable source format and preserve source refs and source hashes in validation or preview responses.
- [ ] `.ts` workflow files are accepted only through the defined MVP TypeScript loader path, such as syntax-stripping/transpile placeholder behavior for supported TypeScript syntax, or rejected with a structured unsupported-loader diagnostic when the loader is not available.
- [ ] Unsupported Node/Bun module loading, package-manager access, arbitrary imports, and direct host APIs fail before runtime execution with diagnostics that name the unsupported capability.
- [ ] Source-map or wrapper line offsets are remapped so diagnostics refer to the customer-authored workflow file when practical.
- [ ] Tests cover a supported `.js` workflow, the defined `.ts` loader behavior, an unsupported import/module case, and source hash stability for unchanged source.
- [ ] Typecheck passes.
- [ ] Tests pass.

### contract-validation-policy-stub-003: Resolve workflow sources through one ordered lookup contract
**Description:** As an API, CLI, MCP, or website user, I want workflow names and file references to resolve consistently so that the same request runs the same source from every interface.

**Acceptance Criteria:**
- [ ] Source requests support the contract kinds `FACTORY_ID`, `FACTORY_INLINE`, `WORKFLOW_FILE`, `WORKFLOW_NAME`, and `INLINE_WORKFLOW`.
- [ ] Workflow name/file resolution uses this order unless an explicit source kind bypasses lookup: project `.claude/workflows`, `~/.you-agent-factory/workflows`, package-relative workflow directories, built-in/global named JavaScript factories, then explicit factory lookup when requested.
- [ ] Resolution responses expose resolved source kind, safe source ref, source hash, orchestrator kind/dialect, and a diagnostic when no source is found or multiple sources conflict.
- [ ] Artifact roots supplied with source requests are absolute, policy-checked, and outside the target repository by default.
- [ ] API, CLI, MCP, and website request normalization share the same observable lookup behavior and conflict diagnostics.
- [ ] Typecheck passes.
- [ ] Tests pass.

### contract-validation-policy-stub-004: Apply bounded read-only policy defaults and preview diagnostics
**Description:** As an operator, I want dynamic workflow previews and session starts to apply a safe default policy so that workflow fan-out and host effects are bounded unless explicitly permitted later.

**Acceptance Criteria:**
- [ ] Effective policy defaults to `mode: READ_ONLY`, `maxAgents: 16`, `concurrency: min(4, maxAgents)`, `maxDepth: 1`, `maxRetries: 0`, network disabled, connectors disabled, danger-full-access denied, empty writable roots, and output audit mode `AUTO`.
- [ ] Policy validation rejects zero or negative concurrency, concurrency above `maxAgents`, unbounded or excessive child limits above the deployment cap, unknown runners/models/reasoning efforts/route profiles/commands/sandbox modes, and writable roots under read-only mode.
- [ ] Read-only policy denies workspace-write workers, direct workflow filesystem writes, direct shell/process access, direct network access, connectors, and danger-full-access before runtime execution.
- [ ] Preview/session creation responses include effective policy, stable policy hash, max child count, max concurrency, runner/model/profile decisions, timeout/budget decisions, and denied capability diagnostics.
- [ ] Tests prove policy hashes are stable across map ordering and that denied capabilities fail closed before any runtime or dispatch side effect.
- [ ] Typecheck passes.
- [ ] Tests pass.

### contract-validation-policy-stub-005: Constrain workflow results and artifact URI projections
**Description:** As a result consumer, I want workflow outputs to be structured and artifact-backed so that API, CLI, MCP, events, and UI can safely render final and partial results.

**Acceptance Criteria:**
- [ ] Workflow `return` values and `workflow.final` values must be structured-cloneable JSON-compatible values; unresolved promises, functions, cyclic values, host handles, and unsupported binary blobs fail with clear diagnostics.
- [ ] A structured workflow result maps to `FactorySessionResult.primaryResult` using existing `WorkContent` JSON parts.
- [ ] Large, image, audio, binary, patch, log, checkpoint, and diagnostic outputs are represented as artifact refs or `you-artifact://sessions/{session_id}/artifacts/{artifact_id}` URIs instead of embedded raw bytes in JSON.
- [ ] Artifact URI parsing rejects malformed session ids, artifact ids, path traversal, host paths, and URIs that do not belong to the requested session.
- [ ] `SESSION_RESULT_UPDATED` events and `GET /factory-sessions/{session_id}/results` project the same result and artifact ids for a fixture workflow result.
- [ ] Typecheck passes.
- [ ] Tests pass.

### contract-validation-policy-stub-006: Surface validation and policy outcomes consistently across public interfaces
**Description:** As a maintainer, I want validation, preview, and policy outcomes to appear consistently across API, CLI, MCP, and website surfaces so that later runtime work can plug into a stable contract.

**Acceptance Criteria:**
- [x] API validation/preview/start responses expose the same validation issues, source resolution metadata, effective policy hash, artifact-root decisions, and structured-result constraints as the shared service contract.
- [x] CLI validation/preview/session-start output shows actionable source, loader, policy, and result-shape diagnostics without requiring users to inspect logs.
- [x] MCP validation/start tools return structured errors for unsupported source, loader, policy, artifact-root, or result capabilities.
- [x] Website run/preview flows have explicit loading, empty, error, and success states for validation and policy preview data, and display source resolution and denied capability diagnostics accessibly.
- [x] Direct browser verification covers the website validation/preview error and success states when browser-visible behavior changes.
- [x] Typecheck passes.
- [x] Tests pass.

## High-Level Technical Design

This work should define behavior at the contract and service-boundary layer. Source validation should parse first, produce diagnostics second, and avoid executing workflow source. Diagnostics should include stable issue codes, customer-readable messages, relevant source refs, and source locations when available.

Source inputs should normalize into one workflow-source request shape before API handlers, CLI commands, MCP tools, or website flows branch into transport-specific presentation. The normalized source should carry kind, original request, resolved safe ref, source hash, orchestrator kind, dialect, and any lookup diagnostics.

Policy resolution should combine requested policy, factory/default policy, and deployment limits into one effective policy. The effective policy should be stable-hashable, safe to include in previews, and used by validation before any runtime, dispatch, network, connector, filesystem, shell, or artifact side effect occurs.

Result projection should reuse `FactorySessionResult` and `WorkContent`. JSON-compatible workflow returns should map to JSON `WorkContent` parts. Outputs too large or unsuitable for JSON should be stored as `FactoryArtifact` records and referenced by scoped `you-artifact://sessions/{session_id}/artifacts/{artifact_id}` URIs.

Frontend state should treat generated API validation and preview responses as canonical. UI projection state should only render loading, empty, error, success, denied-capability, and resolved-source states; it should not duplicate validation or policy logic in browser-only code.

## Functional Requirements

- FR-1: The system must validate inline and file-backed JavaScript workflow sources before preview approval or session creation.
- FR-2: Validation must report syntax, metadata, schema, primitive, unsupported-global, forbidden-host-access, and policy-compatibility issues without executing the workflow.
- FR-3: The MVP must define observable `.js` and `.ts` loader behavior, including clear rejection for unsupported TypeScript, module, import, Node, or Bun features.
- FR-4: Source requests must support `FACTORY_ID`, `FACTORY_INLINE`, `WORKFLOW_FILE`, `WORKFLOW_NAME`, and `INLINE_WORKFLOW`.
- FR-5: Source lookup must use the shared ordered resolver across API, CLI, MCP, and website flows.
- FR-6: Effective policy must default to read-only, bounded fan-out, bounded concurrency, denied network/connectors/danger-full-access, no writable roots, stable policy hashes, and `AUTO` output audit mode.
- FR-7: Policy validation must reject unsupported runner, model, reasoning effort, route profile, command, sandbox mode, budget, concurrency, writable-root, and artifact-root requests before runtime execution.
- FR-8: Workflow final and partial results must be structured-cloneable JSON-compatible values or `WorkContent` artifact references.
- FR-9: Artifact URIs must use `you-artifact://sessions/{session_id}/artifacts/{artifact_id}` and must be scoped to the requested session.
- FR-10: Public validation, preview, session-start, result, event, CLI, MCP, and website responses must expose consistent source, policy, artifact, and result-shape diagnostics.
- FR-11: Generated Go and TypeScript contract artifacts must be refreshed when schema changes require it.
- FR-12: Tests must verify runtime-observable validation, policy, lookup, artifact URI, and result projection behavior without relying on meta-test inventories.

## Non-Goals

- Do not implement full JavaScript runtime execution, checkpointing, dispatch scheduling, provider calls, or live runner integration in this slice.
- Do not grant direct workflow filesystem, shell/process, network, connector, or package-manager access in the MVP.
- Do not implement broad Node/Bun module compatibility.
- Do not introduce workflow-specific canonical runtime nouns that bypass `FactorySession`.
- Do not require live provider credentials for validation, policy, resolver, or result contract tests.
- Do not add broad unrelated cleanup, package restructuring, or route inventory enforcement outside this behavior lane.

## Supporting Technical and UX Considerations

- Backend ownership should keep source parsing, source resolution, policy resolution, artifact hygiene, contract mapping, and runtime side effects isolated.
- Policy errors should fail closed and be visible in preview/start responses before any child dispatch or host side effect.
- Source refs, source hashes, policy hashes, artifact refs, and diagnostics should be safe to expose; raw source or diagnostic artifacts should follow the same redaction and artifact hygiene rules as other diagnostic content.
- CLI and MCP should use structured errors so host agents can decide whether to correct a workflow, ask for approval, or stop.
- Website validation/preview UI should use existing shared controls and status treatments, support keyboard navigation, and remain responsive on narrow screens.
- Browser-visible preview behavior should be directly verified only when this slice changes the website.

## Success Metrics

- A valid JavaScript workflow fixture can be validated and previewed from API, CLI, MCP, and website flows with the same resolved source hash and policy hash.
- Invalid source, unsupported loader behavior, forbidden host access, invalid policy, invalid artifact URI, and invalid result-shape fixtures fail before runtime execution with reviewer-verifiable diagnostics.
- A fixture result containing JSON and artifact refs projects identically through result API and result-updated event payloads.
- No MVP validation or preview path requires live provider credentials or grants direct workflow host access.

## Open Questions

- Should `.ts` MVP support be a minimal transpile/syntax-strip path immediately, or should `.ts` files return a clear unsupported-loader diagnostic until the runtime batch owns transpilation?
- Which deployment-level maximum should be the default absolute cap for planned children: the documented `1000` cap, or a lower product default for local development?
