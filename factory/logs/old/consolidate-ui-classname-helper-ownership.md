# Cleanup Idea: Consolidate UI Classname Helper Ownership

## Why this cleanup exists

The UI currently keeps three identical class-name composition helpers:

- `ui/src/lib/cn.ts`
- `ui/src/lib/cx.ts`
- `ui/src/components/ui/classnames.ts`

They all implement the same `filter(Boolean).join(" ")` behavior, but consumers
are split across multiple import paths. That is unnecessary duplication in a
shared primitive and creates drift risk for no product value.

The internal relevant-files guide already points at `ui/src/lib/cn.ts` as the
shared class-composition utility, so the current state has overlapping owners
for one trivial concern.

## Requested change

Collapse the duplicate classname helpers into one canonical shared owner and
update all current consumers to use it.

Keep this cleanup narrow:

- preserve UI behavior exactly
- do not broaden this into styling rewrites, Tailwind refactors, or component
  redesign
- do not add another wrapper or compatibility layer
- prefer deleting the duplicate files over preserving multiple aliases

Suggested shape:

- Keep `ui/src/lib/cn.ts` as the canonical helper.
- Remove `ui/src/lib/cx.ts`.
- Remove `ui/src/components/ui/classnames.ts`.
- Update all imports that currently use `cx` or the `components/ui/classnames`
  path to use the canonical helper instead.
- If naming churn is simpler, standardize on `cn` at call sites rather than
  preserving multiple local names for the same function.

## Relevant files

- `ui/src/lib/cn.ts`
- `ui/src/lib/cx.ts`
- `ui/src/components/ui/classnames.ts`
- `ui/src/components/ui/index.ts`
- `ui/src/features/bento/agent-bento.tsx`
- `ui/src/features/current-selection/`
- `ui/src/features/export/export-factory-dialog.tsx`
- `ui/src/features/flowchart/`
- `ui/src/features/header/tick-slider-control.tsx`
- `ui/src/features/import/dashboard-import-preview-dialog.tsx`
- `ui/src/features/submit-work/submit-work-card.tsx`
- `ui/src/features/terminal-work/terminal-work-card.tsx`
- `ui/src/features/trace-drilldown/`
- `ui/src/features/work-outcome/`
- `ui/src/features/work-totals/work-totals-card.tsx`
- `ui/src/features/workflow-activity/`

## Acceptance criteria

- There is exactly one shared classname-composition helper implementation in
  the UI source tree.
- `ui/src/lib/cx.ts` and `ui/src/components/ui/classnames.ts` are deleted.
- Frontend code imports the canonical helper from one path instead of mixing
  three equivalent utility owners.
- Existing frontend tests, typecheck, and build behavior remain unchanged.
- No new duplicate class-join helper is introduced during the cleanup.

## Review guidance

Review this as a duplication-removal change, not as a visual change. The main
thing to verify is that the redundant helper owners disappeared and that the UI
still behaves the same through existing behavioral coverage.
