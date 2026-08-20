# JavaScript Child Permission Override Plan

## Status

Proposed. This document is an implementation plan only.

## Problem statement

JavaScript workflows can author `agent.run({skipPermissions: true})`, but the
behavior is not consistent or fully published:

- The canonical Factory Session path carries the value through Factory Runtime,
  Worker Sessions, Workers, and Providers.
- The standalone JavaScript direct child executor records the value but drops it
  before constructing `workers.ProviderInferenceRequest`.
- Native Claude, Codex, Agy, and ACP routes already have provider-specific
  bypass behavior.
- The JavaScript runtime accepts and validates the boolean, but the published
  JavaScript call-behavior contract omits it.
- Its precedence relative to JavaScript `policy.mode` is implicit.

Workflow authors need one explicit, child-scoped break-glass control that works
the same way in every supported JavaScript execution composition.

## Customer outcome

An author can set `skipPermissions: true` for one child to deliberately bypass
that child's Provider approval and sandbox restrictions regardless of
JavaScript policy mode. The override does not alter sibling behavior or disable
routing and resource controls.

## Product decisions

1. `skipPermissions?: boolean` remains the public field.
2. `true` is valid under every recognized `policy.mode` and overrides Provider
   approval and sandbox restrictions for that child.
3. The override does not mutate Factory Session policy or affect siblings.
4. Model/reasoning allowlists, fanout, concurrency, duration, token, and other
   resource budgets continue to apply.
5. Omission and `false` preserve the effective policy and Provider defaults.
6. The field does not grant direct JavaScript VM filesystem, process, module,
   shell, or network access. Those capabilities have separate contracts.
7. A Provider route that cannot truthfully honor the bypass fails before child
   execution rather than silently ignoring it.

## Recommended authoring UX

```javascript
const result = await agent.run({
  label: "autonomous-operator",
  prompt: "Complete the operation without interactive approval.",
  skipPermissions: true,
});
```

The boolean is intentionally visually loud and localized to the child request.
No additional policy-mode change is required.

## UX comparison

| Shape | Advantages | Costs | Decision |
| --- | --- | --- | --- |
| `skipPermissions: true` on `agent.run` | Already accepted, concise, child-scoped, and clearly signals danger. | Provider-specific semantics often bypass both approvals and sandboxing. | Retain and make consistent. |
| Workflow-wide `skipPermissions` | Concise for fully autonomous workflows. | Easy to grant accidentally and difficult to attribute to one child. | Reject. Authors opt in per child. |
| New `DANGER_FULL_ACCESS` policy mode | Makes danger visible in policy projection. | Conflates a workflow/session default with a one-child Provider override. | Do not require it for this field. |
| Nested `permissions` object | Extensible. | Adds a second vocabulary before bounded permission semantics are established. | Defer. |

## Work stories

### Story 1 — Standalone and Factory Session parity

As a JavaScript workflow author, I receive the same permission-bypass behavior
whether the workflow runs through a canonical Factory Session or the supported
standalone JavaScript composition.

Acceptance criteria:

- A boolean value is preserved from static or dynamically constructed
  `agent.run` objects through normalization and dispatch.
- The standalone direct child executor copies the field into
  `workers.ProviderInferenceRequest`.
- The canonical Factory Runtime and Worker Sessions path remains equivalent.
- Omission and `false` arrive at Providers as false.
- Focused tests prove the value at the Provider boundary, not only on an
  intermediate Factory Runtime struct.

### Story 2 — Explicit override precedence

As a workflow author, I can deliberately override permission and sandbox
restrictions for one child under any JavaScript policy mode without changing
unrelated policy controls.

Acceptance criteria:

- Every recognized `policy.mode` accepts `skipPermissions: true`.
- Permission and sandbox denials do not reject that child solely because of the
  effective policy mode.
- Model/reasoning allowlists and resource budgets continue to reject requests
  that exceed them.
