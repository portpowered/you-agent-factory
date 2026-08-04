# Chat and Factory Continuations — Architecture and Delivery Plan

**Status:** Proposed
**Last updated:** 2026-08-04
**Scope:** Factory continuation UX, Factory Definitions, Factory Sessions, Work
lineage, Worker Sessions, Provider Sessions, Petri and JavaScript orchestration,
Chat Sessions presentation, ACP, HTTP/CLI, and dashboard projections.

## Problem statement

Chat and ACP need a continuation model that preserves provider/worker reasoning across completed factory runs when configured, without reopening terminal Factory Sessions or requiring a new canonical Chat Session data model.

## Solution description

Create each follow-up as a successor Factory Session linked to its predecessor, and let the Factory Definition authoritatively choose, per worker role and per scope, how prior context reaches that role: through the provider's native session, through visible prompt payload, or not at all.

## Background

### Existing ownership and constraints

The repository already has the relevant durable owners:

- A `Factory` is an authored definition.
- A `Factory Session` is one execution of a resolved Factory definition.
- `Work` and `Work Request` own admitted payloads and lineage.
- Factory events and Recordings are canonical for execution history and replay.
- Petri tokens/markings and JavaScript checkpoints are private Factory Runtime
  recovery details.
- Worker Sessions own stable worker execution identity, lifecycle, attempts, and
  eventually multi-turn control/event behavior.
- Providers own provider-native execution and return typed Provider Session
  references.
- Provider Sessions expose provider transcript/session inspection.
- Chat Sessions owns chat-oriented attachment, sequencing, and presentation over
  Factory/Worker event streams.
- ACP is a protocol adapter, not an alternative owner of Factory, Work, Worker,
  or Provider state.

A follow-up occurs after one Factory Session has reached a terminal result. It
must not make that terminal Factory Session active again. It may, however,
continue selected Worker Sessions and their exact Provider Session bindings
inside a newly created successor Factory Session when the Factory Definition
declares that continuity.

This distinction matters because an agent harness may retain encrypted or
otherwise inaccessible reasoning in its native Provider Session. Reconstructing
context from the visible answer cannot recover that hidden state. Always
starting a fresh Provider Session can therefore regress quality even when the
visible prompt appears equivalent. Continuity must be a first-class
customer-configurable Factory behavior, not a hard-coded always-new policy.

### Grounded baseline — verified against the tree

Checked at `a1e325164`. This section exists so stories are not written against
imagined surfaces.

**`pkg/services/chat_sessions/` is shipped, not planned.** It is composed in
`pkg/wire/chat_sessions.go` and already serves ACP stdio
(`pkg/transports/acp/internal/stdio/session_prompt.go`).

| Capability | Where | Status |
| --- | --- | --- |
| Session create/read, target selection | `contracts.go:47`, `internal/service/session.go`, `target.go` | Shipped |
| `TargetEpisode` history — immutable, consecutively numbered | `types.go:295`, `transitions.go` | Shipped |
| Turn admission/advancement | `contracts.go:61`, `internal/service/turn.go` | Shipped |
| Active-turn rejection | `errors.go:47` `ErrBusy`, `BusyError` | Shipped |
| Request idempotency | `contracts.go:241` `StartTurnRequest.RequestID RequestIdentity` | Shipped |
| Factory Session binding + pending reconciliation | `contracts.go:124,142`, `internal/service/factory_session_binding.go` | Shipped |
| One Factory Session per episode, permanently | `factory_session_binding.go:59` `FactorySessionConflictError` | Shipped |
| Aggregate sequencing, stream head, attachment ack | `sequencing.go`, `stream_head.go`, `attachment_ack.go` | Shipped |
| Factory target catalog from Operator Settings ACP profile | `factory_target_catalog_contract.go` | Shipped |

This resolves the ACP mapping question mechanically rather than by design
argument. `TargetEpisode` already binds exactly one Factory Session and refuses
a second — which is precisely one link of a continuation chain. **A successor
Factory Session is a new Target Episode on the same Chat Session.** The
continuation-chain head is `session.TargetEpisode`, and the existing
`RecordPendingFactorySession` → `BindFactorySession` reconciliation is already
the crash-safe head update the ACP work requires.

`SetTarget` (`internal/service/target.go:47-56`) already closes the open episode
and opens the next consecutively numbered one, with no same-target no-op guard,
so rolling an episode at the same target needs no new state machine — only a
named operation so the intent is legible in events and authorization.

**`chat_sessions` is not durable.** `internal/service/store.go` is a `map` +
`sync.Mutex`; `doc.go` states persistence is outside the L1 V0 slice.

**Provider-side resume already exists.**

| Capability | Where | Status |
| --- | --- | --- |
| Canonical provider session vocabulary | `pkg/services/providers/identity_contract.go:35` `SessionIDKind` | Shipped |
| `ExecuteRequest.ResumeSession *SessionRef` | `pkg/services/providers/execute_contract.go:99` | Shipped |
| Claude CLI resume wiring | `.../adapters/claude/command.go:81` emits `--resume <id>` | Shipped |
| Session emission on result | claude / codex / acp / agy decoders return `ExecuteResult.SessionRef` | Shipped |
| Provider session inspection/projection | `pkg/services/provider_sessions/contracts.go` | Shipped |

**The actual gap.** `SessionRef` appears in **no** file under
`pkg/services/factory_runtime/` or `pkg/services/recordings/`. Every dispatch
produces a provider session identity and then discards it. Nothing records which
workstation occurrence produced which provider session, so nothing can hand it
back later. Surfacing `SessionRef` out of a dispatch and persisting it against an
authored continuation key is the core new plumbing in this plan — not the
provider layer, which is already done.

**Worker Sessions is single-turn.** `pkg/services/worker_sessions/contracts.go`
exposes `Reserve` / `Get` / `List` / `Start` with an absorbing terminal result.
`workerSession: CONTINUE` cannot ship before the multi-turn contract lands.

**A documented expectation to annotate, not delete.**
`docs/internal/projects/acp-client/design.md:240` asserts a second prompt "will
generate a new work item, not resume an existing item." That remains true: a
follow-up creates new Work, a new Factory Session, and a new marking. Context
continuity happens one level below, inside a worker dispatch. The two are
compatible and the design doc should say so explicitly.

### Continuation, execution resume, and ACP reconnect are different

| Operation | Meaning | Factory Session identity | Worker/Provider behavior |
| --- | --- | --- | --- |
| Continue / follow up | Run the next request using the predecessor's declared continuation policy. | Creates a new successor Factory Session. | Context transport per role follows the Factory Definition. |
| Resume execution | Recover the same paused/interrupted execution. | Retains the same Factory Session ID. | Uses same-execution checkpoints and exact resumable child bindings. |
| Retry continuation | Retry admission/execution as a new successor after a typed failure. | Creates a new Factory Session and preserves the failed one. | Resolved again under the persisted continuation policy and explicit retry rules. |
| Reconnect | Reattach a client to durable/live streams. | Creates nothing. | Creates nothing. |
| ACP `session/load` | Attach and replay the projected conversation history. | Creates nothing. | Creates nothing. |
| ACP `session/resume` | Attach without replaying prior history. | Creates nothing. | Creates nothing. |

Transport handlers and UI copy must not use one ambiguous `resume` operation for
these cases.

### Minimal state principle

P0 does not require canonical `ChatTurn`-as-conversation, chat-message, or
context snapshot resources, and does not require Factory Runtime to consume any
Chat resource.

The continuation graph is represented by immutable Factory Session facts:

- `continuedFromFactorySessionId`;
- `continuationRequestId` for idempotency;
- the resolved continuation policy snapshot/hash;
- the current user input through the ordinary Work/Factory invocation contract;
- stable continuation binding results for participating Worker Sessions and
  Provider Sessions; and
- Factory/Worker/Provider events already required for execution and replay.

A linear history can be derived by walking predecessor/successor links. Chat
presentation projects user input from Work admission and assistant/tool updates
from canonical Factory and Worker event streams. HTTP and CLI callers continue
directly from a Factory Session ID.

Chat Sessions keeps the protocol-level attachment state it already owns —
session identity, target episodes, turn admission, sequencing, stream head — and
its current episode is the continuation-chain head. That is adapter/presentation
state. It does not establish a Chat resource that Factory continuation contracts
depend on, and Factory Runtime never reads it.

P0 permits at most one successor per predecessor Factory Session. The
combination of predecessor identity and request id is idempotent. A different
request attempting to continue a predecessor that already has a successor
returns a typed conflict. Explicit branching is deferred.

### Factory Definitions are authoritative

A Factory Definition decides:

- whether it supports continuation;
- which logical Worker roles have stable continuation binding keys;
- for each role and each scope, how prior context reaches it — native provider
  session, visible prompt payload, or nothing;
- what a role falls back to when its preferred transport is unavailable;
- which prior artifacts a prompt-payload transport may carry;
- whether each Worker Session is continued, forked, or created new;
- which occurrence of a repeated role a key binds;
- what happens when a required binding is missing, ambiguous, incompatible, or
  unsupported;
