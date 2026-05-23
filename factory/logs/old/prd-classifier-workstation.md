# PRD: Classifier Workstation

## Introduction

Add a new classifier workstation capability to the agent factory so one workstation can inspect input work and route it to exactly one authored branch label. This solves the current gap where workstation routing is limited to coarse outcomes such as accepted, continue, rejected, and failed. For classifier workstations, classification should be the normal success path, and `continue` and `rejected` should not be part of the feature's runtime contract. Only `failed` should remain as the non-classification error path.

In version 1, the classifier returns a plain string label such as `approved`, `needs_review`, or `spam`. The workstation uses that label to choose one authored route from a new `classificationRoutes` field.

## Goals

- Allow a workstation to classify input work into exactly one authored branch label.
- Let factory authors declare classification branches explicitly in `factory.json`.
- Keep failure semantics simple: classified branch on success, `FAILED` on unknown label, missing label, timeout, or execution error.
- Avoid overloading `onContinue` and `onRejection` for classifier behavior.
- Preserve clear replay, event, and diagnostic evidence showing which label was selected.

## User Stories

### US-001: Author a classifier workstation in factory config
**Description:** As a factory author, I want to declare a classifier workstation with explicit labeled routes so that routing logic is readable and stable in config.

**Acceptance Criteria:**
- [ ] A workstation can declare a classifier runtime type in `factory.json`.
- [ ] A classifier workstation can declare `classificationRoutes` with one or more entries.
- [ ] Each `classificationRoutes` entry includes a unique `label`.
- [ ] Each `classificationRoutes` entry includes one or more destination outputs.
- [ ] Classifier workstation validation rejects duplicate labels within the same workstation.
- [ ] Classifier workstation validation rejects configs that also rely on `onContinue` or `onRejection` for classifier routing.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-002: Return a plain string classification label from execution
**Description:** As a workstation executor, I want classifier output to be a plain string label so that the contract is simple for model-backed and script-backed flows.

**Acceptance Criteria:**
- [ ] The classifier execution contract accepts a plain string label such as `approved`.
- [ ] Leading and trailing whitespace are trimmed before route matching.
- [ ] Empty output is treated as an execution result that follows the `FAILED` path.
- [ ] Output that does not match any authored label is treated as an execution result that follows the `FAILED` path.
- [ ] Matching is documented with exact case-sensitivity behavior.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-003: Route classified work to exactly one authored branch
**Description:** As a runtime operator, I want the engine to route classifier output to exactly one authored branch so that classification behavior is deterministic and observable.

**Acceptance Criteria:**
- [ ] A successful classifier dispatch selects exactly one `classificationRoutes` entry.
- [ ] The selected route emits work only to that entry's configured outputs.
- [ ] The dispatch does not also use normal `outputs` for accepted routing when a classifier branch is selected.
- [ ] The dispatch does not use `onContinue` or `onRejection`.
- [ ] The selected label is preserved in runtime evidence for replay and diagnostics.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-004: Fail cleanly on invalid classifier output
**Description:** As a factory author, I want invalid classifier results to fail predictably so that misconfigured prompts or unexpected outputs do not silently misroute work.

**Acceptance Criteria:**
- [ ] Unknown labels route through `FAILED`.
- [ ] Missing labels route through `FAILED`.
- [ ] Worker execution errors and timeouts continue to route through `FAILED`.
- [ ] Failure evidence includes enough information to identify the returned label or the reason label resolution failed.
- [ ] If `onFailure` is configured, classifier failures use that route exactly as other workstation failures do.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-005: Expose classifier behavior in events and projections
**Description:** As a user inspecting runtime history, I want to see which classification label was chosen so that I can debug routing decisions.

**Acceptance Criteria:**
- [ ] Dispatch completion events include the selected classifier label when classification succeeds.
- [ ] Replay preserves the selected classifier label.
- [ ] World state or dashboard-facing projections expose the selected classifier label where dispatch outcome details are shown.
- [ ] Failed classifier attempts preserve the ordinary failure reason without inventing a branch label.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-006: Document classifier workstation authoring
**Description:** As a factory author, I want clear documentation for classifier workstations so that I can author them without inferring behavior from code.

