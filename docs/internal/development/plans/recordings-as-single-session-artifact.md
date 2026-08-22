# Recordings As The Single Session Artifact

---
author: operator
last modified: 2026, august, 22
doc-id: PLAN-REC-001
status: proposed
---

# problem statement

We keep two session artifacts for the same run. The weaker one has no size bound and a non-atomic write, and it took the server down by growing to exactly 1 GiB and truncating mid-string. The stronger one is atomic and complete, but it is stored under a bespoke date layout matching neither logs nor metrics, and it is rewritten in full every 250 ms.

## customer ask

Remove durable sessions and replace them with recordings. Write recordings in `yyyy/mm/dd/` format the way metrics and logs already are, with the filename being only `<session-id>.jsonl`.

## solution

Promote the dated-path helper that logs and metrics already use into a shared platform
surface, key the recording filename on the session's UUID and nothing else, through a
collision-safe reservation, change the recording from a whole-file rewrite to an
append-only JSONL stream, bound it with the rotation machinery already in the binary, and
then retire `runtimepersist` behind those read surfaces.

# original document

The 41-agent adversarial evaluation of v0.0.8 ("The Gigabyte Session File", 281 findings,
40 blockers, 22 Aug 2026), verified line by line against this tree at `74e1d6341`.

## Evidence — measured in this tree, not quoted from the report

| | Recording | Durable session |
|---|---|---|
| Root | `~/.you-agent-factory/recordings/` (global, documented) | `<cwd>/.you-agent-factory/durable-sessions/` (cwd-relative, undocumented) |
| Path | `YYYY-MM/YYYY-MM-DD/factory-session-<token>-<HHMMSS>-<uuid>.json` | `<session-id>.json` |
| Built at | `recordings/internal/services/recording_lifecycle/internal/service/live_recording_target.go:51-58` | `execution/runtimepersist/store.go:88` |
| Write | `platform/replay.Local.WriteFile` — `MkdirAll` → `CreateTemp` → `Sync` → `Close` → `Rename`, with a Windows replace-retry loop (`storage.go:37-88`) | `files.WriteFile(path, encoded, 0o600)` — in-place overwrite, no temp, no rename (`store.go:112`) |
| Content | 15 ordered events, factory definition and hashes, full payloads | 2 + 3 records, output text only |
| Terminal state | recorded on `Finalize` | stuck at `RUNNING` / `FinishedAt: null` / `NOT_READY` after a clean exit 0 |
| Survives a hard kill | yes (flushed every 250 ms) | no |
| Read back by `you run` | yes, for replay | **never** |
| Size bound | none | none |

The comparison settles the question the customer ask implies. Durable sessions are not a
smaller, cheaper artifact that recordings duplicate. They are a **strictly weaker copy**
of an artifact we already write, on the only one of the two write paths that is unsafe.

### The outage mechanism

`~default.json` reached exactly 2^30 bytes and cut mid-string, so the server could no
longer parse it and could no longer start. Growth was quadratic in retry count
(`73.45k² + 1362.83k + 3634.09`, R² = 1.000000) because every snapshot re-embeds the
entire `token.history.failure_log`, which has `append()` and no truncation anywhere in the
tree. The write is a plain in-place overwrite, so a process death mid-write leaves a file
that is neither the old snapshot nor the new one.

Three independent guards were all absent on this one path, and all three exist elsewhere
in the same binary:

- **Atomicity** — `platform/replay.Local.WriteFile` does temp-write-sync-rename. Recordings
  get it. `runtimepersist` does not.
- **Rotation and caps** — `platform/rollingfile` carries `MaxSizeMB`, `MaxBackups`,
  `MaxAgeDays`, `rotateForReservation()` and backup pruning
  (`rollingfile.go:37,337,495-508`), wired to logs, metrics and wire transcripts. Neither
  session artifact is wired to it.
- **Collision safety** — `platform/runtimeartifact.Reserver` opens with
  `O_CREATE|O_EXCL` and walks a bounded collision index (`reserve.go:42-58`). Recordings
  avoid collisions only by accident, via the timestamp and UUID in the filename.
  `durableSessionIDPattern` (`store.go:85`) explicitly **accepts `~default` as a storage
  key**, so two sequential runs from one directory overwrite each other with no warning —
  reproduced in the evaluation.