- which continuation settings, if any, a caller may override;
- which prompt a continued role receives; and
- whether a routed/dynamic child without a predecessor binding may start fresh.

Operator or invocation configuration may supply defaults only where the
definition explicitly permits or delegates a choice. It cannot enable
continuation for a definition that does not support it, invent binding keys, or
broaden the definition's allowed transports.

Definitions without an authored continuation policy are not continuable in P0.
This fail-closed behavior makes the definition the portable source of behavior
across CLI, API, ACP, and dashboard invocation.

### Three orthogonal axes

The plan must not collapse three separate questions into one field:

| Axis | Question | Values |
| --- | --- | --- |
| **Context transport** | How does prior context reach this role? | `REUSE_SESSION`, `PROMPT_CONTEXT`, `NONE` |
| **Worker identity** | Does this role keep its Worker Session identity and lineage? | `CONTINUE`, `FORK`, `NEW` |
| **Scope** | Which prior execution are we carrying from? | `withinRun`, `acrossRuns` |

Context transport is about what the model knows. Worker identity is about
lineage, inspection, and control. A role can perfectly well keep a stable Worker
Session while starting a fresh provider conversation, or the reverse.

The current Worker Sessions foundation models one supervised start and an
absorbing terminal result. True Worker Session reuse requires an additive
multi-turn contract such as `StartTurn` and a distinction between Worker Session
lifecycle and individual Worker-turn lifecycle. Because context transport works
through `providers.ExecuteRequest.ResumeSession`, which is already shipped,
**context continuity can land before Worker Session multi-turn does.** The
delivery order below exploits this: the first vertical ships
`workerSession: NEW` with `context.mode: REUSE_SESSION`, which already delivers
the hidden-reasoning continuity that motivates the feature.

A Provider Session reference is typed by provider, kind, and id. `REUSE_SESSION`
must pass that exact reference only after validating runner/provider capability,
definition binding, workspace, model/config compatibility, and authorization.
The system must never guess a Provider Session from workstation order or a
display name.

### Explicitly out of scope for P0

Deferred unless implementation uncovers a correctness dependency:

- general transcript context compaction;
- model-generated summaries;
- standalone context snapshot resources;
- an open-ended context selector beyond the closed `source` enum below;
- retention policy design beyond existing Factory/Worker/Provider records;
- deletion/expiration behavior for continuation chains;
- changing or upgrading the Factory Definition revision between turns;
- cross-revision continuation;
- explicit branching;
- queued or concurrent prompts;
- importing third-party conversations; and
- a canonical Chat conversation persistence model beyond what Chat Sessions
  already owns.

For definition versions, P0 resolves the exact same Factory Definition revision
used by the predecessor and rejects cross-revision continuation. This is a
compatibility constraint, not a design for version changes between turns.

## Rough approximate recommended solution

### Customer experience

A completed Factory result exposes a `Continue` action only when its resolved
Factory Definition declares continuation support.

The continuation form uses the Factory's invocation signature for the next
request and may expose only definition-approved continuation options. The UI can
show a concise policy summary:

- which worker roles continue their provider conversation;
- which receive visible prior context instead;
- which start completely fresh;
- what happens if a provider cannot resume; and
- whether the next request will rerun the full Factory.

The normal action labels are:

- `Continue` — create a successor Factory Session;
- `Resume execution` — recover the same paused/interrupted Factory Session;
- `Retry continuation` — create another immutable attempt after a typed failure;
- `Reconnect` — transport status only.

Degradation must be explicit. "Continued via provider session" and "continued via
visible context" are materially different facts, and a user who believes the
first is happening while the second is must be able to see it. Every fallback is
recorded and surfaced.

The history view is a projection over the Factory continuation chain. Each entry
shows the request, Factory result, per-role transport outcome, and relevant child
tool-call updates.

### Context transport

Continuity is not binary. There are exactly three ways prior context can reach a
worker, and the Factory Definition chooses per role:

| Mode | Carries | Inspectable | Notes |
| --- | --- | --- | --- |
| `REUSE_SESSION` | Hidden harness reasoning plus the provider-native transcript. | No — provider-held and outside Recordings. | Cheapest in tokens, highest coupling to provider capability and retention. |
| `PROMPT_CONTEXT` | Selected prior results/verdicts/artifacts as visible Work payload. | Yes — in Work, Recordings, and replay. | Provider-neutral. Costs tokens. Bounded by a closed `source` enum and a byte budget. |
| `NONE` | Nothing. | n/a | Default. Current behavior for every existing factory. |

`REUSE_SESSION` declares a `fallback` used when the provider session is
unavailable, unsupported, or incompatible:

| `fallback` | Behavior |
| --- | --- |
| `PROMPT_CONTEXT` | Start a fresh provider session and carry the declared visible sources. **Default.** |
| `NONE` | Start completely fresh and record why. |
| `FAIL` | Reject the continuation before any affected child executes. |

Defaulting `fallback` to `PROMPT_CONTEXT` is deliberate. It means a provider that
cannot resume yields a worker that still knows the visible history rather than
one that knows nothing, and it makes the operator kill switch non-destructive:
`providerSessionReuse: DISABLED` degrades every `REUSE_SESSION` to its declared
fallback rather than turning continuity off. An author who genuinely needs
session-or-nothing writes `fallback: FAIL`.

There is deliberately **no** `BOTH` mode. Replaying visible context into an
already-resumed session duplicates what the provider already holds, inflates
tokens, and reads as contradictory to the model. `fallback` chaining is limited
to one level: `REUSE_SESSION` → `PROMPT_CONTEXT` → stop. `PROMPT_CONTEXT` may
not fall back to `REUSE_SESSION`.

### Reuse scope

The same role can want opposite answers depending on which prior execution is
being carried from:

- **`withinRun`** — a repeated occurrence of the role inside one Factory Session.
  `@you/review` revisits `execute-review-work` up to eight times per session
  (`onRejection` → `init`, bounded by `VISIT_COUNT maxVisits: 8`).
- **`acrossRuns`** — the predecessor Factory Session in a continuation chain.

`@you/review` is the case that makes the split necessary. Its author role must
remember its own draft and the reviewer's feedback across cycles, or it rewrites
from scratch and reintroduces the same flaws. Its reviewer role arguably must
*not* remember, because a reviewer defending its earlier verdict is exactly the
anchoring failure adversarial review exists to prevent. One field cannot say
both.

Both scopes default to `NONE`, which is fail-closed and reproduces the current
behavior of every existing factory. A bare `context: { mode: ... }` is shorthand
for `acrossRuns`, leaving `withinRun` at `NONE`.

### Conceptual Factory Definition shape

The exact schema names must be reconciled with existing Factory Definition
conventions. The following is a behavioral sketch:

```yaml
continuation:
  enabled: true

  defaults:
    workerSession: NEW
    missingBinding: FAIL
    instance: LAST
    context:
      withinRun:  { mode: NONE }
      acrossRuns: { mode: NONE }

  bindings:
    - key: drafter
      workstation: draft-fusion
      worker: fusion-drafter
      workerSession: CONTINUE
      context:
        acrossRuns:
          mode: REUSE_SESSION
          fallback: PROMPT_CONTEXT
          source: [PREVIOUS_RESULT]
      promptOnContinue: |-
        You previously drafted an answer in this same conversation...

    - key: reviewer
      workstation: review-review-work
      worker: review-work-reviewer
      workerSession: NEW
      context:
        withinRun:
          mode: PROMPT_CONTEXT
          source: [PRIOR_VERDICTS]
        acrossRuns:
          mode: REUSE_SESSION
          fallback: PROMPT_CONTEXT

  overrides:
    context:
      allowed: [REUSE_SESSION, PROMPT_CONTEXT, NONE]
    workerSession:
      allowed: [CONTINUE, FORK, NEW]
```

`source` is a closed enum over already-canonical artifacts, not a general
selector:

| Source | Carries |
| --- | --- |
| `PREVIOUS_RESULT` | The predecessor's primary result for this work type. |
| `PRIOR_VERDICTS` | Decision-envelope outcomes recorded for this key's earlier occurrences. |
| `PREDECESSOR_ARTIFACTS` | Artifacts the predecessor bound to this key. |

The current request payload always reaches the worker through the ordinary
invocation signature and is never part of `source`. Each spec carries a
`maxBytes` budget; sources that are expired, redacted, unavailable, or truncated
produce typed refs recorded on the binding outcome, never silent omission.

Worker Session modes remain a separate axis:

| Mode | Behavior |
| --- | --- |
| `CONTINUE` | Reuse the same Worker Session ID and start a new immutable Worker turn. |
| `FORK` | Create a successor Worker Session linked to the prior Worker Session. |
| `NEW` | Create a new unrelated Worker Session for this binding. |

Missing-binding modes:

| Mode | Behavior |
| --- | --- |
| `FAIL` | Reject/stop the continuation before affected child execution. |
| `START_NEW` | Create a fresh Worker/Provider path and record why no predecessor binding existed. |
| `SKIP` | Permit the role to remain absent only when the Factory topology and result contract allow it. |

