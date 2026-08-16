# Button primitives

`Button`, `ButtonLink`, and `IconButtonShell` are the reusable action primitives in
`@you-agent-factory/components`. They provide semantic tones, loading behavior,
icon-only sizing, and shared focus-visible treatment without dashboard routes,
providers, API clients, or generated OpenAPI types.

## Import paths

```ts
import {
  Button,
  ButtonLink,
  IconButtonShell,
  buttonVariants,
  type ButtonProps,
  type ButtonLinkProps,
  type IconButtonShellProps,
} from "@you-agent-factory/components";

import {
  Button as PrimitiveButton,
  ButtonLink as PrimitiveButtonLink,
  IconButtonShell as PrimitiveIconButtonShell,
} from "@you-agent-factory/components/primitives";
```

Import `@you-agent-factory/components/styles.css` once in the host application
before rendering buttons. See the package [README](../README.md) for CSS setup.

## Button vs ButtonLink

| Primitive | Renders | Use when |
| --- | --- | --- |
| `Button` | `<button>` (or child via `asChild`) | The action stays on the current view: submit forms, open dialogs, trigger async work, or run in-place commands. |
| `ButtonLink` | `<a>` styled with shared button tokens | Navigation should use a real link: route changes, external URLs, or downloads where anchor semantics and `href` behavior are required. |

`ButtonLink` does not support `loading` because anchor elements do not expose the
same busy/disabled interaction model as buttons. Show progress elsewhere when a
navigation target is not yet ready.

`Button` supports `asChild` for composition, but prefer `ButtonLink` when the
rendered element must be an anchor.

## Semantic button variants

Set `tone` on `Button` and `ButtonLink`, or on `IconButtonShell` for toolbar
actions. Variants map to semantic roles rather than dashboard feature styling.

| `tone` | Role | When to use |
| --- | --- | --- |
| `default` | Primary | The single most important action in a section or dialog footer. |
| `secondary` | Secondary | Supporting actions that still need button emphasis, such as "Save draft". |
| `outline` | Outline | Neutral actions on busy surfaces or paired secondary controls in toolbars. |
| `ghost` | Ghost | Low-emphasis actions on dense panels where borders would add noise. |
| `destructive` | Destructive | Irreversible or high-risk operations such as delete or remove. Keep the label specific; do not rely on color alone. |
| `warning` | Warning | Actions that need caution but are not strictly destructive, such as "Review warnings". |

`IconButtonShell` also supports `tone="dangerGhost"` for compact destructive icon
actions. It keeps icon-button sizing while applying destructive hover treatment.

### Sizes

`Button` and `ButtonLink` accept `size` values such as `default`, `sm`, `lg`,
`pill`, `icon`, and `iconPill`. `IconButtonShell` fixes toolbar-friendly
`h-11 w-11` icon sizing and should be used for compact icon-only controls.

## Loading buttons

Pass `loading` to `Button` (including `IconButtonShell`) when an action is in
progress:

- Sets `aria-busy="true"` and disables the control to prevent duplicate activation.
- Preserves the accessible name from visible text or `aria-label`.
- Renders a centered spinner overlay while keeping label content in the accessibility tree.

Do not attach separate click handlers that ignore the disabled state while
`loading` is true.

## Icon-only accessibility

Icon-only buttons must expose a clear accessible name:

- Prefer visible text when space allows.
- Otherwise provide `aria-label` with a specific action name such as
  `"Refresh jobs"` or `"Export dashboard"`.
- Mark decorative icons with `aria-hidden="true"` so screen readers use the
  button label only.

`IconButtonShell` inherits the `h-11 w-11` / toolbar touch target from the
shared button size tokens. Avoid shrinking icon-only controls below the package
defaults unless the host design system documents an exception.

## Host application responsibilities

The component package owns presentation: markup, semantic tones, focus rings,
loading overlays, and token-backed styling.

Host applications own:

- **Action copy** — button labels, `aria-label` text, and confirmation messaging.
- **Click handlers** — business logic, validation, and error handling.
- **Routing** — `href` values, client-side navigation, and download targets for `ButtonLink`.
- **Domain workflows** — when an action is enabled, loading, or destructive in product context.

Pass data and callbacks into button primitives as props. Do not expect the
package to read app stores, routers, or API layers.

## Storybook examples

Package Storybook stories demonstrate expected states with package imports only:

- `Primitives/Button/Semantic variants` — normal, disabled, destructive, warning,
  and link-like examples.
- `Primitives/Button/Loading and icon only` — loading, focus-visible, icon-only,
  and destructive icon shell examples.

Run `bun run storybook` from `ui/packages/components` or inspect the static
build with `bun run build-storybook`.
