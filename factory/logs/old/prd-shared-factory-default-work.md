# PRD: Shared Factory Default Work

## Introduction

Add support for sharing factories with default seeded work by copying the current contents of the factory's `inputs/` directory into the shared factory artifact. This solves the current gap where a factory can be shared structurally, but recipients do not automatically receive the starter work that makes the factory immediately useful.

In version 1, sharing a factory must copy all current work items found in `inputs/` at share time. Recipients receive an independent copied factory, including copied starter work that they can inspect, edit, or run without affecting the original factory. The first version is limited to backend and API behavior only and must not require any UI changes.

## Goals

- Allow a factory author to share a factory with all current `inputs/` work items included by default.
- Make shared factories immediately usable as ready-to-run starter factories.
- Support reusable team templates by preserving both factory structure and starter work together.
- Keep recipient copies independent so changes to shared starter work do not mutate the original factory.
- Define explicit API and backend copy semantics that are testable and predictable.

## User Stories

### US-001: Include current input work when sharing a factory
**Description:** As a factory author, I want sharing a factory to include the current `inputs/` work items so that the shared factory carries the starter work I already prepared.

**Acceptance Criteria:**
- [ ] When a factory is shared, all work items currently present in its `inputs/` directory are included in the shared artifact by default.
- [ ] The copied input set reflects the contents of `inputs/` at the moment the share action is executed.
- [ ] Sharing a factory with an empty `inputs/` directory still succeeds and produces a shared factory with no seeded work.
- [ ] Existing sharing behavior for the rest of the factory definition remains intact.
- [ ] Relevant backend tests cover populated and empty `inputs/` directory cases.

### US-002: Deliver shared factories as independent copies
**Description:** As a recipient of a shared factory, I want the seeded starter work to be copied into my own factory so that I can modify or consume it without changing the original author's factory.

**Acceptance Criteria:**
- [ ] A recipient receives a full copied factory, not a live linked view into the original factory.
- [ ] Seeded work copied from the original `inputs/` directory is independently editable in the recipient factory.
- [ ] Changes made by the recipient to copied work items do not alter the original factory's `inputs/` contents.
- [ ] Changes made later by the original author do not retroactively update already shared recipient copies.
- [ ] Relevant backend or service tests prove copy independence for both factory metadata and copied work items.

### US-003: Expose seeded-share behavior through the API contract
**Description:** As an API consumer, I want the share contract to clearly define that current `inputs/` work is included so that backend and CLI integrations can rely on consistent behavior.

**Acceptance Criteria:**
- [ ] The API contract describes that sharing includes all current `inputs/` work items by default in v1.
- [ ] Generated or API-facing contract surfaces stay aligned with the implemented share behavior.
- [ ] If the share response includes share metadata, it identifies the resulting shared factory in a way that lets callers retrieve the copied factory and its seeded work.
- [ ] Contract-level tests or API-facing tests cover the seeded-share behavior.
- [ ] Generated-artifact checks and relevant contract verification pass.

### US-004: Preserve copied work fidelity during sharing
**Description:** As a factory author, I want seeded work items to survive sharing without silent loss or mutation so that example and starter work behaves the same after sharing.

**Acceptance Criteria:**
- [ ] Each copied work item preserves its authored content and identifying fields required for later processing.
- [ ] Sharing does not silently drop valid work items that are present in `inputs/` at share time.
- [ ] If an `inputs/` entry cannot be copied, the share operation fails or reports the error through the existing failure path instead of producing a partially silent result.
- [ ] Failure diagnostics identify that the error occurred while copying shared factory starter work.
- [ ] Relevant tests cover successful copy behavior and at least one failure-path case.

### US-005: Document shared factory starter-work behavior
**Description:** As a factory author, I want documentation for shared factory starter work so that I understand what gets copied, when the copy occurs, and what independence guarantees exist.

