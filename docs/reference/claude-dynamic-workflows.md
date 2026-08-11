---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/references/claude-dynamic-workflows
---

# Dynamic Workflows (`Workflow` tool) — Reference

> Compatibility snapshot: this page records the Claude Code `Workflow` schema supplied on
> 2026-08-10 as an external compatibility baseline. It does not describe the currently shipped
> You JavaScript workflow contract. Use [JavaScript workflows](javascript-workflows.md) for
> supported You behavior and [Orchestrators](orchestrators.md) for canonical product vocabulary.

This document describes the exact shape of the `Workflow` tool: what payloads it accepts, what the
script body can call, what each call accepts/returns, and where its capabilities stop (file access,
streaming, multimodal input, etc.). It reflects the tool as currently exposed to Claude Code — treat
it as a live API reference, not marketing copy.

---

## 1. What it is

`Workflow` executes a JavaScript-like orchestration script that fans work out to subagents
deterministically (loops, conditionals, pipelines, barriers), rather than leaving that control flow
to the model's own judgment turn-by-turn. It always runs **in the background**: the calling tool
returns immediately with a task/run id, and a notification arrives when the run finishes. Live
progress can be watched with `/workflows`.

It is gated behind explicit user opt-in (a named "ultracode" trigger, a session-wide ultracode
setting, an explicit request to use a workflow/orchestration, or a saved/named workflow invocation).
It is not something the assistant calls opportunistically.

---

## 2. Top-level tool payload

The `Workflow` tool itself (the thing that gets called, e.g. from Claude Code) accepts:

| Field | Type | Required | Notes |
|---|---|---|---|
| `script` | string (≤ 524,288 chars) | one of `script`/`scriptPath`/`name` | Full self-contained script text, sent inline. |
| `scriptPath` | string | — | Path to a previously-persisted script file. Takes precedence over `script`/`name`. Every invocation auto-persists its script to disk and returns the path — edit that file and re-invoke with `scriptPath` to iterate instead of resending the whole script. |
| `name` | string | — | Name of a predefined/saved workflow (built-in or from `.claude/workflows/`). |
| `args` | any (JSON) | no | Exposed inside the script as the global `args`. Arrays/objects must be passed as real JSON values, not JSON-encoded strings. |
| `resumeFromRunId` | string, pattern `^wf_[a-z0-9-]{6,}$` | no | Resumes a prior run. The longest unchanged prefix of `agent()` calls (same prompt + opts) is served from cache; the first changed/new call and everything after it re-executes live. |
| `title` | string | no | Ignored — set title in the script's `meta` block instead. |
| `description` | string | no | Ignored — set description in the script's `meta` block instead. |

Exactly one of `script`, `scriptPath`, `name` effectively drives execution (`scriptPath` > `script`/`name`).

Concurrency, item, and lifetime caps (fixed by the runtime, not configurable via payload):

- Concurrent `agent()` calls: `min(16, cpu_cores - 2)` per workflow; excess calls queue.
- Items per single `parallel()`/`pipeline()` call: max 4096 (hard error above that, not silent truncation).
- Total `agent()` calls across a workflow's lifetime: 1000 (runaway backstop).
- Default guidance in this session: keep workflows under ~15 agents ("medium" size) unless the user asks for a different scale — this is a soft convention, not an enforced cap.

---

## 3. Script structure

Every script is **plain JavaScript**, not TypeScript — type annotations, interfaces, and generics are
parse errors. The body executes in an async context (`await` works directly at top level of the body).

### 3.1 Required `meta` export

```js
export const meta = {
  name: 'find-flaky-tests',            // required
  description: 'Find flaky tests...',  // required, one line, shown in permission dialog
  whenToUse: '...',                    // optional, shown in the workflow list
  phases: [                            // optional, one entry per phase() call
    { title: 'Scan', detail: 'grep test logs for retries' },
    { title: 'Fix', detail: 'one agent per flaky test', model: 'sonnet' }, // model = override note for that phase
  ],
}
```

`meta` **must be a pure object literal** — no variables, function calls, spreads, or template
interpolation. `phases[].title` strings should exactly match the strings passed to `phase('...')` in
the body; a `phase()` call with no matching `meta.phases` entry still gets its own progress group.

### 3.2 What the script body can use

