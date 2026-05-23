# PRD: Website Localizability

## Introduction

Restructure the React website so product copy, accessibility labels, formatting, metadata, and user-visible status text can be localized without rewriting feature UI. English remains the default authoring and fallback locale, and Simplified Mandarin Chinese for China, identified as `zh-CN`, becomes the first target locale that must be complete enough to verify real customer-facing behavior.

The website already has an early localization foundation in `ui/src/i18n/` and several feature-local message catalogs under `ui/src/features/*/messages/`. This effort turns that partial pattern into a consistent website-wide architecture: stable locale identifiers, feature-owned message catalogs, explicit fallback behavior, locale-aware formatters, and regression coverage for English plus Mandarin.

## Goals

- Standardize website localization infrastructure around English and Mandarin as the required launch locales.
- Replace hardcoded user-facing copy in the main website flows with feature-owned message catalogs.
- Ensure Mandarin copy covers visible text, accessible names, aria labels, empty states, error states, validation messages, dialogs, toasts, metadata, and export/import affordances.
- Use locale-aware APIs for dates, times, numbers, counts, lists, and relative values instead of handcrafted string formatting.
- Make locale resolution explicit and testable, including unsupported-locale fallback to English.
- Keep localization resources close to their owning features while preserving a small shared i18n foundation.
- Provide tests and review gates that prevent newly introduced user-facing strings from bypassing the localization structure.
- Add functional tests that prove locale selection changes rendered UI behavior and that required localized fields are available in English and Mandarin.

## User Stories

### US-001: Define the website locale policy
**Description:** As a developer, I want a documented locale policy so that all future website changes use the same locale identifiers, fallback behavior, and scope boundaries.

**Acceptance Criteria:**
- [ ] `ui/src/i18n/locales.ts` defines English and Mandarin as required supported locales.
- [ ] The Mandarin locale identifier is documented as Simplified Mandarin Chinese for China using canonical locale `zh-CN`.
- [ ] English is documented as the default fallback locale.
- [ ] Unsupported or missing locale inputs resolve to English.
- [ ] Existing locale aliases, if any, are handled deliberately, such as mapping `zh` and `zh-Hans` to `zh-CN`.
- [ ] Unit tests cover default locale resolution, Mandarin resolution, alias resolution, and unsupported locale fallback.
- [ ] Typecheck/lint passes.

### US-002: Establish shared localization utilities
**Description:** As a developer, I want shared localization helpers so that features can resolve messages and format values consistently.

**Acceptance Criteria:**
- [ ] `ui/src/i18n/` exposes typed helpers for resolving feature message catalogs.
- [ ] `ui/src/i18n/` exposes shared formatters for dates, times, numbers, counts, percentages, lists, and relative values where the website needs them.
- [ ] Formatting helpers use `Intl` APIs or an approved localization library instead of manual concatenation.
- [ ] Catalog validation tests prove every required English message field also exists for `zh-CN`.
- [ ] Helper tests prove English and Mandarin produce locale-appropriate formatting for at least dates, numbers, and plural/count-sensitive labels.
- [ ] The fallback behavior is covered by tests.
- [ ] Typecheck/lint passes.

### US-003: Inventory hardcoded website copy
**Description:** As a developer, I want an inventory of hardcoded user-facing strings so that the migration can be planned and reviewed without missing critical UI states.

**Acceptance Criteria:**
- [ ] Add a short migration inventory under `docs/internal/development/` listing major hardcoded copy surfaces by feature.
- [ ] Inventory includes visible text, aria labels, title text, button labels, form labels, empty states, error states, validation messages, dialog copy, table labels, chart labels, and metadata where present.
- [ ] Inventory separates product-authored copy from API-provided data, test fixture names, generated code, and developer-only diagnostics.
- [ ] Inventory identifies which surfaces already use message catalogs and which still need migration.
- [ ] The document links back to `docs/internal/standards/code/general-website-standards.md`.

### US-004: Migrate dashboard shell copy
**Description:** As a website user, I want the dashboard shell to render in my selected language so that core navigation and status controls are understandable in English or Mandarin.

**Acceptance Criteria:**
- [ ] Header, brand lockup, stream status, timeline/tick controls, dashboard summary labels, and primary dashboard action labels resolve through message catalogs.
- [ ] English and Mandarin catalog entries exist for every migrated dashboard shell key.
- [ ] Tests assert that each dashboard shell message field exists for both `en` and `zh-CN`.
- [ ] Accessible names and aria labels are localized alongside visible labels.
- [ ] String interpolation uses full localized templates rather than concatenated fragments.
- [ ] Tests render the dashboard shell in English and Mandarin.
- [ ] Verify in browser using dev-browser skill.
- [ ] Typecheck/lint passes.

### US-005: Migrate import and export flows
**Description:** As a website user, I want import and export dialogs to be localized so that file workflow decisions are clear in English or Mandarin.

