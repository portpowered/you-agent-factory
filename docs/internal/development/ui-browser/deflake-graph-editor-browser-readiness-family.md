# Graph-editor browser readiness deflake ledger

Date: 2026-08-30 (America/Los_Angeles)

## Scope and decision

The characterization section covers story
`deflake-graph-editor-browser-readiness-family-001` and the nonreproducing
decision below completes story `deflake-graph-editor-browser-readiness-family-002`.
It characterizes the two confirmed readiness sites in
`ui/integration/factory-graph-editor.integration.test.mjs` before any
readiness correction. The only code change in these stories is failure-only,
site-labelled diagnostics and first-attempt accounting; no application source,
timeout, retry, sleep, skip, quarantine, assertion, or browser-test inventory
change was made.

The result is nonreproducing in both measured sets. No root cause is proven for
either site, and no shared cause is claimed. Story 002 is therefore constrained
to retaining and using the next-occurrence diagnostics unless a later direct
trace establishes a mechanism.

## Preflight contention and artifact

- Worktree was clean at preflight on branch
  `deflake-graph-editor-browser-readiness-family`.
- Preflight `HEAD`, `origin/main`, and their merge base were all
  `ea63f67065dfd171fe33c45df89ff7e0451b546d`.
- `gh pr list --repo portpowered/you-agent-factory --head
  deflake-graph-editor-browser-readiness-family --state all` returned no PR.
  The open-PR target-file scan found no competing PR touching the target test.
- During the measurement, `origin/main` advanced to
  `8442846cb31f225a8c128228235874aa518ce31c`. No rebase was performed in this
  story; final synchronization is reserved for the handoff story immediately
  before its final push.
- Characterization code artifact: the story's committed test change at the
  story commit. Raw browser artifacts remain under the ignored local
  `.artifacts/ui-browser-integration/graph-editor-readiness/` directory and are
  not committed.

## Environment

- Microsoft Windows 11 Home, build 26200; PowerShell 7.6.5.
- Node `v22.12.0`; Bun `1.4.0`; Vitest `4.1.3`; Vite `7.3.2`; Playwright
  `1.59.1`.
- Chromium executable resolved by Playwright to
  `C:\Users\andre\AppData\Local\ms-playwright\chromium-1217\chrome-win64\chrome.exe`.
  The first real Chromium run launched successfully, so the browser
  availability criterion is not blocked.
- Host reported 24 logical processors.
- The canonical `cd ui && bun run build` attempt exited 1 before Vite because
  the current checkout has eight pre-existing TypeScript errors in unrelated
  current-selection/trace test helpers (`Assertion.not` is missing from their
  inferred type). After lockfile dependency restoration, the required browser
  artifact was produced by `bunx --no-install vite build` followed by
  `node scripts/normalize-dist-output.mjs`, exit 0 (`4830` modules
  transformed). This is a baseline/toolchain limitation, not a failure in the
  changed integration file.

## Procedure

The focused command was run without a test-name filter so all five existing
tests in the file remained in every iteration. Every invocation used
`--no-file-parallelism --maxWorkers 1 --retry=0`; each iteration was a new
first-attempt Vitest process, and a non-zero result would have been counted
without rerunning that iteration.

For each run, the following environment was set:

```powershell
$env:AGENT_FACTORY_BROWSER_ARTIFACT_DIR =
  '<repo>/.artifacts/ui-browser-integration/graph-editor-readiness/<condition>'
$env:AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION = 'false'
$env:AGENT_FACTORY_GRAPH_EDITOR_READINESS_CONDITION = '<ordinary|loaded>'
$env:AGENT_FACTORY_GRAPH_EDITOR_READINESS_ITERATION = '<01..30>'
bunx --no-install vitest run integration/factory-graph-editor.integration.test.mjs `
  --no-file-parallelism --maxWorkers 1 --retry=0 `
  --reporter=json --outputFile='<condition>/vitest-<condition>-<iteration>.json'
```

