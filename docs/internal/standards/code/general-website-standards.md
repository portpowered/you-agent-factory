# General Website Standards

---
author: andreas abdi
last modified: 2026, may, 27
doc-id: STD-016
---

This document defines the baseline standards for websites built in this repository. It is intended to be broad enough for most product surfaces while still being concrete enough to review against.

For this repository, these standards apply directly to the React and Vite UI under `ui/`, which currently uses Bun, TypeScript, Tailwind CSS v4, React Query, Zustand, Storybook, Vitest, Biome, generated OpenAPI types, and Playwright-compatible tooling.

## Usage

Every contributor who changes website UI, frontend state, styling, interaction flows, or frontend tests **MUST** review this standard before implementation or review.

## Quick Rules

- Build pages from reusable UI and feature components instead of bespoke page-only markup.
- Route all network access through typed API modules and stateful hooks; do not scatter direct `fetch` calls throughout components.
- Preserve canonical API or domain models as the editable source of truth when they exist; use view models and projections for UI-specific shapes.
- Keep third-party UI library state, such as graph nodes or table rows, as a projection instead of the durable source of domain behavior.
- Prefer concise icon-led actions and established shared controls over text-heavy toolbars, instructional clutter, or bespoke button variants.
- Represent loading, empty, error, and success states explicitly for every network-backed surface.
- Use shared design tokens and Tailwind utility patterns; avoid one-off colors, spacing, and typography values when a shared token can express the same intent.
- Ship accessible semantics, keyboard support, visible focus, and sufficient color contrast by default.
- Design mobile-first and verify tablet and desktop layouts before merge.
- Optimize Core Web Vitals, asset delivery, and bundle behavior from the start rather than as cleanup work.
- Verify cross-browser behavior and graceful degradation for critical flows.
- Prefer component and functional test coverage over excessive unit-only coverage, while still testing critical logic directly.
- Treat performance, resilience, and observability as product requirements, not polish work.

## Review Checklist

Before approval, reviewers **SHOULD** confirm:

- The page or component uses the shared architecture layers correctly.
- Complex interactive surfaces have a clear canonical model, operation layer, projection adapter, and component wiring layer.
- Data loading, mutations, retries, and caching are handled by approved stateful abstractions.
- Empty, loading, error, and success states are present and intentional.
- Styling uses shared tokens, utilities, and existing patterns instead of custom ad hoc values.
- Actions use the shared button/action vocabulary unless a new reusable primitive is justified.
- The UI remains usable on small, medium, and large viewports.
- Accessibility semantics and keyboard interaction are present for all interactive elements.
- Localizable copy is externalized correctly and resource boundaries remain maintainable.
- The change includes the right mix of unit, component, functional, and integration evidence.
- The change does not introduce obvious performance regressions, hydration issues, layout instability, or unnecessary rerenders.
- Critical flows behave correctly on supported browsers and under degraded network conditions.

## Regulations

### 1. Architecture and Layering

Website code **MUST** be organized around reusable layers with clear dependency direction.

Preferred structure:

- `ui/src/api/` for transport, handwritten API wrappers, request/response normalization, generated OpenAPI types, and API-focused tests
- `ui/src/components/` for shared presentational, dashboard, and primitive UI building blocks
- `ui/src/features/` for feature-specific UI, hooks, view models, state, messages, selectors, and orchestration
- `ui/src/features/<feature>/public/` for the feature's intentional import boundary
- `ui/src/features/<feature>/components/` for feature-owned React components
- `ui/src/features/<feature>/hooks/` for feature-owned stateful React hooks
- `ui/src/features/<feature>/lib/` for feature-owned pure logic, projections, operations, parsers, and helpers
- `ui/src/features/<feature>/messages/` for feature-owned locale/message catalogs
- `ui/src/features/<feature>/state/` for feature-owned Zustand stores, reducers, selectors, and state models
- `ui/src/i18n/` for locale infrastructure, shared formatters, and fallback behavior
- `ui/src/lib/` for cross-feature pure utilities and framework glue
- `ui/src/testing/` for reusable test helpers, fixtures, and render harnesses
- `ui/src/types/` for shared TypeScript types that do not belong to a generated API contract or one feature
- `ui/.storybook/` for Storybook configuration
- `ui/integration/` for integration-oriented frontend tests and fixtures
- `ui/scripts/` for repository-owned frontend checks, generation, and build tooling

