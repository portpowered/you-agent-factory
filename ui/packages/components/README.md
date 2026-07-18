# @you-agent-factory/components

Presentation components, shared design tokens, and small utilities for building
factory-style UIs outside the dashboard. The package is domain-free: it does not
ship data fetching, durable state, business workflows, or product copy.

## Install and workspace setup

Add the package to your application dependencies. In this monorepo the dashboard
uses a workspace link:

```json
{
  "dependencies": {
    "@you-agent-factory/components": "file:./packages/components"
  }
}
```

Published consumers should depend on the same package name
(`@you-agent-factory/components`) from their chosen distribution channel.

Peer dependencies:

- `react` ^19.0.0
- `react-dom` ^19.0.0

Runtime libraries used by category entrypoints (for example `recharts`,
`@xyflow/react`, and Radix UI) are installed as direct package dependencies.
Hosts only need to provide compatible React and React DOM peers.

## Import package styles

Import the package styles entrypoint once in your application CSS (or an
equivalent global stylesheet hook) before rendering components:

```css
@import "@you-agent-factory/components/styles.css";
```

With Tailwind CSS v4, a typical host `styles.css` also imports Tailwind and may
layer app-specific utilities after the package tokens:

```css
@import "tailwindcss";
@import "@you-agent-factory/components/styles.css";

/* Host-only utilities and foundation overrides */
```

The entrypoint exposes shared role tokens, palette presets, typography tokens,
and layout tokens. Host applications do not need dashboard routes, providers,
API clients, or generated OpenAPI types to use these tokens.

## Import components and utilities

Category entrypoints are deep imports under the package name:

```ts
import { COMPONENTS_PACKAGE_NAME } from "@you-agent-factory/components";
import * as primitives from "@you-agent-factory/components/primitives";
import * as forms from "@you-agent-factory/components/forms";
import { cn } from "@you-agent-factory/components/utilities";
```

`COMPONENTS_PACKAGE_NAME` is the stable package identifier
(`"@you-agent-factory/components"`). Category paths include `primitives`, `forms`,
`layout`, `feedback`, `data-display`, `navigation`, `overlays`, `charts`,
`graphs`, `visualizers`, `recipes`, `icons`, `utilities`, `testing`, and
`tokens`.

Use `cn` from `@you-agent-factory/components/utilities` for class name composition
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

1. Import `@you-agent-factory/components/styles.css` in the host app so copied
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
- [Button primitives](./docs/button.md) — semantic variants, `ButtonLink`
  guidance, loading behavior, and icon-only accessibility labels.
- [Typography roles](./docs/typography-roles.md) — `Text`, `Heading`, `Label`,
  `Code`, dense text, truncation, and wrapping
- [Layout and display primitives](./docs/layout-display-primitives.md) —
  `SurfacePanel`, `ActionRow`, `DescriptionList`, and responsive layout guidance
- [`docs/graphs.md`](./docs/graphs.md) — graph node shell, node button, edge,
  viewport surface, handle badge, edge path helpers, React Flow boundary, and
  Storybook example map
- [`docs/factory-topology-replay.md`](./docs/factory-topology-replay.md) —
  controlled prepared-topology rendering, host ownership, endpoint validity,
  messages, selection, and failure containment
- [AlertPanel semantic feedback variants](./docs/feedback-alert-panel.md)
- [Skeleton loading placeholders](./docs/feedback-skeleton.md)
- [CodePanel long-content containment](./docs/data-display-code-panel.md)
- [Table primitives and DataTable](./docs/table-data-table.md)
- [Form input primitives](./docs/forms-input-primitives.md)
- [Form field structure and messaging](./docs/forms-form-field.md) — `FormField`,
  labels, descriptions, helper text, warnings, errors, success messages,
  grouped-control semantics, and host validation responsibilities
- [Form select primitives](./docs/forms-select-primitives.md) — `Select`,
  `NativeSelect`, `EnumSelect`, keyboard behavior, and option contracts
- [Overlay and disclosure primitives](./docs/overlays.md)
- [Widget frame and layout recipes](./docs/widget-frame-recipes.md)

See that directory for component-level documentation as it is added. The docs
shell is plain markdown in version control; it does not require a docs-site
route, generated registry, or additional build-time documentation tooling.

- [Charts](./docs/charts.md) — config, presentation, tooltip and legend,
  state panels, and caller-owned series state

## Development

From `ui/packages/components`:

```bash
bun run typecheck
bun run test
bun run test:build
bun run check:pack
bun run check:installed-consumer
bun run check:package-boundary
bun run check:package-dependency-direction
bun run build-storybook
bun run verify:storybook-browser
```

Package tests use `src/testing/vitest.setup.ts` and `src/testing/render.tsx`
for DOM cleanup, accessible assertions, and user interactions. They do not
require dashboard routes, providers, generated clients, API mocks, React
Query, Zustand, Monaco, or Sonner.

`check:pack` performs a clean production build, creates the registry-format
tarball in a temporary directory, and validates npm's reported file inventory.
It checks every manifest export and transitive local stylesheet or asset
reference, rejects development-only files, and removes the temporary artifact
after verification. `test:build` covers both the compiled output and this real
pack flow, including actionable failure diagnostics.

`check:installed-consumer` installs that registry-format tarball and compatible
React peers into a new temporary npm application, then typechecks and creates a
Vite production bundle without workspace links, source aliases, or package
source files. Chromium loads only the built consumer output at mobile and
desktop viewports and verifies package styles, representative primitive,
feedback, chart, and graph rendering, keyboard activation and focus, accessible
state semantics, the shared React runtime, and page-level overflow. The check
removes its temporary install and closes its verification server on exit.

Package Storybook lives in `.storybook/` and discovers `src/**/*.stories.tsx`
files. Preview decorators import the package token fixture stylesheet and
apply `data-color-palette` locally; they do not mount dashboard session,
i18n, API, React Query, Zustand, Monaco, or Sonner providers.

`check:package-boundary` scans package production source and fails when files
import dashboard API modules, feature modules, generated OpenAPI clients,
dashboard i18n providers, dashboard session providers, React Query, Zustand,
Monaco, or Sonner. Violations report the package file and import path.

`check:package-dependency-direction` scans package production source and fails
when a lower package layer imports a higher layer (for example primitives
importing recipes) or when production source imports testing support modules.
Violations report the package file, import path, and both source and target
layers.

From the repository root:

```bash
make ui-components-typecheck
make ui-components-test
make ui-components-storybook
make ui-components-boundary
make ui-components-dependency-direction
make ui-components-build
make ui-components-pack
make ui-components-installed-consumer
make ui-components-verify
make ui-lint
```

`make ui-components-verify` runs the full component package harness with labeled
failure output. It creates the production build first so package self-imports
resolve on a clean checkout, then runs typecheck, tests, Storybook build,
boundary checks, dependency-direction checks, registry-pack inventory, and the
clean installed-consumer smoke. CI installs Chromium and runs the same harness
in the Lint workflow after dashboard lint.

From the `ui` workspace root:

```bash
make ui-lint
bun run check:semantic-colors
```