Instance selectors — see the cardinality section:

| Selector | Behavior |
| --- | --- |
| `LAST` | Bind the highest-numbered occurrence of the role. Default. |
| `FIRST` | Bind occurrence 1. |
| `NONE` | Never bind; the role always starts fresh even when a binding exists. |

A definition can set defaults, override individual bindings, and state which
values callers may choose. Invocation configuration is validated against those
allowed values and becomes part of the resolved policy snapshot.

### Stable continuation bindings

Every reusable logical child role needs a definition-authored `continuationKey`.
A completed Factory Session records, for each key and occurrence:

```json
{
  "continuationKey": "refiner",
  "workstation": "refine-fusion",
  "worker": "fusion-refiner",
  "occurrence": 1,
  "workerSessionId": "worker-session-42",
  "providerSession": {
    "provider": "codex",
    "kind": "session_id",
    "id": "provider-session-99"
  },
  "resultRef": "result-17",
  "compatibility": {
    "factoryRevision": "rev-7",
    "runnerId": "codex",
    "modelProvider": "CODEX",
    "model": "gpt-5",
    "workspaceRootDigest": "sha256:..."
  },
  "status": "AVAILABLE"
}
```

The successor resolver joins only by the authored key and validates that the
current definition binding still names the compatible role. Workstation names,
worker names, dispatch order, and array position are supporting evidence, not
identity.

**Compatibility is scoped to the transport that needs it.** `REUSE_SESSION`
requires an exact match on `factoryRevision`, `runnerId`, `modelProvider`,
`model`, and workspace root digest. `PROMPT_CONTEXT` is provider-neutral and
requires only `factoryRevision` and role identity — visible payload does not care
which model produced it.

This produces a useful gradient rather than a cliff: changing a model override
between turns no longer dead-ends at `INCOMPATIBLE`, it degrades `REUSE_SESSION`
to `PROMPT_CONTEXT` and says so.

Per-key resolved outcomes, recorded and surfaced:

| Outcome | Meaning |
| --- | --- |
| `SESSION_REUSED` | The exact predecessor/prior-occurrence provider session was passed. |
| `PROMPT_CONTEXT_APPLIED` | A fresh provider session received the declared visible sources. |
| `FELL_BACK_TO_PROMPT_CONTEXT` | `REUSE_SESSION` was requested but unavailable; visible sources were used instead. Reason recorded. |
| `NO_CONTEXT` | The role started fresh with nothing carried. |
| `MISSING` | No predecessor binding existed for this key. |
| `INCOMPATIBLE` | A binding existed but failed the transport's compatibility scope. |
| `UNSUPPORTED` | The resolved provider/runner cannot support the requested transport. |

Worker identity outcomes are recorded separately as `WORKER_CONTINUED`,
`WORKER_FORKED`, or `WORKER_NEW`, so a reader can never confuse "kept its
identity" with "kept its memory."

### Binding cardinality

A continuation key does **not** map one-to-one to a dispatch. `@you/review` is
cyclic, so one predecessor Factory Session can realize up to eight
`execute-review-work` dispatches under a single key.

P0 rule: a key may have many realized occurrences, all recorded, and the authored
`instance` selector chooses exactly one for `acrossRuns` resolution. `LAST` is
the default and is correct for review loops, where the final occurrence produced
the approved artifact and earlier ones were superseded. `withinRun` resolution
always uses the immediately preceding occurrence of the same key.

Genuine parallel fan-out — several concurrent children of one key within one
transition — remains unsupported in P0 and fails admission as ambiguous. That is
a different problem from sequential revisits and needs an authored deterministic
instance-key strategy, deferred by design.

Non-agent workstations (`LOGICAL_MOVE`, such as `review-loop-breaker`) hold no
session and must be rejected at definition validation if given a
`continuationKey`, and skipped without error during resolution.

### Continued prompts

Every packaged worker body currently asserts statelessness. `fusion-drafter`
says *"Assume no prior context"*; `draft-fusion` says *"Start from zero
context"*; `refine-fusion` says *"Start from zero context beyond the original
request and prior draft"*; all three `@you/classify` executors say *"Assume no
prior context"*. A role receiving either transport gets that instruction on top
of context it can actually see, which is a direct contradiction.

Bindings whose effective transport is not `NONE` therefore declare
`promptOnContinue` or `promptFileOnContinue`. When absent, the ordinary prompt is
used and definition compilation emits a `CONTINUATION_PROMPT_UNSPECIFIED`
warning. Authoring these variants for each reference factory is delivery work,
not a footnote.

### Continuation request and result

HTTP/CLI/internal continuation uses the predecessor Factory Session directly:

```json
{
  "continuedFromFactorySessionId": "factory-session-1",
  "requestId": "client-idempotency-key",
  "input": {
    "request": "Now make the answer shorter"
  },
  "continuationOverrides": {
    "bindings": {
      "refiner": {
        "context": { "acrossRuns": { "mode": "PROMPT_CONTEXT" } }
      }
    }
  }
}
```

The service:

1. loads the predecessor Factory Session and exact Factory revision;
2. confirms the predecessor is terminal and has no different successor;
3. loads and validates the authored continuation policy;
4. validates requested overrides against definition allowlists;
5. resolves every binding's effective transport, including fallback, without
   starting external work or assembling payload;
6. rejects atomically if any `FAIL` rule is unsatisfied;
7. creates the successor Factory Session and Work Request;
8. persists the predecessor link, request id, resolved policy, and binding plan;
9. runs the successor from its ordinary Petri/JavaScript entrypoint; and
10. persists actual transport outcomes before exposing child running state.

The result returns the successor Factory Session and a safe continuation summary.
It does not expose raw Provider Session IDs on public customer surfaces unless an
existing privileged inspection contract permits them.

### Minimal persisted state

No Chat conversation table is required. P0 adds only continuation facts to
canonical owners:

**Factory Sessions / Recordings**

- predecessor Factory Session ID;
- successor Factory Session ID or queryable successor relation;
- root Factory Session ID if useful as a denormalized projection;
- continuation request id;
- exact Factory revision;
- resolved policy hash/snapshot;
- per-key, per-occurrence bindings and resolved transport outcomes; and
- ordinary Work, result, artifact, usage, and Factory events.

**Worker Sessions**

- immutable Worker turns/attempts;
- previous/forked Worker Session relationship where applicable;
- Factory Session and continuation-key association;
- exact Provider Session ref used for each turn; and
- transport outcome.

**Chat Sessions / ACP adapter**

- existing session identity, target episodes, turns, sequencing, stream head;
- the current episode as the continuation-chain head;
- durable persistence replacing the in-memory store; and
- no new canonical Chat conversation resource consumed by Factory Runtime.

### Petri behavior

Every follow-up still creates a new Factory Session, new Work, and a new Petri
marking. The predecessor marking and tokens remain historical and never move
into the successor. All guard state resets with the new marking — including
`VISIT_COUNT`, so a continued review factory receives a full fresh visit budget.
Resource capacity counters likewise reset and are never held across sessions.

Continuation bindings influence Worker dispatch, not token identity. When a
transition dispatches a workstation with a continuation key, Factory Runtime and
Worker Sessions apply the pre-resolved plan:

- continue/fork/new the Worker Session;
- apply the effective transport — pass the exact validated Provider Session ref,
  assemble the declared visible sources into the Work payload, or neither; and
- record the actual outcome before child running output is published.

`PROMPT_CONTEXT` payload assembly is bounded, deterministic, and versioned. It
draws only from canonical Work results, decision envelopes, and artifacts already
owned by the predecessor — never from another assembled payload, so context
cannot grow recursively across a chain. Work owns the resulting payload and
lineage.

### JavaScript behavior

Every follow-up invokes a new JavaScript Factory Session from its entrypoint.
JavaScript child dispatch APIs may provide a required `continuationKey` for
children that participate in continuity. The runtime resolves that key against
the precomputed plan; scripts do not receive or forge raw Provider Session refs,
and do not assemble `PROMPT_CONTEXT` payloads themselves.

JavaScript checkpoints and `JavaScriptResumeContext` remain same-execution
recovery only. A continuation never loads the predecessor's JavaScript
checkpoint. Completed child result replay from a checkpoint must not be confused
with cross-Factory-session continuity.

Dynamic JavaScript fan-out is fresh by default unless the definition and child
call together provide a deterministic unique instance key accepted by validation.

### Worker Session evolution

Worker Sessions need a multi-turn contract before `CONTINUE` can ship:

- Worker Session is the stable conversational/execution container;
- Worker Turn is one immutable admission and execution attempt group;
- a completed Worker Turn is terminal, while the Worker Session may accept a
  later turn under serialized policy;
- `StartTurn` accepts resolved execution plus an optional exact Provider Session
  ref and Factory continuation identity;