Rules:

- Page- or screen-level composition **SHOULD** be thin and delegate behavior to feature modules.
- Shared UI primitives **MUST NOT** depend on feature-specific modules.
- Feature modules **MAY** depend on shared components, shared utilities, testing helpers in tests, and API modules.
- Feature root directories under `ui/src/features/<feature>/` **MUST** contain subdirectories only. New feature files belong under an approved subdirectory such as `public/`, `components/`, `hooks/`, `messages/`, `state/`, `selectors/`, `lib/`, or a more specific domain folder.
- Cross-feature imports **SHOULD** go through another feature's `public/` boundary unless a narrower local exception is already established and documented by existing code.
- Feature-local state and hooks **SHOULD** remain inside the owning feature. Create cross-feature `ui/src/lib/` or shared component utilities only when there is a real cross-feature owner and reuse case.
- Network transport, parsing, and server contract details **MUST NOT** live inline inside rendering components.
- Generated API clients **MUST** remain generated artifacts; handwritten wrappers belong alongside them rather than inside them.

Canonical model and projection rules:

- When a feature has a canonical API or domain model, editable feature state **SHOULD** preserve and operate on that model rather than inventing a second durable internal schema.
- UI-specific shapes for third-party libraries, visualizations, tables, canvases, graphs, and drag surfaces **SHOULD** be treated as projections from feature state, not as the source of truth for domain behavior.
- A projection **MUST** be disposable and reproducible from canonical state plus explicit UI state. If a projection cannot be rebuilt without losing editable domain data, the architecture needs a clearer source-of-truth boundary.
- A feature **MUST NOT** reconstruct editable state from a lossy presentation or read-model projection unless the lossiness is documented, covered by tests, and blocked from save/edit paths until complete data is available.

Complex interactive surfaces:

- Editors, builders, graph tools, workflow canvases, multi-step forms with domain rules, and similar complex surfaces **SHOULD** expose pure feature operations or a small service layer for behaviors such as add, remove, connect, disconnect, reorder, validate, and save.
- Components and hooks **MAY** orchestrate those operations, but they **SHOULD NOT** encode domain mutations inline when the rules are shared, stateful, or independently testable.
- Reviewers **SHOULD** be able to point to the canonical model, the operation or service layer, the projection adapter, and the component wiring layer. When those layers are blended into one large component or hook, the change should be treated as an architecture risk.
- If callbacks and state slices are passed through multiple component levels for one feature workflow, authors **SHOULD** introduce a feature view model, controller hook, context, or compact action interface rather than extending prop drilling.
- Compatibility adapters introduced during migration **SHOULD** have an explicit replacement goal or removal condition. New canonical paths **SHOULD NOT** become permanent parallel implementations beside old presentation-specific paths.

### 2. Network and State Management

Frontend state **MUST** distinguish between server state and client state.

Rules:

- Server state **MUST** be handled through approved query or mutation abstractions such as React Query.
- Client-only state **MUST** live in explicit state containers or local component state, depending on scope.
- Components **MUST NOT** issue ad hoc network calls in render paths or event handlers when an API module and hook should own that behavior.
- API access **MUST** be typed, centralized, and reusable.
- Retries, timeout behavior, cancellation, and backoff strategy **SHOULD** be defined deliberately for network-backed flows.
- Persistent client state **MUST** document why persistence is required and what durability boundary is acceptable.
- Optimistic updates **MUST** include rollback behavior or another clear consistency strategy.

Minimum outcomes:

- A user can tell when data is loading.
- A user can recover from a failed request.
- A user does not lose critical in-progress work because of avoidable state placement mistakes.

