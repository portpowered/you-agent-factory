# Standards

This index is the entry point to the standards used by this repository. Read it first, then open the most relevant standard for the task at hand.

## Quick Start

- Treat files under `docs/internal/standards/` as the normative source of truth.
- Read the target standard's summary or quick-rules section before making changes.
- Use supporting guides for examples and rationale, not to override the standard.

## Coding Standards

- `docs/internal/standards/code/code-review-standards.md` — required review behavior and PR quality gates
- `docs/internal/standards/code/planning-standards.md` — required PRD, work-story, and acceptance-criteria planning behavior
- `docs/internal/standards/code/general-backend-standards.md` — required backend architecture, state management, linting, testing, CI, and complexity expectations
- `docs/internal/standards/code/general-website-standards.md` — required website architecture, accessibility, responsive design, styling, performance, and testing expectations

## Customer writing

- [`docs/internal/standards/writing/customer-technical-writing-standard.md`](./writing/customer-technical-writing-standard.md) — the normative entry point for customer-facing labels, procedures, descriptions, examples, warnings, and technical explanations. Authors and reviewers MUST use it when creating or materially revising customer prose.
- [`docs/internal/standards/writing/customer-technical-terms.yaml`](./writing/customer-technical-terms.yaml) — the versioned, parseable register of canonical product terms, approved forms, protected literals, discouraged alternatives, valid surfaces, and meaning owners.
- [Manual customer-prose review checklist](./writing/customer-technical-writing-standard.md#manual-review-checklist) — the review checklist embedded in the normative standard for vocabulary, sentence shape, contract fidelity, safety, exceptions, and the boundary between deterministic findings and human judgment.

## Selection Guidance

- For implementation and review work, start with the code-review standard.
- For PRDs, `prd.json`, task decomposition, and work-story planning, start with the planning standard and then read the review standard.
- For backend and runtime work, also read the general backend standard before making structural, stateful, testing, or CI-related changes.
- For frontend and website work, also read the general website standard before making structural, UI, state, or testing changes.
- For feature work that changes tests, contracts, or public behavior, use the review standard to confirm the required evidence is present.
- If this standards corpus expands, add new standards here and keep this index current.