**Acceptance Criteria:**
- [ ] Import preview dialog copy, activation error copy, loading state, validation state, and action labels resolve through feature-local message catalogs.
- [ ] Export dialog copy, filename helper text, action labels, success/error states, and accessible names resolve through feature-local message catalogs.
- [ ] English and Mandarin catalog entries exist for all import/export messages.
- [ ] Tests assert that each import/export message field exists for both `en` and `zh-CN`.
- [ ] Existing tests are updated to assert localized Mandarin copy for at least one successful state and one error or validation state in each flow.
- [ ] Verify in browser using dev-browser skill.
- [ ] Typecheck/lint passes.

### US-006: Migrate work and activity widgets
**Description:** As a website user, I want work, timeline, trace, and activity widgets to be localized so that operational dashboards can be used by Mandarin-speaking users.

**Acceptance Criteria:**
- [ ] Terminal work, submit work, work totals, work outcome, workflow activity, current selection, timeline, trace drilldown, and flowchart-facing UI copy are routed through feature-local message catalogs where they own product copy.
- [ ] English and Mandarin entries exist for titles, labels, legends, empty states, loading states, error states, table headers, chart labels, tooltips, and accessible names.
- [ ] Tests assert that each migrated widget message field exists for both `en` and `zh-CN`.
- [ ] Dynamic labels, including counts and statuses, use formatter functions or full localized message functions.
- [ ] API-owned identifiers, user-provided names, workstation names, place names, and event payload data remain data values and are not translated unless explicitly product-authored.
- [ ] Component tests cover at least three migrated widgets in Mandarin, including one data visualization or chart label surface.
- [ ] Verify in browser using dev-browser skill.
- [ ] Typecheck/lint passes.

### US-007: Provide app-level locale selection and propagation
**Description:** As a website user, I want the website to use my selected language so that localized copy appears consistently across the app.

**Acceptance Criteria:**
- [ ] The app resolves locale from an agreed source order, with the in-app selection taking precedence over URL parameter and browser language.
- [ ] Locale resolution is centralized and passed through providers or feature seams without prop-drilling through unrelated components.
- [ ] The selected locale is available to shared formatters and feature message resolvers.
- [ ] Switching locale updates visible copy without requiring a full page reload.
- [ ] Locale can still be supplied directly in tests and stories.
- [ ] Tests cover browser-language or configured-locale resolution for Mandarin.
- [ ] Verify in browser using dev-browser skill.
- [ ] Typecheck/lint passes.

### US-008: Add a header language switcher
**Description:** As a website user, I want a language switcher in the dashboard header controls so that I can change between English and Mandarin from the main interface.

**Acceptance Criteria:**
- [ ] The dashboard header includes a compact language switcher in the existing header control area.
- [ ] The switcher offers English and Mandarin Chinese (`zh-CN`) as selectable options.
- [ ] Selecting Mandarin immediately updates visible dashboard copy, accessible names, dialog labels, and locale-aware formatting on the current screen.
- [ ] Selecting English immediately restores English copy and formatting.
- [ ] The control itself is localized, including visible label or accessible name.
- [ ] The selected locale is kept in client-side state for the current app session.
- [ ] Backend/user-profile persistence is not implemented.
- [ ] Component tests cover switching from English to Mandarin and back.
- [ ] Verify in browser using dev-browser skill.
- [ ] Typecheck/lint passes.

### US-009: Add functional localization coverage
**Description:** As a reviewer, I want functional tests that exercise real localized flows so that locale behavior is verified beyond unit-level catalog checks.

**Acceptance Criteria:**
- [ ] Add functional or integration-oriented frontend tests that render the app or primary dashboard shell in English and `zh-CN`.
- [ ] Tests use the language switcher or app-level locale setup to select `zh-CN`.
- [ ] Tests assert that primary header controls, at least one import/export dialog, and at least one work/activity widget render Mandarin text from message catalogs.
- [ ] Tests assert that accessible names, not only visible text, change when Mandarin is selected.
- [ ] Tests assert that at least one date, number, or count field uses Mandarin/China locale formatting.
- [ ] Tests assert that API-provided names or IDs remain unchanged when switching locales.
- [ ] Tests assert fallback to English for an unsupported locale.
- [ ] Functional coverage uses stable user-facing selectors or accessibility queries rather than implementation-only catalog internals.
- [ ] Typecheck/lint passes.

### US-010: Add localization quality gates
**Description:** As a reviewer, I want automated and documented checks so that new website copy does not bypass localization after the migration.

**Acceptance Criteria:**
- [ ] Add or document a lightweight check for hardcoded user-facing strings in `ui/src`, with clear exclusions for tests, generated code, fixtures, API data, and developer diagnostics.
- [ ] Add PR/review guidance that requires new user-facing UI copy to use a feature-owned message catalog.
- [ ] Storybook or component coverage includes at least one Mandarin state for high-value localized components.
- [ ] Tests fail when a required locale is missing a key or field from a typed catalog.
- [ ] Typecheck/lint passes.

## Functional Requirements

