# PRD: Packaged CLI Docs for Mock Workers and Record/Replay

## Introduction

Customers run factories locally with `you run` and need deterministic workflow checks without live model or script provider calls, plus the ability to capture and replay runs for debugging and support. The installed `you docs` surface is the primary way customers read reference material from `./bin/you` without cloning the repository.

Today, mock-worker and record/replay guidance is scattered inside the long `authoring-factories` topic and only summarized in `config`. A customer running `you docs` does not get dedicated topics for `--with-mock-workers`, mock-workers JSON, or the `--record` / `--replay` / `--no-record` controls. The content that exists is hard to discover and mixes workflow sequencing with CLI run-mode reference.

Add two focused packaged docs topics—`mock-workers` and `record-replay`—that explain how to use those commands from the installed binary, keep `docs/reference/` and `pkg/cli/docs/reference/` synchronized, and cross-link from existing topics without duplicating maintainer-only internals.

## Context

### Customer ask

The current `./bin/you docs` output does not document how customers are supposed to use `mock-workers` or `record/replay` commands. Add support for this.

### Problem

- `you docs` lists nine topics; none are named for mock workers or record/replay.
- Runnable command examples and JSON contracts for `--with-mock-workers` live deep inside `authoring-factories`, mixed with factory layout and batch-input guidance.
- Record/replay behavior (default-on recording, generated artifact paths, explicit `--record`, `--replay`, `--no-record`, sensitivity warnings, and mutual exclusion) is similarly buried; `config` only points readers elsewhere.
- Customers who know the feature names cannot run `you docs mock-workers` or `you docs record-replay` and get a complete answer.

### High-level solution

Introduce two new canonical packaged topics registered in `pkg/cli/docs/docs.go`, backed by customer-authored markdown in `docs/reference/mock-workers.md` and `docs/reference/record-replay.md` with synchronized packaged copies. Trim `authoring-factories` and `config` to short summaries plus links to the new owners. Extend CLI and functional smoke tests so the new topics are discoverable from `you docs` and return stable, copy-pasteable command guidance outside the repository docs tree.

## Goals

- Customers can discover mock-worker and record/replay documentation from `you docs` without reading the full authoring workflow guide.
- `you docs mock-workers` explains when to use `--with-mock-workers`, optional config paths, the JSON contract, selection fields, supported `runType` values, default accept behavior, and links to checked-in examples.
- `you docs record-replay` explains default recording, generated artifact locations, explicit `--record` / `--replay` / `--no-record`, sensitivity and retention expectations, and flag combinations the CLI rejects.
- Existing topics remain accurate entry points via cross-links; maintainer internals stay in `docs/internal/development/record-replay.md`.
- Packaged topics work when the repository `docs/` tree is absent (installed-binary behavior).

## Project-Level Acceptance Criteria

- [ ] Running `you docs` lists `mock-workers` and `record-replay` with customer-facing descriptions and `you docs <topic>` commands.
- [ ] Running `you docs mock-workers` prints a dedicated guide that includes `you run --with-mock-workers` usage, optional config path behavior, the `mockWorkers` JSON shape, selection-field semantics, supported `runType` values, and links to `docs/examples/mock-workers.json`.
- [ ] Running `you docs record-replay` prints a dedicated guide that includes default-on recording, generated artifact path contract, `--record`, `--replay`, `--no-record`, sensitivity guidance, and the rule that `--record` and `--replay` cannot be combined.
- [ ] `authoring-factories` and `config` cross-link to the new topics without contradicting `you run --help` or root-command flag text.
- [ ] Canonical `docs/reference/` pages and packaged `pkg/cli/docs/reference/` copies stay synchronized for every touched topic.
- [ ] Focused tests prove index output, topic rendering, and installed-binary smoke behavior for the new topics.
- [ ] Typecheck, lint, and tests pass for the changed packages.

## User Stories

### docs-extensions-mock-workers-and-record-001: Add mock-workers packaged docs topic

**Description:** As a factory author, I want `you docs mock-workers` to print a complete mock-worker guide so I can test routing and outcomes without live provider calls.

**Acceptance Criteria:**

