# youagentfactory/components

Presentation components, shared design tokens, and small utilities for building
factory-style UIs outside the dashboard. The package is domain-free: it does not
ship data fetching, durable state, business workflows, or product copy.

## Install and workspace setup

Add the package to your application dependencies. In this monorepo the dashboard
uses a workspace link:

```json
{
  "dependencies": {
    "youagentfactory/components": "file:./packages/components"
  }
}
```

Published consumers should depend on the same package name
(`youagentfactory/components`) from their chosen distribution channel.

Peer dependencies:

- `react` ^19.0.0
- `react-dom` ^19.0.0

Some category entrypoints (for example charts and graphs) also require the
package's direct dependencies (`recharts`, `@xyflow/react`). Install those in
the host app when you import those categories.

## Import package styles

Import the package styles entrypoint once in your application CSS (or an
equivalent global stylesheet hook) before rendering components:

```css
@import "youagentfactory/components/styles.css";
```

With Tailwind CSS v4, a typical host `styles.css` also imports Tailwind and may
layer app-specific utilities after the package tokens:

```css
@import "tailwindcss";
@import "youagentfactory/components/styles.css";

/* Host-only utilities and foundation overrides */
```

The entrypoint exposes shared role tokens, palette presets, typography tokens,
and layout tokens. Host applications do not need dashboard routes, providers,
API clients, or generated OpenAPI types to use these tokens.

## Import components and utilities

Category entrypoints are deep imports under the package name:

```ts
import { COMPONENTS_PACKAGE_NAME } from "youagentfactory/components";
import * as primitives from "youagentfactory/components/primitives";
import * as forms from "youagentfactory/components/forms";
import { cn } from "youagentfactory/components/utilities";
```

`COMPONENTS_PACKAGE_NAME` is the stable package identifier
(`"youagentfactory/components"`). Category paths include `primitives`, `forms`,
`layout`, `feedback`, `data-display`, `navigation`, `overlays`, `charts`,
`graphs`, `recipes`, `icons`, `utilities`, `testing`, and `tokens`.

Use `cn` from `youagentfactory/components/utilities` for class name composition
in component code instead of dashboard-local helpers.

## Consumer responsibilities

The component package owns presentation: markup, styles, tokens, and generic
utilities. Host applications own everything domain-specific:

- **Data fetching** — API calls, caching, and synchronization with backend
  services.
- **Durable state** — session persistence, undo/redo, and cross-route state
  that outlives a single render.
- **Business workflows** — factory orchestration, validation rules, and product
  behavior tied to your domain model.
- **Domain copy** — user-visible strings, labels, and messaging for your
  product.

Pass data and callbacks into components as props; do not expect the package to
reach into your app's stores, routers, or API layer.

## Theming

Theme through package tokens and semantic roles, not dashboard-specific CSS.

1. **Palette presets** — Set `data-color-palette` on a root element (or
   `:root`) to select a built-in preset. Available presets include
   `factory-dark`, `factory-light`, `material-baseline`, `slate`, and `olive`.
   Presets override `--color-af-foundation-*` keys defined in the package token
   layer.

   ```html
   <html data-color-palette="factory-dark">
   ```

2. **Semantic role tokens** — Components consume roles such as
   `--color-background`, `--color-surface`, `--color-primary`, and typography
   roles from the package token CSS. Prefer these roles in custom styles
   instead of hard-coded hex values or dashboard-only class names.

3. **Host foundation keys** — When you need a custom brand baseline, override
   `--color-af-foundation-*` variables in your host stylesheet after importing
   package styles. Role tokens and palette presets cascade from those keys.

The package does not require the dashboard `styles.css` or dashboard-only
utility sheets.

## Source-copy guidance (documentation only)

Some teams copy component source into their repository to customize markup or
styles while keeping the same token foundation. Supported patterns today:

1. Import `youagentfactory/components/styles.css` in the host app so copied
   components receive the same tokens.
2. Copy the component files you need from the package category you depend on.
3. Keep copied code on the package side of your boundary — do not pull in
   dashboard modules, API clients, or generated OpenAPI types.

**This README describes source-copy as guidance only.** No source-copy CLI,
install command, or code generator is shipped as part of this package. Any
copy-and-adapt workflow is manual and maintained by the host application.

## Per-component documentation

Per-component usage notes, props tables, and examples live in the package docs
directory:

- [`docs/`](./docs/)

See that directory for component-level documentation as it is added. The docs
shell is plain markdown in version control; it does not require a docs-site
route, generated registry, or additional build-time documentation tooling.

## Development

From `ui/packages/components`:

```bash
bun run typecheck
bun run test
bun run build-storybook
bun run verify:storybook-browser
```

Package tests use `src/testing/vitest.setup.ts` and `src/testing/render.tsx`
for DOM cleanup, accessible assertions, and user interactions. They do not
require dashboard routes, providers, generated clients, API mocks, React
Query, Zustand, Monaco, or Sonner.

Package Storybook lives in `.storybook/` and discovers `src/**/*.stories.tsx`
files. Preview decorators import the package token fixture stylesheet and
apply `data-color-palette` locally; they do not mount dashboard session,
i18n, API, React Query, Zustand, Monaco, or Sonner providers.

From the `ui` workspace root:

```bash
make ui-lint
bun run check:semantic-colors
```
