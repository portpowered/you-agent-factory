# C14 — transport/acp/realclient characterization ledger

Status: BLOCKED at characterization. The supported enabled/required local
baseline cannot be recorded on this host because Node.js is `v22.12.0` and the
pinned `acpx@0.13.0` prerequisite requires `v22.13.0` or later. No test
topology or test assertion was edited while recording this ledger.

This is the task-owned ledger for
`fto-c14-pkg-transport-acp-realclient-001`. It records what was observed at
the pre-edit head and does not claim a valid performance denominator.

## Authority and scope

- Recorded head: `fea2e30a499384182d2fabe7038767e3c2f9c5e5`.
- Package: `github.com/portpowered/infinite-you/tests/functional/transport/acp/realclient`.
- Allowed implementation surface: the real-client test tree and this ledger.
- No operator amendment is present in `prd.json`.
- The pre-edit worktree was clean. `prd.json` and `progress.txt` are ignored
  factory scaffolding and are not PR files.

## Environment and baseline authority

| Fact | Observation |
| --- | --- |
| OS/architecture | Windows `Microsoft Windows NT 10.0.26200.0`, `windows/amd64` |
| Go | `go1.25.0 windows/amd64` |
| Node | `v22.12.0` — unsupported for the pinned client |
| npm | `10.9.0` |
| Pinned dependency | `acpx@0.13.0` from the existing test source |
| Enable gate | `INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1` |
| Required gate | `INFINITE_YOU_REQUIRE_ACPX_REAL_CLIENT=1` |
| Short mode | Not used for a valid baseline; required mode rejects `-short` |
| npm/cache policy | No valid client run reached npm. The source allocates an isolated `t.TempDir` home/cache per happy-path case; no cache/network observation is claimed here. |
| Paid/remote calls | Zero; the deterministic provider was not started. |

The required three-run baseline procedure was therefore not authoritative. No
pre-change median is reported, and the unsupported Node diagnostic is excluded
from `GATE-BASELINE` rather than being treated as a skipped or substituted
sample.

## Discovery

Procedure:

```text
go test ./tests/functional/transport/acp/realclient -list .
```

Observed exit status `0`, package output `0.073s`, and exactly these three
top-level identities:

1. `TestRunBoundedCommandTerminatesScenarioDescendants`
2. `TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit`
3. `TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt`

The focused process procedure was also run before any test edit:

```text
go test ./tests/functional/transport/acp/realclient -run '^TestRunBoundedCommand' -count=1 -timeout=5m -v
```

It exited `0`; both helper cases passed (`1.32s` and `0.33s` test walls,
package `1.672s`, outer wall `2205ms`). This proves the current Windows
process-tree helper witnesses only; it is not ACPX evidence or the package
baseline.

## Prerequisite matrix

These commands ran against the unchanged head. Package walls include the two
real helper witnesses; the ACPX case stopped at prerequisite admission where
noted.

| Case | Procedure/gates | Result | Evidence disposition |
| --- | --- | --- | --- |
| RC-PREREQ-001 | `go test ./tests/functional/transport/acp/realclient -short -count=1 -timeout=5m -v` with enable omitted and requirement omitted | Exit `0`; both helper cases passed, the real-client case logged `real acpx client evidence builds the CLI and installs a pinned npm package` and skipped; package wall `2482ms`. | PASS for explicit optional short-mode skip; no real-client evidence is claimed. |
| RC-PREREQ-002 | `go test ... -count=1 -timeout=5m` with enable omitted | Exit `0`; real-client case skips with the existing enable message; package wall `2281ms`. | PASS for optional absence; no false real-client pass. |
| RC-PREREQ-003 | Enable `=1`, requirement omitted, Node `v22.12.0` | Exit `0`; real-client case skips with `pinned acpx@0.13.0 requires Node.js 22.13.0 or later`; package wall `2375ms`. | PASS for optional unsupported-runtime handling; no evidence claim. |
| RC-PREREQ-004 | Enable `=1`, require `=1`, `-short` | Exit `1`; `required lane must run without -short`; package wall `3756ms`. | PASS for fail-closed required short handling. |
| RC-PREREQ-005 | Require `=1`, enable omitted | Exit `1`; `required lane did not enable the pinned client`; package wall `2501ms`. | PASS for fail-closed missing-enable handling. |
| RC-PREREQ-006 | Enable `=1`, require `=1`, no `-short`, Node `v22.12.0` | Exit `1`; `Node.js 22.13.0 or later is required`; the ACPX case stopped before build/client startup; package wall `2738ms`. | PASS for safe required unsupported-runtime classification, but a structured blocker for characterization. |