### 3. Component Design

All UI **MUST** be composed from reusable components with explicit contracts.

Rules:

- Components **MUST** have clear, typed props and a single understandable responsibility.
- Repeated UI patterns **MUST** be extracted once they are used in more than one place or are likely to drift.
- Large components **SHOULD** be split when they mix layout, data access, formatting, and interaction logic in one file.
- Feature components **SHOULD** receive prepared data from hooks or view-model helpers rather than performing dense transformation logic inline.
- Interactive components **MUST** expose disabled, loading, and error-friendly behavior where applicable.
- Components for complex interactive surfaces **SHOULD** receive compact action/view-model props rather than many low-level callbacks that expose internal state machinery.

Action and copy density:

- Toolbars, graph controls, dashboard card headers, dense panels, and repeated item actions **SHOULD** prefer icon-only or icon-plus-short-label controls with accessible names and tooltips when the action is not self-evident.
- Inline explanatory copy **SHOULD** be reserved for empty states, error recovery, destructive confirmations, onboarding moments, and genuinely ambiguous decisions. Do not add visible instructions for controls that can be made clear through standard placement, icons, labels, disabled states, or tooltips.
- Button labels **SHOULD** be short commands such as `Save`, `Discard`, `Connect`, `Retry`, or `Export`. Long rationale, state explanation, and keyboard guidance belong in contextual help, status text, or dialogs only when needed.
- A section or component **SHOULD NOT** repeat the same concept in a heading, paragraph, button label, tooltip, and status pill unless each copy surface adds distinct value.

Shared controls:

- Feature code **SHOULD** use existing shared controls from `ui/src/components/ui/` before creating a bespoke action component. Current shared controls include `Button`, `DashboardActionButton`, `DashboardIconButtonShell`, `DashboardActionRow`, `DashboardStatusPill`, `Dialog`, `Popover`, `Input`, `Select`, `Textarea`, table/data-table primitives, and dashboard shell/typography helpers.
- New bespoke buttons, pills, toggles, segmented controls, dialogs, or toolbar shells **SHOULD NOT** be introduced inside a feature when an existing shared primitive can express the interaction with props or a small reusable extension.
- If a new action/control pattern is needed in more than one place, authors **SHOULD** promote it to a shared component with tests and, where useful, a Storybook story instead of copying Tailwind class clusters across features.
- New button tones or accent treatments **SHOULD NOT** be invented ad hoc. Use existing shared tones and reserve high-emphasis/default or accent styling for the primary action in a local region. Routine actions should generally use outline, secondary, or ghost treatments; destructive tone is only for destructive operations.

Required UI state coverage for network-backed surfaces:

- loading
- empty
- error
- success

Where relevant, also include:

- partial data
- stale data
- permission denied
- destructive action confirmation

### 4. Styling and Design Tokens

Styling **MUST** use the shared design system direction of the repository.

Rules:

- Tailwind utility classes are the default styling mechanism for shared UI and feature layers.
- Colors, spacing, typography, radius, elevation, and motion **SHOULD** come from shared tokens or named utility conventions.
- Direct raw values **SHOULD NOT** be introduced when an existing semantic token can represent the intent.
- Handwritten feature components **SHOULD NOT** create one-off accent button, pill, badge, toolbar, or panel treatments when shared UI primitives and semantic tokens already cover the interaction.
- Ordinary layout spacing **MUST** use the approved Tailwind spacing scale instead of arbitrary bracket values when styling padding, margin, gap, inset, width or height spacing, border radius, scroll margin or padding, and layout rhythm.
- Responsive layout changes **MUST** use the approved breakpoint variants instead of custom bracket variants for ordinary mobile, tablet, and desktop behavior.
- Shared typography, card, button, field, and panel patterns **SHOULD** be centralized and reused.
- Visual language **MUST** remain consistent across screens in the same product area.
- Any lint or static check that enforces token usage **MUST** pass before merge.

Approved spacing tokens for ordinary layout:

