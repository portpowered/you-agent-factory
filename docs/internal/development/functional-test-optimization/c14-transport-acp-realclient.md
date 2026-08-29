# C14 — transport/acp/realclient characterization ledger

Status: CHARACTERIZED. Three supported enabled/required local baseline runs
passed after using an isolated official Node.js `v22.13.0` runtime. No test
topology or test assertion was edited while recording this ledger.

This is the task-owned ledger for
`fto-c14-pkg-transport-acp-realclient-001`. It records the pre-edit
denominator and the supported discovery/profile observations.

## Authority and scope

- Recorded head for the pre-change denominator: `030c045f4cf617a9fa135092a5cda6e6052ac8f2`.
- Package: `github.com/portpowered/infinite-you/tests/functional/transport/acp/realclient`.
- Allowed implementation surface: the real-client test tree and this ledger.
- No operator amendment is present in `prd.json`.
- The pre-edit worktree was clean. `prd.json` and `progress.txt` are ignored
  factory scaffolding and are not PR files.
- A gated timing trace and external ancestry capture were used for one
  diagnostic profile run only; the trace hook was removed after profiling and
  is not part of the implementation.

## Environment and baseline authority

| Fact | Observation |
| --- | --- |
| OS/architecture | Windows `Microsoft Windows NT 10.0.26200.0`, `windows/amd64` |
| Go | `go1.25.0 windows/amd64` |
| Node | `v22.13.0` — isolated official Windows runtime, prepended only for these runs |
| npm | `10.9.2` |
| Pinned dependency | `acpx@0.13.0` from the existing test source |
| Enable gate | `INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1` |
| Required gate | `INFINITE_YOU_REQUIRE_ACPX_REAL_CLIENT=1` |
| Short mode | Not used for a valid baseline; required mode rejects `-short` |
| npm/cache policy | Each happy-path case allocated a fresh `t.TempDir` home and `npm_config_cache`; the first `npx --yes --package acpx@0.13.0 acpx` phase acquired the public package and later phases reused that run's isolated cache. No request payloads or credentials were retained. |
| Paid/remote calls | Zero; the deterministic provider was not started. |

The earlier host-installed Node `v22.12.0` diagnostic remains unsupported
evidence and is excluded from `GATE-BASELINE`. The isolated `v22.13.0` runtime
is the supported denominator; it is the same pinned client and test path, not a
mock or a client substitution.

## Valid pre-change baseline

Procedure, repeated as three separate processes before the characterization
comment edit:

```text
go test ./tests/functional/transport/acp/realclient/... -count=1
```

Environment for each run: `INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1`,
`INFINITE_YOU_REQUIRE_ACPX_REAL_CLIENT=1`, no `-short`, Node `v22.13.0` and
npm `10.9.2` from the isolated official runtime, and the test's own fresh
temporary npm cache/evidence directory. A PowerShell stopwatch captured the
outer process wall; the Go package wall is retained as a cross-check.

| Run | Outer wall | Go package wall | Result |
| --- | ---: | ---: | --- |
| 1 | `38.932s` | `38.062s` | exit `0`, all three tests passed |
| 2 | `44.615s` | `43.551s` | exit `0`, all three tests passed |
| 3 | `49.555s` | `48.535s` | exit `0`, all three tests passed |

`GATE-BASELINE` denominator: outer median `44.615s` (package median
`43.551s`). The spread is retained as observed host contention; no sample was
discarded and no unsupported or skipped run was substituted.

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

The source direct-start comment was corrected after the profile to describe the
seven happy-path starts. One evidence discrepancy remains for the optimization
story to resolve without weakening the runtime assertion: `emitEvidence`
currently serializes/logs `providerInvocations=1` even though the executable
assertion requires exactly two. The baseline's sanitized evidence preserves
that mismatch as an observed pre-change finding.

## Process, phase, wait, and cleanup profile

### Direct and descendant starts

The source-level pre-edit accounting is nine test-owned direct starts: two
helper parents plus seven happy-path commands. The seven happy-path starts are
the Node prerequisite, Git revision, Go build, ACPX version, session creation,
prompt, and close. The profile tracer matched all nine starts to their named
phases; the helper descendants are started by the real `os.Args[0]` test
binary, not by a substitute process.

| Owner/path | Direct parent starts | Descendant/process ownership | Wait/cleanup witness |
| --- | ---: | --- | --- |
| Timeout helper | 1 bounded parent + 1 helper descendant | Windows Job Object with kill-on-close; recorded descendant PID | one-second timeout, tree termination, PID inactivity poll |
| Non-zero helper | 1 bounded parent + 1 helper descendant | Windows Job Object with kill-on-close; recorded descendant PID | non-zero classification, tree termination, PID inactivity poll |
| Happy path | 7: Node probe, Git revision, Go build, ACPX version, session creation, prompt, close | Four `npx` launches resolve through Windows command hosts to Node/ACPX; session and prompt launch the configured current `you.exe`; the prompt launches the deterministic provider twice. | one-minute bounded phase waits; prompt also carries ACPX `--timeout 45`; close plus `t.Cleanup` and zero queue owners |

