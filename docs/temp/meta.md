# Current world state

## System architecture

- Governing priority remains L1 ACP Core → L2 Root Consolidation → L4 ACP
  Worker Events → L3 PSS remainder. D1 forbids a storage engine, D2 keeps
  Recordings' JSONL ledger separate from the process-local Events stream, D3
  forbids composition leases, and D4 lets L1 use a registered consumer-owned
  shim rather than wait for L3 root sealing.
- E03 and W3 are merged and Factory-terminal. T03 must project the canonical
  sequenced Chat stream without transport-owned identity or taxonomy; W4 must
  preserve Runtime result authority while routing dispatch through Worker
  Sessions.
- The accepted T03 behavior still includes ordered MESSAGE, REASONING, USAGE,
  and SESSION metadata through canonical `root.BuildProcess` plus
  `Process.Execute`. Missing Factory Sessions representation is not permission
  to scope-cut L1 or wait on L3.

## Operational notes

- Default session `6612756a-c206-48c7-892c-8fe1dbd908f9` is active. Four direct
  restoration tasks remain at `task:init` with four live executor workers and
  twelve of sixteen executor slots free; the current planner occupies the sole
  planner slot.
- The obsolete restarted `ACP-L2-IMP-PRV-CONTROL` task remains deterministic
  superseded containment behind merged #1754 and generation-safe #1758; it was
  not moved.
- Definitions restoration completed its full delivery boundary through merged
  PR #1782. Because the reset-session ingress created a standalone task with no
  paired idea, its stranded `task:to-complete` token was deliberately moved to
  terminal `task:complete` with request
  `planner-repair-20260804-def-catalog-direct-task-complete-r1`.

# Progressive change notes

## High-level track state

- L1 T03 PR #1772 is mergeable at fresh head `bbc4bc78e`; functional coverage
  is green, while unit coverage and Verification Policy fail. Its restored task retains the
  complete Story 005 canonical streaming proof and forbids a scope cut, W5-W7
  work, or a wait on L3.
- L2 continuation PR #1780 passed required CI and merged from head `46775cabf`
  as `180eb4c266`. Its standalone restoration task remains on a live
  `task:init` executor pending its normal delivery report; W7 is technically
  unblocked by L2 but remains unadmitted behind unfinished higher-priority work.
- L4 W4 PR #1781 head `e582815b2` is cleanly mergeable and terminal-green but
  has exact replay, callback-race, mixed direct/child, correlation/W3 output,
  and CLI-functional-proof blockers. W5-W7 remain unadmitted.
- L3 Factory Sessions #1773 and Definitions correction #1782 are merged.
  Recordings #1779 remains green with four composition/diagnostic/error/logging
  blockers, owned by its live restored delivery task. Later L3 slices remain
  dependency-held pending genuine delivery.
- Hold W5-W7 and every later cleanup packet until current work reaches the
  delivery boundary. A loopback or free executor capacity alone is not evidence
  that strict-priority prerequisites are merged.
