# Overlay and disclosure primitives

Domain-free Dialog, Popover, Collapsible, and ScrollArea primitives built on
Radix UI. Use these components when you need accessible overlays, disclosure
sections, and scrollable regions without importing dashboard features, generated
OpenAPI types, API clients, or app-specific providers.

## Required setup

Import package styles once in your host application before rendering overlay
primitives:

```css
@import "@you-agent-factory/components/styles.css";
```

Overlay shells depend on package role tokens (`--color-surface`,
`--color-outline`, typography roles) and Tailwind utility classes compiled from
those tokens. They do not require dashboard `styles.css`, dashboard providers,
generated OpenAPI types, React Query, Zustand, or app localization context.

## Import paths

Prefer the overlays category entrypoint for tree-shaking and explicit category
boundaries:

```ts
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverTrigger,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  ScrollArea,
  ScrollBar,
} from "@you-agent-factory/components/overlays";
```

The same exports are also available from the package root:

```ts
import { Dialog, Popover, Collapsible, ScrollArea } from "@you-agent-factory/components";
```

## Host ownership

The package owns presentation markup, Radix wiring, focus guards, and default
layout spacing. Host applications own everything domain-specific:

| Concern | Owner |
| --- | --- |
| Dialog titles, descriptions, and body copy | Host |
| Popover trigger labels and panel content | Host |
| Collapsible trigger text and disclosed content | Host |
| Scroll region labels and scrollable content | Host |
| Open/closed boolean state when controlled | Host |
| Close button labels (`closeLabel` on `DialogContent`) | Host |
| Data fetching, routing, and business workflows | Host |

Pass copy, labels, ids, and state into overlay primitives as props and children.
Do not expect the package to read from your routers, stores, or API layers.

## Composition overview

A typical dialog stacks:

1. `Dialog` — root with optional `open` / `onOpenChange` for controlled state.
2. `DialogTrigger` — host-provided trigger button or link.
3. `DialogContent` — portaled panel with built-in overlay and close affordance.
4. `DialogHeader`, `DialogTitle`, `DialogDescription`, and footer slots — host copy.

```tsx
import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@you-agent-factory/components";

export function ConfirmActionDialog({
  description,
  title,
}: {
  description: string;
  title: string;
}) {
  const [open, setOpen] = useState(false);

  return (
    <Dialog onOpenChange={setOpen} open={open}>
      <DialogTrigger className="rounded-lg border border-outline px-4 py-2">
        Open dialog
      </DialogTrigger>
      <DialogContent aria-describedby={undefined} closeLabel="Close dialog">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <p className="text-body-medium text-on-surface">
          Host-provided dialog body copy.
        </p>
      </DialogContent>
    </Dialog>
  );
}
```

Popover, collapsible, and scroll-area examples follow the same host-owned copy
pattern:

```tsx
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ScrollArea,
} from "@you-agent-factory/components/overlays";

export function DisclosureAndScrollExample() {
  return (
    <div className="space-y-6">
      <Popover>
        <PopoverTrigger className="rounded-lg border border-outline px-4 py-2">
          Open popover
        </PopoverTrigger>
        <PopoverContent aria-label="Host-provided popover label">
          <p className="text-body-medium text-on-surface">
            Host-provided popover content.
          </p>
        </PopoverContent>
      </Popover>

      <Collapsible className="w-72 rounded-2xl border border-outline p-3">
        <CollapsibleTrigger className="w-full text-left">
          Toggle details
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-3 text-body-medium text-on-surface-variant">
          Host-provided collapsible content.
        </CollapsibleContent>
      </Collapsible>

      <ScrollArea
        aria-label="Host-provided scroll region"
        className="h-32 w-72 rounded-2xl border border-outline p-3"
      >
        <div className="space-y-3 text-body-medium text-on-surface">
          <p>Scrollable host content.</p>
        </div>
      </ScrollArea>
    </div>
  );
}
```

## Accessible labels and naming

Every overlay surface needs an accessible name supplied by the host. The package
does not invent product copy.

### Dialog

- Render `DialogTitle` for the required accessible name. Radix wires the title to
  the dialog content region.
- Use `DialogDescription` when supplementary text helps screen-reader users
  understand the dialog purpose. Omit it with `aria-describedby={undefined}` on
  `DialogContent` when no description is needed.
- Pass `closeLabel` to `DialogContent` so the built-in close button exposes an
  `aria-label` in the host locale.
- Triggers must have visible text or an `aria-label`.

### Popover

- Popover panels are not modal dialogs. Give `PopoverContent` an accessible name
  with `aria-label` or by including visible heading text inside the panel.
- Triggers must have visible text or an `aria-label`.
- Use `PopoverAnchor` when the visual anchor differs from the trigger element.

