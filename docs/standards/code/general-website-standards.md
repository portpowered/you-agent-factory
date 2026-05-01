# General Website Standards

---
author: andreas abdi
last modified: 2026, may, 1
doc-id: STD-016
---

This document defines the baseline standards for websites built in this repository. It is intended to be broad enough for most product surfaces while still being concrete enough to review against.

For this repository, these standards apply directly to the React and Vite UI under `ui/`, which currently uses Tailwind CSS v4, React Query, Zustand, Storybook, Vitest, and Playwright-compatible tooling.

## Usage

Every contributor who changes website UI, frontend state, styling, interaction flows, or frontend tests **MUST** review this standard before implementation or review.

## Quick Rules

- Build pages from reusable UI and feature components instead of bespoke page-only markup.
- Route all network access through typed API modules and stateful hooks; do not scatter direct `fetch` calls throughout components.
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
- Data loading, mutations, retries, and caching are handled by approved stateful abstractions.
- Empty, loading, error, and success states are present and intentional.
- Styling uses shared tokens, utilities, and existing patterns instead of custom ad hoc values.
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

- `ui/src/components/` for shared presentational or primitive building blocks
- `ui/src/features/` for feature-specific UI, hooks, view models, and orchestration
- `ui/src/hooks/` for cross-feature hooks when they do not belong to one feature
- `ui/src/state/` for app-level client state
- `ui/src/api/` for transport, API bindings, and request/response typing

Rules:

- Page- or screen-level composition **SHOULD** be thin and delegate behavior to feature modules.
- Shared UI primitives **MUST NOT** depend on feature-specific modules.
- Feature modules **MAY** depend on shared components, hooks, state, and API modules.
- Network transport, parsing, and server contract details **MUST NOT** live inline inside rendering components.
- Generated API clients **MUST** remain generated artifacts; handwritten wrappers belong alongside them rather than inside them.

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
- Shared typography, card, button, field, and panel patterns **SHOULD** be centralized and reused.
- Visual language **MUST** remain consistent across screens in the same product area.
- Any lint or static check that enforces token usage **MUST** pass before merge.

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
- Renaming a feature or domain **SHOULD NOT** force broad key churn outside that ownership boundary.
- Reviewers **SHOULD** reject concatenated translated fragments when a full localized message should be authored instead.

Verification:

- Tests **SHOULD** cover at least one non-default locale path for formatting-sensitive UI.
- Snapshot-heavy testing **SHOULD NOT** be the only localization evidence.

### 10. Testing Strategy

Frontend changes **MUST** include evidence at the right testing layer.

The expected testing layers are:

- unit tests for pure logic, formatting helpers, selectors, parsers, and reducers
- component tests for rendered behavior of isolated components and hooks
- functional tests for end-to-end product flows with mocked backend behavior
- integration tests for real UI plus backend integration paths
- performance tests for load, memory, or sustained interaction risks

Rules:

- Most UI confidence **SHOULD** come from component and functional tests.
- Unit tests **SHOULD NOT** dominate coverage at the expense of real rendered behavior.
- Integration tests **SHOULD** focus on contract confidence and regression-prone seams.
- Performance tests **SHOULD** exist for surfaces known to handle high event volume, large datasets, or long-lived sessions.
- Storybook stories **SHOULD** represent meaningful states, not only the happy path.

Minimum expectations for non-trivial UI changes:

- Changed logic has unit coverage where direct logic testing is the clearest fit.
- Changed components or hooks have component-level coverage.
- User-visible flows have functional coverage or a documented reason they do not.
- Critical responsive, accessibility, localization, and browser-compatibility behavior has direct verification where relevant.
- Critical regressions are reproducible in CI.

### 11. Observability and Diagnostics

Frontend systems **SHOULD** be diagnosable when they fail in production-like environments.

Rules:

- User-facing failures **SHOULD** produce actionable messages where possible.
- Developer-facing diagnostics **SHOULD** preserve enough context to debug request failures, rendering failures, and state corruption.
- Logging **MUST NOT** leak secrets, tokens, or sensitive payloads.
- Debug-only helpers **MUST** be intentional and removable, and they **MUST NOT** become hidden production dependencies.

## Delivery Checklist

Before merge, authors **SHOULD** confirm:

- Architecture follows `api`, `state`, `hooks`, `components`, and `features` boundaries.
- Network access is centralized and typed.
- Loading, empty, error, and success states are implemented.
- Styling uses shared Tailwind patterns and semantic tokens.
- Keyboard, semantics, labels, and focus behavior are verified.
- Mobile, tablet, and desktop layouts were checked.
- Localizable copy is externalized and packaged at sensible feature boundaries.
- Critical browser support and graceful degradation were checked.
- Appropriate tests pass at the right layers.
- Performance, resilience, and observability concerns were considered for the affected surface.

## Notes for This Repository

These standards are intentionally general, but the current repository stack suggests the following defaults:

- Use React Query for server-backed state.
- Use Zustand only for client-side application state that does not belong in React Query.
- Keep generated OpenAPI clients under `ui/src/api/generated/` and wrap them with handwritten API modules as needed.
- Use Storybook and Vitest to cover component states, including loading and failure cases.
- Use functional or integration coverage for dashboard flows, event streams, and other async behavior that is hard to trust through unit tests alone.
- When i18n is introduced, keep locale infrastructure centralized and keep message catalogs owned by the features that render them.
