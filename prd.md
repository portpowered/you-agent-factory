# PRD: Local Agent CLI Runtime Taxonomy Split

---
author: Codex
last modified: 2026, june, 15
status: draft
work item: batch-local-agent-cli-runtime-20260615-local-agent-cli-runtime-taxonomy-split
---

## Introduction

Introduce the runtime taxonomy split that separates one-shot inference, agent loops, scripts, and pollers in public factory configuration and CLI/API behavior. Existing docs and config still use `MODEL_WORKER` and `MODEL_INVOKE` as the default model execution vocabulary, which makes OmniVoice and other bounded model operations look like agent execution. This project introduces `INFERENCE_WORKER`, `AGENT_WORKER`, `SCRIPT_WORKER`, and `POLLER_WORKER` for worker capability, plus `INFERENCE_RUN`, `AGENT_RUN`, `SCRIPT_RUN`, and `POLLER_RUN` for workstation behavior, while preserving compatibility aliases for existing factories.

## Context

### Customer Ask

Introduce the new `INFERENCE_WORKER`, `AGENT_WORKER`, `SCRIPT_WORKER`, `POLLER_WORKER` and `INFERENCE_RUN`, `AGENT_RUN`, `SCRIPT_RUN`, `POLLER_RUN` vocabulary with compatibility aliases for existing factories. Treat the split as still unimplemented even though previous runtime taxonomy work items exist in factory inputs and failed live work state.

### Problem

`MODEL_WORKER` and `MODEL_INVOKE` conflate bounded inference operations with agentic execution. OmniVoice is real managed model behavior, but it is still modeled through the old model-worker mental model. Factory authors, CLI users, API clients, and dashboard users need vocabulary that makes the runtime behavior clear without forcing existing factories to migrate immediately.

### Solution

Add the new worker and workstation taxonomy to the public contract, config loader, validation, runtime dispatch, CLI/API projections, dashboard editing surfaces, docs, and examples. Keep `MODEL_WORKER` as a compatibility alias for inference-worker behavior, keep `MODEL_INVOKE` as a compatibility alias for inference-run behavior, and preserve existing hosted/poller compatibility while new factories prefer poller vocabulary. Use behavioral tests to prove load, validation, execution, event emission, save, UI, and docs outcomes.

## Project Acceptance Criteria

- [ ] Factory config, CLI/API payloads, generated Go types, and generated TypeScript types accept the new worker and workstation taxonomy while continuing to accept legacy aliases.
- [ ] Existing factories using `MODEL_WORKER`, `MODEL_INVOKE`, and hosted/poller compatibility vocabulary load, validate, execute, and save without customer edits.
- [ ] New factory creation, CLI/API output, dashboard labels, docs, and examples prefer `INFERENCE_WORKER`, `INFERENCE_RUN`, `AGENT_WORKER`, `AGENT_RUN`, `SCRIPT_WORKER`, `SCRIPT_RUN`, `POLLER_WORKER`, and `POLLER_RUN`.
- [ ] Validation rejects incompatible worker/workstation pairings before dispatch and explains failures with inference, agent, script, or poller terminology.
- [ ] OmniVoice default behavior is represented and tested as inference worker/run behavior, not as agent behavior.
- [ ] Dashboard graph projections and current-factory save flows preserve worker identity, workstation identity, relationships, and layout when loading new or legacy taxonomy values.
- [ ] Quality gate: generated contracts are current, typecheck passes, lint passes, focused backend/API/CLI/UI/docs tests pass, and changed browser-visible UI is verified in browser.

## Goals

- Make one-shot inference behavior first-class and clearly distinct from agent-loop behavior.
- Reserve `AGENT_WORKER` and `AGENT_RUN` for real agent execution semantics.
- Keep legacy factory compatibility during the migration window.
- Align OpenAPI, generated clients, config mapping, validation, runtime dispatch, events, CLI output, dashboard projections, docs, and examples.
- Avoid broad package reshaping unless it is required to deliver observable taxonomy behavior.

## User Stories

### local-agent-cli-runtime-taxonomy-split-001: Accept New Worker Taxonomy With Legacy Aliases

**Description:** As a factory author, I want worker type names that describe inference, agent, script, or poller capability so that config is clearer without breaking existing factories.

**Acceptance Criteria:**

