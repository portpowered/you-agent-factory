# Adversarial Findings — Extensions To The Standing Plans

---
author: operator
last modified: 2026, august, 22
doc-id: PLAN-EXT-001
status: proposed
---

# problem statement

The second adversarial evaluation produced 281 findings and 40 blockers. Most map onto plans we already have, a handful have no owner, and several are refutations of round-one claims that are still shaping how we brief lanes.

## customer ask

Add extensions and corresponding designs for the findings in the evaluation, alongside the
existing plans rather than replacing them.

## solution

Route every material finding to exactly one owning plan, design the ones with no home, and
record the refutations so we stop paying for beliefs that have already been disproved.

# original document

The 41-agent adversarial evaluation of v0.0.8 ("The Gigabyte Session File", 281 findings,
40 blockers, 22 Aug 2026), read in full.

## How to read this document

Findings fall into three buckets, and conflating them is how a 281-finding report becomes
unusable.

- **Routed** — already owned by a standing plan. Listed once, with the owning story. No
  new design; the plan is amended only if the finding sharpens an acceptance criterion.
- **Homeless** — real, material, and owned by nothing. These get designs below.
- **Refuted** — claims from round one that this round disproved. These get corrections,
  because at least one of them is currently written into an operator skill document and is
  actively misinforming lanes.

Claims marked **(verified here)** were checked against this tree at `74e1d6341`. Claims
marked **(reported)** are taken from the evaluation and have **not** been independently
confirmed. A reported claim is a lead, not a fact; the story that owns it opens with a
characterization test that is allowed to refute it. That distinction is the difference
between this document and a bug list.

## Part 1 — Routed findings

| Finding | Status | Owner |
|---|---|---|
| Durable session `~default.json` grew to 2^30 bytes and cut mid-string; server cannot start | verified here | `PLAN-REC-001` REC-0 |
| Durable write is a non-atomic in-place overwrite | verified here (`runtimepersist/store.go:112`) | `PLAN-REC-001` REC-0 |
| `token.history.failure_log` appends with no truncation anywhere | verified here | `PLAN-REC-001` REC-0 / D4 |
| Durable sessions are a strictly weaker copy of the recording | verified here | `PLAN-REC-001` D5 |
| `~default` collides silently across runs from one directory | verified here (`durableSessionIDPattern` accepts it) | `PLAN-REC-001` D2 |
| Rolling-file caps exist and are wired to logs/metrics but not to session artifacts | verified here | `PLAN-REC-001` D4 |
| `you run --work` exits 0 when every work item FAILED | reported | `ci-integration-test-matrix.md` Story 0 |
| No machine-readable failure signal at any verbosity, `--json` included | reported | `ci-integration-test-matrix.md` Story 0 |
| `work show` on a FAILED item reports no reason; `_last_output` can contradict the state | reported | `PLAN-MOCK-001` MOCK-4 |
| `READ_ONLY` is behaviourally identical to `DISABLED` at the provider boundary | verified here (adapters gate on `SkipPermissions` only) | `PLAN-JS-001` Part 1 |
| `onFailure` documented as an object where the schema requires an array | reported | `PLAN-DOC-001` DOC-1, DOC-3 |
| Docs use `places` / `transitions`, which are absent from the real schema | reported | `PLAN-DOC-001` DOC-3 + vocabulary rule below |
| `contracts/javascript/runtime-api.json` missing two of nine `agent.run` fields | verified here | `PLAN-JS-001` JS-P7 = `PLAN-DOC-001` DOC-4 |
| No auth on any HTTP route | reported | `next-work-meta-plan.md` SEC-1 |
| `factory create --dir` path traversal | reported | `next-work-meta-plan.md` SEC-1 |
| `SCRIPT_WORKER` inherits the full parent environment | reported | `next-work-meta-plan.md` SEC-1 |

Two of these deserve a sharpened criterion rather than a new story:

- **Story 0's acceptance must include the all-failed case explicitly.** "Exit non-zero on
  failure" is satisfiable by a run with one failure among many. The row that matters is
  *every* work item FAILED and the process still exits 0, because that is the shape that
  makes the entire integration tier unfalsifiable.
- **MOCK-4's acceptance must forbid contradiction, not just require a reason.** A failure
  reason that coexists with a `_last_output` tag asserting success is worse than no reason
  at all, because it survives review.

## Part 2 — Homeless findings, with designs

### EXT-1. Deleting the default session kills the server

**(reported, 4/4 reproductions.)** `you session delete <default-session-id>` prints
success, exits 0, and the server dies roughly five seconds later. Deleting a non-default
session returns 204 and the process survives. `cancel` and `terminate` return 501.

The exit code is the worst part. A command that reports success and then removes the
process that served it is indistinguishable, to any caller, from a command that worked.

Design:

- **The default session is not deletable.** The request is refused with a diagnostic
  naming the session as the default and stating what would have to change first. Refusal
  is a defined outcome with a defined exit code, not a 500 and not a crash.