- [ ] `docs/reference/mock-workers.md` and `pkg/cli/docs/reference/mock-workers.md` match and document: enabling with `you run --dir <factory> --with-mock-workers` (optional path), empty-config default accept behavior, JSON top-level `mockWorkers` array, selection fields (`workerName`, `workstationName`, `workInputs`, etc.), `runType` values `accept`, `reject`, and `script` with their config objects, and unmatched-dispatch default accept behavior.
- [ ] The page includes copy-pasteable commands using the `you` binary name and references the checked-in example at `docs/examples/mock-workers.json` with a runnable combined example that also uses `docs/examples/startup-work.json` when startup work is relevant.
- [ ] `pkg/cli/docs/docs.go` registers canonical topic `mock-workers` with explicit display order, packaged path, and index description; `SupportedTopics()` and unsupported-topic errors include it in deterministic order.
- [ ] `you docs` index lists `mock-workers` and does not list compatibility aliases for it.
- [ ] Typecheck passes
- [ ] Tests pass

### docs-extensions-mock-workers-and-record-002: Add record-replay packaged docs topic

**Description:** As a factory operator, I want `you docs record-replay` to explain record and replay run modes so I can capture, locate, and replay sensitive run artifacts safely.

**Acceptance Criteria:**

- [ ] `docs/reference/record-replay.md` and `pkg/cli/docs/reference/record-replay.md` match and document: default-on recording when neither `--record` nor `--replay` is passed, generated artifact directory `~/.you-agent-factory/recordings/YYYY-MM/YYYY-MM-DD/`, session-based filename pattern, explicit `--record <path>`, `--replay <path>`, `--no-record`, shutdown `Recording saved: ...` messaging, sensitivity warnings, no automatic retention cleanup in v1, and rejection of `--record` with `--replay` or `--no-record` with `--record`.
- [ ] The page distinguishes record mode (live run writes history) from replay mode (reads artifact instead of dispatching live workers) in customer language without requiring maintainer event-log detail.
- [ ] Includes copy-pasteable `you run` examples using `docs/examples/sample-run.replay.json` for explicit record and replay paths.
- [ ] `pkg/cli/docs/docs.go` registers canonical topic `record-replay` with explicit display order, packaged path, and index description.
- [ ] Typecheck passes
- [ ] Tests pass

### docs-extensions-mock-workers-and-record-003: Cross-link existing packaged topics to the new owners

**Description:** As a customer reading `you docs config` or `you docs authoring-factories`, I want clear pointers to the dedicated mock-worker and record/replay guides so I do not have to search a long workflow page.

**Acceptance Criteria:**

- [ ] `config` retains a brief mention of `--with-mock-workers`, `--record`, `--replay`, and `--no-record` but links to `you docs mock-workers` and `you docs record-replay` as the canonical owners instead of being the only detailed reference.
- [ ] `authoring-factories` keeps workflow sequencing but replaces duplicated deep mock-worker and record/replay sections with short summaries plus links to the new topics; runnable quick-start commands for the review example remain present.
- [ ] `authoring-factories` still links to `docs/examples/README.md` and example JSON files using paths that work from the repository docs tree.
- [ ] `docs/reference/README.md` packaged-topic table includes `mock-workers` and `record-replay` with accurate scope descriptions.
- [ ] Typecheck passes
- [ ] Tests pass

### docs-extensions-mock-workers-and-record-004: Prove packaged CLI docs behavior for new topics

**Description:** As a maintainer, I need automated checks that the installed docs command exposes the new topics reliably so regressions are caught before release.

**Acceptance Criteria:**

