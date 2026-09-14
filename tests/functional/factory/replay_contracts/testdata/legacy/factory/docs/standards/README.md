# Factory standards

This directory is the canonical source of truth for planning and delivery work
performed by the repository's agent factory. Factory workers **MUST** read the
standard for their role and the repository-wide engineering standards relevant
to the surfaces they change.

## Standards

- [Planning standards](./planning-standards.md) define plan contents, behavior
  slicing, acceptance criteria, task sequencing, and progressive verification.
- [Meta-planning standards](./meta-planning-standards.md) define queue shaping,
  batch selection, reconciliation, and feedback-driven replanning.
- [Implementation standards](./implementation-standards.md) define task
  execution, evidence, escalation, and the handoff to review.
- [Review standards](./review-standards.md) define independent verification,
  finding severity, convergence, and validation-loopback behavior.
- [Testing standards](./testing-standards.md) define the authoritative test
  layers, customer-behavior boundary, functional-session model, parallelism,
  compiled-artifact integration lane, and load/static-check placement used by
  planners, implementers, reviewers, and validation workers.

## Templates

- [Plan template](./plan-template.md) is the required shape for design plans and
  PRDs.
- [Task template](./task-template.md) is the required shape for work submitted
  to an implementation agent.
- [Validation loopback template](./validation-loopback-template.md) is the
  required report shape for clean-room, integrated validation.

## Relationship to repository standards

These files specialize, but do not replace, the general standards under
`docs/internal/standards/`. If two rules appear to conflict, the stricter rule
applies unless the plan records a narrow, justified exception. Factory plans
must never weaken repository architecture, testing, accessibility, security,
or review requirements.