The first ordinary run used the default reporter and recorded
`Test Files 1 passed (1)` / `Tests 5 passed (5)`; ordinary runs 02–30 and all
loaded runs used the JSON reporter. The successful build was performed once
before the characterization runs.

The loaded set used one bounded background PowerShell worker, started hidden
and stopped by the same wrapper's `finally` block:

```powershell
$loadScript = '$sum = 0.0; while ($true) { for ($i = 1; $i -le 25000; $i++) { $sum += [Math]::Sqrt($i) } }'
$loadProcess = Start-Process -FilePath (Get-Command pwsh).Source `
  -ArgumentList @('-NoLogo','-NoProfile','-NonInteractive','-Command',$loadScript) `
  -PassThru -WindowStyle Hidden
try {
  # Invoke the exact focused command for loaded iterations 01..30.
} finally {
  if ($loadProcess -and -not $loadProcess.HasExited) {
    Stop-Process -Id $loadProcess.Id -Force
    $loadProcess.WaitForExit()
  }
}
```

Observed cleanup: `LOAD_PROCESS_STOPPED=True`; each scenario's existing
`server.stop()` and `browserPage.close()` ran from `finally`. No iteration was
retried. The harness captured full PNG, HTML, trace, and diagnostics artifacts
before each page close. The existing teardown artifacts contain occasional
`ERR_INCOMPLETE_CHUNKED_ENCODING`, `ERR_CONNECTION_RESET`, and loaded-set
`ERR_CONNECTION_REFUSED` console diagnostics after the assertions; page errors
were zero and these transport-teardown observations did not fail a test. They
are retained as an unproven cleanup edge rather than treated as readiness
evidence.

## Characterization result

| Condition | Site and initiating action | Failures / executed | Not reached / iterations | Test result |
| --- | --- | ---: | ---: | --- |
| Ordinary | Add workstation dialog: activate Add → Workstation, then observe the `Add workstation` dialog | 0 / 30 (0%) | 0 / 30 | 30 first-attempt passes |
| Ordinary | Leave editor observer mode: activate `Leave editor`, then observe Edit mode, zero Add buttons, and the Work graph viewport | 0 / 30 (0%) | 0 / 30 | 30 first-attempt passes |
| CPU-loaded | Add workstation dialog: same action and observation | 0 / 30 (0%) | 0 / 30 | 30 first-attempt passes |
| CPU-loaded | Leave editor observer mode: same action and observation | 0 / 30 (0%) | 0 / 30 | 30 first-attempt passes |

The five-test file therefore produced 150/150 ordinary and 150/150 loaded
passing test executions. The two target scenarios produced the exact
observable value checks on every execution: Add/save observed one successful
PUT with the existing topology/version/workstation/route/prompt assertions;
discard/leave observed Edit mode, a zero-count toolbar Add control, a visible
Work graph viewport, and zero save requests.

## Mechanism assessment

- Add workstation dialog: the action boundary is the menu's Workstation
  button, and the readiness boundary is the visible semantic dialog. All 60
  observations completed, so no failed DOM, actionability, render, network, or
  state transition is available to identify a mechanism.
- Leave editor observer mode: the action boundary is `Leave editor`, and the
  readiness boundary is the post-action Edit mode/toolbar/viewport state. All
  60 observations completed, so no failed transition or trace is available to
  identify a mechanism.
- The sites are kept as distinct accounting boundaries. They share the graph
  card and browser harness, but passing traces do not establish a shared
  state/action path; the common use of `waitFor` is not evidence of a shared
  cause.

## Failure diagnostics shipped for the next occurrence

When characterization metadata and the artifact directory are configured, a
failure in either target scenario is caught before the existing page close.
The test writes `<artifactLabel>.readiness.json` containing the site,
first-attempt flag, reached flag, bounded error details, URL, active element,
and bounded inventories for the graph card, toolbar/buttons, Add menu,
workstation dialog, mode controls, dialogs, and viewport. The existing harness
then writes the uniquely labelled `<artifactLabel>.png`, `.html`,
`.trace.zip`, and `.diagnostics.json` files before closing the page.

