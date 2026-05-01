# Standards

This index is the entry point to the standards used by this repository. Read it first, then open the most relevant standard for the task at hand.

## Quick Start

- Treat files under `docs/standards/` as the normative source of truth.
- Read the target standard's summary or quick-rules section before making changes.
- Use supporting guides for examples and rationale, not to override the standard.

## Coding Standards

- `docs/standards/code/code-review-standards.md` — required review behavior and PR quality gates
- `docs/standards/code/general-backend-standards.md` — required backend architecture, state management, linting, testing, CI, and complexity expectations
- `docs/standards/code/general-website-standards.md` — required website architecture, accessibility, responsive design, styling, performance, and testing expectations

## Selection Guidance

- For implementation and review work, start with the code-review standard.
- For backend and runtime work, also read the general backend standard before making structural, stateful, testing, or CI-related changes.
- For frontend and website work, also read the general website standard before making structural, UI, state, or testing changes.
- For feature work that changes tests, contracts, or public behavior, use the review standard to confirm the required evidence is present.
- If this standards corpus expands, add new standards here and keep this index current.
