# Code Review Standards

---
author: andreas abdi
last modified: 2026, april, 18
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
- Request changes for unexplained stateful helper paths, hidden side effects, special-case subsystem dispatch, dead code, or Go functions longer than 80 lines without a documented exception.

## Review Checklist

Before approval, reviewers **SHOULD** confirm:

- The change solves the stated problem and does not obviously regress existing behavior.
- Edge cases and failure paths have been considered.
- Architecture and dependency direction still fit the area being changed.
- The code is understandable and matches established patterns.
- New or changed behavior has appropriate tests.
- Review comments are clearly marked as blocking or non-blocking.
- AI-generated code, if present, has been checked against real APIs, real behavior, and project conventions.

## Regulations

### 1. Review for Correctness First

A reviewer **MUST** verify that the code does what it claims to do before evaluating any other quality.

### 2. Evaluate Design and Architecture

A reviewer **MUST** evaluate whether the change fits the existing system design.

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
