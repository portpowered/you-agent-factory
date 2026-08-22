# Documentation Generated From Testable Sources

---
author: operator
last modified: 2026, august, 22
doc-id: PLAN-DOC-001
status: proposed
---

# problem statement

Documentation is authored independently of the code it describes, so consistency is maintained by hand and fails silently; copying from `you docs` is a reliable way to produce a broken configuration.

## customer ask

Stop relying on docs being consistent. Make the docs reference actual factory files and other testable artifacts, fill the doc files in by codegen, and let the examples run as functional tests rather than needing a bespoke CI harness.

## solution

Invert the dependency. Every checkable element of a doc becomes a projection of an artifact that already has to be correct — a real factory directory, the Cobra command tree, or a Go contract. Prose stays hand-written; everything a reader could copy is transcluded or generated, and a CI check fails when a doc and its source disagree.

# original document

The 33-agent adversarial evaluation of v0.0.8 — root cause R6, "the documentation's own
examples do not run" — and this repository's `docs/reference/` packaging contract.

## Evidence

Every one of these was found by an agent copying from our own documentation.

| Defect | Where | Cost |
|---|---|---|
| `onFailure` shown as an object where the schema requires an array | authoring guide, "Minimal Workflow" `factory.json` | Three separate agents lost time to one snippet |
| Canonical examples use bare top-level `await`, which the runtime rejects | JavaScript guide | Two agents reported it as two different bugs; the mandatory `(async function(){…})()` wrapper is documented nowhere |
| A `you workflow status\|result\|dispatches\|artifacts` command family that does not exist | JS docs and the orchestrators topic | `you workflow --help` prints root help and exits 0, so the mistake hides itself |
| `@you/goal` example uses `--provider` / `--model`, which that factory does not bind | reference example | Flags accepted, ignored, run fails with an error that never mentions flags |
| `contracts/javascript/runtime-api.json` missing `executorProvider` and `resourceId` | generated contract artifact | Two of nine `agent.run` fields absent; three sources disagree on the field list |

The last row is the tell. That artifact is nominally generated, and it still drifted by
two fields. **The problem is not authoring discipline. It is that nothing fails when a
doc and its subject disagree.**

### What `docs-reference-smoke` actually does

```
docs-reference-smoke:
	$(MAKE) docs-reference-check              # markdown lint
	go test ./pkg/transports/cli/docs/...     # the docs command
	go test ./pkg/transports/cli -run TestDocsCommand_
	go test ./tests/functional/smoke -run TestDocsCommandSmoke_
	go test ./tests/functional/factory/definitions -run '^TestFactoryValidationDocsCommandDescribesStaticGate$$'
```

It lints markdown and tests that the `docs` *command* works. It never parses, validates,
or executes a single example inside a doc. Every defect in the table above passes it.

## Design — three classes of doc content

The whole plan is one classification rule.

**Class P — prose.** Concepts, rationale, when-to-use. Hand-written, unconstrained,
never generated. This is most of the words and none of the risk.

**Class T — transcluded.** Anything a reader could copy and run: factory JSON/YAML,
workflow scripts, batch payloads, command invocations. These **must not exist in the doc
file as authored text.** They are pulled at generation time from a real artifact on disk
that is independently validated and executed. If the artifact changes, the doc changes.
If the artifact stops working, a test goes red before the doc is ever regenerated.

**Class G — projected.** Tables and lists derived from a contract: `agent.run` field
tables, flag lists, subcommand families, error-code tables, policy field references.
Generated from the Go source of truth — `javascript_child.go`, the Cobra command tree,
the error-code registry — never hand-maintained.

The rule for reviewers is one sentence: **if a reader could paste it, it is Class T or G,
and it may not be typed into a markdown file by a human.**

## Mechanism

### Transclusion markers

A doc declares what it includes and from where:

```markdown
<!-- transclude: examples/minimal-workflow/factory.json -->
<!-- /transclude -->
```

`cmd/docscodegen` fills the fenced block between the markers from the named path.
Region selection for partial includes uses named anchors in the source file (a comment
marker in JSON/YAML, a `// region:` comment in Go), never line numbers — line numbers
are the same silent-drift failure in a new costume.

### Projection markers

```markdown
<!-- project: agent-run-fields -->
<!-- /project -->
```

Each projection id maps to a generator function reading a Go source of truth. Adding a
field to `javascript_child.go` changes the table on the next generate; nobody edits the
doc.

### The gate