An external Windows ancestry capture of one supported profile run observed 46
process instances below the test invocation. It retained only process names,
parent relationships, and relative observation order; no command-line payload,
prompt, response, environment value, or host path was retained.

| Observed process name | Instances | Profile interpretation |
| --- | ---: | --- |
| `node.exe` | 12 | npm/ACPX and provider-side Node descendants |
| `cmd.exe` | 10 | `npx.cmd`, provider command, and Windows command-host edges |
| `git.exe` | 8 | Git's process chain for revision/build metadata |
| `go.exe` | 3 | test/build tool descendants |
| `link.exe` | 2 | Go linker descendants |
| `you.exe` | 2 | configured production executable descendants |
| `realclient.test.exe` | 3 | package test process and helper process edge observed by the sampler |
| `tasklist.exe` | 2 | Windows PID-state probes in cleanup witnesses |
| `esbuild.exe` | 1 | ACPX/npm dependency descendant |
| `conhost.exe` | 3 | Windows console descendants |

The profile run's direct phase trace is authoritative for the nine bounded
test-owned starts. The ancestry sampler is supplementary for short-lived
descendants (some can exit between 50ms snapshots); it confirms the external
process families and ownership edges without turning a sampled count into a
false exact-start claim.

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

One supported diagnostic profile run (instrumented only with a temporary gated
trace, package wall `32.564s`, outer ancestry-capture wall `34.066s`) produced
the following phase walls. The trace recorded no arguments or output payloads.

| Phase | Bound | Observed wall | Outcome |
| --- | ---: | ---: | --- |
| Node prerequisite probe | none | `51.8ms` | supported version |
| `read-revision` / Git | `60s` | `80.4ms` | success |
| `build-current-you` / Go | `60s` | `7.388s` | success |
| `verify-acpx-version` / npx | `60s` | `9.063s` | success |
| `create-session` / npx | `60s` | `3.460s` | success |
| `complete-prompt` / npx | `60s` plus ACPX `45s` | `5.088s` | success |
| `close-session` / npx | `60s` | `4.479s` | success |
| Timeout helper | `1s` | `1.012s` | expected timeout, cleanup passed |
| Non-zero helper | `1s` | `48.4ms` | expected non-zero, cleanup passed |

The four separate ACPX phases account for `22.090s` in this profile and each
starts a fresh `npx` process while retaining the same scenario-local npm
cache. The first version phase includes public npm acquisition; later phases
reuse the same isolated cache. This identifies phase/process startup and
package acquisition as the smallest safe optimization candidate. The build
and helper process witnesses are retained because they prove the current
binary and process-tree failure semantics.

## Matrix disposition and smallest safe next step

| Matrix rows | Current disposition |
| --- | --- |
| RC-PREREQ-001 through RC-PREREQ-006 | Observed/reconciled above; optional absence skips and required absence fails closed before startup. The original Node `v22.12.0` required-lane failure remains unsupported evidence; the `v22.13.0` enabled/required runs passed. |
| RC-PROC-001 and RC-PROC-002 | Exercised by the focused local-real Windows helper run; both process-tree cleanup witnesses passed. |
| RC-CLIENT-001 | Exercised in the supported profile and all three baseline runs through pinned ACPX, the current built binary, real stdio, and the deterministic provider; sanitized evidence was emitted. |
| RC-CLIENT-002 and RC-CLIENT-003 | Retained by bounded runner/parser/assertion and registered cleanup ownership; direct malformed/output/recovery injection remains with the unchanged lower ACP gates and later real-client run. |
| RC-CLIENT-004 and RC-CLIENT-005 | Later repeat/race gates; not claimed here. |
| RC-BOUNDARY-001 | Supported happy path observed one 16-character ephemeral prompt, one prompt result, and exactly two provider markers; the sanitized evidence field still reports one and is a story-002 correction target. |
| RC-AUTH-001 | Not applicable by contract; no credential-bearing edge was added. |
| RC-CAPACITY-001 | Not applicable by contract; no load claim is made. |

The smallest safe optimization candidate is deferred until the valid profile:
measure whether the four separate ACPX phase invocations are the dominant
intrinsic cost, then consider only a test-local consolidation that preserves
the pinned package, fresh session state, current binary, real stdio/process
edges, each phase assertion, and cleanup. The helper process cases are not a
candidate for consolidation because their independent timeout and non-zero
process-tree witnesses are the behavior under test. The evidence-count
discrepancy must be corrected in the preservation story, not used as a reason
to weaken the exact two-call assertion.

## Characterization result

`GATE-BASELINE`, `GATE-PROFILE`, `GATE-DISCOVERY`, and `GATE-PREREQ` are
satisfied for story 001. The initial host-installed Node `v22.12.0` result was
a real, safe prerequisite diagnostic but did not satisfy the denominator. The
narrow environmental remedy was an official, isolated Node `v22.13.0`
runtime; the three required runs then passed without client substitution,
skipping, or topology restructuring.

Remaining gates are optimization parity/performance, repeat/race, clean-room
validation, and current-head PR evidence owned by stories 002 and 003.