- one active Worker Turn per Worker Session in P0;
- events and inspection identify both Worker Session ID and Worker Turn ID; and
- cancel/pause/resume target the active turn without rewriting prior turns.

Compatibility for existing one-shot Worker Session reads must be planned and
tested. Existing terminal fields can project the latest/only turn during a
transition period, but the canonical multi-turn contract must not pretend that a
terminal Worker Turn makes its reusable Worker Session permanently unusable.

### ACP and Chat presentation

One ACP session represents one projected continuation chain, carried by one Chat
Session. On the first prompt, the adapter starts a Factory Session and binds it
to the open Target Episode. On the next prompt, it rolls a new episode at the
same target and calls the continuation operation using the predecessor Factory
Session, updating the head only after idempotent admission succeeds. The shipped
`RecordPendingFactorySession` → `BindFactorySession` pair is the crash-safe head
update; no new reconciliation mechanism is needed.

`session/load` reconstructs presentation from the Factory continuation chain and
source events. `session/resume` reattaches without replay. Neither operation
continues execution.

Chat Sessions remains a continuation/application façade plus a
projection/sequencing service. It does not persist its own conversation domain.
If later UX requirements cannot be represented from continuation chains — draft
messages, mixed targets, branching, or non-Factory conversation items — that
evidence can justify a canonical Chat model in a later ADR.

### Side effects and safety

| Risk | Failure mode | Required mitigation |
| --- | --- | --- |
| Silent reasoning loss | `REUSE_SESSION` fails and the role starts fresh, losing hidden harness state without anyone noticing. | Default `fallback: PROMPT_CONTEXT` preserves visible history; every degradation records `FELL_BACK_TO_PROMPT_CONTEXT` with a reason and surfaces it. |
| Misreported continuity | UI says "continued" when the role only received visible context. | Distinct outcomes for `SESSION_REUSED` and `PROMPT_CONTEXT_APPLIED`; disclosure names the transport, not a boolean. |
| Wrong session reuse | A child inherits another role's provider history. | Authored keys, exact typed refs, exact-match compatibility scope for `REUSE_SESSION`. |
| Wrong occurrence reuse | A cyclic factory binds a superseded early revision instead of the final one. | Authored `instance`; `LAST` default; every occurrence recorded. |
| Reviewer anchoring | A continued adversarial reviewer defends its earlier verdict instead of judging the new draft. | `withinRun: PROMPT_CONTEXT` gives independent reasoning with visible knowledge of prior verdicts; behavioral test asserts approval is reachable. |
| Author amnesia | A review author rewrites from scratch each cycle and reintroduces fixed flaws. | `withinRun: REUSE_SESSION` on the author role. |
| Context growth | Payload from turn N is re-embedded into turn N+1's payload, compounding. | `PROMPT_CONTEXT` draws only from canonical results/envelopes/artifacts, never from another assembled payload; `maxBytes` per spec. |
| Secret amplification | Prior tool output or artifacts are copied into a new prompt, logs, and events. | Closed `source` enum, redaction before assembly, sanitized diagnostics. |
| Duplicate successor | Crash or concurrent request creates two P0 continuations from one predecessor. | Request idempotency and unique predecessor-successor admission constraint. |
| Partial continuation | Some children start before a required transport fails. | Resolve the complete plan before any child/provider execution. |
| Cross-tenant/session leak | A forged predecessor or Provider Session ref crosses authorization boundaries. | Resolve refs only from the authorized predecessor's canonical ledger; never accept raw customer refs. |
| Provider incompatibility | Runner/model/workspace changed but the old session is passed anyway. | Exact-match compatibility scope for `REUSE_SESSION`; degrade to `PROMPT_CONTEXT`. |
| Contradictory continued prompt | A role with context is told to assume none. | `promptOnContinue`; `CONTINUATION_PROMPT_UNSPECIFIED` compile warning. |
| Stop-token inheritance | A resumed provider transcript already contains the stop token and terminates instantly. | Stop-token scanning scoped to the current turn's output only. |
| Guard-state leakage | `VISIT_COUNT` or resource counters carry into the successor. | New marking per successor; explicit reset assertions. |
| Worker Session concurrency | Two successor factories add turns to the same Worker Session. | One active Worker Turn, reservation/CAS, typed conflict or explicit future queue. |
| Topology drift | A renamed/duplicated role maps to the wrong old child. | Same revision in P0; authored unique keys; reject ambiguous/missing mappings. |
| Classifier stickiness | Reusing its session is mistaken for reusing its label. | Execute classification every turn; only context is carried. |
| Token contamination | Prior Petri marking is treated as continuation state. | New marking/tokens; continuity affects dispatch binding only. |
| Checkpoint confusion | Predecessor JavaScript checkpoint is loaded into successor. | Cross-session checkpoint rejection; separate continuation and execution-resume types. |
| Duplicate external effects | Retry/recovery reruns tools or writes. | Keep retry, same-execution resume, and continuation distinct; no automatic retry. |
| Provider retention dependency | Provider-native history disappears outside platform retention. | `REUSE_SESSION` reports unavailable and degrades; P0 does not invent retention guarantees. |

## Variants and context-relative complications

Each decision below lists at least three variants, when each fits, and the
recommended P0 choice.

### Canonical continuation state

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Introduce a canonical Chat conversation model now | Product already needs drafts, branching, mixed targets, chat metadata, and non-Factory messages. | Adds a second durable lifecycle before those needs are proven. |
| B. Use Factory Session predecessor/successor facts as the continuation chain | Factory-backed chat where each prompt runs a Factory. | Chat-only features may later require an additional model. |
| C. Reopen one Factory Session for all prompts | Purpose-built non-terminal interactive runtimes. | Breaks ordinary terminal semantics and risks replaying effects. |

**Recommendation: B.** Chat Sessions keeps the attachment/sequencing state it
already owns and maps its current Target Episode to the chain head.

### Authority for continuation policy

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Chat/ACP chooses the transport | One fixed application owns every Factory and understands all topology. | Makes the same Factory behave differently across transports and leaks child internals. |
| B. Operator defaults choose globally | Homogeneous fleets with identical worker/provider behavior. | Defaults cannot safely identify role-specific needs or route changes. |
| C. Factory Definition owns policy and constrained overrides | Portable factories with different continuity needs per role. | Requires authored schema, validation, and migration of continuable factories. |

**Recommendation: C.** Operator/invocation defaults apply only through explicit
definition delegation and may not broaden authored policy.

### Shape of the continuity setting

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Boolean/binary reuse per role | Factories where the only question is "same provider session or not". | Cannot express "fresh reasoning, but aware of what was already said" — the adversarial-review case — and forces a cliff between full memory and amnesia. |
| B. Reuse mode plus a separate factory-wide input-carry block | Migration from the current shape. | Two mechanisms answering one question; input carry cannot vary per role, so fusion's drafter and refiner must share it. |
| C. One per-role transport enum: `REUSE_SESSION`, `PROMPT_CONTEXT`, `NONE`, with declared fallback | Any factory whose roles have different memory needs. | Requires a bounded `source` spec and a budget; reintroduces a small slice of deferred context machinery. |

**Recommendation: C.** It collapses two mechanisms into one, allows per-role
carry, makes degradation a gradient instead of a cliff, and makes the operator
kill switch non-destructive. Its cost is the closed `source` enum and byte
budget, which is the honest price and is far short of a general context selector.

### Reuse scope

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. One setting covering all prior executions | Purely linear factories that never revisit a workstation. | Cannot express the review author needing memory across cycles while the reviewer must not. |
| B. Condition expressions (`when:` / `otherwise:`) on each binding | Highly dynamic policies. | Introduces a mini expression language and a large validation surface; conflicts with resolve-before-execute. |
| C. Two explicit scopes, `withinRun` and `acrossRuns`, each taking a transport spec | Linear and cyclic factories alike. | One extra level of nesting; both remain static and fully pre-resolvable. |

**Recommendation: C**, both defaulting to `NONE`. Runtime-decided transports —
letting a guard or workflow script choose per dispatch — are rejected outright:
they break "resolve the complete plan before any child executes", let scripts
forge policy, and make replay uninspectable.

Session lanes — authoring a lane key so that "same lane ⇒ same session",
subsuming `instance`, scope, and fan-out instance keys into one concept — are the
natural generalization and should get an ADR after this ships. They are not P0:
lanes cannot express availability/fallback semantics on their own, and accidental
cross-role lane collision needs validation design first.

### Worker Session continuity

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Continue the same Worker Session with immutable Worker turns | Stable logical roles that should retain identity/history. | Requires Worker Sessions to evolve beyond one absorbing terminal result. |
| B. Fork a successor Worker Session | Audit-sensitive flows wanting explicit per-Factory child identity with lineage. | Needs a fork relationship and its own transport decision. |
| C. Start a new Worker Session | Roles that are intentionally stateless or lack a predecessor. | Loses identity continuity; unrelated to what the model remembers. |

**Recommendation: Definition-configurable A/B/C**, orthogonal to transport.
`CONTINUE` requires the Worker multi-turn contract and therefore lands after the
first vertical.

