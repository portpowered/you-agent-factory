# Stability cleanup: ownership snapshot atomicity

This is the committed source plan for the ownership snapshot publication
stability cleanup. It governs the existing `ownershipinventoryfreeze` command
and the six protected snapshots S-03 through S-08.

## Problem and desired outcome

`ownershipinventoryfreeze` can leave S-03 through S-08 at mixed generations when
a later canonical-file replacement fails. The current command builds some
payloads and writes the six destinations sequentially through direct file
writes, without a group rollback boundary.

One invocation must either publish one deterministic six-file generation or,
after a handled failure, return every canonical regular target to its complete
pre-run bytes, existence, and permission bits. Success output is emitted only
after publication and cleanup complete.

The recommended approach is a private, destination-local transaction: prepare
and validate all payloads, preflight the root and destination ancestors, stage
every payload, commit in canonical order with backup-first replacement, and
recover in reverse order with retained bytes and exact modes as the safe
fallback. The work is three sequential tasks: characterization and filesystem
seam, grouped publication, and delivered-command/loopback evidence.

## Behavior and boundaries

A maintainer or protected regeneration workflow invokes the no-argument
`go run ./cmd/ownershipinventoryfreeze` command. The command must prepare,
validate, serialize, and stage all six deterministic payloads before replacing a
canonical file. A handled failure must return the complete pre-run generation:
existing regular files retain their bytes and permission bits, initially absent
files remain absent, and recoverable staging or backup artifacts are removed.

The change is limited to the existing `cmd/ownershipinventoryfreeze` and
`internal/ownershipinventory` boundary. It does not change snapshot schemas,
classification policy, comparison semantics, canonical path order, or the
protected allowlist.

The consistency guarantee ends when the one serialized publisher returns. The
repository root must already exist and be a directory. Directory and file
permissions are supplied by the operating system; no network or paid service is
involved.

## Contract and compatibility

Authored source: `cmd/ownershipinventoryfreeze/main.go` and private publication
helpers under `internal/ownershipinventory`.

Current:

```text
command: go run ./cmd/ownershipinventoryfreeze
arguments: none
replacement-order: S-03, S-04, S-05, S-06, S-07, S-08
success-stdout: six ordered "wrote <path> (<count> <unit>)" lines
failure-state: direct sequential writes can leave earlier files replaced when a later write fails
```

Proposed:

```text
command: go run ./cmd/ownershipinventoryfreeze
arguments: none
replacement-order: S-03, S-04, S-05, S-06, S-07, S-08
success-stdout: the same six ordered lines, emitted only after group commit and cleanup
failure-state: all payloads are prepared and staged before replacement; handled replacement failures restore pre-run regular files, modes, and absence
```

The change is additive to failure handling. Invocation, success-line text,
canonical paths, JSON shapes, bytes, and consumers remain compatible. No
generated API artifacts or migrations are involved. Reverting the publisher,
command routing, and focused tests together restores the prior implementation;
the committed snapshot contents require no rollback.

## Scope 2 — Grouped snapshot publication

Implement one private publisher that:

- builds and validates S-03 through S-08 using the existing builders and
  validators;
- serializes the complete six-entry write set before filesystem mutation;
- preflights the supplied repository root, rejecting symlink or non-directory
  intermediate destination components;
- creates missing destination directories component by component;
- captures original existence, bytes, and permission bits for every regular
  target;
- creates destination-local stages and backup placeholders;
- commits in canonical S-03 through S-08 order with backup-first replacement;
- rolls back in reverse order, using retained original bytes and exact mode as
  a fallback when a backup rename fails; and
- reports primary, rollback, unrecoverable-recovery, and cleanup errors with
  position and path context without exposing payload contents.

The private filesystem seam is controlled only by package tests. Production
uses the stateless operating-system implementation. No public fault flag,
environment hook, new package, workflow edit, or snapshot-content change is
allowed.

## Scope 3 — Delivered command and handoff evidence

Prove the actual no-argument command in disposable repositories with two
successful runs and one handled non-regular-target failure. Record ordered
stdout, exit status, six-file hashes and modes, worktree state, and transaction
residue. Run the focused package suite, ownership comparison gate, package
maintenance and size gates, the supported Windows replacement lane, and the
required current-head CI handoff. The implementation stops after the final
head is pushed, the PR is open, CI has started, and blocking feedback is
addressed; terminal CI, merge, conflict resolution, and post-merge behavior
belong to review.