- **Deletion of any session with a live runtime is refused** unless the runtime is stopped
  first. Deletion is not an implicit shutdown.
- **`cancel` and `terminate` stop returning 501.** They are the operations that make the
  refusal above actionable — refusing a delete because a runtime is live is only
  reasonable if there is a supported way to stop that runtime. Either they are implemented
  in this story, or they are removed from the surface. A documented command that returns
  501 is a worse contract than an absent one.
- **Flag parity is part of acceptance.** `session delete` is reported to ignore `--server`
  and require `--port`; every session subcommand must accept the same server-addressing
  flags, and a flag that is accepted must be used. This is the same class as the
  `@you/goal --provider` defect in `PLAN-DOC-001`: accepted, ignored, no diagnostic.

Acceptance: deleting the default session refuses with a named diagnostic and a non-zero
exit, and the server is still serving afterwards. Deleting a stopped non-default session
still succeeds. This is an unhappy path, so it is asserted in the **functional** tier —
the integration tier is happy-path only and carries no row for it.

### EXT-2. One error code absorbs twenty root causes

**(reported.)** `CLI_COMMAND_FAILED` / "command failed" is emitted for at least 20 distinct
root causes across 12+ commands. Critically, `--debug` shows the real cause **is computed**
— and then discarded before it reaches the user.

This is not a missing-diagnostics problem. It is a **reporting** problem: the information
exists at the point of failure and is destroyed on the way out. That is the same shape as
the mock-worker no-match fall-through in `PLAN-MOCK-001` — the system distinguishes two
outcomes and then throws the distinction away.

Design:

- **Errors carry a stable, specific code from origin to exit.** The code is assigned where
  the failure is detected, not where it is printed.
- **`CLI_COMMAND_FAILED` becomes reserved for genuinely unclassified failures**, and its
  occurrence count becomes a ratchet: a counted drift gate, in the same shape as backend
  lint, so the number can only go down. This is the mechanism that makes the cleanup
  finishable rather than perpetual.
- **The default verbosity carries the cause.** If `--debug` can name it, the default
  output can name it. `--debug` adds detail, never the identity of the failure.
- **One functional-tier case per reclassified code**, asserting the exact code for a
  constructed failure. A code with no test is the next code to rot. These do not belong in
  the integration tier, whose I8 asserts a validation verdict and never a message.

Acceptance: the unclassified-failure count is measured before and after, is strictly lower,
and is gated so it cannot rise; three named causes that today produce `CLI_COMMAND_FAILED`
produce distinct codes at default verbosity.

### EXT-3. Unknown fields warn instead of failing

**(reported.)** Unknown fields in a factory definition produce a warning and validation
still passes. Combined with the documentation defects in `PLAN-DOC-001`, this is why a
copied-from-docs config runs and does the wrong thing: the misspelled or obsolete key is
silently ignored.

This is the `non-empty is not a validity check` failure in another costume — the system
accepts input it does not understand and reports success.

Design:

- **Unknown fields are validation errors by default.** The diagnostic names the field, its
  path, and the nearest valid field name.
- **One escape, explicit and narrow**: a documented flag or manifest key that downgrades
  unknown fields to warnings, for forward-compatibility during an upgrade. It is opt-in,
  and its use appears in the run diagnostics.
- **The migration is bounded and measurable.** Before flipping the default, run validation
  in strict mode across every factory in `examples/`, `packages/packaged-factories/`, and
  the `PLAN-DOC-001` DOC-1 corpus, and fix what it finds. Flip the default only when that
  set is clean — otherwise strictness lands as a mass breakage rather than as a gate.
- Sequenced **after** DOC-1, because DOC-1 is what makes the corpus exist to measure
  against.

Acceptance: a config with a misspelled key fails validation naming the key and the
suggestion; the escape hatch is documented and its use is visible in diagnostics; every
shipped and documented factory validates clean in strict mode.

### EXT-4. `you metrics` ignores `--server` and scans the machine

**(reported: 23,743 files, 972 MB, 26–50 s.)** The command accepts a `--server` flag,
disregards it, and walks the machine-wide artifact tree instead of the addressed server's
scope.

Two defects stacked: a flag accepted and ignored, and an unbounded scan presented as a
query. The second is why the first has never been noticed — the command does eventually
return something.

Design:

- **`--server` scopes the query.** Metrics for a server are the metrics that server wrote.
- **The scan is bounded by default** — a time window, defaulting to something small, with
  the window stated in the output. An unbounded scan is available explicitly, never by
  default.
- **The dated layout is what makes bounding cheap.** Metrics already write
  `YYYY/MM/DD/`, so a window is a directory-prefix filter rather than a full walk. This is
  the same property `PLAN-REC-001` REC-5 relies on for the history half of
  `you session list`, and the two commands should share the window-selection helper rather
  than each growing their own.
