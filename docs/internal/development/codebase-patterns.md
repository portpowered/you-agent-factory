# Codebase patterns

> Extracted from the repository's former root-level `progress.txt` agent worklog
> when that file was untracked. These are accumulated engineering findings from
> the ACP delivery lanes (`ACP-L1`, `ACP-L4-W6`, `ACP-L4-W7`), preserved here
> because they encode non-obvious runtime invariants that are easy to violate.
>
> These are observations, not normative policy. Normative rules live under
> `docs/internal/standards/`.

## Worker Sessions and the workstation pool boundary

- Construct each Factory Session's Worker Sessions service only after its
  session-owned `WorkstationPoolBoundary` exists, so dispatch and cancellation
  share the exact same boundary.
- Worker Session controls must use `context.WithoutCancel` when calling the pool
  boundary; dispatch termination remains observable through the boundary
  callback rather than caller-context cancellation.
- An asynchronous `WorkstationPoolBoundary.Publish` must not return until Workers
  has admitted the dispatch into an exact cancellable queue or running set (or
  has already returned a terminal result); Worker Sessions may then safely issue
  exact cancellation.
- A pool boundary that offers synchronous completion must forward Workers'
  cancellable-admission acknowledgement before it waits for the terminal result;
  otherwise a valid running dispatch becomes unreachable to its supervising
  control plane.
- A late admission callback must never move a terminal Worker Session back to
  `RUNNING`; terminal lifecycle snapshots are absorbing under every
  control/completion interleaving.
- When Workers reports `ALREADY_CANCELED`, its terminal callback can still be in
  flight; `Terminate` must join that callback before returning its idempotent
  terminal snapshot, while ordinary `Cancel` may retain its non-joining no-op
  contract.
- Any successful `Terminate` no-op caused by an earlier accepted control must
  join the same supervision callback before returning; `controlDone` only
  serializes the boundary effect and is not a terminal-result barrier.

## Factory controls and fan-out

- Scope Factory child controls from `DISPATCH_WORKER_SESSION_ASSOCIATION` events'
  `context.requestId`, then retain one detached canonical-ledger snapshot per
  committed control; never rescan live dispatches or reselect on retry.
- A turn-targeted Factory control must carry the immutable turn and control
  identities; cache its detached fan-out result by both so retries cannot select
  associations committed later.
- When a turn-scoped stop also ends the Factory Runtime, serialize Run's pool
  teardown behind Worker Sessions fan-out so the pool cannot preempt exact child
  controls.
- A committed Chat turn control must carry its immutable `TurnID` plus stable
  `ControlRequest.RequestID` to Factory Runtime as `TurnID`/`ControlID`;
  on-demand target cancellation must fan out before invocation-context
  cancellation and retain that result across runtime replacement.
- A committed Chat close needs a control-aware target termination operation:
  preserve its opaque control ID and turn ID through Factory Sessions, fan out
  before target cleanup, and keep generic close separate for process lifecycle
  cleanup.

## Provider Session references and continuation

- For a resumable provider attempt, bind the provider-authored typed session
  reference at its native live-observation point; post-result metadata cannot
  authorize an in-flight pause.
- Provider Session references must stay as complete `providers.SessionRef` values
  bound to the Worker Session's own immutable turn and dispatch correlation;
  never recreate one from runner, model, current provider, or a bare session ID.
- When Workers must publish before its session-owned Worker Sessions service is
  constructed, use a session-local bridge that binds once during Worker Sessions
  factory construction and forwards typed-reference progress only after
  association succeeds.
- `ProviderSessionMetadata` is response-only compatibility data: forward
  metadata-only fragments unchanged, but only an exact typed
  `providers.SessionRef` may create a Worker Session association or enable
  continuation.
- When a resume path must preserve provider-specific identity, carry a cloned
  typed `providers.SessionRef` on the Workers execution request and prioritize it
  over any legacy bare session-ID compatibility field.
- An exact Worker Session continuation must validate both Providers
  `ContinueResult.Reference` and any returned `ExecuteResult.SessionRef` before
  publishing output; when the execute-level field is omitted, propagate the
  already-validated exact reference rather than reconstructing one.
- Preserve provider-specific continuation classifications only on in-process
  Worker result fields excluded from Factory event serialization, then map
  whitelisted enum values into the Worker Session terminal snapshot.

## Testing and delivery

- For a cross-service behavioral proof that needs one service's internal concrete
  collaborator, compose the peer through its owner-published `wire.New...` API
  inside the collaborator's package-level integration test; do not add a
  test-only production seam.
- Runtime tests inject a Worker Sessions service by wrapping it in a
  `WorkerSessionsFactory`, keeping runtime construction dependent on the public
  factory contract.
- Worker Sessions root and internal service packages have 100% unit-coverage
  baselines; cover control interleavings through controlled boundary callbacks
  and channel handshakes, never sleeps or source-inventory tests.
- For a required CI failure, compare the failing path on the reviewed head and PR
  base before repairing it; do not expand a delivery lane to fix a defect
  unchanged on both revisions.