### Collapsible

- `CollapsibleTrigger` must expose visible trigger text or `aria-label`.
- Radix reflects expanded/collapsed state through `aria-expanded` on the trigger.
- Nested collapsibles should use distinct trigger labels so users can tell
  sections apart.

### ScrollArea

- When the scroll region is meaningful on its own, pass `aria-label` or
  `aria-labelledby` on `ScrollArea` pointing at host-visible labeling text.
- Focusable children inside the viewport remain keyboard reachable; do not
  suppress focus outlines inside the scroll viewport.

## Trigger and content relationships

| Primitive | Trigger | Content | Package behavior |
| --- | --- | --- | --- |
| Dialog | `DialogTrigger` opens the modal | `DialogContent` portals above the page | Focus moves into the dialog, is trapped while open, and returns to the trigger on close |
| Popover | `PopoverTrigger` toggles the popover | `PopoverContent` portals near the trigger | Focus stays in the popover while open; outside interaction dismisses |
| Collapsible | `CollapsibleTrigger` toggles disclosure | `CollapsibleContent` expands inline | Trigger `aria-expanded` mirrors open state |
| ScrollArea | No separate trigger | Children scroll inside the viewport | Viewport keeps native scrolling and focus visibility |

Host apps choose trigger elements, placement classes, and panel children. The
package wires Radix primitives and default shell styling only.

## Focus, Escape, and controlled open state

### Dialog

- Opens from its trigger, moves focus into the dialog, and traps focus while
  open.
- Closes on Escape and returns focus to the trigger.
- Supports controlled usage with `open` and `onOpenChange` on `Dialog`, or
  uncontrolled usage with `defaultOpen`.

### Popover

- Opens from its trigger and supports controlled `open` / `onOpenChange`.
- Closes on Escape and outside interaction.
- Returns focus to the trigger or another sensible host-provided target after
  dismiss.

### Collapsible

- Toggles from keyboard activation on `CollapsibleTrigger` (Enter/Space).
- Supports controlled `open` / `onOpenChange` or uncontrolled `defaultOpen`.
- Exposes expanded/collapsed state to assistive technology through Radix trigger
  semantics.

### ScrollArea

- Remains keyboard reachable when it contains focusable or scrollable content.
- Preserves focus outlines inside the viewport (`outline-none` applies to the
  viewport shell, not to focusable descendants).

Use controlled open state when multiple host controls need to open, close, or
reflect the same overlay. Use uncontrolled state for simple trigger-only flows.

## Mobile sizing and overflow guidance

Long host content must stay reachable without forcing horizontal page overflow.

### Dialog

- `DialogContent` uses `max-h-dvh`, horizontal inset padding, and `overflow-y-auto`
  so tall content scrolls inside the dialog shell.
- Keep body copy in a single column. Avoid fixed widths wider than the viewport.
- Review the `MobileViewport` Storybook story before shipping long forms or dense
  tables inside dialogs.

### Popover

- Default `PopoverContent` width is responsive (`w-72` / `sm:w-80`). Add
  `max-h-*` and `overflow-y-auto` for long panels.
- Use `collisionPadding`, `side`, and `align` props when placing popovers near
  viewport edges so content stays on screen.

### Collapsible

- Disclosed content expands inline. Constrain width with host layout classes and
  avoid unbreakable horizontal content inside narrow panels.

### ScrollArea

- Set explicit `className` height/width bounds on `ScrollArea` so overflow is
  obvious.
- Add `ScrollBar` with `orientation="horizontal"` only when horizontal scrolling
  is intentional.
- Use `viewportClassName` or `viewportProps` when the host needs additional
  viewport semantics.

## Key props

### `DialogContent`

| Prop | Purpose |
| --- | --- |
| `closeLabel` | Accessible name for the built-in close button. Defaults to `"Close"`. |
| `closeDisabled` | Renders a disabled close affordance without dismiss behavior. |
| `className` | Host layout overrides on the portaled panel. |

### `PopoverContent`

| Prop | Purpose |
| --- | --- |
| `align` / `side` / `sideOffset` | Placement relative to the trigger. |
| `collisionPadding` | Keeps content inside the viewport near screen edges. |
| `className` | Host sizing and overflow classes (`max-h-*`, `overflow-y-auto`). |

### `Collapsible`

| Prop | Purpose |
| --- | --- |
| `open` / `onOpenChange` | Controlled disclosure state. |
| `defaultOpen` | Initial state for uncontrolled usage. |

### `ScrollArea`

| Prop | Purpose |
| --- | --- |
| `viewportClassName` / `viewportProps` | Host viewport overrides. |
| `type` | Radix scrollbar visibility (`auto`, `always`, `scroll`, `hover`). |
| `aria-label` / `aria-labelledby` | Host-provided region labeling. |

