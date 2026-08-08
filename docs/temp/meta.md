# Current world state

## System architecture

- ACP delivery is governed by `docs/internal/projects/acp-program/README.md`
  and D1-D8. Priority remains L1, then L2, L4, and L3.
- L1 ACP Core is merged through headless `acpx`; the remaining editor-client
  matrix is an external human-acceptance prerequisite.
- L1 T03 and L4 W1-W7 are merged through `origin/main` `809b1f4c9`. T03 owns
  generic Chat streaming/reconnect; L4 owns Worker child projection, downstream
  control fan-out, and exact Provider continuation.
- In the user-authorized L3 durable batch, Factory Sessions declares intent;
  Factory Runtime derives the target and exclusively owns opaque mutable state.
- Remaining durable Recordings work is isolated to historical JSONL
  projection/query and one coherent caller migration; it does not own live
  Events cursors, ACP projection, or subscriptions.

## Operational notes

- Default PETRI session `1e56c033-3b24-465c-ab2f-92025631eb49` is active with
  zero failed priority tokens, three active executor tasks, and four in-flight
  dispatches including the current planner pass.
- Authored validation PR #1800 is mergeable at `2c0980fa6` with exact-head CI
  starting and no clearing review yet. Packaging/snapshots PR #1801 is
  conflicting at `498a0ddeb` with deterministic review blockers and is already
  on its valid processor correction path.
- The Recordings projection/query task has a clean local implementation commit
  `60b1235d9` and no PR yet. Both planner loopbacks and all downstream durable
  ownership ideas remain correctly dependency-held.

# Progressive change notes

- Preserve D1/D2: no storage engine; Recordings JSONL remains canonical and
  Events remains in-memory/session-scoped.
- Preserve D3-D5: shared composition surfaces, registered thin shims, and no
  cleanup bundled into feature work.
- Do not re-open T03/W5 while migrating durable historical Recordings reads.
  Reassess further Recordings consumer or alias work only at its loopback.
