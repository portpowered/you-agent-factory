# Dashboard scrollbar consistency

## Contract

The dashboard has one semantic scrollbar presentation contract for native
scrollports, the shared Radix `ScrollArea`, and Monaco editor sliders. Scroll
ownership remains unchanged: `document.documentElement` owns document scroll,
bento cards keep their existing local `ScrollArea` viewport, and ordinary
controls and panels keep their existing `overflow-*` element.

Native scrollports inherit the base recipe in `ui/src/styles.css` by behavior,
not by feature-local classes. The portable declaration uses
`scrollbar-color: var(--color-outline-variant) transparent` and
`scrollbar-width: thin`. Chromium/WebKit pseudo-elements use a 10 CSS-pixel
allocation in both axes, a transparent track, a 1 CSS-pixel transparent inset,
and a rounded thumb whose approximately 8 CSS-pixel visible body uses
`outline-variant` at rest and `on-surface-variant` while hovered or active.
The recipe never changes `overflow`, scroll thresholds, geometry, or scrollbar
visibility policy.

When `forced-colors: active` is enabled, authored colors, sizing, and pseudo-
element treatment revert so the browser and operating system provide the
accessible system scrollbar. Overlay and auto-hidden platform behavior remains
platform controlled.

The Radix viewport is an intentional exception: its shared primitive hides the
native scrollbar and renders exactly one custom track/thumb. It must not be
converted to native scrolling or receive a duplicate scrollbar. Monaco's
synthetic slider is a separate token projection: its resting
`scrollbarSlider.background` uses `outline-variant`, while hover and active use
`on-surface-variant`. It keeps its existing editor dimensions and guard-selector
zero-width behavior.

## Production native-scrollport audit

The audit covers document scrolling, shared textareas, package table/code/data
panels, dialogs, prompt/document editor fallbacks, submit-work and invocation
panels, worker-session payload/timeline panels, session tabs, workflow activity
details, and factory graph editor menus/controls. These surfaces retain their
existing overflow owner and receive the base native recipe automatically.

| Surface | Existing owner | Presentation contract |
| --- | --- | --- |
| Dashboard document | `document.documentElement` | Native base recipe |
| Submit/work textareas | Existing `<textarea>` | Native base recipe |
| Tables, code/payload, dialogs, tabs, activity details | Existing `overflow-*` element | Native base recipe |
| Factory graph menus and controls | Existing popover/menu child | Native base recipe; regression verification only |
| Bento-card body | Shared Radix `ScrollArea` viewport | Intentional single custom Radix track/thumb |
| Monaco prompt/document editor | Monaco synthetic slider | Shared semantic theme projection |
| Guard-selector editor | Monaco synthetic slider | Intentional zero-width scrollbar |

Graph-editor source files remain sibling-owned and are not edited by this lane;
the base-layer effect is verified as a regression surface.

## Evidence

Focused coverage proves the compiled native recipe's semantic roles, geometry,
cross-browser fallback, and forced-colors escape. Existing dashboard single-
scroll, textarea, and Radix `ScrollArea` behavior tests continue to prove
overflow ownership and interaction. Direct browser verification must create
actual overflow and exercise document, textarea, vertical, horizontal, Radix,
Monaco, and graph-menu surfaces at narrow and desktop widths, 200% zoom, and
forced-colors where supported.

Monaco theme coverage proves that prompt, document, and guard-selector themes
receive the same palette-derived slider roles. Direct browser verification must
also confirm actual editor overflow, palette switching, keyboard and pointer
interaction, and the guard-selector's hidden scrollbar contract.