### Binding identity

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Match by workstation/worker name | Simple static factories during migration. | Names are mutable, fan-out duplicates them, and route changes create ambiguity. |
| B. Match by previous dispatch order | Fixed sequential pipelines. | Any topology or concurrency change misbinds. |
| C. Definition-authored stable continuation keys | Static and dynamic factories needing durable role identity. | Requires uniqueness validation and dynamic instance-key rules. |

**Recommendation: C.** Names remain validation evidence, not canonical identity.

### Binding cardinality for repeated roles

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Require exactly one realized binding per key | Purely linear factories. | Fails outright on `@you/review`, which legitimately revisits one workstation up to eight times. |
| B. Bind every occurrence and choose at dispatch | Factories that vary their own revisit count between turns. | Non-deterministic; the plan can no longer be fully resolved before execution. |
| C. Record every occurrence, resolve one by an authored `instance` selector | Linear and cyclic factories alike. | Requires occurrence recording and one more authored field. |

**Recommendation: C** with `LAST` as the default. Parallel fan-out under one key
remains ambiguous and fails admission in P0.

### Continued-role prompting

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Reuse the ordinary prompt unchanged | Prompts already written to tolerate prior context. | Every packaged factory says "assume no prior context", contradicting both transports. |
| B. Machine-prepend a continuation preamble | Rapid migration across many factories. | Contradictory instructions remain in the body; behavior is provider-dependent. |
| C. Author an explicit `promptOnContinue` per binding | Reference factories and any factory where continuity is material. | One more authored asset per continuable role. |

**Recommendation: C**, with a compile-time warning when absent so the gap is
visible rather than silent.

### Missing or incompatible binding behavior

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Fail the whole continuation before execution | All-or-nothing factories where continuity is required. | Reduces availability when one history is missing. |
| B. Start the affected role fresh and record the outcome | Best-effort workflows. | May regress quality; must never be silent. |
| C. Skip the role | Optional route/role whose result contract permits absence. | Unsafe for required pipeline stages. |

**Recommendation: Definition-configurable A/B/C.** Note that with default
`fallback: PROMPT_CONTEXT`, most "missing session" cases never reach this
decision at all — they degrade transport first and only consult `missingBinding`
when no binding exists for the key.

### Fusion continuation

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. `REUSE_SESSION` for both drafter and refiner | Follow-ups may need both roles' hidden reasoning. | Higher provider dependency and cost. |
| B. Fresh drafter, `REUSE_SESSION` refiner | Follow-ups are primarily refinements of the final answer. | Fresh drafter may diverge from prior analysis. |
| C. `PROMPT_CONTEXT` for both | Provider-neutral deployments or when replayability matters more than hidden reasoning. | Larger prompts; loses harness-internal reasoning. |

**Recommendation: A as the reference policy**, with `fallback: PROMPT_CONTEXT`,
and all three authorable and testable.

### Classifier continuation

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. `REUSE_SESSION` on the classifier, classify again every turn | Prior intent helps interpret "actually implement all of it". | Must not reuse the previous label as the new decision. |
| B. `PROMPT_CONTEXT` carrying the previous result | Classification should be independent but informed. | Loses hidden classifier reasoning. |
| C. `NONE` | Each request should be judged in isolation. | Follow-up pronouns and elision may be misread. |

**Recommendation: A as the reference policy**, with classification executed every
turn. Route-specific downstream roles follow their own bindings.

### Adversarial review continuation

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. `REUSE_SESSION` for the reviewer within a run | Convergence matters most; the reviewer checks whether its points were addressed. | Anchoring — the reviewer defends its earlier verdict, which is the failure adversarial review exists to prevent. |
| B. `NONE` for the reviewer within a run | Maximum independence. | The reviewer re-raises settled points and can burn the eight-visit budget without converging. |
| C. `PROMPT_CONTEXT` with `PRIOR_VERDICTS` for the reviewer within a run | Adversarial review generally. | Costs tokens; requires decision envelopes to be retained per occurrence. |

**Recommendation: C.** It separates reasoning from knowledge: the reviewer
evaluates each draft with no hidden commitment to its earlier position, while
still knowing which concerns were already raised. Unlike provider-held memory it
is fully inspectable in Recordings. The author role takes `REUSE_SESSION` within
the run, since it must remember its own draft.

### Definition revision behavior

| Variant | When it fits | Complications |
| --- | --- | --- |
| A. Resolve latest revision on every continuation | Experimental deployments. | Keys and policy can change unexpectedly. |
| B. Continue the predecessor's exact revision | Reproducible P0 and stable mapping. | Upgrades require a separate future operation/new chain. |
| C. Allow caller-selected compatible revision | Mature schema migration support. | Requires compatibility policy that is explicitly out of scope. |

**Recommendation: B for P0.** Cross-revision continuation is rejected.

## Validation against representative factories

The design must hold for four authored topologies. Each is a delivery vertical
below, and each proves a distinct property.

### `@you/fusion` — linear two-stage

Authored: `draft-fusion` (`fusion-drafter`) → `refine-fusion` (`fusion-refiner`);
states `init → draft → complete`.

Reference policy: keys `drafter` and `refiner`, both
`acrossRuns: { mode: REUSE_SESSION, fallback: PROMPT_CONTEXT, source: [PREVIOUS_RESULT] }`,
`instance: LAST`. `withinRun` is `NONE` — neither workstation revisits.

| Turn | Expected behavior |
| --- | --- |
| 1 | Both workstations run. Bindings recorded for both keys with provider session refs and result refs. |
| 2 ("make it shorter") | New successor Factory Session, new Work, fresh tokens at `init`. Both workstations run again, each resuming its exact predecessor session. Each runs exactly once. |
| 2 with the provider unable to resume | Both degrade to `PROMPT_CONTEXT` carrying `PREVIOUS_RESULT`, reported as `FELL_BACK_TO_PROMPT_CONTEXT`. The turn still succeeds. |

**Proves:** the basic key round-trip, both transports, and the fallback gradient.

**Delivery note:** both worker bodies assert zero prior context and need
`promptOnContinue` variants.

### `@you/plan-execute` — linear with a capacity-1 resource

Authored: `plan-request` (`prd-planner`) → `execute-plan` (`prd-executor`);
resource `plan-execute-agent-slot` capacity 1; `workPropagation: PRESERVE_INPUT`;
both workers carry `stopToken: <COMPLETE>`; both use `promptFile:` assets.

Reference policy: keys `planner` and `executor`, both `acrossRuns: REUSE_SESSION`.

On turn 2 ("also add tests") the planner remembers the plan it wrote and the
executor remembers what it built.

**Proves:** keys survive `PRESERVE_INPUT`; a capacity-1 resource still serializes
and its counter resets rather than leaking; `stopToken` still terminates a
resumed turn rather than matching a token already in the transcript;
`promptFileOnContinue` resolves for file-based prompts.

### `@you/classify` — divergent routing

Authored: `classify-request` (`CLASSIFIER_WORKSTATION`, `PRESERVE_INPUT`) routing
to exactly one of `execute-small` / `execute-medium` / `execute-large`, each with
its own worker, provider parameter, and model parameter.

Reference policy: key `classifier` with `acrossRuns: REUSE_SESSION`
(classification still executes every turn); per-branch keys `executor.small`,
`executor.medium`, `executor.large`, each
`acrossRuns: { mode: REUSE_SESSION, fallback: PROMPT_CONTEXT }` and
`missingBinding: START_NEW`.

| Turn | Expected behavior |
| --- | --- |
| 1 | "Explain this briefly" → label `small` → `execute-small` runs. Binding recorded under `executor.small`. |
| 2 | "Actually implement all of it" → classifier resumes its own session but re-derives the label → `large` → `execute-large` has no binding, so `missingBinding: START_NEW` applies. |

Per-branch keys mean the mismatch never arises; the compatibility scope is the
backstop if an author collapses them into one key.

**Proves:** no sticky routing, no cross-worker session bleed, and that a key with
no predecessor binding follows its authored `missingBinding` policy rather than
silently reusing whatever was available. Also proves the compatibility gradient:
changing `${smallModel}` between turns degrades that key to `PROMPT_CONTEXT`
rather than dead-ending.

### `@you/review` — adversarial cycle

Authored: `execute-review-work` → `review-review-work`
(`outcomeFormat: decision-envelope`), `onRejection` back to `init`, plus
`review-loop-breaker` (`LOGICAL_MOVE`, `VISIT_COUNT maxVisits: 8`).

Reference policy — the case that requires the scope split:

```yaml
- key: author
  workstation: execute-review-work
  context:
    withinRun:  { mode: REUSE_SESSION, fallback: PROMPT_CONTEXT }
    acrossRuns: { mode: REUSE_SESSION, fallback: PROMPT_CONTEXT }
  instance: LAST

- key: reviewer
  workstation: review-review-work
  context:
    withinRun:  { mode: PROMPT_CONTEXT, source: [PRIOR_VERDICTS] }
    acrossRuns: { mode: REUSE_SESSION, fallback: PROMPT_CONTEXT }
  instance: LAST
```

