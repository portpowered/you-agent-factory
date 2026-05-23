# Website Semantic Color Token Grammar

Date: 2026-05-23

## Purpose

This document defines the canonical color grammar for website components under
`ui/src/`. Contributors should style components by semantic UI intent rather
than by local slash-opacity math such as `text-af-ink/72` or `bg-af-overlay/6`.

The grammar uses two layers:

- foundation palette tokens: low-level visual-system values that control brand,
  atmosphere, and implementation detail
- semantic component tokens: the approved component-facing vocabulary for
  surfaces, text, chrome, statuses, and readable on-color pairs

## Canonical Owners

- [ui/src/styles.css](/Users/abdifamily/infinite-you/.claude/worktrees/prd-website-semantic-color-token-consolidation/ui/src/styles.css:1)
  owns the Tailwind v4 `@theme` token declarations that the website compiles.
- This document owns the contributor-facing semantic grammar and naming rules.

## Layer 1: Foundation Palette Tokens

Foundation tokens are not the normal authoring surface for component styling.
They exist so the product can keep one controlled visual system underneath the
semantic layer.

Current foundation groups in `ui/src/styles.css` include:

- background and surface pigments such as `af-bg`, `af-bg-start`,
  `af-bg-mid`, `af-canvas`, and `af-surface`
- neutral readable ink such as `af-ink`, `af-code-ink`, and `af-overlay`
- brand and status pigments such as `af-accent`, `af-info`, `af-success`, and
  `af-danger`
- derived visual-system helpers such as chart colors, muted edge colors, and
  shadows

Contributors should not treat these foundation names as permission to assemble
new component-level recipes ad hoc. If a component needs a new recurring
meaning, add a semantic token instead of composing more slash-opacity variants
inside leaf components.

## Layer 2: Approved Semantic Component Grammar

Component code under `ui/src/` should use the following semantic grammar.
Story 002 implements this vocabulary in the shared token layer; until then,
this grammar is the approval contract for migration work.

### Surface Roles

- `af-background`: the app shell and page backdrop
- `af-surface`: the default card, panel, or contained surface
- `af-surface-subtle`: a quieter nested or supporting surface
- `af-surface-raised`: an elevated surface such as floating panels, dialogs, or
  emphasized containers

### Chrome Roles

- `af-border`: the default divider or container edge
- `af-border-strong`: a stronger boundary for selected or emphasized structure
- `af-overlay`: a scrim, veil, or mixed overlay treatment
- `af-focus-ring`: the canonical visible focus treatment

### Text Roles

- `af-text`: the default readable body or heading text color
- `af-text-muted`: supporting copy that should remain comfortably readable
- `af-text-subtle`: lower-emphasis metadata, captions, or de-emphasized labels
- `af-text-disabled`: unavailable or disabled text
- `af-text-inverse`: readable text placed on a strong colored or inverse
  surface

### Status Roles

Each status family should expose both a main surface or foreground role and an
approved readable on-color pair:

- accent: `af-accent` and `af-on-accent`
- success: `af-success` and `af-on-success`
- warning: `af-warning` and `af-on-warning`
- danger: `af-danger` and `af-on-danger`
- info: `af-info` and `af-on-info`

Status families may later grow supporting semantic roles such as borders,
subtle fills, or emphasis text, but those additions must stay within the same
family naming and be added centrally before routine component usage.

Current centrally approved supporting status roles include:

- accent: `af-accent-surface` and `af-accent-border`
- success: `af-success-surface` and `af-success-border`
- warning: `af-warning-surface` and `af-warning-border`
- danger: `af-danger-surface` and `af-danger-border`
- info: `af-info-surface` and `af-info-border`

## Naming Rules

- Prefer roles that describe UI meaning, not rendering math.
- Use `surface`, `text`, `border`, `overlay`, `focus-ring`, and status-family
  names as the ordinary component-facing vocabulary.
- Use `on-*` names only for readable foreground content placed on a colored
  status or inverse surface.
- Do not introduce secondary, tertiary, or generic foreground naming as routine
  emphasis tokens for this system. Those names hide intent and quickly drift
  between components.
- Do not encode component emphasis through one-off opacity suffixes when the
  meaning is really muted text, subtle chrome, or a supporting surface.

## Contributor Rules

- When the component intent matches an approved semantic role, use that role.
- When multiple components need a meaning that the grammar does not express,
  add a new semantic token centrally before rolling it out.
- When only one component currently needs a novel state, decide whether the
  state is product meaning that should recur. If yes, promote it to the
  semantic layer. If not, isolate the exception and document why it cannot be
  expressed through the approved grammar.
- Do not add new routine component classes that depend on palette-specific
  slash-opacity math such as `text-af-ink/72`, `bg-af-overlay/6`,
  `border-af-accent/35`, or equivalent raw `rgb(from ...)` recipes in feature
  code.

## Migration Guidance

- Migrate shared primitives and helpers before feature leaves so downstream
  surfaces inherit the standard vocabulary.
- Preserve existing loading, empty, error, success, hover, focus, selected, and
  disabled meaning while translating old color recipes into semantic intent.
- If a component needs color meaning that is not captured by this grammar, stop
  and add the semantic token first instead of normalizing the exception into
  more ad hoc utility math.
