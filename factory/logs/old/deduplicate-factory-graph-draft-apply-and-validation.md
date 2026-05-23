# Cleanup Idea: Deduplicate Factory Graph Draft Apply And Validation

## Why this cleanup exists

The editable-factory graph editor still keeps two parallel copies of the same
draft-to-definition transformation logic.

Today:

- `ui/src/features/factory-graph-editor/factory-graph-draft-apply.ts` owns the
  canonical "apply the draft onto the editable factory definition" path used to
  build the pending saved definition.
- `ui/src/features/factory-graph-editor/factory-graph-draft-validation.ts`
  repeats the same helper logic locally to construct a pending definition shape
  for validation and to resolve final workstation worker assignments.

The duplicated logic currently includes the same or near-identical ownership for:

- `applyNamedEntityChanges(...)`
- `applyWorkStateChanges(...)`
- `buildPendingFactoryDefinition(...)`
- worker-assignment resolution for a workstation after edge changes

That overlap creates drift risk inside one feature-owned surface with no product
benefit. The graph editor is new enough that the correct cleanup is to collapse
parallel owners early instead of letting the duplication harden.

## Requested change

Collapse the duplicated draft transformation helpers onto one canonical owner
inside the factory graph editor feature and reuse them from validation.

Keep this cleanup narrow:

- preserve current editor behavior exactly
- do not broaden this into graph redesign, runtime API changes, or store changes
- do not add another abstraction layer beyond one canonical helper owner
- prefer deleting the duplicate validation-local helpers instead of preserving
  parallel copies
- keep tests behavioral around validation outcomes and pending-definition
  results, not helper-location assertions

Suggested shape:

- Keep `ui/src/features/factory-graph-editor/factory-graph-draft-apply.ts` as
  the canonical owner for shared draft application helpers.
- Reuse the shared named-entity, work-state, and worker-assignment application
  helpers from `factory-graph-draft-validation.ts` instead of keeping local
  copies there.
- Reuse one canonical pending-definition builder for the validation path, while
  preserving the current contract where save-building still returns `null` when
  validation fails.
- Delete the duplicate helper implementations from
  `factory-graph-draft-validation.ts` once all validation paths flow through the
  shared owner.

## Relevant files

- `ui/src/features/factory-graph-editor/factory-graph-draft-apply.ts`
- `ui/src/features/factory-graph-editor/factory-graph-draft-validation.ts`
- `ui/src/features/factory-graph-editor/factory-graph-draft.ts`
- `ui/src/features/factory-graph-editor/factory-graph-draft.test.tsx`

## Acceptance criteria

- Draft application and draft validation no longer keep separate copies of the
  same named-entity, work-state, pending-definition, and workstation
  worker-assignment logic.
- The graph editor has one canonical owner for the shared draft transformation
  helpers.
- `buildPendingFactoryDefinition(...)` still preserves untouched editable
  factory content and still returns `null` for invalid drafts at the public call
  site.
- Validation still reports the same behavioral outcomes for duplicate
  identifiers, missing required fields, incompatible edges, and missing final
  worker assignments.
- Verification stays focused on observable graph-editor behavior, for example:
  `pnpm vitest ui/src/features/factory-graph-editor/factory-graph-draft.test.tsx`
  or the repository's current equivalent frontend test command.

## Review guidance

Review this as a feature-local duplication cleanup. The main thing to verify is
that the parallel draft-transformation helpers disappeared and that existing
graph-editor validation and pending-definition behavior still matches current
tests.