`review-loop-breaker` holds no key.

**Proves:** the scope split — one factory, two roles, opposite `withinRun`
answers; the cardinality rule, where the author workstation may run eight times
and `LAST` binds the occurrence that produced the approved artifact; that
`VISIT_COUNT` resets so the successor gets a full budget; that `LOGICAL_MOVE`
workstations are skipped; and that a reviewer which rejected three times can
still reach `approved` because each cycle reasons independently while seeing the
prior verdicts as visible evidence.

## Contract changes

### A. Factory definition — `factory.yaml`

Owned by `pkg/services/factory_definitions/`. All fields additive and optional;
every existing factory keeps current behavior when they are absent.

Factory level: the `continuation` block sketched above — `enabled`, `defaults`,
`bindings[]`, `overrides`. **There is no factory-wide `input.mode` block**;
context carry is per binding, which is what lets fusion's drafter and refiner
differ.

Binding level: `key`, `workstation`, `worker`, `workerSession`, `missingBinding`,
`instance`, `promptOnContinue` / `promptFileOnContinue`, and `context` with
`withinRun` / `acrossRuns` specs, each carrying `mode`, `fallback`, `source`, and
`maxBytes`.

Validation rules added to definition compilation:

- keys non-empty, unique within the factory, `[a-z0-9][a-z0-9._-]*`;
- `continuationKey` rejected on non-agent workstation types (`LOGICAL_MOVE`);
- a binding's `workstation` and `worker` must exist and name each other
  consistently;
- `fallback` legal only on `mode: REUSE_SESSION`, and only one level — a fallback
  may not itself be `REUSE_SESSION`;
- `source` legal only where the effective transport can be `PROMPT_CONTEXT`
  (directly or via fallback), and every member must be in the closed enum;
- `PRIOR_VERDICTS` rejected unless the workstation declares an `outcomeFormat`
  that produces decision envelopes;
- `withinRun` with a non-`NONE` mode rejected on workstations with no revisit
  path in the compiled graph;
- `instance: NONE` forced on `CLASSIFIER_WORKSTATION` bindings unless the author
  supplies an explicit acknowledgement field;
- `overrides.*.allowed` values must be subsets of the legal enums;
- `maxBytes` must be positive and within the operator ceiling;
- `CONTINUATION_PROMPT_UNSPECIFIED` warning when a binding whose effective
  transport can be non-`NONE` declares no continued prompt.

### B. Operator configuration

Owned by `pkg/services/operator_settings/`. Extends the existing
`ACPAgentProfile` (`DefaultTarget`, `AllowedTargets`):

```yaml
acpAgentProfile:
  defaultTarget: factory:@you/fusion
  allowedTargets:
    - factory:@you/fusion
  continuation:
    enabledTargets: []              # allowlist; empty means continuation disabled
    providerSessionReuse: DISABLED  # ENABLED | DISABLED
    promptContextMaxBytes: 65536    # ceiling; a definition may only narrow
    maxChainLength: 50
    defaults:                       # applied only where the definition delegates
      context: { mode: PROMPT_CONTEXT }
```

Effective policy = definition ∧ operator, with the definition winning on every
narrowing and the operator unable to broaden.

`providerSessionReuse: DISABLED` degrades every `REUSE_SESSION` to its declared
`fallback` rather than forcing `NONE`. Because `fallback` defaults to
`PROMPT_CONTEXT`, a deployment that cannot or will not use provider-native resume
still gets working continuity, fully inspectable, with no definition change. This
makes the kill switch a transport switch rather than a feature switch, and makes
it safe to leave `DISABLED` during early rollout. The resolved policy hash is
persisted on the successor.

### C. Internal service interfaces and interactions

| Service | Change | Kind |
| --- | --- | --- |
| `factory_definitions` | Parse/validate/compile the `continuation` block including per-scope context specs; expose resolved policy and binding declarations | Additive |
| `factory_sessions` | `Continue(ctx, ContinueRequest) (ContinueResult, error)`; predecessor/successor relation; resolved policy snapshot; binding plan persistence | New op + additive state |
| `factory_sessions` | Binding ledger read: `ResolveBindings(predecessorID, revision) ([]ResolvedBinding, error)`, returning per-key effective transport after fallback | New op |
| `factory_runtime` | Consume the pre-resolved plan at dispatch; select occurrence per `instance` for `acrossRuns` and the preceding occurrence for `withinRun`; never resolve keys itself | Internal |
| `factory_runtime` | Assemble bounded `PROMPT_CONTEXT` payload from canonical results/envelopes/artifacts; record omissions and truncations | New behavior |
| `factory_runtime` | Emit `WorkstationContextBound{key, workstation, worker, occurrence, scope, mode, outcome, workerSessionID, providerSessionRef, sourceRefs, compatibility}` on every agent dispatch completion | New event |
| `workers` | `WorkstationDispatchRequest` gains `ResumeSession *providers.SessionRef`, `CarriedContext *ContextPayload`, and `ContinuedPrompt bool` | Additive |
| `workers` | Dispatch result surfaces `SessionRef` upward — currently observed in `progress_observations.go` and dropped | Additive |
| `worker_sessions` | `StartTurn` multi-turn contract; Worker Session ID vs Worker Turn ID split in contracts, events, inspection, controls | Additive, gates `CONTINUE` |
| `providers` | None — `ExecuteRequest.ResumeSession` and `ExecuteResult.SessionRef` already exist | None |
| `recordings` | New event types for continuation admission, transport resolution, transport outcome, and fallback | Additive |
| `chat_sessions` | `RollEpisode` so the ACP head advances without overloading `SetTarget`; durable `Store` replacing the in-memory map | Additive + implementation |
| `operator_settings` | `continuation` block on the ACP agent profile | Additive |

**The interaction that is genuinely new — the dispatch round trip:**

```
factory_sessions.Continue(predecessor, requestId, input, overrides)
  → factory_definitions: resolve policy at the predecessor's exact revision
  → factory_sessions.ResolveBindings(predecessor)
       → per key: preferred transport, compatibility check, fallback applied
       → atomic reject if any FAIL rule is unsatisfied
  → create successor Factory Session + Work Request, persist plan
  → factory_runtime dispatches workstation W under key K, scope S
      → plan[K][S] gives the effective transport
      → REUSE_SESSION   → workers.Dispatch(ResumeSession: ref)
                            → providers.Execute → claude --resume <id>
      → PROMPT_CONTEXT  → assemble bounded payload from canonical sources
                            → workers.Dispatch(CarriedContext: payload)
      → NONE            → workers.Dispatch(...)
      → ExecuteResult.SessionRef surfaces back up
      → recordings: WorkstationContextBound
  → ledger updated before child running output is published
```

The new arrows are `SessionRef` surfacing out of `workers`, the bounded payload
assembly, and the ledger write closing the loop. Everything below
`providers.Execute` already works.

### D. Public API and generated artifacts

Authored in `api/openapi-main.yaml` and `api/components/`, regenerated with
`make generate-api`; handlers in `pkg/transports/http`, mappers in
`pkg/transports/mapping`, UI adapters in `ui/src/api/`. Never hand-edit generated
files.

New/changed public surfaces: the continuation request/result; a continuation
policy read for the `Continue` form; a per-key transport outcome summary on
Factory Session reads; and continuation-chain traversal. The summary names the
transport and any degradation reason — never raw Provider Session IDs, Worker
Session internals, markings, checkpoints, assembled payload bodies, or filesystem
paths.

## Delivery — vertical slices

Each vertical goes end-to-end through every layer for one factory and is
independently shippable behind `enabledTargets`. No horizontal layer story ships
on its own.

### V0 — Foundation

- `factory_definitions`: parse/validate/compile the `continuation` block —
  bindings, transports, scopes, sources, instance, overrides, prompts.
- `factory_sessions`: predecessor/successor relation, `Continue` admission with
  request-id idempotency and single-successor conflict, resolved policy snapshot.
- `chat_sessions`: `RollEpisode`; durable store replacing the in-memory map.
- Vocabulary and truth-table docs; annotate
  `docs/internal/projects/acp-client/design.md:240`.

**Exit:** a synthetic single-workstation fixture completes two turns as a
two-link chain with `context.mode: NONE`, surviving restart and rejecting a
second successor.

### V1 — `@you/fusion` end-to-end

The reference vertical. Ships both transports and the fallback path with
`workerSession: NEW`, delivering hidden-reasoning continuity without waiting on
Worker Sessions multi-turn.

- Authored Fusion `continuation` block: keys `drafter`, `refiner`;
  `promptOnContinue` for both.
- `workers.WorkstationDispatchRequest.ResumeSession` + `CarriedContext` +
  `ContinuedPrompt`; `SessionRef` surfaced out of dispatch results.
- `factory_runtime` plan consumption, bounded payload assembly, and
  `WorkstationContextBound` event.