- FR-1: The website must support English and Mandarin as required locales for user-facing UI.
- FR-2: The default locale must be English.
- FR-3: Missing, unsupported, or malformed locale inputs must fall back to English.
- FR-4: The Mandarin target must be explicitly defined as Simplified Mandarin Chinese for China using canonical locale `zh-CN`.
- FR-5: Locale aliases must be handled deliberately, including at least mapping `zh` and `zh-Hans` to `zh-CN`.
- FR-6: User-facing copy must live in typed message catalogs owned by the rendering feature or shared component.
- FR-7: Shared primitives may only own localized copy when the primitive itself owns the user-facing text.
- FR-8: Feature message catalogs must include English and Mandarin entries before a migrated feature is considered complete.
- FR-9: Message keys must be stable, descriptive, and scoped by feature or domain rather than layout position.
- FR-10: Localized messages with variables must use full localized templates or functions, not concatenated fragments.
- FR-11: Dates, times, numbers, percentages, counts, and lists must use locale-aware formatting helpers.
- FR-12: Validation text, empty states, error states, loading states, dialog text, aria labels, chart labels, table headers, and metadata must be included in localization scope.
- FR-13: API-provided data, generated OpenAPI code, user-authored content, IDs, names, and operational event payload values must not be translated as product copy.
- FR-14: The app must expose a central locale resolution path usable by components, hooks, stories, and tests.
- FR-15: Localization tests must include at least one non-default Mandarin render path for each migrated high-value flow.
- FR-16: TypeScript must detect missing required locale keys in message catalogs.
- FR-17: The implementation must follow `docs/internal/standards/code/general-website-standards.md`, especially the internationalization and resource packaging section.
- FR-18: The header must include an in-app language switcher for English and `zh-CN`.
- FR-19: The implementation must include functional coverage that verifies locale switching changes rendered visible copy, accessible names, and locale-aware formatted values.
- FR-20: Functional coverage must verify that product-authored fields are localized while API/user-provided data values remain stable across locale changes.

## Non-Goals

- Do not translate backend API response payloads or generated OpenAPI types as part of this PRD.
- Do not introduce right-to-left layout support in this phase.
- Do not require professional translation workflow integrations, translation memory, vendor management, human translation review, or continuous localization platforms in this phase.
- Do not localize maintainer-only docs, internal factory logs, test fixture names, or developer diagnostics unless they appear directly in user-facing UI.
- Do not require Japanese or Korean completion as part of this PRD; current partial `ja` and `ko` catalogs are experimental future-locale work.
- Do not redesign the dashboard visual system beyond layout fixes required for Mandarin text length and readability.
- Do not add server-side locale negotiation or backend/user-profile locale persistence.

## Design Considerations

- Mandarin text may be shorter or longer depending on the phrase; controls must avoid fixed-width assumptions that clip labels.
- Buttons, dialogs, charts, tooltips, tables, and dense dashboard widgets must be checked at mobile, tablet, and desktop sizes.
- Layout should continue to use existing Tailwind and component patterns.
- Data-heavy operational views should remain scannable and compact; localization should not turn dashboard surfaces into explanatory or marketing-style layouts.
- Accessible names must match the localized intent of visible labels even when the visible label uses an icon-only control.
- The header language switcher should fit the existing header control pattern and avoid crowding status/timeline controls on mobile.

## Technical Considerations

- Prefer evolving the existing `ui/src/i18n/` helpers and feature-local `messages/` folders over introducing a large new abstraction immediately.
- Use `zh-CN` as the canonical Mandarin locale and provide compatibility aliases for existing `zh` usage.
- Keep resource packages feature-local: `ui/src/features/<feature>/messages/`.
- Keep shared locale registry, fallback policy, and formatters in `ui/src/i18n/`.
- Consider a React locale provider near the app root for app-wide locale propagation while preserving direct message resolver functions for unit tests.
- If a third-party i18n library is introduced, document why it is needed beyond the current typed catalog approach and keep migration incremental.
- Avoid snapshot-only proof. Prefer tests that assert actual rendered labels, accessible names, and formatted values.
- Functional tests should confirm behavior at the rendered UI boundary, while catalog/unit tests should confirm full key and field availability for English and `zh-CN`.
- Confirm Mandarin font rendering does not require additional font assets; if it does, document bundle-size and performance impact before adding assets.

## Success Metrics

- 100% of primary dashboard shell, import/export, and work/activity widget product-authored copy has English and Mandarin catalog entries.
- 100% of required message fields for migrated catalogs are present for both `en` and `zh-CN`.
- Zero known hardcoded user-facing strings remain in migrated production components, excluding documented data, fixture, generated, and diagnostics categories.
- Unit/component tests cover locale resolution, fallback behavior, Mandarin rendering, and required field availability for high-value flows.
- Functional tests prove that selecting `zh-CN` changes visible copy, accessible names, and formatted fields on at least the dashboard shell, one import/export flow, and one work/activity widget.
- Mandarin dashboard smoke path can be verified in browser without clipped primary controls or unreadable chart/table labels.
- New UI copy added after the migration is caught by review guidance, type coverage, or a hardcoded-string check before merge.

## Open Questions

- Should locale preference survive browser refresh through local storage, or is current-session state enough for the first implementation?
- Should the language switcher be hidden when the website is embedded in a constrained dashboard context?