- Use named and scale-backed Tailwind utilities such as `p-0`, `px-1`, `py-1.5`, `p-2`, `p-3`, `p-4`, `p-6`, `p-8`, `m-0`, `mx-auto`, `mt-2`, `gap-1`, `gap-2`, `gap-3`, `gap-4`, `gap-6`, `space-y-2`, `space-x-3`, `inset-0`, `top-4`, `h-4`, `w-6`, `min-h-0`, `max-w-sm`, `rounded-sm`, `rounded-md`, `rounded-lg`, and semantic project classes that wrap the same scale.
- Prefer the nearest approved scale value that preserves the intended hierarchy. For example, replace `p-[18px]` with `p-4` or `p-5` based on the surrounding component rhythm, replace `gap-[14px]` with `gap-3` or `gap-4`, and replace `rounded-[10px]` with `rounded-lg`.
- Use component variants or shared primitives when the same spacing recipe appears in multiple places. For example, a repeated panel shell should become a primitive or named class instead of repeating one-off utility clusters.

Approved responsive variants for ordinary layout:

- Use mobile-first base classes plus Tailwind breakpoint variants such as `sm:`, `md:`, `lg:`, `xl:`, and `2xl:` for standard viewport changes.
- Use Tailwind range variants such as `max-sm:` or `max-md:` only when the inverted condition is clearer than rewriting the base layout; do not use bracketed breakpoints such as `max-[767px]:`, `min-[920px]:`, or `[@media(...)]` for ordinary layout.
- If a product surface needs a recurring non-default breakpoint, name it in the Tailwind or project token layer first, document why the default breakpoints are insufficient, and use the named variant consistently.

Allowed intrinsic-value exceptions:

- Intrinsic visualization geometry may keep direct numeric values when the number describes the visualization rather than reusable spacing rhythm, such as chart view boxes, graph canvas extents, node coordinates, axis tick geometry, or drag bounds.
- Viewport and container sizing may keep direct values when they express a real runtime constraint, such as `min-h-[100dvh]`, viewport clamps, split-pane library sizing, or third-party component dimensions that cannot be expressed through the spacing scale without changing behavior.
- Generated artifacts, third-party style hooks, data-driven transforms, and asset metadata may keep required numeric values, but handwritten UI should isolate and comment the exception when it is not obvious from the surrounding code.
- Runtime `ui/src` arbitrary width or height utilities that are true intrinsic sizing exceptions **MUST** carry the inline marker `tailwind-exception: intrinsic-sizing` on the same line or immediately above the class usage so the repo-owned lint guard can distinguish documented exceptions from ordinary layout drift.
- Exceptions must not be used for routine padding, margin, gap, inset, radius, or breakpoint choices that can be expressed with approved Tailwind tokens.

Recommended token categories:

- `background`, `foreground`, `muted`, `accent`, `danger`, `success`, `warning`, `info`
- text roles such as `heading`, `body`, `supporting`, `code`
- spacing scale tokens for padding, gap, inset, and layout rhythm
- border and overlay tokens
- motion tokens for duration and easing

### 5. Accessibility

Accessibility is required behavior, not a best-effort enhancement.

Rules:

- Interactive elements **MUST** use semantic HTML where possible.
- Non-semantic interactive containers **MUST NOT** replace buttons, links, inputs, labels, lists, tables, or headings without strong justification.
- Every interactive control **MUST** be keyboard reachable and operable.
- Focus indicators **MUST** remain visible and meet contrast expectations.
- Forms **MUST** provide labels, error messaging, and programmatic relationships between controls and validation text.
- Icons **MUST** have accessible names when they convey meaning.
- Color **MUST NOT** be the only means of communicating state.
- Heading order and landmark usage **SHOULD** preserve a sensible document outline.
- Tables, dialogs, menus, disclosure widgets, and drag interactions **MUST** follow their expected accessibility patterns.

Verification:

- Automated accessibility checks **SHOULD** run in component and functional test suites.
- High-risk flows **SHOULD** receive manual keyboard and screen-reader spot checks.
- Changes **MUST** target WCAG 2.2 AA behavior unless a stricter requirement is documented elsewhere.

