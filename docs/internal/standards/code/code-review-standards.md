# Code Review Standards

---
author: andreas abdi
last modified: 2026, july, 30
doc-id: STD-015
---

This document defines the required review behavior for all code changes. It is written for authors and reviewers who need a fast checklist first and supporting detail second.

## Usage

Every contributor **MUST** review this standard before conducting or requesting a code review.

## Quick Rules

- Review correctness before style or preference.
- Check design fit, readability, and test coverage on every non-trivial change.
- Make review comments specific, actionable, and classified as blocking or non-blocking.
- Approve when the change is correct and within standards, even if you would have written it differently.
- Request changes for correctness bugs, security issues, missing required tests, or standards violations.
- Review AI-generated code with extra scrutiny.
- Reject feature PRs that include generated one-off artifacts or prohibited task-management files.
- Request changes when new user-facing production UI copy bypasses the feature-owned localization catalog path or the repo's hardcoded-copy quality gate without a documented exception.
- Request changes for unexplained stateful helper paths, hidden side effects, special-case subsystem dispatch, dead code, or Go functions longer than 80 lines without a documented exception.
- Request changes when backend operations bypass service interfaces, operational behavior is exposed as floating functions, dependencies are hidden in constructor bags, services are constructed outside `wire/`, or a secondary injection path is introduced.
- Request changes when new or changed service operations lack structured, safe, actionable operation logs without a documented high-volume exception.
- Request changes when a functional-test PR violates any of the five functional-test construction preferences in [general-backend-standards.md §7](./general-backend-standards.md#7-testing-strategy-and-test-pyramid) without a documented, in-scope exception.
- Request changes when functional tests bypass `root.BuildProcess` + `Process.Execute` by default or invoke a built `you` CLI without proving OS/process-boundary behavior that `BuildProcess` cannot express.
- Request changes when functional tests use HTTP/API for ordinary customer flows without an API-owned contract or explicit CLI+API parity justification.
- Request changes when functional tests replace external effects outside `edges.Edges` or prefer custom in-process provider fakes over `ProviderCommandRunner` and other command-runner edge mocks.
- Request changes when functional tests use `--with-mock-workers` / `MockWorkers` outside `tests/functional/workers/mock/...` cells that own the workers/mock feature.
- Request changes when functional tests add sleeps or timeout-padded wait helpers as the default synchronization strategy without in-code justification for why deterministic observation or edge mocking cannot substitute.

## Review Checklist

Before approval, reviewers **SHOULD** confirm:

- The change solves the stated problem and does not obviously regress existing behavior.
- Edge cases and failure paths have been considered.
- Architecture and dependency direction still fit the area being changed.
- Backend services use direct single injection, service-root interfaces, implementation methods, `wire/`-owned construction, and operation logging as required by the general backend standard.
- The code is understandable and matches established patterns.
- New or changed behavior has appropriate tests.
- Review comments are clearly marked as blocking or non-blocking.
- AI-generated code, if present, has been checked against real APIs, real behavior, and project conventions.
- For PRs that change functional tests under `tests/functional/...`, the five construction preferences from [general-backend-standards.md §7](./general-backend-standards.md#7-testing-strategy-and-test-pyramid) are satisfied: `root.BuildProcess` + `Process.Execute` by default (built CLI only for OS/process-boundary proof), CLI-over-API for ordinary flows, external effects replaced only through `edges.Edges` with `ProviderCommandRunner`/command-runner edge mocks preferred, mocked Codex over MockWorkers except in workers/mock feature cells, and RC-fix over sleeps or timeout-padded wait helpers unless justified in-code.

## Regulations

### 1. Review for Correctness First

A reviewer **MUST** verify that the code does what it claims to do before evaluating any other quality.

### 2. Evaluate Design and Architecture

A reviewer **MUST** evaluate whether the change fits the existing system design.

For backend changes, this includes enforcing the service, injection, construction, and logging rules in the [general backend standard](./general-backend-standards.md): operational behavior belongs on injectable service implementations; peer calls use service-root interfaces; dependencies are injected directly once; `wire/` owns production service construction; and service operations emit structured, safe, actionable logs.

### 3. Verify Readability and Maintainability

A reviewer **MUST** evaluate whether the code is understandable and maintainable.

### 4. Confirm Test Coverage

A reviewer **MUST** verify that the change includes appropriate tests.

### 5. Make Feedback Actionable and Specific

Every review comment **MUST** explain the problem, the requested change, and why it matters.

### 6. Classify Comments as Blocking or Non-Blocking

Every review comment **MUST** be clearly classified as blocking or non-blocking.

### 7. Know When to Approve and When to Request Changes

A reviewer **MUST** approve when the change is correct, well-tested, and conforms to standards, and **MUST** request changes for correctness issues, security issues, missing required tests, or standards violations.

### 8. Apply Additional Scrutiny to AI-Generated Code

AI-generated code **MUST** receive the same or greater scrutiny as human-written code, especially for hallucinated APIs, stale patterns, hidden side effects, and subtle edge-case bugs.

### 9. Enforce Functional-Test Construction Preferences

When a PR changes functional tests, a reviewer **MUST** request changes if any of the five construction preferences in [general-backend-standards.md §7](./general-backend-standards.md#7-testing-strategy-and-test-pyramid) is violated without a documented, in-scope exception:

1. Functional application tests **MUST** construct through `root.BuildProcess` and execute through `Process.Execute` by default. A built `you` CLI **MAY** be used only when the cell must prove OS/process-boundary behavior that `BuildProcess` cannot express.
2. Functional tests **MUST** prefer public CLI invocation over HTTP/API for ordinary customer flows. HTTP or API entry **MAY** be used only for API-owned contracts or explicit CLI+API parity cells.
3. External effects **MUST** be replaced only through `edges.Edges`. Functional tests **MUST** prefer `ProviderCommandRunner` and other command-runner edge mocks over custom in-process provider fakes.
4. Functional tests **MUST** prefer mocked Codex or another real inference-provider variant through the command-runner edge and sanitized goldens over `--with-mock-workers` / `MockWorkers`, except for cells under `tests/functional/workers/mock/...` that own the workers/mock feature.
5. Functional tests **MUST NOT** add sleeps or timeout-padded wait helpers as the default synchronization strategy. Any sleep, polling loop, or timeout-padded wait helper **MUST** include an in-code justification for why deterministic observation or edge mocking cannot substitute.
