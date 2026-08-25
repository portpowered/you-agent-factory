# Material color role migration — rollout and regression

---
doc-id: DEV-MATERIAL-COLOR-ROLLOUT
---

Maintainer guide for the Material-inspired color, typography, spacing, and palette migration (PRD: material color role migration).

## Rollout order

Implement and review in this sequence. Later phases assume earlier ones are merged.

| Phase | Story | Deliverable | Status |
| --- | --- | --- | --- |
| 1. Taxonomy | US-001 | Role tokens in `color-role-tokens.css`; taxonomy in [material-color-role-taxonomy.md](./material-color-role-taxonomy.md) | Complete |
| 2. Product `af-*` role wiring | US-002 | Role-backed `--color-af-*` keys in `color-role-tokens.css` (alias file removed) | Complete |
| 3. Accent rebalance | US-003 | Calmer secondary/tertiary foundation keys; accent contrast Storybook | Complete |
| 4. Shared primitive semantics | US-004 | Accent vs semantic tone policy in buttons, pills, lists | Complete |
| 5. Shared neutral surfaces | US-005 | Role utilities in `ui/src/components/ui/` | Complete |
| 6. Typography & text color | US-006 | `typography-role-*`, `text-color-role-tokens.css`, `af-dashboard-*` mappings | Complete |
| 7. Layout spacing | US-007 | `layout-role-tokens.css`, `layout-primitives.tsx` | Complete |
| 8. Palette selector | US-008 | Five presets, `DashboardPaletteMenu`, session persistence | Complete |
| 9. Feature & graph surfaces | US-009 | `ui/src/features/**` + `dashboard-graph.tsx` on role utilities | Complete |
| 10. Regression & cleanup plan | US-010 | This doc, consolidated regression contracts, Storybook overview | Complete |

## Regression evidence

### Focused unit / contract tests

| Area | Test file | What it proves |
| --- | --- | --- |
| Role token wiring | `ui/src/styles/theme-role-regression.component.test.ts` and `ui/src/features/work-outcome/components/work-chart/work-chart-color-role-behavior.component.test.tsx` | Compiled Material role resolution and rendered chart-series palette behavior |
| Palette presets | `ui/src/styles/color-palette-presets.test.ts` | Five palettes override foundation keys |
| Typography scale | `ui/packages/components/src/styles/token-styles.test.ts` | Material scale tokens and utilities are exposed by the shared package styles |
| Layout scale | `ui/src/styles/layout-role-tokens.test.ts` | Layout spacing roles registered |
| Shared primitive semantics | `ui/src/components/ui/shared-primitive-semantic-color-roles.test.ts` | No semantic misuse for brand emphasis |
| Shared primitive neutrals | `ui/src/components/ui/shared-primitive-neutral-surface-roles.test.ts` | Neutral chrome on role utilities |
| Shared primitive disabled text | `ui/src/components/ui/shared-primitive-disabled-text-color-roles.test.ts` | Disabled/muted copy on `text-on-surface-disabled` in input, panel trigger, chart legend, action button spinner |
| Calendar accent/text | `ui/src/components/ui/calendar-color-roles.test.ts` | DayPicker selected, today, outside, disabled, and weekday cells on role utilities |
| Feature & graph surfaces | `ui/src/features/feature-surface-color-roles.test.ts` | Features avoid transitional tokens; graph/header samples use roles |
| Prompt-editor neutrals | `ui/src/components/prompt-editor/prompt-editor-surface-role-behavior.test.tsx` | Monaco shells, diagnostics rows, and resize handle on role utilities |
| Graph chrome | `ui/src/components/dashboard/dashboard-graph.test.tsx` | React Flow frame constraints; role CSS variables on canvas/controls |
| Migration index | `ui/src/styles/theme-role-regression.component.test.ts` | Regression contract files remain wired |

Run targeted UI checks from the repo root:

```bash
cd ui && bun install
cd ui && bun run tsc
cd ui && bun x vitest run src/styles/theme-role-regression.component.test.ts \
  src/styles/color-role-tokens.test.ts \
  src/styles/color-palette-presets.test.ts \
  src/features/feature-surface-color-roles.test.ts \
  src/components/ui/shared-primitive-neutral-surface-roles.test.ts \
  src/components/ui/shared-primitive-disabled-text-color-roles.test.ts \
  src/components/ui/calendar-color-roles.test.ts \
  src/components/prompt-editor/prompt-editor-surface-role-behavior.test.tsx \
  src/components/ui/shared-primitive-semantic-color-roles.test.ts \
  src/components/dashboard/dashboard-graph.test.tsx
```