Available in-script: `agent()`, `parallel()`, `pipeline()`, `phase()`, `log()`, `workflow()`, the
globals `args` and `budget`, and standard JS built-ins (`JSON`, `Math`, `Array`, etc.).

**Explicitly unavailable inside the script body**: `Date.now()`, `Math.random()`, argless
`new Date()` (all would break resumability — pass timestamps in via `args` instead, and stamp
results with wall-clock time after the workflow returns), any filesystem access, and any direct
Node.js API access. The script cannot read/write files itself.

---

## 4. Core functions

### 4.1 `agent(prompt, opts?)`

Spawns one subagent. Signature:

```ts
agent(
  prompt: string,
  opts?: {
    label?: string,        // overrides the display label in progress UI
    phase?: string,        // explicitly assigns this call to a phase/progress group
                            // (use inside pipeline()/parallel() to avoid races on global phase() state)
    schema?: object,        // JSON Schema — forces a StructuredOutput tool call
    model?: string,         // overrides model for this call only (usually omit — inherits session model)
    effort?: 'low' | 'medium' | 'high' | 'xhigh' | 'max',  // reasoning effort override (usually omit)
    isolation?: 'worktree', // runs the agent in a fresh git worktree (expensive; only for parallel
                             // agents that mutate files and would otherwise conflict)
    agentType?: string,     // custom subagent type (e.g. 'general-purpose', 'code-reviewer') instead
                             // of the default workflow subagent; resolved from the same registry as
                             // the interactive Agent tool; composes with `schema`
  }
): Promise<any>
```

- **Without `schema`**: resolves to the subagent's final text output (a string). Subagents are told
  their final text *is* the return value, not a human-facing message — so they return raw data, not
  prose addressed to a user.
- **With `schema`**: the subagent is forced to call a `StructuredOutput` tool matching that JSON
  Schema; `agent()` resolves to the validated object. Validation happens at the tool-call layer, so
  a malformed call causes the model to retry — the caller doesn't need to parse text.
- Resolves to `null` if: the user skips the agent mid-run, or the subagent dies on a terminal API
  error after retries. Always `.filter(Boolean)` result arrays before use.
- `opts.model` / `opts.effort`: default to omitting both — they inherit the resolved session
  model/effort, which is correct in almost all cases.
- `opts.isolation: 'worktree'`: creates a real git worktree per agent (~200–500ms + disk cost); the
  worktree auto-removes itself if the agent made no changes. Use only for genuine parallel-write
  conflicts.
- `opts.agentType`: lets a workflow agent run as any registered subagent type (same registry as the
  interactive `Agent` tool), gaining that type's tool access/system prompt instead of the generic
  workflow default.

**File read/write support**: yes, indirectly. Individual agents spawned by `agent()` have full tool
access (Read, Grep, Glob, Bash, Edit/Write, and any session-connected MCP tool, resolved on demand
via `ToolSearch`) — so a workflow can read, search, and write files by delegating that work to an
agent. The orchestration **script itself** has no filesystem or Node API access; all I/O happens
inside the subagents it spawns, and only their return values flow back into the script.

MCP caveat: interactively-authenticated MCP servers (e.g. a claude.ai-authenticated server) may be
unavailable to agents in headless/cron execution contexts, even though they work in an interactive
session.

### 4.2 `pipeline(items, stage1, stage2, ...)`

```ts
pipeline(items: any[], ...stages: (prev: any, item: any, index: number) => Promise<any>): Promise<any[]>
```

- Runs each item through all stages **independently, with no barrier between stages** — item A can be
  in stage 3 while item B is still in stage 1. This is the **default** pattern for multi-stage work;
  wall-clock cost is the slowest single-item chain, not the sum of per-stage slowest items.
- Every stage callback receives `(prevResult, originalItem, index)` — later stages can reference the
  original item/index directly without threading extra context through earlier stages' return values.
- If a stage throws for a given item, that item's remaining chain is short-circuited to `null` and
  later stages are skipped for it (other items are unaffected).

### 4.3 `parallel(thunks)`

```ts
parallel(thunks: Array<() => Promise<any>>): Promise<any[]>
```

- Runs all thunks concurrently and is a **hard barrier** — it awaits every thunk before returning.
- A thunk that throws (or whose `agent()` call errors) resolves to `null` in the output array; the
  `parallel()` call itself never rejects. Always `.filter(Boolean)` before consuming results.