- `factory_sessions` ledger persistence and `ResolveBindings` with fallback.
- Operator Settings `continuation` block; Fusion added to `enabledTargets`.
- HTTP/CLI continuation operation; dashboard `Continue` action and transport
  summary; ACP prompt path rolling the episode and continuing the head.
- Full T1–T8 tier for Fusion, including acpx e2e.

**Exit:** three Fusion turns over one acpx session with both keys reusing their
exact predecessor session, proven by observed session-ID equality and `--resume`
argv; the same three turns with the provider unable to resume degrade to
`PROMPT_CONTEXT`, still succeed, and report `FELL_BACK_TO_PROMPT_CONTEXT`.

### V2 — `@you/plan-execute`

- Authored keys `planner`, `executor`; `promptFileOnContinue` assets.
- Capacity-1 resource counter reset across successors.
- Stop-token scoping fix for resumed turns.

**Exit:** three turns; both keys reused; `<COMPLETE>` still terminates each turn;
the resource is never held by two sessions.

### V3 — `@you/classify`

- Authored `classifier` key plus per-branch executor keys with
  `missingBinding: START_NEW`.
- Compatibility-scope enforcement and the `REUSE_SESSION` → `PROMPT_CONTEXT`
  degradation on model change.

**Exit:** turn 1 `small` → turn 2 `large`; the label is re-derived; the small
executor's session never reaches the large executor; the absent binding follows
`START_NEW` with a visible outcome.

### V4 — `@you/review`

- Authored `author` and `reviewer` keys with opposite `withinRun` transports.
- `withinRun` resolution, occurrence recording, `LOGICAL_MOVE` skip,
  `VISIT_COUNT` reset assertions.
- `PRIOR_VERDICTS` source assembly from decision envelopes.
- Reviewer independence behavioral test.

**Exit:** a run that cycles ≥3 times yields one continuous author session and
three independent reviewer sessions, each reviewer seeing prior verdicts as
visible evidence; the successor receives a full fresh visit budget; a reviewer
that rejected three times can approve.

### V5 — Worker Sessions multi-turn and `CONTINUE`

Deliberately last, because context transport already delivers the customer value
and this is the largest contract change.

- `StartTurn`; Worker Session vs Worker Turn identity split across contracts,
  events, inspection, and controls; one active turn per session.
- Backward compatibility for existing one-shot reads.
- Promote reference factories from `workerSession: NEW` to `CONTINUE` and re-run
  every vertical's T4 tier unchanged.

**Exit:** every vertical passes identically under `CONTINUE`, with Worker Session
IDs now stable across the chain.

### V6 — JavaScript parity and hardening

- JavaScript child `continuationKey` contract, validation, checkpoint separation;
  Petri/JavaScript parity fixtures.
- Retry-continuation UX and duplicate-effect warning.
- Metrics and alerting.
- Enable remaining packaged factories as evidence accrues.

### Deferred (not scheduled)

Session lanes as a unifying identity mechanism; branching; prompt queueing;
transcript compaction; model-generated summaries; an open-ended context selector;
retention design; cross-revision continuation; parallel fan-out instance keys;
and a canonical Chat conversation model. Each requires an ADR with at least three
variants and a recommendation.

### Task shaping

Before submitting a vertical's work to a Factory worker, reshape it through
`docs/internal/standards/templates/task-templates.md` and use this plan's
absolute path as the original document.

## Definitive test plan

Every tier runs per vertical. A vertical is not done until every applicable row
passes for its factory. Tests synchronize on deterministic events, never sleeps.

### T1 — Pure contract and policy tests

| ID | Assertion |
| --- | --- |
| T1.1 | The operation truth table: for each of continue / retry / execution resume / reconnect / `session/load` / `session/resume`, whether it creates a Factory Session, Work, Worker turn, or external effect. |
| T1.2 | Definition validation: duplicate keys, blank keys, key on `LOGICAL_MOVE`, binding naming a nonexistent workstation or mismatched worker, override values outside the legal enums. |
| T1.3 | Transport validation: `fallback` on a non-`REUSE_SESSION` mode rejected; `fallback: REUSE_SESSION` rejected; `source` outside the closed enum rejected; `PRIOR_VERDICTS` without a decision-envelope `outcomeFormat` rejected; `maxBytes` non-positive or above the operator ceiling rejected. |
| T1.4 | Scope validation: `withinRun` non-`NONE` on a workstation with no revisit path in the compiled graph is rejected; `instance: NONE` forced on classifiers absent acknowledgement. |
| T1.5 | `CONTINUATION_PROMPT_UNSPECIFIED` fires exactly when a binding whose effective transport can be non-`NONE` declares no continued prompt. |
| T1.6 | Definitions with no `continuation` block reject continuation with a typed unsupported outcome; both scopes default to `NONE`. |
| T1.7 | Effective policy: operator cannot enable a non-continuable definition, invent keys, or broaden `overrides.allowed`; definition narrowing of `maxBytes` always wins. |
| T1.8 | `providerSessionReuse: DISABLED` degrades every `REUSE_SESSION` to its declared fallback — **not** to `NONE` — and a binding with `fallback: FAIL` fails rather than silently proceeding. |
| T1.9 | Compatibility scope: `REUSE_SESSION` requires exact match on revision / runner / modelProvider / model / workspace digest (five negative cases); `PROMPT_CONTEXT` requires only revision and role and survives a model change. |
| T1.10 | `RollEpisode` closes exactly one episode and opens one at the same target under `ExpectedVersion`; rejects while a turn is active with `BusyError`. |
| T1.11 | `BindFactorySession` still rejects a second distinct ID on one episode — regression guard on shipped behavior. |

### T2 — Continuation admission, idempotency, and durability

| ID | Assertion |
| --- | --- |
| T2.1 | Same `requestId` returns the same successor; no second Factory Session. |
| T2.2 | A different request against a predecessor that already has a successor returns a typed conflict. |
| T2.3 | Two clients racing the same `requestId` produce exactly one successor. Under `-race`. |
| T2.4 | Two clients racing different request ids produce one winner and one typed conflict. |
| T2.5 | Crash injected at each boundary — post-resolve/pre-create, post-create/pre-persist-plan, post-persist/pre-run, post-run/pre-ledger-write — converges on one successor or one typed failed continuation. One test per boundary. |
| T2.6 | Restart reconstructs the chain, resolved policy, plan, and per-key outcomes byte-identically. |
| T2.7 | A non-terminal predecessor cannot be continued. |
| T2.8 | Cross-revision continuation is rejected. |
| T2.9 | Cross-tenant predecessor IDs, caller-supplied provider refs, and workspace escalation all fail closed. |

### T3 — Runtime, lineage, and isolation

| ID | Assertion |
| --- | --- |
| T3.1 | The successor produces a new marking; no token ID or place from the predecessor appears. |
| T3.2 | `VISIT_COUNT` and every guard counter reset in the successor (review fixture). |
| T3.3 | Resource capacity counters reset; a capacity-1 resource is never held by two sessions (plan-execute fixture). |
| T3.4 | Replay reproduces Work lineage, transition order, and every transport-outcome event. |
| T3.5 | An external-effect fixture workstation executes at most once per admitted continuation. |
| T3.6 | Classifiers execute on every continuation even when their own session is reused. |
| T3.7 | Predecessor JavaScript checkpoints are rejected by the successor; same-execution recovery still works. |
| T3.8 | Petri/JavaScript parity: identical public transport outcomes for equivalent factories. |

### T4 — Context transport (the core requirement)

Run against a recording provider harness capturing exact provider CLI argv,
provider-issued session IDs, and rendered prompt text, so assertions are on
observed behavior rather than internal intent. Every row runs per vertical.

**`REUSE_SESSION`**

| ID | Assertion |
| --- | --- |
| T4.1 | Session identity equality: for each `REUSE_SESSION` key the provider session ID observed on turn 2 equals the ID recorded on turn 1. Per key, per factory. |
| T4.2 | Argv proof: the turn-2 invocation carries the resume flag with the turn-1 ID (`--resume <id>` for Claude; the adapter equivalent for codex/acp/agy). Turn 1 carries none. |
| T4.3 | Three-turn chain: turn 3 resumes turn 2's session, not turn 1's. |
| T4.4 | Outcome recorded is `SESSION_REUSED`, and no `CarriedContext` payload is attached. |
| T4.5 | Two provider families: T4.1 and T4.2 pass on Claude and Codex. |

**`PROMPT_CONTEXT`**

| ID | Assertion |
| --- | --- |
| T4.6 | A `PROMPT_CONTEXT` binding starts a **new** provider session — the turn-2 session ID differs from turn 1 and no resume flag is present. |
| T4.7 | The rendered prompt contains exactly the declared `source` content; a source not declared is absent. |
| T4.8 | `maxBytes` truncation and expired/redacted/unavailable sources produce typed refs on the outcome, never silent omission. |
| T4.9 | No recursion: a three-turn chain's turn-3 payload is assembled from canonical results and contains no turn-2 assembled payload. Asserted on payload bytes. |
| T4.10 | `PRIOR_VERDICTS` carries the decision envelopes of the key's earlier occurrences, in order, and nothing else. |
| T4.11 | Outcome recorded is `PROMPT_CONTEXT_APPLIED`. |