### 6. Responsive Design

Every website surface **MUST** work on mobile, tablet, and desktop viewports.

Rules:

- Design and implementation **MUST** start from the smallest supported viewport and expand upward.
- Content **MUST NOT** require horizontal scrolling except for intentionally scrollable regions such as large data tables or diagrams.
- Touch targets **SHOULD** be large enough for mobile use.
- Dense information layouts **MUST** degrade gracefully on narrow screens.
- Sticky panels, data visualizations, and split panes **MUST** preserve core usability on smaller breakpoints.
- Text **MUST** remain readable without zooming at supported viewport sizes.

Verification:

- Component or Storybook tests **SHOULD** cover major breakpoints.
- Functional tests **SHOULD** confirm at least one mobile and one desktop path for primary user journeys.

### 7. Performance and Resilience

Websites **MUST** remain responsive under realistic load and failure conditions.

Rules:

- Core Web Vitals **SHOULD** be treated as release criteria for critical pages and user journeys.
- Initial render paths **SHOULD** avoid blocking on non-critical data when progressive disclosure is possible.
- Expensive calculations **SHOULD** be moved out of render or isolated behind memoization only when measurement or clear evidence justifies it.
- Large lists, charts, or graphs **SHOULD** use virtualization, aggregation, or progressive rendering when scale demands it.
- Realtime or polling views **MUST** define refresh cadence, teardown behavior, and failure handling explicitly.
- Error boundaries **SHOULD** protect major UI regions where a partial failure is preferable to full-page failure.
- Assets **MUST** be sized, compressed, and bundled intentionally to avoid avoidable regressions in startup cost.
- Images **SHOULD** use modern formats, responsive sizing, and lazy loading where appropriate.
- Code splitting and lazy loading **SHOULD** be used for large routes, heavy visualizations, and infrequently used UI paths.
- Motion and micro-interactions **SHOULD** remain smooth without blocking input or degrading low-powered devices.
- The application **SHOULD** remain usable under slow or intermittent network conditions.

Verification:

- Lighthouse or equivalent performance checks **SHOULD** exist for critical pages.
- Performance budgets **SHOULD** be defined for bundle size, key page weight, or other relevant bottlenecks in mature surfaces.
- Long-running or high-volume surfaces **SHOULD** have targeted performance or memory regression coverage.

### 8. Browser Compatibility and Progressive Enhancement

Critical website flows **MUST** work across the repository's supported browser set.

Rules:

- Supported browsers **MUST** be defined per product surface or inherited from the repository default support policy.
- Critical flows **MUST** be verified on major evergreen browsers before release when the affected area is high value or high risk.
- Features that rely on newer browser APIs **MUST** provide fallback behavior or graceful degradation when practical.
- Layout, navigation, forms, and critical visualizations **SHOULD** fail softly rather than become unusable when a non-essential enhancement is unavailable.
- Browser-specific fixes **MUST** be documented in code comments only when the reason would otherwise be unclear to a future maintainer.

Verification:

- Functional tests **SHOULD** include at least the primary supported browser path.
- Manual spot checks **SHOULD** cover secondary supported browsers for critical journeys.

### 9. Internationalization and Resource Packaging

User-facing websites **MUST** be structured so localization can be added or scaled without rewriting the UI.

Rules:

- User-visible copy **MUST NOT** be hardcoded deep inside reusable components when that copy is intended to vary by locale.
- Resource keys **MUST** be stable, descriptive, and scoped by feature or domain rather than by page position.
- Messages **SHOULD** support interpolation, pluralization, gender, list formatting, dates, times, numbers, and currencies through localization tooling rather than manual string building.
- Locale-aware formatting **MUST** use platform or library locale formatters instead of handcrafted formatting logic.
- Fallback locale behavior **MUST** be explicit.
- Right-to-left support **SHOULD** be considered when the product may target RTL locales.
- Copy used in validation, empty states, toasts, dialogs, accessibility labels, and metadata **MUST** be included in localization scope.