## Dependencies and ownership

| Task | Semantic dependency | Owned surface |
| --- | --- | --- |
| Characterize current command and add private filesystem seam | None | Characterization fixtures and `internal/ownershipinventory` seam |
| Publish the six-file transaction | Characterization/seam | `cmd/ownershipinventoryfreeze` and grouped publisher/tests |
| Prove delivered command and handoff | Grouped publisher | Disposable command probes, repository gates, PR evidence, and read-only loopback |

The tasks are sequential because the second task must preserve the first task's
characterization and the third task must exercise the delivered second-task
path. No parallel task may edit the command or grouped publisher.

## Non-goals

- changing committed S-03 through S-08 bytes, schemas, format versions, or
  ownership/classification policy;
- changing the protected eleven-path allowlist or regeneration workflow;
- adding a package, public API, configuration field, or environment-controlled
  fault mechanism;
- providing crash or power-loss durability after abrupt termination;
- isolating same-root concurrent readers or publishers;
- adding network, provider, model, UI, OpenAPI, event, or persisted-schema
  behavior; or
- repairing unrelated repository or CI failures outside the touched command
  and ownership-inventory package.

## Non-functional and rollout constraints

The transaction performs one six-payload build/serialize/stage/commit and adds
no process, goroutine, polling, network, or paid dependency. Temporary names
are created by the operating system beside each destination. Only the six
compile-time relative paths, their required in-root directory components, and
their destination-local stage/backup artifacts may be touched. Errors include
phase, position, path, and recovery context but never payload contents.

There is no feature flag or migration. Stop delivery on changed snapshot bytes,
schema or policy drift, an outside-root mutation, unrecovered originals,
leftover recoverable residue, a Windows replacement failure, an ownership gate
failure, or a required PR check failure. Roll back by reverting the grouped
publisher and rerun the unchanged snapshot generator only after any named
retained recovery artifact has been restored by the operator.

## Acceptance Criterion 4 — Failure and boundary matrix

The implementation and focused tests own these observable cases:

| Case | Detection and recovery | Required result and proof |
| --- | --- | --- |
| Invalid candidate or unclassified live unit | Existing builder/validator before filesystem work | Fail before staging; preserve targets and emit no success lines; focused validation tests. |
| Missing destination directories | Component-wise in-root checks and creation | Create only required directories and publish exact payloads; local-real success test. |
| Existing regular or missing target | Lstat/read/mode capture before staging | Publish new bytes with the existing or declared new-file mode; success and mixed-existence tests. |
| Directory, symlink, or other non-regular target/ancestor | Root and every destination component are Lstat-checked | Fail before commit with position, path, and kind; outside sentinel remains unchanged; boundary test. |
| Stage create, write, close, mode, permission, or capacity failure | Controlled filesystem operation seam | Fail before replacement and remove created stages; per-operation fault matrix. |
| Replacement failure at positions 1 through 6 | Reverse backup restore or remove newly created target | Restore every original byte and exact mode, restore initial absence, and clean residue; repeated position tests. |
| Backup-rename rollback failure | Retained-byte write followed by exact chmod | Report both faults and clean residue when recovery succeeds; POSIX umask regression. |
| Total recovery failure | Retain the original backup and stop cleanup for it | Name the canonical and retained recovery artifact; unrecoverable-recovery test. |
| Cleanup failure | Return cleanup error with residue path | Never report clean success; cleanup diagnostic test. |
| Distinct-root concurrent publishers | No process-global transaction state | Complete independently without collision or residue; concurrent-root test. |
| Fixed write-set shape | Validate exactly six compile-time entries | Reject missing, extra, empty, duplicate, or out-of-order entries; invariant tests. |
| Timeout and abrupt cancellation | No context or timeout contract exists | Remain outside handled-error claims; loopback records the limit and does not claim crash recovery. |

The command has no service logger or remote dependency; stdout/stderr are its
operational diagnostics. Test artifacts and PR comments carry the detailed
verification evidence.

## Verification boundary

The primary evidence is package integration with the real local filesystem plus
controlled filesystem faults for deterministic failure positions. The actual
command probe and current-head CI provide delivered-artifact evidence.
Crash recovery, power-loss durability, same-root reader isolation, and
post-merge main behavior remain unproven by this plan and are not acceptance
claims.