- [ ] Public factory config and API payloads accept `INFERENCE_WORKER`, `AGENT_WORKER`, `SCRIPT_WORKER`, and `POLLER_WORKER` wherever worker type values are accepted.
- [ ] Existing factories using `MODEL_WORKER`, `SCRIPT_WORKER`, and hosted-worker compatibility values still load and validate successfully.
- [ ] `MODEL_WORKER` projects to inference-worker behavior unless a future explicit agent migration rule identifies otherwise.
- [ ] Hosted-worker compatibility preserves existing poller behavior while allowing new factories to use `POLLER_WORKER`.
- [ ] Generated OpenAPI, Go, and TypeScript artifacts reflect the accepted worker vocabulary.
- [ ] Contract and config tests cover new worker values and legacy aliases through observable load/projection outcomes.
- [ ] Typecheck passes
- [ ] Tests pass

### local-agent-cli-runtime-taxonomy-split-002: Accept New Workstation Taxonomy With Legacy Aliases

**Description:** As a factory author, I want workstation type names that describe inference runs, agent runs, script runs, or poller runs so that workflow behavior is clear and legacy workstations still execute.

**Acceptance Criteria:**

- [ ] Public factory config and API payloads accept `INFERENCE_RUN`, `AGENT_RUN`, `SCRIPT_RUN`, and `POLLER_RUN` wherever workstation type values are accepted.
- [ ] Existing factories using `MODEL_INVOKE` still load, validate, and execute as inference-run behavior.
- [ ] Existing poller workstations still load, validate, and execute through poller-run compatibility behavior.
- [ ] New authored factory documents save new workstation names without downgrading to legacy names.
- [ ] Legacy factory save behavior follows the documented compatibility policy and does not silently break executable factories.
- [ ] Contract and config tests cover new workstation values and legacy aliases through observable load/projection/save outcomes.
- [ ] Typecheck passes
- [ ] Tests pass

### local-agent-cli-runtime-taxonomy-split-003: Validate Worker And Workstation Compatibility

**Description:** As a factory author, I want invalid worker/workstation pairings to fail with behavior-specific messages so I can correct taxonomy mistakes before dispatch.

**Acceptance Criteria:**

- [ ] Validation accepts compatible pairings: `INFERENCE_RUN` with `INFERENCE_WORKER`, `AGENT_RUN` with `AGENT_WORKER`, `SCRIPT_RUN` with `SCRIPT_WORKER`, and `POLLER_RUN` with `POLLER_WORKER`, including supported legacy aliases.
- [ ] Validation rejects incompatible pairings such as `AGENT_RUN` with `INFERENCE_WORKER`, `INFERENCE_RUN` with `AGENT_WORKER`, and `POLLER_RUN` with non-poller workers.
- [ ] Validation findings identify the incompatible worker and workstation values and use inference, agent, script, or poller terminology.
- [ ] Legacy config reports actionable findings without requiring immediate migration to new names.
- [ ] Tests cover valid pairings, invalid pairings, and legacy aliases using behavior assertions rather than source-file or route inventories.
- [ ] Typecheck passes
- [ ] Tests pass

### local-agent-cli-runtime-taxonomy-split-004: Preserve Runtime Execution And Events For Inference Compatibility

**Description:** As a maintainer, I want new inference taxonomy and legacy model aliases to produce equivalent runtime outcomes so compatibility aliases do not hide dispatch or event regressions.

**Acceptance Criteria:**

- [ ] A configured `INFERENCE_WORKER` with `INFERENCE_RUN` executes one bounded inference operation and emits canonical dispatch/work result events.
- [ ] Legacy `MODEL_WORKER` plus `MODEL_INVOKE` execution produces the same observable inference-run result shape during the migration window.
- [ ] OmniVoice default configuration or fixtures use inference worker/run behavior and do not require agent-loop fields to execute.
- [ ] `AGENT_RUN` does not execute against an inference worker and fails validation before dispatch with an actionable compatibility finding.
- [ ] Runtime tests verify dispatch outcome, emitted events, routed output, and failure classification without asserting helper or package topology.
- [ ] Typecheck passes
- [ ] Tests pass

### local-agent-cli-runtime-taxonomy-split-005: Expose The New Taxonomy In CLI And API User Flows

**Description:** As a CLI or API user, I want validation, inspection, and factory output to use the new runtime vocabulary so I can understand worker and run behavior consistently.

**Acceptance Criteria:**

