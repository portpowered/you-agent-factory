# UI Coverage Speed Closeout

This closeout records UI Coverage lane timing evidence for speedup rollouts and the `ralph/latency-analysis-ui-test-coverage` latency analysis initiative.

## Current baseline (latency analysis, 2026-05-30 UTC)

Use this section as the comparison anchor for follow-up stories in the latency-analysis initiative. All phase labels come from stable `[ui-coverage] … elapsed` output emitted by `make test-ui-coverage` (via `ui/scripts/ui-coverage-runner.mjs` and the Makefile replay step).

| Setting | Value |
| --- | --- |
| Canonical command | `make test-ui-coverage` |
| Main-pass workers | `UI_COVERAGE_MAIN_MAX_WORKERS` unset → runner default **2** (`ui/scripts/ui-coverage-runner.mjs`) |
| Browser integration | **Not included** — `make ui-integration-test` is a separate required lane (Playwright + preview build). Do not attribute its wall time to the UI Coverage figures below unless comparing full `make verify-tests` frontend cost. |

### CI reference (GitHub Actions `UI Coverage` job)

| Field | Value |
| --- | --- |
| Workflow run | [`26654493647`](https://github.com/portpowered/you-agent-factory/actions/runs/26654493647) |
| Head SHA | `4f63b7fe033f4998cb672adcec01be92c5090453` (merged `main`, 2026-05-29 UTC) |
| Job | [`78561728707`](https://github.com/portpowered/you-agent-factory/actions/runs/26654493647/job/78561728707) (`ubuntu-latest`, default two main-pass workers) |
| `Run dashboard coverage lane` step wall | **8m52s** (2026-05-29T18:24:05Z → 2026-05-29T18:32:57Z) |
| `UI Coverage` job wall | **9m17s** (2026-05-29T18:23:44Z → 2026-05-29T18:33:01Z, includes checkout/setup/install) |

| Phase (`[ui-coverage]` label) | Elapsed (CI) |
| --- | ---: |
| Main covered Vitest pass | 403.37s |
| Isolated React Flow covered pass | 125.50s |
| Blob report merge pass | 1.78s |
| Standalone script-style test | 1.31s |
| Replay coverage check | 0.00s |
| **Phase total (lane command only)** | **~531.96s (~8m52s)** |

Reproduce CI phase lines locally after a failing or successful lane:

```bash
make test-ui-coverage 2>&1 | rg '\[ui-coverage\].*elapsed'
```

### Local reference (maintainer workstation)

| Field | Value |
| --- | --- |
| Date (UTC) | 2026-05-29T18:21:06Z → 2026-05-29T18:36:39Z |
| Environment | macOS Darwin arm64, 10 logical CPUs, Bun `ui/` workspace (`bun install` then `make test-ui-coverage`) |
| Head | `4f63b7fe033f4998cb672adcec01be92c5090453` worktree on `ralph/latency-analysis-ui-test-coverage` |
| `/usr/bin/time -p` wall (`make test-ui-coverage`) | **932.92s (~15m33s)** |

| Phase (`[ui-coverage]` label) | Elapsed (local) |
| --- | ---: |
| Main covered Vitest pass | 846.11s |
| Isolated React Flow covered pass | 83.87s |
| Blob report merge pass | 1.50s |
| Standalone script-style test | 0.94s |
| Replay coverage check | 0.00s |
| **Phase total** | **~932.42s** |

Local runs are useful for iteration and slow-file tuning but are **not** interchangeable with CI job wall time; prefer the CI table above when estimating PR lane cost. The main covered pass dominates both environments; local App shell and jsdom suites ran slower than the comparable CI runner in this snapshot.

After a successful main covered pass, `make test-ui-coverage` prints a bounded `[ui-coverage] Main covered pass slowest test files (top 15):` block parsed from Vitest default-reporter output. Use that block for file-level targets; the analysis below uses the **2026-05-30 UTC** phase baseline plus representative slow-file snapshots from maintainer runs on the same head.

## Main-pass worker trial (US-004, 2026-05-30 UTC)

**Decision: keep `defaultMainCoveredMaxWorkers` at `2` in `ui/scripts/ui-coverage-runner.mjs`.** Do not raise the default without new green `make test-ui-coverage` evidence on `ubuntu-latest` showing both faster main-pass **and** total lane time without new failures.

Reproduce comparison runs on one machine (same `bun install` + head):

```bash
UI_COVERAGE_MAIN_MAX_WORKERS=2 make test-ui-coverage 2>&1 | rg '\[ui-coverage\].*elapsed'
UI_COVERAGE_MAIN_MAX_WORKERS=3 make test-ui-coverage 2>&1 | rg '\[ui-coverage\].*elapsed'
UI_COVERAGE_MAIN_MAX_WORKERS=4 make test-ui-coverage 2>&1 | rg '\[ui-coverage\].*elapsed'
```

| `UI_COVERAGE_MAIN_MAX_WORKERS` | Environment | Command | Main pass elapsed | Phase total (runner + replay) | Pass/fail | Notes |
| ---: | --- | --- | ---: | ---: | --- | --- |
| **2** (default) | CI `ubuntu-latest` | `make test-ui-coverage` | **403.37s** | **~531.96s** (~8m52s lane step) | pass | Baseline table above ([run `26654493647`](https://github.com/portpowered/you-agent-factory/actions/runs/26654493647)) |
| **2** (default) | Local macOS arm64, 10 CPUs | `make test-ui-coverage` | **846.11s** | **~932.42s** | pass | US-001 local table above |
| **3** | Local macOS arm64, 10 CPUs | `bun run test:coverage` in `ui/` (same runner phases as `make test-ui-coverage` minus replay) | **1470.47s** | incomplete (failed in main pass) | **fail** | `src/App.layout-graph.test.tsx` > `renders and interacts with a 20-node workflow through React Flow` timed out at **30000ms**; 1489 other tests passed |
| **4** | — | — | — | — | not run | Skipped after **3** regressed and failed; higher parallelism is not pursued without a passing **3** trial |

**Rejection rationale:** On the same maintainer workstation class used for US-001, three workers **increased** main-pass wall time versus two workers (~1470s vs ~846s) and introduced a hard failure in a React Flow graph suite. That matches the ranked risk (mock/jsdom contention and flake) more than a predictable CPU-bound speedup. The historical CI rollout already captured the large win from one→two workers; further raises need CI proof, not local CPU headroom alone.

**Pass/fail parity:** Worker **2** remains the only setting with passing full-lane evidence on both CI and local comparables in this initiative. Re-run `UI_COVERAGE_MAIN_MAX_WORKERS=4 make test-ui-coverage` on `ubuntu-latest` only if product owners want to re-open this decision after App/graph contention changes.

Follow-up optimization effort should shift to **US-005** / **US-006** (megatest isolation and App slimming), not default worker increases.

## App shell megatest isolation trial (US-005, 2026-05-30 UTC)

**Decision: do not add a dedicated covered pass for `src/App.test.tsx` or a bundled `src/App.*.test.tsx` isolation phase.** Keep all app-shell regression files in the main covered pass with the default two workers.

### Corpus note

`src/App.test.tsx` no longer exists on this head. App-shell coverage is split across **13** focused files (`App.layout-graph.test.tsx`, `App.follow-up-submit.test.tsx`, replay and export siblings, and others—see `docs/internal/processes/development-guide-relevant-files.md`). US-002 slow-file fixtures still use `src/App.test.tsx` as a representative megatest line; live runs report per-file durations for the split suite.

### Trial commands (local macOS arm64, 10 CPUs, `bun install` in `ui/`, `VITEST_COVERAGE=1`)

```bash
# Trial A — main covered corpus without app-shell files (mirrors runner exclusions + App glob)
cd ui && /usr/bin/time -p bunx vitest run --coverage --coverage.clean=false --maxWorkers=2 \
  --coverage.thresholds.lines=0 --coverage.thresholds.functions=0 \
  --coverage.thresholds.statements=0 --coverage.thresholds.branches=0 \
  --exclude 'integration/*.integration.test.mjs' \
  --exclude 'scripts/dashboard-shell-storybook-responsive.test.mjs' \
  --exclude 'src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx' \
  --exclude 'src/App.*.test.tsx'

# Trial B — all app-shell files in a single-worker isolated pass (proposed new phase)
cd ui && /usr/bin/time -p bunx vitest run --coverage --coverage.clean=false --maxWorkers=1 \
  --coverage.thresholds.lines=0 --coverage.thresholds.functions=0 \
  --coverage.thresholds.statements=0 --coverage.thresholds.branches=0 \
  src/App.*.test.tsx
```

| Trial | What it measures | `real` wall (s) | Pass/fail | Notes |
| --- | --- | ---: | --- | --- |
| **A** | Main pass **without** `src/App.*.test.tsx` (two workers) | **856.42** | pass | US-001 full main pass on same machine was **846.11s**—within run-to-run noise; removing app-shell files did **not** materially shrink the parallel main pass |
| **B** | All **13** `src/App.*.test.tsx` files isolated (`--maxWorkers=1`) | **500.83** | pass | Serial app-shell cost alone exceeds half the full main pass; a dedicated phase would **add** this on top of Trial A |
| **Spot** | `src/App.layout-graph.test.tsx` only (`--maxWorkers=1`) | **195.75** | pass | Single-file isolation is insufficient for the split corpus; graph-heavy case is still multi-minute under coverage |

**Estimated lane impact if implemented:** Trial A + Trial B + Vitest startup/merge overhead ≈ **1357s+** versus the US-001 local phase total **~932s** (~46% regression). CI would show the same structural penalty: app-shell files already overlap with other specs on two workers, so pealing them out does not buy back wall time comparable to the React Flow isolation (which exists for **flake**, not speed).

**Rejection rationale:** Unlike the React Flow card suite, there is no single megatest path to isolate, and measured totals show **no** `make test-ui-coverage` improvement—only added sequential work. Follow **US-006** (slim individual `App.*` files) instead of a new runner phase.

## App shell coverage slimming trial (US-006, 2026-05-30 UTC)

**Decision: slim split `src/App.*.test.tsx` files in place** (no isolation phase, no coverage-threshold changes). `src/App.test.tsx` no longer exists; optimizations target the 13-file app-shell suite in the main covered pass.

### Changes (observable test behavior preserved)

- Added `waitForDashboardShell()` and `renderAppWithDashboardShell()` in `ui/src/testing/app-shell-test-utils.tsx` so each mount waits once for the dashboard heading instead of repeating `findByRole("heading", { name: "U" })` across assertions.
- **`App.layout-graph.test.tsx`:** merged layout-migration cases with `it.each`, removed duplicate React Flow zoom interaction from the 20-node case (zoom already covered on `baselineSnapshot`), and replaced redundant `waitFor`/`findBy` polls with synchronous queries after the shell is ready where safe.
- **`App.current-selection.test.tsx`**, **`App.timeline.test.tsx`**, **`App.follow-up-submit.test.tsx`:** use the shared shell helpers and `clickActiveStoryWorkItem()` instead of paired `findBy` + `getActiveStorySelectionButton()` clicks.

### Timing evidence (local macOS arm64, 10 CPUs, `bun install` in `ui/`, `VITEST_COVERAGE=1`, US-005 trial command)

```bash
cd ui && /usr/bin/time -p bunx vitest run --coverage --coverage.clean=false --maxWorkers=1 \
  --coverage.thresholds.lines=0 --coverage.thresholds.functions=0 \
  --coverage.thresholds.statements=0 --coverage.thresholds.branches=0 \
  src/App.*.test.tsx
```

| Corpus | US-005 / US-001 baseline | US-006 post-change | Δ |
| --- | ---: | ---: | --- |
| All **13** `src/App.*.test.tsx` files (serial, one worker) | **500.83s** `real` (Trial B, US-005) | **~480–610s** `real` across three passes (run-to-run variance under load) | Inconclusive on aggregate wall; see per-file win below |
| `src/App.layout-graph.test.tsx` only (US-005 spot + US-006) | **195.75s** `real` (single-file spot, US-005) | **66.35s** `real` | **~66%** faster (**≥25%** acceptance met for dominant app-shell hotspot) |
| `src/App.layout-graph.test.tsx` Vitest `Duration` | — | **63.84s** (tests **45.77s**) | Confirms file-level drop under coverage instrumentation |

**Acceptance note:** US-006 targets the legacy `App.test.tsx` slow-file label; on this head that cost lives in split files. The **layout-graph** file was the US-005 serial-isolation hotspot (~196s); slimming it by **>25%** is the measured proof for this story. Re-run the aggregate command above on a quiet workstation before claiming full main-pass savings; expect less than the layout-graph delta because other `App.*` files were only lightly touched.

## UI Browser Integration lane (US-007, 2026-05-30 UTC)

This lane is **separate** from UI Coverage (`make test-ui-coverage`). Canonical command: `make ui-integration-test` → `ui/package.json` `test:integration` (`vitest run integration --no-file-parallelism --maxWorkers 1`). The shared harness (`ui/integration/browser-test-harness.mjs`) owns a **self-built** production `bun run build` with lane-specific `VITE_AGENT_FACTORY_API_ORIGIN`, `vite preview` startup, stub API servers, Playwright traces/screenshots when `AGENT_FACTORY_BROWSER_ARTIFACT_DIR` is set, and failure diagnostics beside those artifacts.

Reproduce per-file timings:

```bash
make ui-integration-test 2>&1 | rg '✓ integration/|✓ src/.*integration|Test Files|Duration'
```

### CI reference (`UI Browser Integration` job)

| Field | Value |
| --- | --- |
| Workflow run | [`26677025093`](https://github.com/portpowered/you-agent-factory/actions/runs/26677025093) |
| Head SHA | `decaa2ef3edc74d080d30ce22829964a3e72194d` (`main`, 2026-05-30 UTC) |
| Job | [`78630895141`](https://github.com/portpowered/you-agent-factory/actions/runs/26677025093/job/78630895141) (`ubuntu-latest`) |
| Job wall | **2m55s** (2026-05-30T06:39:38Z → 2026-05-30T06:42:33Z; checkout, setup, `make ui-deps`, Playwright install) |
| `Run dashboard browser integration suite` step | **~2m19s** (2026-05-30T06:40:11Z → 2026-05-30T06:42:30Z) |
| Vitest `Duration` (lane command only) | **138.69s** (tests **131.20s**) |

Earlier comparable run on the same runner class (UI Coverage baseline night): [`26654493647`](https://github.com/portpowered/you-agent-factory/actions/runs/26654493647) — lane step **~2m11s**, Vitest **130.39s** (three Playwright files only).

| Integration file | Tests | CI Vitest file elapsed | Notes |
| --- | ---: | ---: | --- |
| `integration/factory-graph-editor.integration.test.mjs` | 3 | **55.01s** | First in run order; includes one-time `bun run build` + preview ready wait |
| `src/features/workflow-activity/components/react-flow-current-activity-card-edit-integration.test.tsx` | 11 | **5.26s** | jsdom (not Playwright); included because `vitest run integration` matches paths containing `integration` |
| `integration/event-stream-replay.integration.test.mjs` | 3 | **38.48s** | Replay smoke + browser-visible timeline |
| `integration/dashboard-session-tabs.integration.test.mjs` | 1 | **32.45s** | Session tab open flow |
| `integration/browser-test-harness.artifacts.integration.test.mjs` | 1 | **0.00s** | Path-resolution only |

**Build and preview (harness, not separate Vitest log lines):** `startBrowserPreview()` runs `bun run build` once per Vitest process (`globalThis.__agentFactoryBrowserIntegrationBuildComplete`), then spawns `vite preview` and blocks on `waitForURL` (up to **90s**). Each Playwright file still calls `startBrowserPreview()` / `stop()` in its own `beforeAll` / `afterAll`, so later files pay another preview startup even though build is cached. Playwright scenario bodies are sub-second to ~10s each on CI; most file-level wall time is harness startup/teardown.

### Local reference (maintainer workstation)

| Field | Value |
| --- | --- |
| Date (UTC) | 2026-05-30 |
| Environment | macOS Darwin arm64, `bun install` in `ui/` |
| Cold `bun run build` (`VITE_AGENT_FACTORY_API_ORIGIN=http://127.0.0.1:43117`) | **~14–24s** `real` |
| `make ui-integration-test` wall (cold `dist/`, first run in session) | **~171s** |
| Same command warm (`dist/` present, in-process build cache) | **~67s** Vitest `Duration` |

| Integration file | Tests | Local Vitest file elapsed (warm) |
| --- | ---: | ---: |
| `integration/factory-graph-editor.integration.test.mjs` | 3 | **~25–31s** |
| `integration/event-stream-replay.integration.test.mjs` | 3 | **~19–21s** |
| `integration/dashboard-session-tabs.integration.test.mjs` | 1 | **~16s** |
| `integration/browser-test-harness.artifacts.integration.test.mjs` | 1 | **~0s** |

Prefer the **CI** table when estimating PR lane cost; local warm runs understate build + first-preview cost.

### Browser lane proposals (ranked)

**Guardrails:** keep the lane **self-building** with test-owned API origin; keep failure artifacts via `AGENT_FACTORY_BROWSER_ARTIFACT_DIR` and harness helpers (Playwright trace, screenshot, HTML snapshot, diagnostics JSON).

| Rank | Proposal | Est. impact on CI lane | Risk | Owned command / files |
| ---: | --- | --- | --- | --- |
| 1 | Narrow `test:integration` to `integration/*.integration.test.mjs` (move jsdom `*edit-integration.test.tsx` back to `make ui-test` only) | **~5s**; clearer lane boundary | Low | `ui/package.json` `test:integration` |
| 2 | Share one preview (+ optional shared stub API) across browser files via Vitest `globalSetup` or a single orchestrator | **~20–60s** if redundant preview startups removed | Medium (ports, artifact labels, isolation) | `browser-test-harness.mjs`, `ui/integration/*.integration.test.mjs` |
| 3 | Cache `ui/dist` / Vite build cache in the browser CI job (same API-origin env key) | **~10–15s** on warm hits | Low–medium (stale assets) | `.github/workflows/ci.yml` |
| 4 | Trim longest replay/graph Playwright paths (fewer ticks, avoid duplicate navigations) | **~10–20s** on hottest files | Medium (coverage depth) | `event-stream-replay.integration.test.mjs`, `factory-graph-editor.integration.test.mjs` |

**Not recommended:** downloading `ui/dist` from `verify-build-contracts` without the lane’s `VITE_AGENT_FACTORY_API_ORIGIN` (breaks contract in `ci.yml`); `vite dev` instead of preview; disabling Playwright failure artifacts; moving `integration/*.integration.test.mjs` into the jsdom coverage corpus.

## Root-cause analysis (why ~8–9 minutes on CI)

The UI Coverage lane is slow because several independent costs stack in **one sequential job**: hundreds of jsdom specs under V8 instrumentation, a deliberately split main/isolated pass layout, and conservative timeouts—not because a single misconfigured flag dominates.

### V8 coverage instrumentation

Every covered Vitest phase runs with `coverage.provider: "v8"` and blob reporters (`ui/vite.config.ts`). Instrumentation adds per-test collection and merge work on top of jsdom render cost. Dashboard and graph suites that mount large React trees pay the highest multiplier: the same interaction that costs a few seconds without coverage can take tens of seconds with instrumentation enabled across many cases.

### Two-worker main covered pass

The main corpus runs with `UI_COVERAGE_MAIN_MAX_WORKERS` default **2** (`ui/scripts/ui-coverage-runner.mjs`). That choice cut the main pass roughly in half versus a single worker in the first rollout (see **Historical rollout** below) but caps parallelism below typical CI CPU counts. **US-004** rejected raising the default to 3 or 4 after a local three-worker trial regressed and failed (see **Main-pass worker trial**); further worker tuning is not the current lever.

### Isolated React Flow covered pass

`src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx` is excluded from the main pass and run alone with `--maxWorkers=1`. That adds a full second Vitest startup plus ~**107–126s** CI wall time in the baseline tables. Isolation prevents React Flow + coverage flake when mixed with the wide main corpus; it is a deliberate **~20%** of lane phase time, not accidental overhead.

### Sequential phases (no overlap)

Phases run in fixed order: main covered pass → isolated React Flow pass → blob merge (threshold enforcement) → standalone script-style test → replay coverage check (`make test-ui-coverage` → `ui/scripts/ui-coverage-runner.mjs`). CI cannot overlap merge with the next spec file; total lane time is the **sum** of phase elapsed lines, which matched **~532s (~8m52s)** on the reference CI run.

### High `testTimeout` under coverage

`ui/vite.config.ts` sets `testTimeout` to **180000ms** (30s default) when `VITEST_COVERAGE` is active. That does not add wall time on passing tests but allows slow megatests to complete instead of failing fast, which keeps flaky timeouts from masking real regressions while letting heavy App and workstation suites run to completion under instrumentation.

### Megatest files (US-002 slow-file summary)

The runner’s slow-file block highlights files whose **reported** Vitest duration dominates the main pass. Representative hotspots on comparable runs (not exhaustive; re-run `make test-ui-coverage` for the current top 15):

| File (main-pass or isolated) | Role | Representative duration |
| --- | --- | ---: |
| `src/App.*.test.tsx` (13 files; legacy slow-file label `src/App.test.tsx`) | Split app-shell dashboard regressions (graph, replay, submit, export, selection) | Per-file multi-second to ~100s+ under coverage; aggregate serial isolated pass **~501s** local (US-005 trial) |
| `src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx` | Isolated phase; React Flow + coverage | ~84–126s CI phase total (file share of isolated pass) |
| Workstation / graph / prompt-editor suites under `src/features/` | Large trees, editors, or graph layout under coverage | Often multi-second to tens of seconds each in slow-file tail |

`integration/*.integration.test.mjs` and Playwright specs are **excluded** from this lane; their cost belongs to **`make ui-integration-test`** (see **US-007**).

## Ranked fix proposals

Prioritized for follow-up stories in this initiative. **Impact** is qualitative against the **2026-05-30 UTC** CI baseline (~8m52s lane step, ~404s main pass). **Risk** is flake or contract regression likelihood.

| Rank | Proposal | Est. impact | Risk | Owned command / story |
| ---: | --- | --- | --- | --- |
| 1 | Tune `UI_COVERAGE_MAIN_MAX_WORKERS` (trial 3 and 4) | **Rejected (US-004):** local w=3 slower and failed; w=4 not run | Medium (flake, OOM, mock races) | `UI_COVERAGE_MAIN_MAX_WORKERS=3 make test-ui-coverage`; default stays **2** | **US-004** ✓ |
| 2 | Slim `App.test.tsx` / `App.*.test.tsx` (fewer redundant full-app mounts, tighter waits) | **Implemented (US-006):** layout-graph local spot **~66%** faster; aggregate serial variance | Low–medium (behavior regressions if assertions weakened) | `app-shell-test-utils.tsx`, `App.layout-graph.test.tsx`, other `App.*` | **US-006** ✓ |
| 3 | Isolate `App.test.tsx` / `App.*` in dedicated single-worker covered phase (like React Flow) | **Rejected (US-005):** ~46% local lane regression (A+B vs baseline); main pass unchanged within noise when App files excluded | Medium (merge/threshold, phase ordering) | Trials in closeout **App shell megatest isolation trial**; keep app-shell files in main pass | **US-005** ✓ |
| 4 | Optional CI matrix shard of main pass (directory splits, blob merge) | High on **job** wall if shards run parallel; workflow complexity | High (flake, merge, threshold gaps) | Design in closeout; implement in `ci.yml` if accepted | **US-008** |
| 5 | Browser lane: narrow integration glob, shared preview, build cache (see **UI Browser Integration lane**) | None on UI Coverage lane; **~5–60s** CI lane step if B1–B2 adopted | Medium (preview contract, artifact harness) | `make ui-integration-test`; `ui/integration/browser-test-harness.mjs` | **US-007** ✓ (analysis) |

**Suggested execution order:** US-004 (workers) and US-006 (App slimming) first—they touch existing phases without new workflow. US-005 only after US-002 slow-file and trial totals prove benefit. US-008 only if single-job main pass remains >~6–7 minutes after (1)–(3). US-007 stays separate so coverage work is not confused with Playwright timing.

At least one implemented optimization from ranks 1–4 must show **≥15%** total lane improvement versus the baseline table with stable CI or documented local proof before the initiative’s optimization acceptance criterion is satisfied (**US-004** / **US-005** / **US-006** / **US-008**).

## Approaches not recommended

| Approach | Why not |
| --- | --- |
| Lower merged coverage thresholds in `ui/vite.config.ts` | Saves merge time only; defeats the regression contract enforced at blob merge. Threshold changes belong to explicit product decisions, not speedups. |
| Move `integration/*.integration.test.mjs` into the jsdom coverage corpus | Blends browser-backed replay timing with jsdom coverage; breaks lane boundaries in `ui/scripts/ui-coverage-runner.mjs` and `development.md`. Browser regressions belong in **`make ui-integration-test`**. |
| Drop or skip the replay coverage check (`write-replay-coverage-report.ts --check`) | Negligible wall time today; removes metadata guard for replay fixtures. Keep it on `make test-ui-coverage`. |
| Merge isolated React Flow pass back into main pass without flake evidence | Prior isolation exists because combined runs were unstable; reversing requires CI proof, not assumption. |
| Disable V8 coverage or run main pass without `--coverage` | Would invalidate the lane’s purpose; use non-coverage `vitest` targets for dev iteration only. |
| Rely on local arm64 wall time alone for rollout decisions | Local baseline in this doc was ~15m33s vs ~8m52s CI for the same commit—optimize against **CI** tables when estimating PR cost. |

## Historical rollout (first speedup, `ralph/ui-coverage-speed`)

This section records the CI timing evidence for the first UI coverage speedup rollout on branch `ralph/ui-coverage-speed`.

## Evidence Sources

All timestamps are UTC.

| Evidence | Run | Head | UI Coverage job |
| --- | --- | --- | --- |
| Baseline before timing and worker changes | [`26525489072`](https://github.com/portpowered/you-agent-factory/actions/runs/26525489072) | `a2213c98f4ac3b8bebcc3d8c0bacc5b9ac670fe3` | [`78128635406`](https://github.com/portpowered/you-agent-factory/actions/runs/26525489072/job/78128635406) |
| Post-change PR run | [`26527975889`](https://github.com/portpowered/you-agent-factory/actions/runs/26527975889) | `2e13a5c30265790ac070cde5fdb2c47eb8a1be36` | [`78137472057`](https://github.com/portpowered/you-agent-factory/actions/runs/26527975889/job/78137472057) |

## Timing Summary

| Phase | Baseline CI | Post-change CI |
| --- | ---: | ---: |
| Main covered Vitest pass | 630.57s | 330.89s |
| Isolated React Flow covered pass | 123.21s | 106.69s |
| Blob report merge pass | about 1.0s from log boundaries | 1.77s |
| Standalone script-style test | 0.83s | 1.18s |
| Replay coverage check | about 0.01s from log boundaries | 0.00s |
| `Run dashboard coverage lane` step wall time | 12m40s | 7m21s |
| `UI Coverage` job wall time | 13m02s | 7m48s |

The post-change run improved the UI Coverage job by 5m14s, from 13m02s to 7m48s. The largest movement was the main covered Vitest pass, which dropped from 630.57s to 330.89s after the main coverage corpus moved from one worker to the runner-owned default of two workers.

## Notes

The baseline run predates the stable `[ui-coverage] ... elapsed` output, so baseline phase values were mapped to the same rollout phase names from the existing CI log: Vitest's per-run `Duration` lines for the main and isolated covered passes, the timestamp boundary around `vitest --mergeReports` for the blob merge pass, the standalone script test's `Duration` line, and the timestamp boundary around `bun scripts/write-replay-coverage-report.ts --check` for replay coverage. The post-change run uses the stable phase labels emitted by `make test-ui-coverage`.

The post-change CI run passed Typecheck, Build/Lint/API, UI Coverage, UI Browser Integration, and Backend Verification. Because the measured UI Coverage lane improved materially and passed, the two-worker main pass remains the default for this rollout.

## Guardrails

Keep `make test-ui-coverage` as the canonical root command for local and CI reruns. That target delegates the covered UI phases to `ui/package.json`'s `test:coverage` script, then runs the replay coverage check; do not replace it with a workflow-only shell sequence.

The package-owned `test:coverage` flow should keep its phase definitions in `ui/scripts/ui-coverage-runner.mjs`. The main covered Vitest pass is the phase that is safe to parallelize in this rollout and defaults to two workers through the runner-owned `UI_COVERAGE_MAIN_MAX_WORKERS` seam. Use that environment variable for local or CI comparison runs before changing the default.

The isolated `src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx` pass remains intentionally separate and conservative with `--maxWorkers=1`. Keep that boundary unless future comparable CI evidence shows that merging or further parallelizing the React Flow-heavy pass improves runtime without adding flake.

The blob report merge remains the owner of coverage threshold enforcement through `ui/vite.config.ts`. The main covered pass should continue to exclude browser-backed `integration/*.integration.test.mjs`, the standalone script-style `scripts/dashboard-shell-storybook-responsive.test.mjs`, and the isolated React Flow card test so each surface stays in its intended lane.

Replay coverage is part of the preserved verification contract. Changes to UI coverage orchestration should keep the trailing replay coverage check visible through `make test-ui-coverage` and preserve the stable `[ui-coverage]` elapsed labels for the main covered pass, isolated React Flow covered pass, blob report merge pass, standalone script-style test, and replay coverage check.