- Use only when a later step genuinely needs *all* prior results together (e.g. dedup across a full
  result set, an early-exit on zero findings, or a stage that explicitly compares items to each
  other). "It's conceptually separate" or "cleaner code" are not sufficient justification —
  `pipeline()` already models separate-but-independent stages, and a real barrier wastes the idle
  time of every thunk that finishes early.

### 4.4 `workflow(nameOrRef, args?)`

```ts
workflow(nameOrRef: string | { scriptPath: string }, args?: any): Promise<any>
```

- Runs another workflow as a sub-step of the current one and returns whatever it returns.
- Accepts a saved workflow's name, or `{ scriptPath }` for a script written to disk earlier in the
  same run.
- The child shares the parent's concurrency cap, agent counter, abort signal, and token budget; its
  agents render nested under a `▸ name` group in `/workflows`, and its token spend counts toward the
  parent's `budget.spent()`.
- The `args` param passed here becomes the *child's* `args` global.
- **Nesting is exactly one level** — calling `workflow()` from inside a child workflow throws.
- Throws on an unknown name, an unreadable `scriptPath`, or a child script syntax error; callers can
  catch this to degrade gracefully.

### 4.5 `phase(title)`

```ts
phase(title: string): void
```

Starts a new named phase; subsequent `agent()` calls (without an explicit `opts.phase`) are grouped
under this title in the progress UI. Should match a `meta.phases[].title` string when one exists.

### 4.6 `log(message)`

```ts
log(message: string): void
```

Emits a one-line progress message to the user, shown as a narrator line above the progress tree.
Recommended whenever a workflow silently bounds its own coverage (top-N sampling, no-retry limits,
dropped items) — silent truncation reads to the user as "covered everything" when it didn't.

---

## 5. Globals

### 5.1 `args`

Whatever value was passed as the top-level `args` input, verbatim (`undefined` if not supplied). Used
to parameterize a saved/named workflow (a research question, a target file list, a config object)
without a side-channel file. Pass real JSON arrays/objects in the tool call — a JSON-*string* reaches
the script as a single string and breaks `.filter`/`.map` calls on it.

### 5.2 `budget`

```ts
budget: {
  total: number | null,   // the turn's token target parsed from a user directive like "+500k"; null if unset
  spent(): number,        // output tokens spent this turn across the main loop AND all workflows (shared pool)
  remaining(): number,    // max(0, total - spent()), or Infinity if total is null
}
```

`budget.total` is a **hard ceiling** when set — once `spent()` reaches it, further `agent()` calls
throw. Typical uses:

```js
// Dynamic depth
while (budget.total && budget.remaining() > 50_000) {
  const result = await agent('Find bugs.', { schema: BUGS_SCHEMA })
  bugs.push(...result.bugs)
}

// Static fleet sizing
const FLEET = budget.total ? Math.floor(budget.total / 100_000) : 5
```

Always guard on `budget.total` truthiness before looping on `remaining()` — with no target set,
`remaining()` is `Infinity` and an unguarded loop runs to the 1000-agent lifetime cap.

---

## 6. Return value

Whatever the top-level script `return`s becomes the workflow's result, delivered to the caller via
the completion notification (not inline/streamed — see §8).

---

## 7. Resume semantics

Any run can be resumed with the same `scriptPath` and `resumeFromRunId`. The runtime replays the
script and, for each `agent()` call in order, reuses the cached result if the call's `(prompt, opts)`
exactly match a prior successful call at that position; the first call that differs (or is new) runs
live, and everything after it re-executes normally. Same script + same `args` ⇒ 100% cache hit
(no-op resume). This makes it cheap to fix a bug in stage 3 of a 5-stage pipeline without re-running
stages 1–2.

Diagnosing a resumed/completed run: read `<transcriptDir>/journal.jsonl`, which records each agent's
actual return value — don't assume a cached result was non-empty without checking. If no journal is
available, per-agent `agent-<id>.jsonl` files in the transcript directory are the fallback source for
hand-authoring a continuation script.

---

## 8. Streaming / real-time behavior

- The `Workflow` tool call itself is **fire-and-forget**: it returns a task id immediately, and the
  final result arrives later as a single notification — there is no token-level streaming of the
  final return value back into the conversation.
- The closest thing to "streaming" is `log()` (explicit narrator lines) plus the live progress tree
  visible via `/workflows`, which updates as agents/phases complete. Neither delivers partial
  *results* mid-run to the calling conversation — only progress/status.