### What nobody reads

`grep` for `WalkDir`, `filepath.Glob` and `ReadDir` across `pkg/services/recordings`
returns nothing outside tests. No production code enumerates the recordings root; every
reader is handed an explicit path. And the `2006-01`/`2006-01-02` layout is constructed in
**exactly one place** (`live_recording_target.go:52`) with no consumer parsing it back.

That is both the reason the layout was free to diverge from logs and metrics, and the
reason correcting it is contained. It is also why REC-5 in this plan adds the enumeration
surface: a dated layout with no reader is a convention, not a capability.

## Design

### D1. One dated layout, one implementation

`calendarDatedDir` — `filepath.Join(rootDir, at.Format("2006"), at.Format("01"),
at.Format("02"))` — already exists at
`pkg/platform/internal/runtimeartifact/paths.go:58` and is exactly what the customer ask
describes. It is unexported, and it lives under `pkg/platform/internal/`, so
`pkg/services/recordings` **cannot import it**.

Copying the four-line function into recordings is the wrong fix: it would make three
implementations of one convention, which is the same drift class this plan exists to
close.

Instead, extend the public wrapper that already sits over it.
`pkg/platform/runtimeartifact` exports `Reserver`, whose `Reserve(root, at, kind, suffix)`
already composes the dated directory with an `O_CREATE|O_EXCL` reservation and a bounded
collision walk. It builds the *filename* from a timestamp, which is not what we want here.
Add a sibling:

```
ReserveNamed(root string, at time.Time, name, ext string) (string, error)
    → root/2006/01/02/<name><ext>, reserved O_EXCL,
      falling to <name>-2<ext>, <name>-3<ext>, … on collision,
      bounded by the existing maxPathCollisions
```

Recordings consume `ReserveNamed`. Logs, metrics and recordings then share one dated-path
implementation and one collision policy.

### D2. The filename is the session UUID, and nothing else

`<session-id>.jsonl` where the session id is **solely the UUID**. No `factory-session-`
prefix, no `HHMMSS`, no `dur-sess-` prefix, no registry alias, no extra identity appended.
The name of the file is the identity of the session.

The mechanism for this already exists and is already correct — it is simply not the one
the recording target reads.

- `livesession.EnsureRuntimeID` (`livesession/session.go:116-134`) **already mints a UUID**
  for any session still carrying the `~default` registry alias, at construction, and fails
  loudly if the generator returns empty.
- `livesession.CanonicalID` (`session.go:99-107`) **already returns that UUID** in
  preference to the alias. Its own doc comment says it: *"Default-route sessions keep the
  ~default registry alias but expose a UUID runtime identity to clients."*
- `livesession.IsUUIDID` (`session.go:109`) already exists as the predicate.

So a UUID is available on the same object at the moment the recording target is planned.
The alias reaches the filename because the recording path reads a different field:

- `recordings/transports/cli/adapter.go:76-78` — when `ReportedSessionID` is empty it
  substitutes the **literal string** `"~default"`
  (`DefaultReportedFactorySessionID`, line 15).
- `pkg/transports/cli/run/run.go:684` passes `defaultFactorySessionID` — the alias — in.
- `live_recording_target.go:61` then string-substitutes that alias into the filename.

That is the whole defect. A UUID identity was minted, a canonical accessor for it exists,
and the transport hands the recording the registry alias instead.

**Rules:**

1. **`~default` is a registry alias, never a stored identity.** It may appear in a request,
   a URL, or a flag; it may not appear in a filename, a storage key, or a recording header.
   The recording target takes `CanonicalID`, not `ReportedSessionID`.
2. **A non-UUID identity is a planning error, not a fallback.** `IsUUIDID` gates it. If the
   identity is not a UUID, the target is refused with a diagnostic naming the identity —
   we do not quietly write under an alias, and we do not mint a second one behind the
   caller's back at a layer that has no business owning identity.
3. **The identity vocabulary collapses to one shape.** `durableSessionIDPattern`
   (`runtimepersist/store.go:85`) currently accepts three — `dur-sess-[a-f0-9]{32}`,
   `~default`, and a UUID. Two of those disappear with the package at REC-7; the point of
   naming it here is that the *reason* it accepts three is that no layer ever decided which
   one was canonical. This plan decides: the UUID.