No `.readiness.json` failure bundle exists in this nonreproducing run; the
absence is itself recorded here, while the bounded next-occurrence path is
committed and ready to capture one without a rerun.

Target artifact inventory:

- Ordinary: 240 target files (60 site attempts × PNG/HTML/trace/diagnostics).
- CPU-loaded: 240 target files (60 site attempts × PNG/HTML/trace/diagnostics).
- Failure-specific readiness bundles: 0 ordinary, 0 loaded.

## Timing direction and limitations

For the JSON-reported runs (ordinary 02–30; loaded 01–30), target test
durations were:

| Site | Ordinary average | Loaded average | Ordinary range | Loaded range |
| --- | ---: | ---: | ---: | ---: |
| Add workstation | 36,472 ms | 28,864 ms | 28,047–47,608 ms | 21,887–40,741 ms |
| Leave editor | 34,025 ms | 26,525 ms | 24,616–44,219 ms | 19,942–36,829 ms |

The loaded sample did not show a latency increase, but these are noisy local
test durations with artifact capture and a shared host; they are not a product
performance claim and do not prove universal flake probability.

## Gate status and remaining edges

- `GATE-CHARACTERIZE`: PASS for exact 30 ordinary + 30 loaded full-file
  executions and per-site fractions above.
- `GATE-DIAGNOSTIC`: PASS for the committed pre-close capture path and the
  harness artifact contract; no failure instance occurred to populate a
  site-context bundle.
- `GATE-COMPONENT`: not applicable; no application code changed.
- `GATE-CORRECTION` / `GATE-STRESS`: not applicable on the story-002
  nonreproducing branch because no root cause or correction was proven or
  shipped; post-fix rates are `N/A`.
- `GATE-FULL-BROWSER` / `LOOPBACK-01`: deferred to the clean-artifact
  validation story.
- Canonical `bun run build` remains blocked by the unrelated baseline errors
  listed above; the story-002 `bun run typecheck` and changed-file syntax and
  Biome checks passed.
- Still unproven: universal failure probability, a root cause at either site,
  correction effectiveness, full browser-tier coexistence, clean-room final
  artifact behavior, current-main rebase, remote CI, review feedback, and
  merge.

Verification performed for the diagnostic change:

```text
node --check ui/integration/factory-graph-editor.integration.test.mjs    PASS
cd ui && bunx --no-install biome check integration/factory-graph-editor.integration.test.mjs    PASS
```

## Story 002 decision: retain diagnostics without a speculative correction

Story 001's evidence is the required direct trace input for story 002. Each
site completed 30 ordinary and 30 CPU-loaded first-attempt executions with
`0/30` failures and `0/30` not-reached in both conditions. There is therefore
no failing rendered, network, actionability, or explicit-ready transition to
name as a test-owned or product-owned mechanism. The Add workstation dialog
and Leave editor observer-mode sites remain distinct accounting boundaries;
their common graph card, harness, or `waitFor` calls do not prove a shared
cause.

The committed pre-close active-element and bounded node inventory remains the
next-occurrence diagnostic. No timeout, retry, rerun, sleep, fixed polling,
stable window, skip, quarantine, conditional, assertion, or test-count change
was made. No application source changed, so `GATE-COMPONENT` is not
applicable; no correction shipped, so `GATE-CORRECTION` and `GATE-STRESS` are
not applicable and post-fix rates are `N/A`.

Story-002 UI-quality evidence:

```text
cd ui && bun run typecheck                                           PASS (exit 0)
cd ui && bun run check                                               FAIL (exit 1; 174 unrelated base-tree diagnostics)
node --check ui/integration/factory-graph-editor.integration.test.mjs  PASS
cd ui && bunx --no-install biome check integration/factory-graph-editor.integration.test.mjs PASS
```

The full check's diagnostics are outside the changed integration file and
were not repaired in this scoped story. Clean final-artifact behavior,
full-browser-tier coexistence, loopback, rebase, PR workflow, and review remain
story-003 edges.