- **Accepted-and-ignored flags are a class, not an incident.** This story adds a check
  that every flag a command declares is read somewhere in that command's path. The
  `@you/goal --provider` defect, the `session delete --server` defect and this one are the
  same bug three times.

Acceptance: `you metrics --server <a>` and `--server <b>` return different results on a
host running both; the default query completes in under a second on this machine's current
tree; the flag-usage check fails on a deliberately re-introduced ignored flag.

### EXT-5. Replay serves stale output after a prompt change

**(reported.)** Changing a prompt and re-running serves the recorded output for the old
prompt. Drift is detected — but only as a WARN on stderr, indistinguishable from ordinary
noise.

The detection exists. The reporting is the defect, exactly as in EXT-2.

Design:

- **Input drift is an error by default in replay**, naming the input that changed and the
  recording it diverged from. Serving output recorded for a different input is not a
  degraded success; it is a wrong answer delivered confidently.
- **`--allow-drift` is the explicit opt-in** for the legitimate case of replaying an old
  recording against edited inputs, and it prints what it is ignoring.
- **The drift signal is part of the artifact**, not only stderr. It appears in the
  recording's terminal record so a later reader can tell that a replay was served over
  drift.

Acceptance: an edited prompt replayed without the flag exits non-zero naming the changed
input; with the flag it succeeds and reports the drift; the recording records that drift
was tolerated.

### EXT-6. Secrets in recordings

**(reported.)** Recordings store worker secrets verbatim. This is separated from
`PLAN-REC-001` deliberately: redaction policy deserves review on its own merits, not as a
rider on a storage-path migration.

Design (story **EXT-SEC-2**):

- **Redaction at the recording boundary, not at the read surface.** A secret that reaches
  disk is already leaked; filtering it on the way out protects nothing.
- **Redaction is by declared provenance, not by pattern matching.** Values that arrived
  from a secret source are marked at the source and redacted structurally.
  Regex-scanning for things that look like secrets is a detector that fails open, and
  fails open silently.
- **Redaction is visible.** A redacted field is present, typed, and marked redacted, so a
  reader can tell the difference between "absent" and "withheld" — the same reason a
  truncated failure log must carry its dropped-count.
- Pattern scanning is worth exactly one thing: a **test** that greps a recording produced
  by a factory holding known secrets and fails if any appears. That is a guard, not a
  mechanism.

Acceptance: a run with a declared secret produces a recording containing no occurrence of
the secret value, with the field present and marked redacted; the grep guard fails when
the redaction is removed.

## Part 3 — Refutations, and one correction we owe

Round two disproved several round-one claims. Recording them matters because a false
finding costs the same as a real one, and one of these is currently written down where
lanes will read it.

Refuted by this round:

- Workspace-location suppression.
- `--provider` / `--model` silently dropped by `@you/goal`. *(The flags-accepted-and-ignored
  class is still real — see EXT-4 — but this specific instance did not reproduce.)*
- **Codex requiring a git repository.** This one lives in an operator skill document, where
  it is wrong and is briefing every lane that reads it. Correcting it is the one action
  item in this section, and it is operator work, not lane work.
- `agy-clip-qa` rejecting `outputContract`.
- `factory list --json` non-determinism.
- A global run lock.
- **Their own earlier claim that durable sessions preserved work across a crash.** They do
  not — this round measured that a durable session does not survive a hard kill, which is
  part of why `PLAN-REC-001` can retire them without loss.

Confirmed working, and worth stating because a report of 40 blockers reads as a system in
worse shape than this one is: the JavaScript orchestrator sandbox, recordings themselves,
the rolling-file infrastructure, server startup, the dashboard, `session list`,
`session create`, loopback-only binding, and codex at 7 for 7.

## Part 4 — Sequencing

Only three ordering constraints are real; everything else is parallel.

1. **EXT-3 after `PLAN-DOC-001` DOC-1.** Strict validation needs the corpus to measure
   against, or it lands as a mass breakage.
2. **EXT-2 before the integration tier's error-code rows.** Asserting a code that is about
   to be reclassified is churn.
3. **EXT-4 and `PLAN-REC-001` REC-5 share the window-selection helper.** Whichever lands
   first builds it; the second consumes it. Neither waits on the other, but the standing
   rules doc must say which owns it, or both will write one.

EXT-1, EXT-5 and EXT-6 are independent and can start immediately.

## Verification

`make test` for all stories. EXT-1 and EXT-4 are unhappy paths and are asserted in the
functional tier, not the integration tier — but both remain gated on Story 0's truthful
exit codes, because a case that cannot fail proves nothing wherever it lives. EXT-3 requires `make test-functional` plus a
strict-mode pass over every shipped factory. EXT-6 requires the grep guard to be
demonstrated failing before it is trusted; a guard nobody has watched fail is not evidence.

## Delivery loop

Implementation finishes when its final head is pushed, the PR is open, CI has started,
and blocking review feedback is addressed. Review owns terminal-and-passing CI, conflict
resolution, and merge. CI-run evidence goes in a PR comment, never a commit.