4. **Collision safety stays as a guard, not as the mechanism.** Because a UUID is unique
   per session, same-day collisions should not occur at all — which is exactly why
   `ReserveNamed`'s `O_EXCL` open matters. A collision now means an identity was reused,
   and that must surface as an error. It must never be absorbed by an overwrite, which is
   the durable-sessions defect, nor hidden by a timestamp in the name, which is how
   recordings have been avoiding the question.

One thing gets simpler for free. `live_recording_target.go:60-64` currently produces two
paths — a `ServicePath` containing a placeholder token and a `ReportedPath` derived from it
by `strings.ReplaceAll` of that token with the session id. Once the filename is the UUID,
the two paths are the same path and the substitution is deleted.

### D3. JSONL, and why the format change is the actual fix

The requested extension change is not cosmetic. `.jsonl` is the fix for a defect
recordings share with durable sessions.

`Recorder.Flush` calls `MarshalArtifact`, which is `json.MarshalIndent` over the **entire**
artifact (`replay/artifact.go:143-160`), on a 250 ms dirty-flush loop
(`recorder.go:19,186,244`). File size is O(events), but **bytes written is O(events²)**.
Recordings survived where durable sessions did not because the recording does not re-embed
an ever-growing failure log inside every record — it hit the linear term, not the quadratic
one. The write amplification is real either way.

Target format, `agent-factory.replay.v2`:

- **Line 1** — header record: schema version, `recordedAt`, the session UUID,
  factory identity and hashes.
- **Lines 2..n** — one event per line, in sequence order, appended as they are recorded.
- **Final line** — terminal record: `finishedAt`, terminal state, flush diagnostics.

Flush appends only the bytes recorded since the last flush and syncs. There is no rename,
because there is nothing to replace. Atomicity moves from whole-file replacement to
per-line append, which is the property this format actually offers:

> A truncated `.json` artifact costs the whole file. A truncated `.jsonl` artifact costs
> the last line.

That is the difference between "the server cannot start" and "the last event is missing" —
and the 1 GiB file that caused the outage would have been fully readable up to its last
complete line.

The reader accepts both versions: `v1` (single JSON document, old layout) stays readable
forever; nothing writes it after REC-4.

### D4. Bounds, from machinery already in the binary

Two separate bounds, because the two growth terms have different causes.

- **Format-level (quadratic).** Removed by D3. Append-only means no record is rewritten,
  so no field can be re-embedded per flush.
- **Content-level (linear).** `token.history.failure_log` is append-only with no
  truncation anywhere. Cap it: retain a head and a tail with an explicit dropped-count in
  between. A truncated history that says how much it dropped is honest; an unbounded one
  is a time bomb with a linear fuse.
- **File-level.** Wire recordings to `platform/rollingfile`'s existing `MaxSizeMB` /
  `MaxBackups` / `MaxAgeDays`. It is the same machinery already governing logs, metrics
  and wire transcripts, and no session artifact is currently attached to it.

### D5. Retiring durable sessions

Everything durable sessions carry is a proper subset of the recording — session identity,
state, start/finish, per-work terminal state, and output text. Retirement is therefore not
a feature deletion; it is removing the second, weaker implementation of an artifact we
already write.

The order matters. `runtimepersist` is imported by 16 files, 11 of them tests, plus
`cmd/durableruntimeconstructioncheck`, `pkg/services/factory_sessions/wire/application_graph.go`
and `cmd/pkgboundarycheck/converged_boundaries_test.go`. The read surfaces move first, the
writes go quiet second, the package dies last.

**One behavior change must be decided, not absorbed.** Durable sessions are
**cwd-relative**; recordings are **global**. Retiring one into the other moves a
per-project artifact into a shared root. The recommendation is to keep the global root and
rely on session identity for separation, because session ids are unique across projects
and the global root is the documented one — but this is a visible change for anyone
running two projects side by side, and it belongs in the release note rather than in a
diff nobody reads.

### D6. Migration and compatibility

- **No conversion of historical recordings.** Old paths stay readable under the v1 reader.
  A converter is a non-goal — it is work proportional to history, for a read surface that
  has to support both versions anyway.
- **Existing `durable-sessions/*.json` are left on disk**, unread after REC-7, and
  documented as safe to delete. We do not delete a user's files as a side effect of a
  refactor.