- [ ] CLI factory validation and inspection output displays new taxonomy names for newly authored factories.
- [ ] CLI and API responses for legacy factories remain understandable by either preserving legacy values with documented alias behavior or projecting them consistently according to compatibility policy.
- [ ] Error output for incompatible taxonomy values identifies the expected behavior class and the provided values.
- [ ] CLI/API behavior remains equivalent for shared invocation and validation flows.
- [ ] Focused CLI/API tests cover new taxonomy output, legacy alias output, and incompatible pairing errors.
- [ ] Typecheck passes
- [ ] Tests pass

### local-agent-cli-runtime-taxonomy-split-006: Preserve Dashboard Projection And Save Behavior

**Description:** As a dashboard user, I want the graph editor and current-factory views to display and save the new taxonomy without losing graph identity, layout, or accessibility.

**Acceptance Criteria:**

- [ ] Dashboard graph projections preserve worker IDs, workstation IDs, edge relationships, and layout references after loading factories with new taxonomy values.
- [ ] Dashboard graph projections preserve the same identity and layout references after loading legacy `MODEL_WORKER`, `MODEL_INVOKE`, and hosted/poller-compatible factories.
- [ ] New factory creation and editor controls prefer labels and values for inference, agent, script, and poller behavior.
- [ ] Loading, empty, validation-error, save-error, and successful-save states remain explicit when taxonomy-related values are edited.
- [ ] Changed controls remain keyboard-operable and expose accessible labels that make the selected behavior class clear.
- [ ] Focused UI tests cover projection/save behavior for new and legacy values.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill

### local-agent-cli-runtime-taxonomy-split-007: Update Docs And Examples To Teach The Split

**Description:** As a factory author, I want docs and examples to use the new taxonomy so I learn the intended vocabulary and understand legacy alias behavior.

**Acceptance Criteria:**

- [ ] Public reference docs explain `INFERENCE_WORKER`/`INFERENCE_RUN`, `AGENT_WORKER`/`AGENT_RUN`, `SCRIPT_WORKER`/`SCRIPT_RUN`, and `POLLER_WORKER`/`POLLER_RUN` in customer-facing terms.
- [ ] Docs describe `MODEL_WORKER`, `MODEL_INVOKE`, and hosted/poller compatibility as migration aliases rather than removed behavior.
- [ ] OmniVoice or default inference examples use inference worker/run terminology and do not describe harnessless inference as agent behavior.
- [ ] Agent examples or placeholders use `AGENT_WORKER` and `AGENT_RUN` only for agent-loop behavior and do not imply agent runtime implementation is part of this taxonomy split.
- [ ] Packaged reference-doc smoke checks pass for changed docs.
- [ ] Typecheck passes
- [ ] Tests pass

## High-Level Technical Design

### Canonical Taxonomy

- Worker capability values:
  - `INFERENCE_WORKER`: performs one bounded inference operation such as TTS, embeddings, transcription, image generation, chat, or text generation.
  - `AGENT_WORKER`: runs an agent loop that may plan, call tools, maintain transcript state, and decide completion.
  - `SCRIPT_WORKER`: runs deterministic command or script work.
  - `POLLER_WORKER`: runs long-lived ingress polling work.
- Workstation behavior values:
  - `INFERENCE_RUN`: resolves operation bindings, calls an inference worker, and routes content output.
  - `AGENT_RUN`: starts or resumes one agent run for a work item and routes the final agent result.
  - `SCRIPT_RUN`: runs script or command behavior.
  - `POLLER_RUN`: runs ingress polling and submits new work.

### Compatibility Policy

- Keep legacy names accepted in config, CLI, and public API payloads during this migration window.
- Project `MODEL_WORKER` to inference-worker behavior unless a future explicit agent migration rule exists.
- Project `MODEL_INVOKE` to inference-run behavior.
- Preserve existing hosted/poller behavior while allowing `POLLER_WORKER` and `POLLER_RUN` as the preferred vocabulary.
- Do not remove legacy enum values or require customers to migrate existing factories in this project.

### Contract And Generated Artifact Alignment

- Author OpenAPI changes in component fragments, then regenerate bundled OpenAPI, generated Go server/client types, and generated TypeScript types.
- Keep backend config mapping, public API surface mapping, CLI adapters, dashboard generated-type consumers, and contract tests aligned with the generated contract.
- Public docs and API surfaces should use customer-facing taxonomy rather than internal Petri-net terms.

### Runtime And Validation Boundaries

