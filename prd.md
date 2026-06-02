# PRD: Factory Import Preserves Inbox Gitkeep

## Introduction

Factory import and named-factory replace materialize a fresh split-layout directory from the imported portable payload. That staging swap can remove tracked `.gitkeep` sentinels under canonical input inboxes—especially `factory/inputs/BATCH/default/.gitkeep`. Factory docs and the repository artifact contract treat those sentinels as required so inbox directories exist in clean checkouts and agents can submit batch requests. Import must activate imported factory content without stripping those sentinels or treating them as portable authored content.

## Context

### Customer ask

Running factory import deletes the `.gitkeep` sentinel for the batch input directory. Preserve inbox `.gitkeep` files during import/activation, keep portable export behavior unchanged (`.gitkeep` is not bundled factory content), and add regression coverage when `factory/inputs/BATCH/default/.gitkeep` exists before import.

### Problem

- Named-factory **replace** (`ReplaceNamedFactory` / import activation) builds a clean staging directory and atomically swaps it in, which drops files not carried in the portable payload.
- Portable bundled collection **ignores** `.gitkeep` by design (`isPortableBundledIgnoredFile`), so sentinels are never exported or re-materialized.
- `ensureDefaultInputChannelDirectories` creates `inputs/<workType>/default/` for declared work types but does not write `.gitkeep` and does not cover the **`BATCH`** inbox (a canonical batch channel, not a work type).
- Maintainers lose a tracked inbox directory after import; batch submission paths that expect `factory/inputs/BATCH/default/` break in git and on disk.

### Solution

After named-factory materialization (create and replace), **ensure canonical input inbox sentinels** on disk: create missing `inputs/<channel>/default/` directories and empty `.gitkeep` files for the repository canonical inbox set (at minimum `BATCH`, plus default channels for work types declared in the factory config). Do not add `.gitkeep` to portable export payloads. Add focused unit and functional tests proving import/replace leaves `factory/inputs/BATCH/default/.gitkeep` present while stale non-sentinel input files are still replaced per existing semantics.

## Goals

- Preserve or restore `.gitkeep` under canonical factory input inboxes after factory import, replace, and save materialization.
- Keep portable export/import payload free of `.gitkeep` as authored bundled content.
- Add regression coverage for import when `factory/inputs/BATCH/default/.gitkeep` exists locally.
- Avoid preserving arbitrary stale input payloads when import intends to replace starter content.

## Project-Level Acceptance Criteria

- [ ] Given a factory root with `factory/inputs/BATCH/default/.gitkeep`, completing factory import or named-factory replace leaves that path on disk as a regular file.
- [ ] Import/replace still updates factory definition, bundled portable files, and split runtime files according to existing import semantics.
- [ ] Stale non-sentinel files under input default channels (for example old `starter.md` not in the imported payload) are not preserved unless today's portable semantics already preserve them.
- [ ] Export and roundtrip comparisons do not require `.gitkeep` entries in the public factory payload; portable collect continues to skip `.gitkeep`.
- [ ] Regression tests fail before the fix and pass after it at the appropriate layer (`pkg/config` and/or `pkg/service`, plus functional bootstrap portability where applicable).
- [ ] No change to portable export payload schema, batch ingestion semantics, or global dotfile preservation rules.
- [ ] Quality gate: Go typecheck, lint, and targeted tests for touched packages pass.

## User Stories

### prd-factory-import-preserve-inbox-gitkeep-001: Regression test for BATCH gitkeep across named-factory replace

**Description:** As a factory maintainer, I need a failing-then-passing test that proves named-factory replace does not delete `factory/inputs/BATCH/default/.gitkeep` when it existed before replace.

**Acceptance Criteria:**

- [ ] A test in `pkg/config` or `pkg/service` seeds a named factory directory with `inputs/BATCH/default/.gitkeep`, runs `ReplaceNamedFactory` (or the service save/replace path used by import) with a valid portable factory payload, and asserts `inputs/BATCH/default/.gitkeep` still exists afterward.
- [ ] The same test asserts a stale non-sentinel file under an input default channel (for example `inputs/task/default/stale.md` not present in the payload) is absent after replace when existing portable semantics would drop it.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-factory-import-preserve-inbox-gitkeep-002: Ensure canonical inbox sentinels after materialization

**Description:** As a factory maintainer, I want import materialization to restore canonical inbox `.gitkeep` sentinels so batch and work-type inboxes remain present after replace.

**Acceptance Criteria:**

- [ ] After `writeFactorySplitLayout` / named-factory persist commit, the factory directory contains `inputs/BATCH/default/.gitkeep` (create parent directories and an empty file when missing).
- [ ] For each work type declared in the factory config, `inputs/<workType>/default/.gitkeep` exists after materialization when the default channel directory is ensured (empty file if missing; do not overwrite non-empty sentinel content if already present).
- [ ] Sentinel restoration runs for both create and replace persist paths, not only replace.
- [ ] prd-factory-import-preserve-inbox-gitkeep-001 test passes without weakening assertions.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-factory-import-preserve-inbox-gitkeep-003: Export/import functional smoke preserves BATCH sentinel