**Fallback and degradation**

| ID | Assertion |
| --- | --- |
| T4.12 | With the predecessor provider session made unavailable, a `REUSE_SESSION` binding with default fallback runs successfully via visible context and records `FELL_BACK_TO_PROMPT_CONTEXT` with a reason. |
| T4.13 | `fallback: FAIL` instead rejects before any affected child executes. |
| T4.14 | `fallback: NONE` runs with nothing carried and records `NO_CONTEXT`. |
| T4.15 | A model change between turns degrades `REUSE_SESSION` to `PROMPT_CONTEXT` rather than failing; a `PROMPT_CONTEXT` binding is unaffected by the same change. |
| T4.16 | Kill switch: `providerSessionReuse: DISABLED` makes every `REUSE_SESSION` behave exactly as its fallback, with transcripts identical to an authored `PROMPT_CONTEXT` binding. |
| T4.17 | Degradation is never reported as reuse in events, projections, or the public summary. |

**Scope**

| ID | Assertion |
| --- | --- |
| T4.18 | Review, ≥3 cycles: the author key yields **one** provider session across all cycles; the reviewer key yields **one per cycle**, all distinct. |
| T4.19 | Each reviewer cycle's rendered prompt contains the prior cycles' verdicts and no others. |
| T4.20 | `withinRun` and `acrossRuns` resolve independently: a key with `withinRun: NONE, acrossRuns: REUSE_SESSION` starts fresh on each revisit but resumes across turns. |
| T4.21 | Reviewer independence: a reviewer that rejected 3× in turn 1 reaches `approved` in turn 2. |

**Identity, cardinality, and integrity**

| ID | Assertion |
| --- | --- |
| T4.22 | `instance: LAST` binds occurrence N in a run that cycled N times; `instance: FIRST` binds occurrence 1; `instance: NONE` never binds. |
| T4.23 | Cross-role isolation: classify turn 1 `small` → turn 2 `large`; the small executor's session and results never reach any other worker. |
| T4.24 | Continued prompt selection: a binding with non-`NONE` effective transport renders `promptOnContinue`; a `NONE` binding renders the ordinary prompt. Asserted on rendered text. |
| T4.25 | Stop-token scoping: a resumed turn whose transcript already contains `<COMPLETE>` still executes and terminates on its own new stop token. |
| T4.26 | An interrupted predecessor dispatch records no `AVAILABLE` binding; the successor treats that key as `MISSING`. |
| T4.27 | Ledger completeness: every transport application is preceded by a persisted binding event; no reuse or carry occurs without one. |
| T4.28 | Worker identity is orthogonal: `workerSession: CONTINUE` with `context.mode: NONE` keeps the Worker Session ID while starting a fresh provider session, and the reverse also holds. From V5. |

### T5 — Transport equivalence

| ID | Assertion |
| --- | --- |
| T5.1 | CLI, HTTP, and ACP continuation route through one service contract and produce identical chains, plans, and outcomes for the same inputs. |
| T5.2 | Only definition-approved override values are accepted; a disallowed override is rejected identically everywhere. |
| T5.3 | Caller-supplied raw provider session refs are rejected on every transport. |

### T6 — ACP conformance and e2e via acpx

Run against the built `you` binary over ACP stdio, driven by acpx as an
independent external client. Complements `tests/functional/transport/acp/stdio/`.

| ID | Assertion |
| --- | --- |
| T6.1 | Three sequential `session/prompt` calls in one acpx session yield one Chat Session, three Target Episodes, three Factory Sessions in one chain, stable ordering. |
| T6.2 | Turn 2 generates a new work item rather than resuming an existing one — preserving `design.md:240` — while T4.1 simultaneously holds. Asserted in one test. |
| T6.3 | `session/load` replays the projected chain and starts no execution. |
| T6.4 | `session/resume` attaches, replays nothing, starts no execution. |
| T6.5 | `session/close` releases resources per its declared cancellation policy. |
| T6.6 | Disconnect mid-turn, reconnect: no new execution, no duplicate dispatch, no continuation until a new prompt. |
| T6.7 | Factory child workers project as stable tool calls scoped to their own Factory Session across all three turns. |
| T6.8 | Worker session tool-call enumeration and streaming remain correct across a continuation boundary. |
| T6.9 | Model/target enumeration for packaged factories still works mid-chain. |
| T6.10 | The chain head advances idempotently across a restart between prompts. |
| T6.11 | No custom `factory/continue` or `factory/resume` ACP method is advertised or accepted. |
| T6.12 | Slow consumer and multiple observers: retained-to-live handoff preserves aggregate order; no observer sees a duplicate continuation. |

Run T6.1, T6.2, T6.6, and T6.7 once per vertical against that vertical's factory.

### T7 — Dashboard and human acceptance

| ID | Assertion |
| --- | --- |
| T7.1 | The UI distinguishes continue, execution resume, retry continuation, and reconnect with accessible labels and keyboard operation. |
| T7.2 | The policy summary names each role's transport — provider session, visible context, or nothing — rather than a boolean. |
| T7.3 | Degradation, required-failure, conflict, and unavailable-history states each render explicit copy in narrow layouts. |
| T7.4 | History renders as a projection over the continuation chain. |
| T7.5 | Human acceptance: Zed, Neovim, and Obsidian each run a three-turn Fusion conversation and one Review conversation, confirming coherent transcripts and understandable transport disclosure. Gates V1 and V4 exit. |

### T8 — Security and disclosure

| ID | Assertion |
| --- | --- |
| T8.1 | Logs, events, metrics, API payloads, and CLI output contain no raw provider session IDs, checkpoints, secrets, assembled payload bodies, or absolute filesystem paths. Asserted by field allowlist, not regex. |
| T8.2 | Redaction is applied before `PROMPT_CONTEXT` assembly, so a redacted predecessor result never reaches a successor prompt. |
| T8.3 | A continued session cannot widen workspace roots granted at chain creation. |
| T8.4 | Forged predecessor IDs and cross-tenant refs fail closed with sanitized diagnostics. |

### Verification commands

```
make test                    # short Go suite
make ui-test / make ui-lint  # dashboard behavior
make generate-api            # after any api/ change
make api-smoke               # public REST contract changes
make docs-reference-smoke    # packaged CLI docs
make verify-fast             # typecheck + short UI/unit + short Go
make test-functional         # T2–T5 tiers
make verify-pr               # PR gate
```

`-race` is mandatory for T2.3, T2.4, and T6.12.

## Rollout

Per vertical, in order (V0 → V1 → V2 → V3 → V4 → V5 → V6):

1. Land the vertical's definition contracts and validation.
2. Land its service, runtime, and transport changes with
   `providerSessionReuse: DISABLED` and an empty `enabledTargets`. Because
   `DISABLED` degrades to `PROMPT_CONTEXT` rather than off, this step already
   exercises the full continuity path.
3. Enable the target in development profiles — prove the chain, idempotency, and
   restart behavior on visible context alone.
4. Enable `providerSessionReuse` and validate the full T4 tier against real
   providers across at least two families.
5. Observe fallback rates, incompatibility rates, payload sizes, and cost before
   widening `enabledTargets`.

Track continuation admissions and conflicts, binding resolution by key, transport
selected versus applied, fallback and incompatibility rates, payload size and
omissions, instance selection, Worker Session mode, latency and cost, route
changes, restart reconciliation, and ACP head-update failures. Fallback and
incompatibility rates are the leading indicators that authored keys have drifted
from topology; payload size is the leading indicator that a `source` list is too
broad. Never log raw provider session identifiers or message content in metrics.

Delivery of a vertical is complete only after required CI is terminal and
passing, blocking review feedback is resolved, conflicts and generated artifacts
are reconciled, and the implementation PR is actually merged.

## References

Repository references:

- `docs/architecture/data-model.md`
- `docs/architecture/architecture.md`
- `docs/architecture/structures.md`
- `docs/architecture/packaged-structure.md`
- `docs/internal/standards/STANDARDS.md`
- `docs/internal/standards/code/planning-standards.md`
- `docs/internal/standards/templates/task-templates.md`
- `docs/internal/projects/acp-client/design.md`
- `docs/temp/acp-factory-worker-sessions-architecture.md`
- `pkg/services/factory_definitions/`
- `pkg/services/factory_sessions/`
- `pkg/services/chat_sessions/`
- `pkg/services/worker_sessions/`
- `pkg/services/work/`
- `pkg/services/factory_runtime/`
- `pkg/services/recordings/`
- `pkg/services/workers/`
- `pkg/services/providers/`
- `pkg/services/provider_sessions/`
- `pkg/services/operator_settings/`
- `packages/packaged-factories/factories/fusion/`
- `packages/packaged-factories/factories/plan-execute/`
- `packages/packaged-factories/factories/classify/`
- `packages/packaged-factories/factories/review/`