- Factory sessions continue to own orchestration, event emission, dispatch lifecycle, replay, cancellation, and primary result selection.
- The taxonomy update should map new public values onto existing runtime behavior paths unless the behavior truly differs.
- Validation owns compatibility checks and should produce direct, actionable findings before dispatch.
- Model host lifecycle and agent-harness execution are not implemented by this taxonomy split, but docs may state that inference and agent workers borrow ready model capacity while factory sessions decide factory behavior.

### Frontend State And Projection Boundaries

- The current-factory document remains canonical for editable factory configuration.
- Graph nodes, edges, labels, and layout are projections of the canonical factory document and must not become a separate source of truth for taxonomy values.
- Save operations should update selected worker or workstation behavior values without mutating unrelated IDs, relationships, layout data, or legacy compatibility fields.

## Functional Requirements

- FR-1: The public worker type contract must include `INFERENCE_WORKER`, `AGENT_WORKER`, `SCRIPT_WORKER`, and `POLLER_WORKER`.
- FR-2: The public workstation type contract must include `INFERENCE_RUN`, `AGENT_RUN`, `SCRIPT_RUN`, and `POLLER_RUN`.
- FR-3: The system must continue accepting and executing legacy `MODEL_WORKER`, `MODEL_INVOKE`, and hosted/poller compatibility values during the migration window.
- FR-4: Legacy `MODEL_WORKER` must map to inference-worker behavior unless a future explicit agent migration rule identifies otherwise.
- FR-5: Legacy `MODEL_INVOKE` must map to inference-run behavior.
- FR-6: New factory authoring and save flows must preserve new taxonomy values.
- FR-7: Legacy factory save flows must preserve executable behavior and follow a documented compatibility policy.
- FR-8: Validation must reject incompatible worker/workstation pairings before dispatch.
- FR-9: Validation findings must name the behavior class involved: inference, agent, script, or poller.
- FR-10: Runtime dispatch and emitted events must preserve existing inference behavior for new inference taxonomy and legacy aliases.
- FR-11: CLI/API output must expose taxonomy behavior consistently for validation, inspection, and invocation surfaces.
- FR-12: Dashboard graph projections must preserve node identity, relationships, and layout references when loading and saving new or legacy taxonomy values.
- FR-13: New UI creation flows, labels, docs, and examples must prefer new taxonomy names.
- FR-14: OmniVoice default inference behavior must use inference worker/run terminology and must not require agent-worker configuration.

## Non-Goals

- Removing `MODEL_WORKER`, `MODEL_INVOKE`, or hosted/poller compatibility values.
- Implementing llama.cpp process supervision or changing model host lifecycle ownership.
- Integrating `go-agent-harness` or implementing agent-loop execution semantics.
- Redesigning the graph editor beyond taxonomy labels, compatible controls, projections, and save behavior.
- Reorganizing runtime packages unless required to preserve observable taxonomy behavior.
- Changing internal Petri-net semantics or exposing Petri-net terms in public docs.

## Supporting Technical And UX Considerations

- Start from authored OpenAPI fragments and regenerate derived artifacts rather than hand-editing generated files.
- Prefer generated enum types in backend and frontend code over handwritten duplicate enum lists.
- Keep compatibility mapping explicit and covered by focused tests so future alias retirement can be planned safely.
- UI controls that expose taxonomy values should use accessible labels and keyboard-operable select, segmented-control, or menu patterns consistent with existing dashboard primitives.
- Error messages should tell authors what value is incompatible and which behavior class is expected.
- Docs should describe the behavior model in customer-facing vocabulary and avoid making internal model-host or Petri-net implementation details the primary explanation.
- Verification should include `make generate-api`, focused config/API/CLI/backend/runtime tests, focused UI tests where UI changes occur, browser verification for visible UI changes, and docs-reference smoke checks when packaged docs change.

## Success Metrics

- New factories can be authored without using `MODEL_WORKER` for default inference behavior.
- Legacy factory fixtures continue to load, validate, execute, and save without customer action.
- Validation errors clearly distinguish inference, agent, script, and poller incompatibilities.
- CLI/API and dashboard views present the same behavior class for a given factory configuration.
- Dashboard save/reload preserves graph identity and layout across both new and legacy taxonomy values.
- OmniVoice is represented and tested as inference behavior, not agent behavior.

## Open Questions

- Should legacy `MODEL_WORKER` values be preserved exactly after editing a legacy factory, or normalized to `INFERENCE_WORKER` once the user saves?
- Should hosted/poller legacy values be preserved exactly after editing a legacy factory, or normalized to `POLLER_WORKER` and `POLLER_RUN` once the user saves?