- **The corrupt artifact in the wild is a diagnostic, not a silent skip.** A 1 GiB
  unparseable file must produce a message naming the path and the fact that it is
  oversized, not a generic parse error. That is REC-7's second half.

## Delivery — eight independently mergeable steps

**REC-0. Stop the bleeding.** Cap `token.history.failure_log`, and make the
`runtimepersist` write atomic by routing it through the same temp-sync-rename shape
`platform/replay` already uses. Lands first and alone. The atomic write becomes moot at
REC-7, and is still worth its thirty lines today, because the retirement is seven merges
away and the outage is not hypothetical.

**REC-1. Characterization.** Pin today's path, filename, format and flush behavior —
including an assertion that **two same-day default-session runs produce two distinct
files**. That assertion is the regression guard for D2, and it must be written before the
filename changes, not after.

**REC-2. `ReserveNamed`.** Add it to `pkg/platform/runtimeartifact`, delegating to the
single `calendarDatedDir`. No recordings change in this step. Mergeable on its own value:
it makes the dated layout a shared surface rather than a private one.

**REC-3. Adopt the layout and the filename.** Recordings move to
`YYYY/MM/DD/<session-uuid>.json` via `ReserveNamed`, reading `livesession.CanonicalID`
rather than `ReportedSessionID`; `ServicePath` and
`ReportedPath` converge and the token substitution is deleted. Still `.json`, still
whole-file. Separating the *path* change from the *format* change is deliberate — a
bisect can then tell them apart.

**REC-4. JSONL v2.** Append-only writer, dual-version reader, extension becomes `.jsonl`.
This is the format change, alone, with the write-amplification measurement as its
acceptance evidence: bytes written for an N-event run must be linear in N.

**REC-5. Enumeration, through `you session list`.** There is no `you recording list`.
A recording *is* a session's history, so a second command listing them would be a second
vocabulary for one concept — the same duplication this plan exists to remove, reintroduced
at the read surface. `you session list` today enumerates the live registry; this step
extends it to the recorded history under the dated layout, so one command answers "what
sessions are there" for both live and finished runs, keyed by the same UUID in both halves.
Live-only and history-only views are flags on that command, not commands of their own.
This is also the read surface REC-6 needs, and the first time anything has enumerated
recordings at all.

**REC-6. Back the durable read surfaces with recordings.** Session inspection reads the
recording. Durable writes go behind a flag defaulting to off. No package is deleted yet,
so this step is revertible.

**REC-7. Delete.** Remove `runtimepersist`, `DirForProjectRoot`, `durableSessionIDPattern`,
`cmd/durableruntimeconstructioncheck`, and the wire and boundary-check entries. Add the
oversized-artifact diagnostic. `make lint` and `cmd/pkgboundarycheck` are the gates here.

## Non-goals

- **No change to what a recording contains**, beyond header and terminal framing. Content
  changes are a separate plan, and mixing them into a format migration would make the diff
  unreviewable.
- **No conversion of historical artifacts**, and no deletion of files already on disk.
- **No new export or remote destination** for recordings.
- **No replay-semantics change.** Replay reads the same events in the same order; only the
  bytes on disk are reshaped.
- **No secret redaction here.** Recordings storing worker secrets verbatim is a real
  finding, and it is owned by `adversarial-findings-extensions.md` story EXT-SEC-2 so the
  redaction policy is reviewed on its own merits rather than as a rider on a path change.

## Verification

`make test`, plus targeted tests near `pkg/services/recordings/`,
`pkg/platform/runtimeartifact/`, and
`pkg/services/factory_sessions/internal/execution/`. REC-3 and REC-4 warrant
`make test-functional`, since replay and session lifecycle both cross the changed surface.
REC-7 requires `make lint` for the boundary and construction checks. REC-6 is what makes
integration test I6 (resume) passable, since resume then reads the recording rather than
the durable snapshot that does not survive a hard kill; that test is REC-6's acceptance
evidence. REC-3, REC-4 and REC-5 add no integration tests of their own — the tier is
capped at eight.

## Delivery loop

Implementation finishes when its final head is pushed, the PR is open, CI has started,
and blocking review feedback is addressed. Review owns terminal-and-passing CI, conflict
resolution, and merge. CI-run evidence goes in a PR comment, never a commit.
