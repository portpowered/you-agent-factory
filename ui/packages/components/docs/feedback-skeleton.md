# Skeleton

Non-interactive loading placeholders for feedback and layout surfaces. Use
`Skeleton` for pulse affordances inside busy regions; pair it with
`aria-busy="true"` on a parent when the surrounding surface is still loading.

## Import

```ts
import { Skeleton } from "@you-agent-factory/components";
```

Category entrypoint:

```ts
import { Skeleton } from "@you-agent-factory/components/feedback";
```

## Behavior

- Renders a `div` with `aria-hidden="true"` so assistive technology does not
  treat placeholder bars as content.
- Uses package overlay token `bg-af-overlay` with `animate-pulse` and
  `rounded-xl` for consistent loading chrome.
- Accepts standard `className` sizing (`h-*`, `w-*`, `max-w-*`) so consumers
  can mirror dashboard panel layouts.

## Loading and empty feedback

- **Panel loading** — wrap multiple skeleton bars in a parent with
  `aria-busy="true"`. Hide decorative skeleton groups with `aria-hidden="true"`
  on the wrapper when the parent already exposes busy state.
- **Alert loading** — prefer `<AlertPanel semantic="loading" />` when the whole
  feedback surface should show a standardized loading state with status label
  and skeleton placeholders.
- **Empty states** — use `<AlertPanel semantic="empty" />` for intentional empty
  feedback; do not use `Skeleton` alone to imply emptiness.

## Usage example

```tsx
<div aria-busy="true" className="grid gap-3">
  <Skeleton className="h-4 w-32" />
  <Skeleton className="h-28 w-full" />
</div>
```

## Storybook

Package Storybook stories under **Feedback/Skeleton**:

- `feedback-skeleton--compact` — short placeholder bars
- `feedback-skeleton--full-width` — wide block placeholder
- `feedback-skeleton--panel-loading-layout` — dashboard-style panel loading