### Storybook visual review

| Fixture | Storybook path |
| --- | --- |
| Migration overview (all pillars) | `Agent Factory/UI/Theme Role Migration Overview` |
| Accent contrast | `Agent Factory/UI/Color Role Accent Contrast` |
| Neutral surfaces | `Agent Factory/UI/Color Role Neutral Surfaces` |
| Typography hierarchy | `Agent Factory/UI/Typography Role Hierarchy` |
| Layout primitives | `Agent Factory/UI/Layout Role Primitives` |
| Palette selector | `you-agent-factory/Dashboard/Color Palette Selector` |
| Graph surfaces (feature) | `ui/src/features/trace-drilldown/components/trace-graph-surfaces.stories.tsx`, `ui/src/features/factory-graph-editor/components/flow/factory-graph-editor-flow.stories.tsx`, `ui/src/features/workflow-activity/components/react-flow-current-activity-card.stories.tsx` |

After story changes: `make ui-storybook` then `make ui-test-storybook` (or the targeted Storybook Vitest lane in [development.md](./development.md)).

## Cleanup phase (post-migration)

Alias layer removal is **complete**. Role-backed `--color-af-*` product keys live in `ui/packages/components/src/styles/color-role-tokens.css`, the shared Material role source imported by the dashboard. The chart, edge, graph-control/focus, and overlay follow-up is complete with 20 consumed aliases backed by shared Material roles and six zero-consumer aliases retired after the final audit: `--color-af-chart-grid-line`, `--color-af-chart-selection-fill`, `--color-af-chart-selection-stroke`, `--color-af-chart-cursor`, `--color-af-chart-active-dot-stroke`, and `--color-af-overlay-subtle`.

### Gate checklist (maintain)

1. `ui/src/features/**` — no `bg-af-surface-*`, `text-af-text`, `bg-af-accent-*` (enforced by `feature-surface-color-roles.test.ts`).
2. `ui/src/components/ui/**` — neutral and semantic contracts green (see shared-primitive `*.test.ts` files).
3. `ui/src/components/ui/calendar.tsx` — DayPicker selected, today, outside, disabled, and weekday cells on role utilities (enforced by `calendar-color-roles.test.ts`); `ui-foundation.test.tsx` and Storybook foundation showcase assert behavior/labels only and do not pin transitional accent/text class substrings on the calendar primitive.
4. `ui/src/components/prompt-editor/**` — Monaco shells, diagnostics rows, and resize handle on role utilities (enforced by `prompt-editor-surface-role-behavior.test.tsx`); RTL consumer tests in the same folder assert behavior only and do not pin transitional neutral class substrings.
5. Storybook overview and palette selector reviewed on all five palettes.
6. Full `cd ui && bun run tsc` and theme regression vitest bundle (above) pass.

### Completed alias removal

1. Removed the obsolete alias stylesheet and its `@import` from `ui/src/styles.css`.
2. Inlined role-backed `--color-af-*` definitions into `ui/packages/components/src/styles/color-role-tokens.css`; kept unrelated foundation and presentation aliases in their established stylesheet.
3. Removed the obsolete alias-only test; compiled role resolution is asserted by `ui/src/styles/theme-role-regression.component.test.ts`, and rendered chart behavior is asserted by `ui/src/features/work-outcome/components/work-chart/work-chart-color-role-behavior.component.test.tsx`.
4. **Bulk migrator removed (complete).** The one-shot bulk class replacer was deleted after US-009; bulk migration is finished. Do not restore it. If transitional `af-*` patterns reappear, fix violations using `ui/src/features/feature-surface-color-roles.test.ts` and targeted manual edits.
5. [material-color-role-taxonomy.md](./material-color-role-taxonomy.md) documents supported `--color-af-*` product keys and the completed chart/edge/graph-control/focus/overlay role pass, including the six retired zero-consumer aliases.

### Tokens outside this follow-up

Foundation inputs and other presentation aliases outside the 26-key follow-up remain in their established stylesheet according to their existing consumers. The six retired keys above have no remaining production consumers; every other target is accounted for above and is not deferred.

## Related docs

- [material-color-role-taxonomy.md](./material-color-role-taxonomy.md) — role families and supported product keys
- [material-typography-role-taxonomy.md](./material-typography-role-taxonomy.md) — typography and text color roles
- [material-layout-role-taxonomy.md](./material-layout-role-taxonomy.md) — layout spacing primitives