The first discovery attempt used the wrong PowerShell argument expansion and
returned `no Go files` for `.`; it was corrected immediately with the exact
command above. It is not evidence for any gate.

## Executable assertion inventory

The inventory below is source-based and retained for parity checks after the
supported baseline becomes available. It is not a claim that the blocked
ACPX success path passed.

### TestRunBoundedCommandTerminatesScenarioDescendants

- Starts a real helper parent and descendant through `os.Args[0]`.
- Uses the one-second `runBoundedCommandWithTimeout` bound.
- Requires the safe `timeout` classification.
- Terminates the platform-owned process tree (Unix process group or Windows
  Job Object) and polls the recorded descendant PID until inactive.
- Owns its temporary PID file and test directories through `t.TempDir`.

### TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit

- Starts a real helper parent and descendant through `os.Args[0]`.
- Requires the safe `non-zero exit` classification.
- Terminates the owned process tree after the parent exits non-zero and polls
  the recorded descendant PID until inactive.
- Owns its temporary PID file and test directories through `t.TempDir`.

### TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt

The happy-path assertions that remain active in the source are:

- prerequisite admission: no `-short`, explicit enable/require semantics, and
  Node `22.13.0+` before startup;
- current revision: `git rev-parse HEAD` returns a full safe 40-character
  hexadecimal revision;
- current production executable: `go build -o <temporary binary> ./cmd/factory`;
- structured client configuration: `.acpxrc.json` selects
  `you-real-client` with argv `[<current binary>, server, acp]`;
- exact pinned client version: `acpx --version` is `0.13.0`;
- fresh session: `sessions new` reports `session_ensured`, `created=true`, a
  client record identity, and an ACP session identity;
- negotiated state: the persisted record has a protocol version and target
  `factory:@you/factory-builder`;
- one ephemeral prompt: the fixed input is 16 `x` characters and is not
  retained in evidence;
- ACP result stream: JSON is parseable, no protocol error is present, a
  non-empty assistant message is observed, at least one Worker `tool_call`
  and `tool_call_update` are observed, and exactly one `end_turn` is observed;
- provider boundary: every marker is the deterministic `codex` provider and
  the exact invocation assertion is two (classify then answer);
- close identity: `sessions close` reports `session_closed` and a non-empty
  ACP session identity;
- cleanup: partial-session `t.Cleanup` retries close, the platform process
  tree is terminated by the bounded runner, and the disposable ACPX queues
  directory has zero entries;
- sanitized evidence: only revision, version, protocol/session/target
  classifications, provider name/count, cleanup, and outcome are emitted.

Two pre-existing source discrepancies are recorded for the optimization story
to resolve without weakening the runtime assertion: the source comment says
the normal direct-start count is `2 + 2 + 6 = 10` while the Node prerequisite
probe adds a seventh happy-path parent start, and `emitEvidence` currently
serializes/logs `providerInvocations=1` even though the executable assertion
requires exactly two. Neither discrepancy was changed in this
characterization-only iteration.

## Process, phase, wait, and cleanup profile

### Direct and descendant starts

The corrected pre-edit accounting is:

| Owner/path | Direct parent starts | Descendant/process ownership | Wait/cleanup witness |
| --- | ---: | --- | --- |
| Timeout helper | 1 bounded parent + 1 helper descendant | Real `os.Args[0]`; process group/Job Object | one-second timeout, tree termination, recorded PID inactive |
| Non-zero helper | 1 bounded parent + 1 helper descendant | Real `os.Args[0]`; process group/Job Object | non-zero classification, tree termination, recorded PID inactive |
| Happy path | 7: Node probe, Git revision, Go build, ACPX version, session creation, prompt, close | `npx`/ACPX starts the configured current binary; the binary starts the deterministic provider. These descendants were not reached on this host. | one-minute bounded phase waits; prompt also carries ACPX `--timeout 45`; close plus `t.Cleanup` and zero queue owners |

