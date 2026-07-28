# Packaged Service Structure — checklist

Granular implementation and admission checklist for the packaged-service-structure
program. Cross-links the committed decision artifacts and planner hold text.

## Runtime — Checkpoint/Recovery (IMP-RUN-04)

**Decision:** [`dec-run-rec-durability.md`](dec-run-rec-durability.md) (**DEC-RUN-REC-DURABILITY**)

**Admission status:** **dependency-ready** after DEC-RUN-REC-DURABILITY is
Factory-complete. The sole remaining blocker is **not** a missing durability
decision owner.

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
- [ ] **IMP-RUN-04 implementation** (`factory_runtime/checkpoint_recovery`) —
  **not shipped in DEC-RUN-REC-DURABILITY**; admit as a separate future packet
  when executor capacity and CTR-RUN/CTR-REC prerequisites remain terminal

### IMP-RUN-04 implementation packet (future — not this decision packet)

When admitting IMP-RUN-04 implementation:

- Runtime-owned opaque CheckpointStore port inside `checkpoint_recovery`
- Process-local/default adapter sufficient for compatible-restore and
  corrupt-checkpoint proofs
- No Petri/JavaScript internals on the peer surface
- No top-level Checkpoint service
- Recordings-backed durable CheckpointStore adapter remains follow-on after
  Recordings durable log work

## Planner cross-links

| Surface | IMP-RUN-04 guidance |
| --- | --- |
| [`plan.md`](plan.md) Runtime sequence step 7 | Decision closed; implementation dependency-ready |
| `docs/temp/meta.md` | IMP-RUN-04 dependency-ready once DEC Factory-complete |
| [`dec-run-rec-durability.md`](dec-run-rec-durability.md) | Authoritative ownership and phase table |
