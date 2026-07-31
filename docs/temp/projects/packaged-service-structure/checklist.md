# Packaged Service Structure — checklist

Granular implementation and admission checklist for the packaged-service-structure
program. Cross-links the committed decision artifacts and planner hold text.

## Runtime — Checkpoint/Recovery (IMP-RUN-04)

**Decision:** [`dec-run-rec-durability.md`](dec-run-rec-durability.md) (**DEC-RUN-REC-DURABILITY**)

**Admission status:** **Factory-terminal** after IMP-RUN-04 PR #1580 merged
(`3bf957012`). DEC-RUN-REC-DURABILITY remains the ownership decision record;
Recordings-backed durable checkpoint bytes remain follow-on.

- [x] DEC-RUN-REC-DURABILITY decision note checked in under
  `docs/temp/projects/packaged-service-structure/`
- [x] Decision states Runtime owns opaque `checkpoint_recovery` and Recordings
  owns durable history/artifact authority
- [x] Decision authorizes IMP-RUN-04 to proceed with Runtime opaque CheckpointStore
  port + process-local/default adapter
- [x] Decision defers Recordings-backed durable checkpoint storage until after
  Recordings durable log/cursor/retention
- [x] Plan Runtime sequence step 7 cites DEC-RUN-REC-DURABILITY; ownership
  decision is no longer open ([`plan.md`](plan.md))
- [x] **IMP-RUN-04 implementation** (`factory_runtime/checkpoint_recovery`) —
  Factory-terminal via PR #1580 / `3bf957012` (`pss-imp-run-04-checkpoint-recovery`);
  opaque CheckpointStore + process-local adapter shipped; Recordings-backed
  durable checkpoint bytes remain follow-on after Recordings durable log

### IMP-RUN-04 implementation packet (future — not this decision packet)

When admitting IMP-RUN-04 implementation:

- Runtime-owned opaque CheckpointStore port inside `checkpoint_recovery`
- Process-local/default adapter sufficient for compatible-restore and
  corrupt-checkpoint proofs
- No Petri/JavaScript internals on the peer surface
- No top-level Checkpoint service
- Recordings-backed durable CheckpointStore adapter remains follow-on after
  Recordings durable log work

## DEC-RUN-REC-DURABILITY changed-path lease proof

Lease matrix: [`plan.md`](plan.md) **Changed-Path Lease Matrix
(DEC-RUN-REC-DURABILITY)**.

- [x] Diff does not add or modify `pkg/services/factory_runtime/**` (including
  `checkpoint_recovery`)
- [x] Diff does not add or modify `pkg/services/recordings/**` for durable
  log/cursor/retention or durable checkpoint storage
- [x] Diff does not create a top-level Checkpoint service package; plan language
  keeps Checkpoint/Recovery as a Runtime private subservice
- [x] Changed paths stay within the DEC-RUN-REC-DURABILITY lease:
  `docs/temp/projects/packaged-service-structure/**`, optional IMP-RUN-04 hold
  text in `docs/temp/meta.md`, plus supporting durable-artifact infrastructure
  (`.gitignore` exceptions, `docs/internal/projects/packaged-service-structure/README.md`
  cross-link index)

### Verification commands (merge base `main`)

Run from the repository root after fetching `main`:

```sh
# Forbidden implementation surfaces must be empty
test -z "$(git diff --name-only main...HEAD -- pkg/services/factory_runtime pkg/services/recordings)"

# No checkpoint implementation paths in the packet diff
! git diff --name-only main...HEAD | rg -i 'pkg/services/.*/checkpoint'

# Observed changed paths (2026-07-28 UTC, branch pss-dec-run-rec-durability)
git diff --name-only main...HEAD
```

Expected changed paths for this packet:

- `.gitignore`
- `docs/internal/projects/packaged-service-structure/README.md`
- `docs/temp/meta.md`
- `docs/temp/projects/packaged-service-structure/README.md`
- `docs/temp/projects/packaged-service-structure/checklist.md`
- `docs/temp/projects/packaged-service-structure/dec-run-rec-durability.md`
- `docs/temp/projects/packaged-service-structure/plan.md`

## Planner cross-links

| Surface | IMP-RUN-04 guidance |
| --- | --- |
| [`plan.md`](plan.md) Runtime sequence step 7 | Decision closed; implementation dependency-ready |
| `docs/temp/meta.md` | IMP-RUN-04 dependency-ready once DEC Factory-complete |
| [`dec-run-rec-durability.md`](dec-run-rec-durability.md) | Authoritative ownership and phase table |