**Acceptance Criteria:**
- [ ] Documentation explains that v1 shares copy all current work items from `inputs/`.
- [ ] Documentation explains that copying happens at share time, not continuously after sharing.
- [ ] Documentation explains that recipient factories receive independent copies.
- [ ] Documentation includes at least one concrete example of sharing a factory with seeded starter work.

## Functional Requirements

1. FR-1: The system must include all current work items in a factory's `inputs/` directory when that factory is shared.
2. FR-2: The copied input set must reflect the state of the `inputs/` directory at share time.
3. FR-3: The system must create an independent copied factory for the recipient rather than a live linked reference to the original factory.
4. FR-4: Copied work items in the recipient factory must be independently editable and runnable without mutating the original factory.
5. FR-5: Later changes to the original factory or its `inputs/` directory must not retroactively change already shared recipient copies.
6. FR-6: Sharing must continue to succeed when the source factory has no work items in `inputs/`.
7. FR-7: The share API or contract surface must explicitly define that current `inputs/` work is included by default in v1.
8. FR-8: Any generated contract artifacts and API-facing types must remain aligned with the share behavior.
9. FR-9: Copied work items must preserve the content and fields required for downstream processing after sharing.
10. FR-10: The system must not silently omit valid `inputs/` work items during sharing.
11. FR-11: If copying seeded work fails, the share operation must return or surface an observable error through the existing failure path.
12. FR-12: Error reporting for seeded-work copy failures must identify the share operation and the affected starter-work copy behavior clearly enough to debug the issue.
13. FR-13: The first version must require no UI changes and must be deliverable through backend and API behavior only.
14. FR-14: Documentation must define share-time copy semantics, recipient independence, and empty-input behavior.

## Non-Goals

- No UI changes or new UI controls for selecting which work items to include.
- No per-share option to include none or only some `inputs/` work items in v1.
- No live synchronization between the original factory and recipient copies after sharing.
- No support for sharing work from directories other than the factory `inputs/` directory in v1.
- No redesign of the broader factory sharing model beyond adding seeded starter-work copy behavior.
- No deduplication, merging, or conflict-resolution workflow for copied starter work in the first version.

## Design Considerations

- The feature should feel like a natural extension of existing factory sharing rather than a separate export workflow.
- Terminology should consistently describe the copied `inputs/` contents as starter work, seeded work, or default work without mixing meanings in the same contract.
- Because there is no UI change in v1, any user-facing explanation must come from existing docs, API descriptions, or CLI/help text surfaces if those already participate in sharing flows.

## Technical Considerations

- The implementation likely touches the factory share path, the service-layer copy behavior, and API-facing contract descriptions for shared factories.
- The copy operation should use the same ownership and cloning discipline expected elsewhere in the repository so recipient copies cannot accidentally share mutable backing state with the original.
- Contract and generated-artifact alignment matters because the repository uses OpenAPI and generated surfaces across backend and website consumers.
- Tests should prove share-time snapshot behavior, recipient independence, empty `inputs/`, successful copy fidelity, and copy-failure reporting.
- If work items have identity fields that must remain unique per factory instance, implementation should define whether identities are preserved or regenerated, as long as downstream processing remains correct and documented.

## Success Metrics

- A factory author can share a ready-to-run starter factory without manually recreating input work elsewhere.
- Every shared factory includes the current `inputs/` work items automatically in v1.
- A recipient can inspect or run the shared factory's starter work without depending on the original factory remaining available.
- No UI work is required to adopt or validate the first version of the feature.

## Open Questions

- What exact share API or service entrypoint currently owns factory sharing, and does it already expose metadata that should now mention seeded work?
- If copied work items contain per-factory identifiers, should v1 preserve those identifiers verbatim or regenerate recipient-local identifiers during the copy?
- Should share failures be all-or-nothing for seeded work copy, or is there any existing partial-share pattern that must be preserved?
- Are there any existing size or count limits for `inputs/` that should also bound share-time copying in v1?
