# AlertPanel

Semantic feedback panels for neutral, informational, success, warning, danger,
error, loading, and empty states. Use `AlertPanel` when a surface needs a
contained message with token-backed tone, accessible roles, and optional status
labels — without dashboard routes, providers, or feature state.

## Import

```ts
import {
  AlertPanel,
  AlertPanelText,
  AlertPanelTitle,
  type AlertPanelSemanticVariant,
} from "@you-agent-factory/components";
```

Category entrypoint:

```ts
import {
  AlertPanel,
  AlertPanelText,
  AlertPanelTitle,
} from "@you-agent-factory/components/feedback";
```

Import `@you-agent-factory/components/styles.css` once in the host application so
semantic container and border tokens resolve correctly.

## Semantic variants

Prefer the `semantic` prop for reviewer-visible, all-in-one feedback states.
Each variant resolves tone, layout, default `role`, optional status labels, and
loading placeholders through package-owned semantics.

| `semantic` | When to use | Default `role` | Status label | Visual treatment |
| --- | --- | --- | --- | --- |
| `neutral` | General copy without urgency | `status` | — | `border-outline`, `bg-surface-container-low`, `text-on-surface-variant` |
| `info` | Helpful context or guidance | `status` | — | `border-af-info-border`, `bg-info-container`, `text-on-info-container` |
| `success` | Completed or healthy outcomes | `status` | — | `border-af-success-border`, `bg-success-container`, `text-on-success-container` |
| `warning` | Caution before continuing | `alert` | — | `border-af-warning-border`, `bg-warning-container`, `text-on-warning-container` |
| `danger` | Destructive or high-risk actions | `alert` | — | `border-af-danger-border`, `bg-error-container`, `text-on-error-container` |
| `error` | Failed requests or blocking faults | `alert` | `Error` | Same tokens as `danger` with explicit error label |
| `loading` | In-flight work without final copy | `status` | `Loading` | Neutral tone, built-in skeleton placeholders, `aria-busy` |
| `empty` | No data yet, dashed empty surface | `status` | `Empty` | Neutral tone, dashed border layout |

Legacy `tone` and `variant` props remain for migration compatibility. New
consumers should use `semantic` so roles, labels, and loading placeholders stay
consistent.

## Design token mapping

Alert tones map to semantic role tokens defined in
`src/styles/color-role-tokens.css`:

- **Neutral** — `--color-outline`, `--color-surface-container-low`,
  `--color-on-surface-variant`
- **Info** — `--color-info-container`, `--color-on-info-container`,
  `--color-af-info-border`
- **Success** — `--color-success-container`, `--color-on-success-container`,
  `--color-af-success-border`
- **Warning** — `--color-warning-container`, `--color-on-warning-container`,
  `--color-af-warning-border`
- **Danger / error** — `--color-error-container`, `--color-on-error-container`,
  `--color-af-danger-border`

Typography uses package role utilities (`text-body-medium`,
`text-body-small`) rather than dashboard-only aliases.

## Accessibility

- Warning, danger, and error variants default to `role="alert"`; neutral, info,
  success, loading, and empty default to `role="status"`.
- `error`, `loading`, and `empty` render an uppercase status label so state is
  not communicated by color alone.
- `loading` sets `aria-busy="true"` and renders skeleton placeholders when no
  children are provided.
- Compose titles with `AlertPanelTitle` and body copy with `AlertPanelText`.

## Usage examples

Neutral guidance:

```tsx
<AlertPanel semantic="neutral">
  <AlertPanelTitle>Factory session idle</AlertPanelTitle>
  <AlertPanelText>Submit work to start orchestration.</AlertPanelText>
</AlertPanel>
```

Loading without custom markup:

```tsx
<AlertPanel semantic="loading" />
```

Empty state with dashed layout:

```tsx
<AlertPanel semantic="empty">
  <AlertPanelTitle>No dispatches yet</AlertPanelTitle>
  <AlertPanelText>Run a workflow to populate this list.</AlertPanelText>
</AlertPanel>
```

## Storybook

Package Storybook exposes each semantic variant under **Feedback/AlertPanel**:

- `feedback-alertpanel--semantic-variants` — all eight states in one grid
- `feedback-alertpanel--neutral`, `--info`, `--success`, `--warning`,
  `--danger`, `--error-state`, `--loading`, `--empty`

Stories use package imports and the package token fixture decorator only; they
do not mount dashboard providers.