- Within a `pipeline()`, items do stream through stages independently (no artificial lockstep), but
  that's an internal scheduling property, not an external streaming interface.

---

## 9. Multimodal input

There is no dedicated multimodal parameter on the `Workflow` tool — `args` is plain JSON (strings,
numbers, arrays, objects); there is no first-class image/audio/file-blob input type documented for
it. Multimodal content reaches a workflow only indirectly: an individual `agent()` can be pointed at
a file path (e.g. an image) and use its own `Read` tool to load and reason over it, exactly as an
interactive agent would. So: **no native multimodal ingestion at the orchestration-script level;
yes, indirectly, per-agent, via file paths and the `Read` tool.**

---

## 10. Denotation of work / progress structure

"Denotation of work" (making the shape of a multi-agent run visible/labelable) is supported via:

- `meta.phases`: declares the named phases shown up front in the permission dialog and progress UI.
- `phase(title)`: starts a phase at runtime; matches by exact title string to a `meta.phases` entry.
- `opts.phase` on `agent()`: assigns a call to a phase explicitly, needed inside `pipeline()`/
  `parallel()` where relying on the last `phase()` call would race across concurrent items.
- `opts.label` on `agent()`: per-call display name override, independent of phase grouping.
- `log(message)`: freeform narrator lines interleaved with the phase/progress tree.
- Nested `workflow()` calls render as a `▸ name` sub-group in the progress view.

There is no separate "work breakdown" object distinct from these — phases + labels + log lines *are*
the denotation mechanism.

---

## 11. Design patterns baked into the tool's guidance

These are conventions the tool description recommends, not separate API surface:

- **Adversarial verify**: N independent subagents each asked to *refute* a finding; keep the finding
  only if a majority fail to refute it.
- **Perspective-diverse verify**: distinct verifiers by lens (correctness/security/perf/repro) rather
  than N identical refuters.
- **Judge panel**: generate N independent attempts from different angles, score with parallel judges,
  synthesize from the winner while grafting good ideas from runners-up.
- **Loop-until-dry**: keep spawning finders until K consecutive rounds surface nothing new (for
  unknown-size discovery problems where a fixed count would miss the tail).
- **Multi-modal sweep** (search-modality diversity, not multimodal-data): parallel agents each
  searching a different way (by container, content, entity, time), blind to each other's results.
- **Completeness critic**: a final agent asks what modality/claim/source was missed; its answer seeds
  the next round.
- Explicit guidance against unjustified `parallel()` barriers — see §4.3.

---

## 12. Quick capability checklist

| Capability | Supported? | Notes |
|---|---|---|
| Deterministic multi-agent orchestration (loops/conditionals/fan-out) | Yes | That's the tool's purpose. |
| Structured JSON output per agent call | Yes | Via `opts.schema` → forced `StructuredOutput` tool call. |
| File read/write | Indirect | Only inside spawned agents (Read/Edit/Write/Bash/MCP); script itself has no FS access. |
| Streaming partial results to the caller | No | Fire-and-forget; single completion notification. Progress-only streaming via `log()`/`/workflows`. |
| Multimodal input (images/audio) at the script/args level | No | `args` is plain JSON. Agents can load files themselves via `Read`. |
| Resume / incremental re-run | Yes | `resumeFromRunId`, prefix-cached on identical `(prompt, opts)`. |
| Token budget enforcement | Yes | `budget.total` is a hard ceiling; `budget.spent()`/`remaining()` for pacing. |
| Nested workflows | Yes, one level | `workflow()`; nesting inside a child throws. |
| Custom subagent types per call | Yes | `opts.agentType`. |
| Isolated parallel file mutation | Yes | `opts.isolation: 'worktree'` (expensive, use sparingly). |
| Non-JS script languages / TypeScript | No | Plain JS only; TS syntax is a parse error. |
| `Date.now()` / `Math.random()` / `new Date()` in-script | No | Would break resumability; pass timestamps via `args` or stamp after return. |
| Item cap per `parallel()`/`pipeline()` call | 4096 | Hard error above, not silent truncation. |
| Total agents per workflow lifetime | 1000 | Runaway-loop backstop. |
| Concurrent agents in flight | `min(16, cores-2)` | Excess calls queue automatically. |
| Requires explicit user opt-in to invoke | Yes | Named trigger, session ultracode flag, explicit request, or named/saved workflow. |
