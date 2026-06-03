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
| 2. Compatibility aliases | US-002 | `color-role-aliases.css` maps transitional `af-*` → roles | Complete |
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
| Role token wiring | `ui/src/styles/color-role-tokens.test.ts` | Accent saturation contract (primary > secondary/tertiary) |
| Alias layer | `ui/src/styles/color-role-aliases.test.ts` | Each transitional `af-*` aliases to documented role |
| Palette presets | `ui/src/styles/color-palette-presets.test.ts` | Five palettes override foundation keys |
| Typography scale | `ui/src/styles/typography-role-tokens.test.ts` | Material scale tokens and utilities exist |
| Layout scale | `ui/src/styles/layout-role-tokens.test.ts` | Layout spacing roles registered |
| Shared primitive semantics | `ui/src/components/ui/shared-primitive-semantic-color-roles.test.ts` | No semantic misuse for brand emphasis |
| Shared primitive neutrals | `ui/src/components/ui/shared-primitive-neutral-surface-roles.test.ts` | Neutral chrome on role utilities |
| Feature & graph surfaces | `ui/src/features/feature-surface-color-roles.test.ts` | Features avoid transitional tokens; graph/header samples use roles |
| Graph chrome | `ui/src/components/dashboard/dashboard-graph.test.tsx` | React Flow frame constraints; role CSS variables on canvas/controls |
| Migration index | `ui/src/styles/theme-role-regression.test.ts` | Regression contract files remain wired |

Run targeted UI checks from the repo root:

```bash
cd ui && bun install
cd ui && bun run tsc
cd ui && bun x vitest run src/styles/theme-role-regression.test.ts \
  src/styles/color-role-aliases.test.ts \
  src/styles/color-role-tokens.test.ts \
  src/styles/color-palette-presets.test.ts \
  src/features/feature-surface-color-roles.test.ts \
  src/components/ui/shared-primitive-neutral-surface-roles.test.ts \
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
| Graph surfaces (feature) | `ui/src/features/trace-drilldown/components/trace-graph-surfaces.stories.tsx`, `factory-graph-editor-flow.stories.tsx`, `react-flow-current-activity-card.stories.tsx` |

After story changes: `make ui-storybook` then `make ui-test-storybook` (or the targeted Storybook Vitest lane in [development.md](./development.md)).

## Cleanup phase (post-migration)

Execute only after grep shows no production consumers of transitional class names or alias-only tokens.

### Gate checklist

1. `ui/src/features/**` — no `bg-af-surface-*`, `text-af-text`, `bg-af-accent-*` (enforced by `feature-surface-color-roles.test.ts`).
2. `ui/src/components/ui/**` — neutral and semantic contracts green (see shared-primitive `*.test.ts` files).
3. Storybook overview and palette selector reviewed on all five palettes.
4. Full `cd ui && bun run tsc` and theme regression vitest bundle (above) pass.

### Removal steps

1. Delete `ui/src/styles/color-role-aliases.css` and remove its `@import` from `ui/src/styles.css`.
2. Remove duplicate `af-*` entries from `ui/src/styles.css` that only existed for alias indirection (keep foundation keys, overlays, semantic borders, chart keys until replaced).
3. Delete `ui/src/styles/color-role-aliases.test.ts` after aliases are gone.
4. **Bulk migrator removed (complete).** The one-shot bulk class replacer was deleted after US-009; bulk migration is finished. Do not restore it. If transitional `af-*` patterns reappear, fix violations using `ui/src/features/feature-surface-color-roles.test.ts` and targeted manual edits.
5. Update [material-color-role-taxonomy.md](./material-color-role-taxonomy.md) to drop transitional tables and mark role utilities as the only API.

### Tokens that may remain after alias cleanup

Foundation palette keys (`af-foundation-*`), overlays (`af-overlay`), semantic border opacities (`af-*-border`), chart series keys (`af-chart-*`), and graph-edge keys without role equivalents stay in `styles.css` until a follow-up adds roles or retires the product keys.

## Related docs

- [material-color-role-taxonomy.md](./material-color-role-taxonomy.md) — role families and alias map
- [material-typography-role-taxonomy.md](./material-typography-role-taxonomy.md) — typography and text color roles
- [material-layout-role-taxonomy.md](./material-layout-role-taxonomy.md) — layout spacing primitives