Thus the prior `10` normal-path direct-start figure is `2 + 2 + 6` before
counting the Node probe; the current pre-edit total is `11` external starts
when the prerequisite probe is included. Internal starts performed by Go,
`npx`, ACPX, or the built executable are not counted as direct test-owned
starts, but are retained as descendant edges to profile in the supported run.

### Wait and timeout topology

- `node --version` uses `Cmd.Output()` with no explicit test timeout and must
  complete before build/client startup.
- `git rev-parse`, `go build`, and all four `npx` phases use the one-minute
  `runBoundedCommand` default.
- The prompt passes ACPX `--timeout 45` inside the one-minute outer bound.
- The bounded runner waits on `Cmd.Wait()` in a goroutine; completion returns
  output, while timeout/non-zero paths terminate the owned tree and then wait
  for process completion.
- Process-tree helper tests use a one-second bound and a one-second
  recorded-PID deadline with a 10ms OS-state poll ticker. The descendant's
  `time.Hour` loop is a held process fixture, not timeout padding or a test
  sleep.
- No new sleep, broader timeout, queue owner, provider process, or ACPX child
  was introduced by this iteration.

### Phase and cache observations

No supported happy-path phase wall, descendant PID, npm download/cache result,
ACP session record, provider marker, or queue teardown result is available:
the required Node gate failed in `0.04s` inside the `2738ms` package wall.
The valid three-sample baseline and full phase/process profile remain blocked
until a supported Node runtime is available. Existing C06 evidence reports
the old `10`-start accounting and an explicit Node `v22.12.0` skip/fail-close;
it is historical context, not a valid C14 denominator.

## Matrix disposition and smallest safe next step

| Matrix rows | Current disposition |
| --- | --- |
| RC-PREREQ-001 through RC-PREREQ-006 | Observed/reconciled above; optional absence skips and required absence fails closed before startup. |
| RC-PROC-001 and RC-PROC-002 | Exercised by the focused local-real Windows helper run; both process-tree cleanup witnesses passed. |
| RC-CLIENT-001 | Retained in source, not executable on the unsupported runtime; no success evidence claimed. |
| RC-CLIENT-002 and RC-CLIENT-003 | Retained by bounded runner/parser/assertion and registered cleanup ownership; direct malformed/output/recovery injection remains with the unchanged lower ACP gates and later real-client run. |
| RC-CLIENT-004 and RC-CLIENT-005 | Later repeat/race gates; not claimed here. |
| RC-BOUNDARY-001 | Retained source assertion for the 16-character ephemeral prompt, one prompt result, and two provider calls; not claimed exercised here. |
| RC-AUTH-001 | Not applicable by contract; no credential-bearing edge was added. |
| RC-CAPACITY-001 | Not applicable by contract; no load claim is made. |

The smallest safe optimization candidate is deferred until the valid profile:
measure whether the four separate ACPX phase invocations are the dominant
intrinsic cost, then consider only a test-local consolidation that preserves
the pinned package, fresh session state, current binary, real stdio/process
edges, each phase assertion, and cleanup. The helper process cases are not a
candidate for consolidation because their independent timeout and non-zero
process-tree witnesses are the behavior under test. The two evidence/comment
discrepancies above must also be corrected in the preservation story, not
used as a reason to weaken the exact two-call assertion.

## Structured blocker

- Failed gate: `GATE-BASELINE` and the supported happy-path portion of
  `GATE-PROFILE`.
- Reproduction: on the unchanged recorded head, run with
  `INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1` and
  `INFINITE_YOU_REQUIRE_ACPX_REAL_CLIENT=1`; the package exits `1` with
  `real ACP evidence prerequisite failed: Node.js 22.13.0 or later is required`.
- Impact: no valid three-run pre-edit median or real ACPX phase/process/cache
  profile exists, so optimization would have no supported denominator and
  could silently replace the real edge.
- Safe work completed: discovery, prerequisite matrix, helper process-tree
  witnesses, source assertion inventory, direct-start correction, and
  cleanup/wait ownership record.
- Narrowest delta requested: provide a supported Node.js `22.13.0+` runtime
  for this unchanged checkout, then rerun the exact three isolated required
  package samples and finish the missing real-client profile before any test
  topology edit. Do not substitute, skip, or restructure the client.