- Siblings that omit the field continue to use normal policy evaluation.
- The override never becomes mutable session-global state.
- Resume and replay preserve the authored value on the corresponding dispatch.

### Story 3 — Provider capability truth

As an operator, I receive the requested bypass or a clear pre-execution failure;
the system never silently degrades it.

Acceptance criteria:

- Claude, Codex, and Agy command construction emits the exact supported bypass
  flag when requested.
- ACP permission selection chooses the existing bypass behavior when requested.
- A Provider without bypass support fails before starting the child attempt with
  a stable capability diagnostic.
- Diagnostics identify the Provider capability mismatch without exposing the
  prompt, credentials, or raw command line.

### Story 4 — Published and observable contract

As an author or operator, I can discover and inspect whether permission bypass
was requested and used.

Acceptance criteria:

- Static and dynamic validation accept only a boolean; non-boolean values fail
  before dispatch with the existing safe field-level diagnostic.
- The authored JavaScript symbol catalog, call-behavior descriptor, schemas,
  examples, errors, and generated package projection include the field.
- CLI reference docs describe the field as a dangerous child-level override
  that does not disable routing or resource limits.
- Dispatch records, events, recordings, and replay retain the boolean without
  exposing Provider command lines or sensitive payloads.
- Preview may warn about a literal `true` value but does not reject it because of
  policy mode.

## Implementation plan

### Contracts and validation

- Keep `skipPermissions` in the closed JavaScript child field set and make its
  policy precedence explicit in Factory Runtime's orchestrator contract.
- Add the optional boolean and its typed error case to the authored JavaScript
  call-behavior descriptor and `contracts/javascript/runtime-api.json`.
- Regenerate derived contract artifacts from canonical sources.

### Runtime and execution

- Preserve the existing canonical Factory Runtime/Worker Sessions propagation.
- Copy `SkipPermissions` in
  `pkg/services/factory_sessions/internal/execution/direct_child_executor.go`.
- Keep Provider-specific flags and ACP permission behavior owned by Providers.
- Add capability detection or a stable unsupported outcome before Provider
  execution where a route cannot honor the request.

### Observation and documentation

- Preserve the boolean in JavaScript child records and canonical dispatch/event
  projections through recording and replay.
- Update `docs/reference/javascript-workflows.md` and
  `docs/reference/orchestrators.md` with the exact precedence and limitations.

## Test plan

- JavaScript normalization tests for omission, false, true, and non-boolean
  dynamic values.
- Factory Session integration tests for canonical and standalone execution.
- Provider adapter tests that inspect Claude/Codex/Agy command arguments and ACP
  permission decisions.
- Policy tests proving the override applies under every mode while routing and
  resource limits remain enforced.
- Recording/replay tests proving child scoping and retained observation.
- JavaScript contract and packaged CLI documentation smoke tests.

## Quality gates

- `go test ./pkg/services/factory_runtime/...`
- `go test ./pkg/services/factory_sessions/internal/execution/...`
- `go test ./pkg/services/providers/...`
- `make javascript-contract-smoke`
- `make contracts-check`
- `make docs-reference-smoke`
- `make verify-fast`, followed by `make verify-pr`

## Out of scope

- Direct JavaScript filesystem operations; see
  `docs/internal/development/plans/javascript-workflow-file-operations-plan.md`.
- Workspace-write roots, filesystem mutation, shell/process access, network
  globals, secrets, or connectors.
- A workflow-wide permission bypass default.
- Unrelated Provider, Worker, or Petri orchestration refactoring.

## Original documents

- `docs/internal/development/plans/dynamic-workflows/dynamic-workflow-design.md`
- `docs/internal/standards/code/planning-standards.md`

## Delivery boundary

Implementation is complete only when canonical and standalone execution,
Provider behavior, published contracts, generated artifacts, documentation,
recording/replay behavior, and required CI are terminal and passing; blocking
review feedback and conflicts are resolved; and the pull request is merged.