Layout helpers exported from the overlays category:

- `OVERLAY_DIALOG_CONTENT_SHELL_CLASS` — default dialog content padding shell.
- `OVERLAY_DIALOG_BODY_CLASS` — body spacing helper for dialog children.
- `OVERLAY_FORM_GROUP_CLASS` — header/footer grouping spacing.

## Accessibility expectations

- Dialogs require a `DialogTitle` or equivalent `aria-labelledby` relationship.
- Popover panels need host-provided names via `aria-label` or visible headings.
- Collapsible triggers must be keyboard operable buttons or links with visible
  text or `aria-label`.
- Scroll regions that organize distinct content need host-provided
  `aria-label` / `aria-labelledby`.
- Do not remove focus outlines from interactive children inside overlays.
- Icon-only triggers and close buttons must include `aria-label`.
- Controlled open state does not remove the host responsibility to supply
  accessible names and trigger text.

## Storybook visual reference

Package Storybook lives under `Overlays/*`. Stories use package imports and
package token decorators only — no dashboard providers.

| Component | Story | Storybook id | Demonstrates |
| --- | --- | --- | --- |
| Dialog | Default | `overlays-dialog--default` | Baseline dialog render |
| Dialog | Long content | `overlays-dialog--long-content` | Vertical overflow inside the shell |
| Dialog | Controlled open | `overlays-dialog--controlled-open` | Host-controlled `open` state |
| Dialog | Escape close | `overlays-dialog--escape-close` | Escape dismiss and focus return |
| Dialog | Mobile viewport | `overlays-dialog--mobile-viewport` | Narrow viewport reachability |
| Dialog | Keyboard focus | `overlays-dialog--keyboard-focus` | Focus trap and keyboard operation |
| Popover | Default | `overlays-popover--default` | Baseline popover render |
| Popover | Long content | `overlays-popover--long-content` | Scrollable popover panel |
| Popover | Controlled open | `overlays-popover--controlled-open` | Host-controlled `open` state |
| Popover | Keyboard open | `overlays-popover--keyboard-open` | Keyboard trigger activation |
| Popover | Viewport placement | `overlays-popover--viewport-placement` | Collision-safe edge placement |
| Popover | Keyboard focus | `overlays-popover--keyboard-focus` | Focus and dismiss behavior |
| Collapsible | Default | `overlays-collapsible--default` | Baseline disclosure |
| Collapsible | Open | `overlays-collapsible--open` | Expanded default state |
| Collapsible | Controlled | `overlays-collapsible--controlled` | Host-controlled disclosure |
| Collapsible | Nested content | `overlays-collapsible--nested-content` | Nested disclosure sections |
| Collapsible | Keyboard focus | `overlays-collapsible--keyboard-focus` | Keyboard toggle |
| ScrollArea | Default | `overlays-scroll-area--default` | Vertical overflow scrolling |
| ScrollArea | Horizontal overflow | `overlays-scroll-area--horizontal-overflow` | Intentional horizontal scroll |
| ScrollArea | Keyboard focus | `overlays-scroll-area--keyboard-focus` | Nested focus reachability |
| ScrollArea | Mobile width | `overlays-scroll-area--mobile-width` | Narrow viewport scrolling |

Run package Storybook locally:

```bash
cd ui/packages/components
bun run storybook
```

Browser verification for overlay stories:

```bash
cd ui/packages/components
bun run verify:storybook-browser
```

## Allowed dependencies

Overlay source may import:

- Package utilities (`cn` from `@you-agent-factory/components/utilities`)
- Package layout helpers from `overlay-layout.ts`
- Radix overlay primitives (`@radix-ui/react-dialog`, `react-popover`,
  `react-collapsible`, `react-scroll-area`)
- Package token CSS via the host `styles.css` import
- React and `react-dom` peer dependencies

Overlay source must **not** import:

- Dashboard feature modules, routes, or providers
- Generated OpenAPI clients or dashboard API adapters
- React Query, Zustand, Monaco, Sonner, or dashboard i18n/session providers
- Factory/work/session/provider domain types

`check:package-boundary` enforces these rules in CI.

## Source-copy guidance

Teams may copy overlay files into a host repository to customize markup while
keeping the same token foundation:

1. Import `@you-agent-factory/components/styles.css` in the host app.
2. Copy only the overlay files you need from `src/overlays/`.
3. Keep copied code on the package side of your boundary — do not pull dashboard
   modules into copied overlay files.
4. Retain host ownership of labels, trigger text, open state, and overflow-safe
   content.

**No source-copy CLI or generator ships with this package.** Copy-and-adapt
workflows are manual and maintained by the host application.