**Description:** As a maintainer running portability smoke tests, I want export/import activation to leave the batch inbox sentinel on disk in a realistic API harness.

**Acceptance Criteria:**

- [ ] `tests/functional/bootstrap_portability` export/import harness (or adjacent smoke) seeds `inputs/BATCH/default/.gitkeep` on the source factory directory before import and asserts the post-import factory directory on disk still has that file.
- [ ] Existing export/import API contract assertions in the harness remain unchanged (no requirement for `.gitkeep` in exported factory JSON).
- [ ] Typecheck passes
- [ ] Tests pass

### prd-factory-import-preserve-inbox-gitkeep-004: Portable export still ignores gitkeep

**Description:** As an export/import user, I do not want `.gitkeep` treated as authored portable content in export payloads or roundtrip comparisons.

**Acceptance Criteria:**

- [ ] `collectSharedFactoryStarterWork` / portable bundled read paths do not add `.gitkeep` files to bundled file lists when scanning input directories.
- [ ] `GetCurrentFactory` / export readback tests that seed `.gitkeep` on disk continue to omit `.gitkeep` from inlined bundled files (behavior aligned with `pkg/service/factory_test.go` portable readback patterns).
- [ ] `go test ./pkg/config/portabletests/...` and portable bundled file unit tests pass without assertion weakening.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- FR-1: Named-factory persist (create and replace) must leave canonical inbox sentinel files on disk after materialization.
- FR-2: At minimum, `factory/inputs/BATCH/default/.gitkeep` must exist after import/replace materialization completes successfully.
- FR-3: For each work type in the factory config, ensure `factory/inputs/<workType>/default/.gitkeep` exists when the default input channel directory is created.
- FR-4: Portable bundled file collection and export must continue to ignore `.gitkeep` (no schema or payload change).
- FR-5: Stale non-sentinel input files must not be preserved solely because of this change; only sentinel files and directories required for canonical inboxes are ensured.
- FR-6: Add automated regression coverage at unit and functional layers for the BATCH sentinel case.

## Non-Goals

- No change to portable export payload schema or OpenAPI `Factory` bundled-file shape.
- No change to batch ingestion, submit, or `FACTORY_REQUEST_BATCH` processing semantics.
- No global preservation of arbitrary dotfiles (for example `.env`, `.DS_Store`).
- No requirement to bundle `.gitkeep` content in export PNG/JSON payloads.
- No UI-visible behavior change (dashboard import preview and confirm flows stay the same).

## High-Level Technical Design

**Root cause:** Import activation persists via staged `writeFactorySplitLayout` + atomic directory replace. Staging contains only portable payload files; `.gitkeep` is excluded from collection and `BATCH` is outside work-type channel creation.

**Approach:** Extend finalize step of split-layout write (after `materializePortableBundledFiles` and input channel mkdir) with `ensureCanonicalInputInboxSentinels(targetDir, cfg)`:

1. Always ensure `inputs/BATCH/default/` and an empty `.gitkeep` when absent.
2. For each non-empty work type name in `cfg.WorkTypes`, ensure `inputs/<workType>/default/.gitkeep` when the default channel directory exists or is created.
3. Use `O_CREATE|O_EXCL` or equivalent "create if missing" semantics so existing non-empty sentinels are not clobbered.

**Ownership:** `pkg/config` (layout expansion / persist commit). Service import paths already call `ReplaceNamedFactory` / persist; no separate UI-only fix.

**Verification surfaces:**

- Unit: `pkg/config` layout/runtime persist tests; optional `pkg/service` factory save/replace test mirroring import.
- Functional: `tests/functional/bootstrap_portability` export/import harness on-disk assertion.

## Supporting Technical and UX Considerations

- Canonical inbox paths are documented in `factory/docs/overview.md` and enumerated in `internal/testpath/artifact_contract.go` (BATCH, idea, plan, task, thoughts).
- `.gitignore` already force-includes `!factory/inputs/*/default/.gitkeep`; restoring sentinels keeps git tracking aligned.
- Dashboard import (`activateImportedFactoryForSession`) and CLI factory save converge on service persist → `pkg/config` replace; a single config-layer fix covers all import entrypoints.

## Success Metrics

- Importing or replacing a factory no longer removes `factory/inputs/BATCH/default/.gitkeep` in maintainer workflows.
- Existing portable export/import and `pkg/config/portabletests` remain green.
- No increase in stale input files surviving import beyond pre-change portable rules.

## Open Questions

**Resolved for implementation:** Recreate the **canonical repository inbox sentinel set** after materialization (BATCH plus work-type default channels declared in config), not only preserve pre-existing local sentinels. This fixes BATCH even when it was never part of the portable payload and matches the artifact contract. Pre-existing non-sentinel input files are unaffected.
