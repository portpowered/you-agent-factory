# Align dashboard fixture event typing with the generated UI event contract

## Why

The dashboard fixture files under `ui/src/components/dashboard/fixtures/` currently author event objects that do not typecheck against the generated OpenAPI `FactoryEvent` contract. This blocks widening `ui/tsconfig.app.json` to cover more of the UI surface and will become a repeated source of friction as CI expands beyond the first-pass scaffold.

## Problem

- `bun run tsc` only passes today when the UI TypeScript project is scoped to `ui/src/api/**`.
- Fixture event objects omit or reshape fields compared with the generated `FactoryEvent` schema, including required envelope fields and newer payload shapes.
- As a result, future CI stories cannot simply broaden the typecheck surface without mixing unrelated fixture-contract cleanup into those changes.

## Proposed follow-up

- Decide whether the fixture layer should:
  - conform directly to the generated `FactoryEvent` contract, or
  - use a dedicated fixture-specific authoring type that is converted into canonical events before tests and Storybook consume it.
- After choosing the ownership model, widen `ui/tsconfig.app.json` so the normal UI typecheck covers the intended fixture and Storybook files.

## Expected payoff

- Future CI stories can add stronger UI verification without reopening the same contract mismatch.
- Dashboard fixtures become a trustworthy typed surface instead of an unchecked compatibility layer.