Resource package guidance:

- Resource packages **SHOULD** be split by feature, domain, or bounded context rather than placed in one monolithic application-wide file.
- Shared primitives and design-system copy **SHOULD** live in a small shared resource package used across features.
- Feature resource packages **SHOULD** sit near the owning feature code so UI changes and copy changes move together.
- Large admin or dashboard areas **MAY** load feature resource packages lazily with the route or feature bundle.
- Do not split resource packages so aggressively that common workflows require fetching many tiny catalogs.
- Generated or vendor-owned message catalogs **MUST** be kept separate from handwritten product copy.

Recommended structure:

- `ui/src/i18n/` for framework setup, locale registry, shared formatters, and fallback policy
- `ui/src/features/<feature>/messages/` for feature-owned message catalogs
- `ui/src/components/<shared>/messages/` only when shared components genuinely own reusable user-visible copy

Review expectations:

- Adding a new feature **SHOULD** add or extend a feature-local message catalog.
- New user-facing UI copy in `ui/src/` **SHOULD** be authored through a feature-owned message catalog, and reviewers **SHOULD** treat new hardcoded production JSX copy, textual component props, or accessibility labels as a blocking issue unless the literal is a documented non-product diagnostic exception.
- Legitimate non-product diagnostic literals **MUST** use the inline `hardcoded-ui-copy-exception: non-product-diagnostic` marker near the literal instead of adding product copy back to `ui/scripts/hardcoded-ui-copy-baseline.txt`.
- Renaming a feature or domain **SHOULD NOT** force broad key churn outside that ownership boundary.
- Reviewers **SHOULD** reject concatenated translated fragments when a full localized message should be authored instead.

Verification:

- `cd ui && bun run check:localized-copy` **SHOULD** pass. That check intentionally excludes tests, stories, generated API code, fixtures, developer-testing seams, and feature message catalogs while enforcing that the hardcoded-copy baseline stays empty for product UI copy.
- Tests **SHOULD** cover at least one non-default locale path for formatting-sensitive UI.
- Snapshot-heavy testing **SHOULD NOT** be the only localization evidence.

### 10. Testing Strategy

Frontend changes **MUST** include evidence at the right testing layer.

The expected testing layers are:

- unit tests for pure logic, formatting helpers, selectors, parsers, and reducers
- component tests for rendered behavior of isolated components and hooks
- app-level functional tests for product flows with mocked backend behavior
- integration tests under `ui/integration/` for broader frontend behavior and fixtures
- Storybook stories and Storybook test-runner coverage for reusable UI states when applicable
- performance tests for load, memory, or sustained interaction risks

Rules:

- Most UI confidence **SHOULD** come from component and functional tests.
- Unit tests **SHOULD NOT** dominate coverage at the expense of real rendered behavior.
- Integration tests **SHOULD** focus on contract confidence and regression-prone seams.
- Performance tests **SHOULD** exist for surfaces known to handle high event volume, large datasets, or long-lived sessions.
- Storybook stories **SHOULD** represent meaningful states, not only the happy path.
- Complex interactive surfaces **SHOULD** test domain operations and projection adapters directly, in addition to rendered component behavior.
- Third-party UI library integrations **SHOULD** have a small number of focused component or functional tests proving that user interactions dispatch the intended feature operations and that projected state visibly changes.

Expected evidence for complex interactive surfaces:

- operation tests for pure feature behaviors such as add, remove, connect, disconnect, reorder, validate, and save preparation
- projection tests that map canonical feature state plus UI state into third-party library state or rendered view models
- hook or mutation tests for networked save behavior, cache invalidation, stale data handling, and recoverable failures
- focused component tests for toolbar, dialog, and control wiring
- functional or integration tests for the highest-risk user interactions that depend on browser or third-party library behavior

Minimum expectations for non-trivial UI changes:

- Changed logic has unit coverage where direct logic testing is the clearest fit.
- Changed components or hooks have component-level coverage.
- User-visible flows have functional coverage or a documented reason they do not.
- Critical responsive, accessibility, localization, and browser-compatibility behavior has direct verification where relevant.
- Critical regressions are reproducible in CI.

Repository test and check commands:

- `cd ui && bun run check` runs Biome, semantic color checks, Tailwind spacing-token checks, and feature-root file checks.
- `cd ui && bun run tsc` runs TypeScript project checking.
- `cd ui && bun run test:unit` runs Vitest unit and component tests outside `ui/integration/`.
- `cd ui && bun run test:integration` runs integration tests under `ui/integration/`.
- `cd ui && bun run test` runs the standard frontend unit and integration suites.
- `cd ui && bun run test-storybook` and `cd ui && bun run storybook:test-runner:ci` cover Storybook-driven UI behavior when a change touches reusable or story-covered components.
- `cd ui && bun run generate-api` regenerates `ui/src/api/generated/openapi.ts` from `api/openapi.yaml`; generated output should not be hand-edited.

### 11. Observability and Diagnostics

Frontend systems **SHOULD** be diagnosable when they fail in production-like environments.

Rules:

- User-facing failures **SHOULD** produce actionable messages where possible.
- Developer-facing diagnostics **SHOULD** preserve enough context to debug request failures, rendering failures, and state corruption.
- Logging **MUST NOT** leak secrets, tokens, or sensitive payloads.
- Debug-only helpers **MUST** be intentional and removable, and they **MUST NOT** become hidden production dependencies.

## Delivery Checklist

Before merge, authors **SHOULD** confirm:

- Architecture follows the current `api`, `components`, `features`, `i18n`, `lib`, `testing`, and `types` boundaries, with feature-local `hooks/` and `state/` where applicable.
- Feature files live under approved feature subdirectories, and intentional cross-feature imports use `public/` boundaries.
- Complex interactive surfaces keep canonical state, feature operations, projections, and component wiring separated.
- UI actions use shared primitives such as `Button`, `DashboardActionButton`, `DashboardIconButtonShell`, and `DashboardActionRow` before adding feature-specific button or toolbar components.
- Dense surfaces avoid unnecessary explanatory copy and use concise icon-led actions with accessible names/tooltips where appropriate.
- Network access is centralized and typed.
- Loading, empty, error, and success states are implemented.
- Styling uses shared Tailwind patterns and semantic tokens.
- Keyboard, semantics, labels, and focus behavior are verified.
- Mobile, tablet, and desktop layouts were checked.
- Localizable copy is externalized and packaged at sensible feature boundaries.
- Critical browser support and graceful degradation were checked.
- Appropriate tests pass at the right layers.
- Operation and projection tests exist for complex editors, builders, graphs, canvases, and similar stateful UI surfaces.
- `cd ui && bun run check` and relevant `bun run test:*` commands pass or have documented scope-specific exceptions.
- Performance, resilience, and observability concerns were considered for the affected surface.

## Notes for This Repository

These standards are intentionally general, but the current repository stack suggests the following defaults:

- Use Bun scripts from `ui/package.json` for frontend development, checking, build, API generation, and tests.
- Use React Query for server-backed state.
- Use Zustand only for client-side application state that does not belong in React Query, and prefer feature-local `state/` folders for feature-owned stores.
- Keep generated OpenAPI clients under `ui/src/api/generated/` and wrap them with handwritten API modules as needed.
- Keep API access modules under `ui/src/api/<domain>/` and route all component data access through typed feature hooks or API wrappers.
- Keep feature root directories file-free; add implementation files under feature subdirectories such as `components/`, `hooks/`, `lib/`, `messages/`, `public/`, and `state/`.
- Use Storybook, Vitest, and Testing Library to cover component states, including loading and failure cases.
- Use functional or integration coverage for dashboard flows, event streams, React Flow behavior, and other async or third-party-library behavior that is hard to trust through unit tests alone.
- Keep locale infrastructure under `ui/src/i18n/` and keep message catalogs owned by the features that render them.