**Acceptance Criteria:**
- [ ] Workstation reference docs explain when to use a classifier workstation.
- [ ] Docs define the plain-string output contract with examples.
- [ ] Docs explain that classifier workstations do not use `onContinue` or `onRejection`.
- [ ] Docs explain how invalid or unknown labels fail.
- [ ] Docs include at least one example factory snippet using `classificationRoutes`.

## Functional Requirements

1. FR-1: The system must support a new classifier workstation runtime type that performs label-based routing.
2. FR-2: A classifier workstation must accept one successful classification result per dispatch, represented as a plain string label.
3. FR-3: Factory authors must configure classifier branches through a new `classificationRoutes` field on the workstation definition.
4. FR-4: Each `classificationRoutes` entry must include a unique authored label and one or more output destinations.
5. FR-5: When classifier execution returns a label that exactly matches an authored route, the system must route work to that route's outputs only.
6. FR-6: Classifier workstations must not use `onContinue` or `onRejection` as part of normal classifier behavior.
7. FR-7: For classifier workstations, unknown labels, missing labels, parse failures, execution errors, and timeouts must resolve to `FAILED`.
8. FR-8: If a classifier workstation has `onFailure` outputs, those outputs must be used for failed classifier executions.
9. FR-9: The system must record the selected classifier label in dispatch-completion evidence when classification succeeds.
10. FR-10: Replay and runtime projections must preserve the selected classifier label so users can inspect past routing decisions.
11. FR-11: Validation must reject classifier configurations with duplicate labels, empty labels, or invalid route definitions.
12. FR-12: Validation must reject ambiguous authored configurations where classifier routing is mixed with `onContinue` or `onRejection`.
13. FR-13: Documentation must define the classifier authoring contract, the string-label output format, and failure behavior.

## Non-Goals

- Multi-label fanout in one classifier dispatch.
- Structured JSON classifier output in v1.
- A default classifier branch for unknown labels.
- Replacing ordinary `MODEL_WORKSTATION`, `LOGICAL_MOVE`, `REPEATER`, or `CRON` behavior outside classifier-specific flows.
- Automatic migration of existing `onContinue` or `onRejection` workflows into classifier workstations.
- Fuzzy label matching, semantic matching, or confidence-score-based routing.

## Design Considerations

- The authoring shape should make branch intent obvious in `factory.json`.
- Label names should be easy to read in diagnostics and event history.
- The docs should show examples with concrete labels like `approved`, `needs_changes`, and `spam`.
- The feature should feel like a routing primitive, not like a hidden prompt trick.

## Technical Considerations

- This feature will require public contract changes in OpenAPI and generated surfaces.
- Runtime config structs and validation will need a new field for classifier routes.
- Workstation execution and transition routing will need a path that resolves a successful label to one authored branch instead of the current accepted/continue/rejected grouping.
- Event, replay, and projection surfaces will need a stable place to store the selected label.
- Existing failure behavior should remain intact so classifier errors reuse the current failure path.
- Tests should cover config validation, execution output handling, transition routing, event history, replay, and service-level integration behavior.

## Success Metrics

- A factory author can express one-to-one classification routing in a single workstation without needing extra review or router workstations.
- Invalid classifier output never silently routes to the wrong branch.
- Runtime history for a classified dispatch clearly shows which label was chosen.
- The feature can be documented with a small, understandable config example that does not require internal implementation knowledge to use.

## Open Questions

- Should label matching be case-sensitive or normalized to a canonical form such as lowercase?
- Should classifier workstations be a brand-new runtime `type`, or should they be represented as a specialized mode of model workstation in the public contract?
- Should `outputs` be forbidden entirely on classifier workstations, or allowed for unrelated future use while remaining inactive for classified success routing?
- Should script-backed workers and model-backed workers both support classifier output from day one, or should v1 target model-backed workers first?
- What is the best event-field shape for the selected label so replay and UI consumers can use it without ambiguity?
