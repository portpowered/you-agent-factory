# CodePanel

Contained `<pre>` surface for generated scripts, logs, and diagnostic output.
`CodePanel` keeps long single-line and multi-line content inside the panel
without causing page-level horizontal overflow or overlapping header controls.

## Import

```ts
import { CodePanel } from "@you-agent-factory/components";
```

Category entrypoint:

```ts
import { CodePanel } from "@you-agent-factory/components/data-display";
```

## Long-content containment

`CodePanel` applies layout classes that bound width and overflow at the panel
boundary:

- `min-w-0 w-full max-w-full` — shrink inside flex/grid parents instead of
  forcing page overflow
- `overflow-x-auto` — horizontal scroll for unbreakable tokens when wrapping is
  insufficient
- `whitespace-pre-wrap` and `[overflow-wrap:anywhere]` — wrap long single lines
  inside the panel
- `font-mono text-code-medium text-code` — package code typography tokens

Place labels and action buttons in a sibling header row with `flex min-w-0` and
`shrink-0` on controls so long code does not overlap panel chrome.

## Vertical scrolling

Set `maxHeight` when multi-line blocks should scroll inside the panel instead
of expanding the page:

| `maxHeight` | Behavior |
| --- | --- |
| `none` (default) | Grows with content; no vertical scroll region |
| `sm` | `max-h-48 overflow-y-auto` |
| `md` | `max-h-72 overflow-y-auto` |
| `lg` | `max-h-96 overflow-y-auto` |

Scrollable panels receive `tabIndex={0}` by default so keyboard users can
focus the code region and scroll with arrow keys. Focus rings use
`focus-visible:outline-primary`.

## Surface and padding

| Prop | Values | Purpose |
| --- | --- | --- |
| `surface` | `high` (default), `low` | `bg-surface-container-high` vs `bg-surface-container-low` |
| `padding` | `compact` (default), `default` | `p-2` vs `p-3` |

## Responsive expectations

- At narrow widths, long single-line JSON or shell output wraps inside the
  panel; horizontal scroll remains available for unbreakable segments.
- At desktop widths, combine `maxHeight="lg"` with a `max-w-*` parent when code
  blocks should stay readable beside side panels.
- Header labels should use `min-w-0 shrink` so truncated titles do not push
  action buttons off-screen.

## Usage examples

Short snippet:

```tsx
<CodePanel>const value = 1;</CodePanel>
```

Long single line inside a bounded column:

```tsx
<div className="w-full max-w-md">
  <CodePanel>{longSingleLineString}</CodePanel>
</div>
```

Long multi-line log with vertical scroll:

```tsx
<CodePanel maxHeight="md" padding="default" surface="low">
  {longMultiLineOutput}
</CodePanel>
```

Header row with copy action (controls stay visible):

```tsx
<div className="grid w-full max-w-md gap-2">
  <div className="flex min-w-0 items-center justify-between gap-2">
    <span className="min-w-0 shrink text-body-medium text-on-surface">
      Generated script
    </span>
    <button className="shrink-0" type="button">
      Copy
    </button>
  </div>
  <CodePanel maxHeight="md" padding="default" surface="low">
    {code}
  </CodePanel>
</div>
```

## Storybook

Package Storybook stories under **Data Display/CodePanel**:

- `data-display-codepanel--short-code`
- `data-display-codepanel--long-single-line`
- `data-display-codepanel--long-multi-line`
- `data-display-codepanel--long-code` — narrow viewport with header controls
- `data-display-codepanel--desktop-long-code` — desktop viewport with taller
  scroll region

Stories use package imports and token fixture styles only.