- [ ] `pkg/cli/docs/docs_test.go` expects `mock-workers` and `record-replay` in supported-topic order, index markdown, and raw markdown markers for each new topic.
- [ ] `pkg/cli/root_docs_test.go` and other root-command docs tests list the new topics in help or unsupported-topic error text where applicable.
- [ ] `tests/functional/smoke/cli_docs_smoke_test.go` runs `you docs mock-workers` and `you docs record-replay` from a temp working directory without a local `docs/` tree and asserts stable headings and command markers (for example `--with-mock-workers`, `--record`, `--replay`, `--no-record`, and example artifact paths).
- [ ] Unsupported-topic errors list all canonical topics including the two new ones in deterministic order.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- FR-1: Register `mock-workers` and `record-replay` as canonical packaged docs topics with positive display order after existing run-adjacent topics and before or after `templates` per a stable ordering documented in the registration table.
- FR-2: `you docs mock-workers` returns raw markdown with no CLI wrapper prose, matching the authored reference page.
- FR-3: `you docs record-replay` returns raw markdown with no CLI wrapper prose, matching the authored reference page.
- FR-4: Mock-workers documentation must reflect the runtime JSON contract validated by `LoadMockWorkersConfig` / `ParseMockWorkersConfig` (including `mockWorkers`, `runType`, `rejectConfig`, `scriptConfig`, and work-input selectors).
- FR-5: Record-replay documentation must align with `you run` flag behavior and long help in `pkg/cli/root.go` for default recording, sensitivity, and conflicting flag rejection.
- FR-6: All customer-facing command examples in new pages use the installed binary name `you`, not legacy `agent-factory` or `infinite-you` invocations.
- FR-7: Maintainer-only event-log, fixture promotion, and divergence interpretation remain in `docs/internal/development/record-replay.md`; the customer topic links to it only as optional further reading for contributors.

## Non-Goals

- No new CLI flags or changes to mock-worker or replay runtime behavior beyond documentation.
- No website or OpenAPI contract changes.
- No automatic pruning or encryption of replay artifacts.
- No meta-tests that only walk `docs/reference` link graphs or assert file inventories without checking CLI output.
- No removal of `authoring-factories` as the workflow-oriented entry point.

## Technical Design

### Packaged docs ownership

Follow `docs/internal/development/packaged-cli-docs-surface.md`:

1. Author canonical markdown under `docs/reference/`.
2. Copy to `pkg/cli/docs/reference/`.
3. Register in `topicDocuments` in `pkg/cli/docs/docs.go`.
4. Update `pkg/cli/docs/docs_test.go`, root docs tests, and `cli_docs_smoke_test.go`.

### Content extraction strategy

- **mock-workers.md:** Customer guide focused on `--with-mock-workers`, optional path sentinel behavior (flag present without path uses empty config), JSON schema, matching semantics, run types, and example commands. Pull accurate field tables from existing `authoring-factories` section `## Test Workflows With Mock Workers` and `pkg/config` validation rules.
- **record-replay.md:** Customer guide focused on CLI controls and artifact handling. Pull from existing `authoring-factories` step 3 record/replay bullets and customer-safe portions of `docs/internal/development/record-replay.md` (record paths, flags, sensitivity); omit maintainer test-promotion workflows.
- **authoring-factories.md:** Keep the numbered workflow; replace long duplicated sections with 2–4 sentence summaries and links `you docs mock-workers` / `you docs record-replay`.

### Display order

Place new topics after `config` (display order ~25 and ~26) so run-mode references sit near configuration and before work-type reference material, or immediately after `authoring-factories` if that keeps workflow-adjacent topics grouped—implementers should pick one order, apply it consistently in `docs.go` and tests, and document the choice in the registration table comment if non-obvious.

### Examples and paths

Continue using repository-owned examples under `docs/examples/` (`mock-workers.json`, `startup-work.json`, `sample-run.replay.json`). Docs pages should show paths relative to a cloned repository for copy-paste and mention that installed-binary users can pass any local path for `--with-mock-workers` and replay artifacts.

## Supporting Considerations

- Keep `docs/examples/README.md` aligned if command examples change.
- Update `docs/internal/processes/development-guide-relevant-files.md` only if the packaged-docs inventory row needs the new topic names (optional, same PR if touched).
- `you run --help` remains the concise flag reference; packaged docs provide narrative and examples.
- Replay artifacts are sensitive; both new topic and `record-replay` must warn customers not to commit real recordings.

## Success Metrics

- A customer can answer “how do I run with mock workers?” using only `you docs mock-workers` in under one minute.
- A customer can answer “where did my recording go?” and “how do I replay it?” using only `you docs record-replay`.
- Functional docs smoke tests pass from a clean temp directory without the repo `docs/` tree.
- No increase in unsupported-topic confusion: canonical topic list in errors matches index.

## Open Questions

None. Scope, ownership split, and verification surfaces are defined by existing packaged-docs standards and current CLI behavior.