`make docs-generate` rewrites in place. `make docs-check` regenerates into a temp tree
and diffs — non-zero on any difference, naming the doc, the marker, and the source. This
is the same shape as `make interfaces-all` for generated clients, and it belongs in the
same CI job so the class of failure is familiar.

A transclusion pointing at a nonexistent path, or a projection id with no generator, is a
hard error. A dangling pointer must never degrade to an empty block — that is how the
current `rules:` pointer problem happened in payloads, and it is the same failure mode.

## Where the examples actually run

This is the part that answers "run them via functional tests rather than CI".

Because Class T content lives in **real directories** rather than inside markdown, it is
already reachable by the normal test tiers, and no doc-specific harness is needed:

- **Validation** — every transcluded factory directory is validated in the functional
  tier. `config validate` must pass on all of them. The `onFailure`-shape defect dies
  here, at parse time, in under a second. The integration tier's I8 asserts only that the
  validator's verdict is right on a small corpus; it is not where the whole corpus runs.
- **Execution** — every transcluded factory that is meant to be runnable gets a
  mock-worker run in the functional tier. It must reach a complete terminal state and exit
  0. The bare-top-level-`await` defect dies here, and the JavaScript example specifically
  is also the factory that integration test I3 drives.
- **Command existence** — the projected subcommand families come from the Cobra tree, so
  a documented command that does not exist cannot be projected. `you workflow
  status|result|dispatches|artifacts` becomes unrepresentable rather than wrong.
- **Flag binding** — a documented invocation for a named factory is checked against that
  factory's `invocationSignature`. The `@you/goal --provider` defect dies at generation
  time, before CI.

So the doc examples are not tested *by a docs harness*. They are tested because they are
ordinary factories in the ordinary suite, and the docs merely point at them. That is the
whole idea: **there is no separate thing to keep consistent.**

## Delivery — strangler, six independently mergeable steps

**DOC-1. Example corpus.** Move the factories currently embedded in reference docs into
real directories under `examples/`, and add them to functional-tier validation. Docs are left
untouched and still contain their hand-typed copies. Merging this alone is already
valuable: it is the first time those examples are checked at all, and it will surface the
`onFailure` defect immediately.

**DOC-2. `cmd/docscodegen` with transclusion only.** Generator, markers, `docs-generate`,
`docs-check`. Convert exactly one topic as the proof case. `docs-check` is advisory in
this step, not gating.

**DOC-3. Migrate reference topics in reviewable batches.** Each batch converts a group of
topics from authored snippets to transclusions of the DOC-1 corpus. Each batch merges on
its own and each leaves `docs/reference/` correct. Diffs are large but mechanical, and a
regenerate reproduces them exactly.

**DOC-4. Projection support.** Add `project:` markers and the first three generators:
`agent-run-fields` from `javascript_child.go`, the command tree from Cobra, and the
policy field table. `agent-run-fields` is the proof case, because it is a known-stale
artifact — verified 2026-08-22 as missing two of nine fields — so the first generate
produces a real, previously invisible correction. This step is also
`javascript-workflow-parity-and-permissions.md` story JS-P7.

**DOC-5. Turn on the gate.** `docs-check` becomes required in the lint job. Landing it
after DOC-3 and DOC-4 means the gate goes green-to-red only on genuine future drift,
never on the migration itself.

**DOC-6. Execution.** Add the runnable subset of the corpus to the functional tier under
mock workers. Sequenced last because it depends on truthful exit codes (integration
Story 0) — before that, a broken example would run and report success. Only the
JavaScript example graduates to the integration tier, as test I3; the rest stay
functional, because the integration tier is capped and a ninth test requires retiring an
existing one.

## Non-goals

- **No generated prose.** Explanations stay hand-written. Generating them would produce
  worse docs and would not fix a single defect above.
- **No literate-programming rewrite.** Docs stay markdown; only bounded regions are
  machine-owned.
- **No change to `you docs <topic>` packaging.** `docs/reference/` remains the canonical
  authored location and continues to be embedded. Generation runs before packaging.
- **No line-number-based includes**, ever.

## Verification

`make docs-reference-smoke` after any change under `docs/reference/`, plus the new
`make docs-check`. DOC-1 and DOC-6 additionally require the integration tier from
`ci-integration-test-matrix.md`. DOC-4 requires `make interfaces-all`, since it changes a
generated contract artifact.

## Delivery loop

Implementation finishes when its final head is pushed, the PR is open, CI has started,
and blocking review feedback is addressed. Review owns terminal-and-passing CI, conflict
resolution, and merge. CI-run evidence goes in a PR comment, never a commit.
