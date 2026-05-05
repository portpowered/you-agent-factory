# meta view

## world state

- as of `2026-05-05T14:04:03.9441625-07:00`, local `HEAD` on `main` points to
  `d868ab7`
  (`update teh words on the readme to be a bit clearer`)
  and matches `origin/main`
- the local worktree is clean apart from ignored workflow inputs
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`

## workflow truth

- `factory/factory.json` still defines five work types: `thoughts`, `idea`,
  `plan`, `task`, and `cron-triggers`
- the checked-in maintainer loop remains:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:to-complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:to-complete`
  `consume` completes same-name `idea` + `task` pairs once both reach
  `to-complete`
- topology details that still matter:
  - `process` and `review` run in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity is `10`; each staffed workstation requests
    `1`
  - hourly `cleaner` emits `cron-triggers:complete`
  - `executor-loop-breaker` fails `task:init` after `process` visit `50`
  - `review-loop-breaker` fails `task:in-review` after `review` visit `10`

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- `.gitignore` still ignores live workflow submissions under `factory/inputs/**`
  except those sentinel paths
- the file watcher still enforces the documented three-segment watched-input
  contract and no longer accepts direct
  `factory/inputs/<work-type>/<file>` submissions as an implicit `default`
  channel fallback
- the visible canonical inboxes currently contain only the tracked `.gitkeep`
  sentinels; there is no ignored local idea or batch residue staged right now

## customer-ask truth

- the import/export P0 lane is now materially closed on `main`:
  - merged PRs `#67`, `#68`, `#69`, `#70`, `#71`, `#72`, and `#93` now cover
    the prompt template, split-layout, non-success-route, dialog, and
    supported portable bundled-file portions of that ask
  - canonical export now omits inline bundled-file content for supported
    portable `factory/scripts/**` and `factory/docs/**` entries while keeping
    the transport records needed for portability
  - runtime loading and validation now accept the supported disk-backed
    portable bundled-file path without rehydrating supported inline ownership
  - package, integration, service, and functional coverage now protect the
    supported bundled-file round-trip behavior through export, import, and
    runtime loading
- the selected-work current-selection ask is materially satisfied on `main`
  through merged PRs `#74` and `#77`
- the submit-work copy ask is satisfied on `main` through merged PR `#75`
- merged PR `#83` satisfied the header-verbosity copy reduction ask on `main`:
  visible `Factory state`, `Stream`, `Export PNG`, and `Current` toolbar text
  labels are gone
- merged PR `#84` satisfied the work-outcome chart ask on `main`:
  the chart no longer teaches detached axis labels and now carries increased
  axis spacing through rendered chart behavior
- merged PR `#85` satisfied the remaining dashboard branding and header
  iconography lane on `main`:
  - `ui/index.html`, `ui/fallback_dist/index.html`, and
    `ui/fallback_dist/assets/index.js` no longer ship the old
    `Agent Factory` / triangle shell contract
  - `ui/src/features/header/dashboard-header.tsx`,
    `ui/src/features/header/dashboard-status-panel.tsx`,
    `ui/src/features/bento/agent-bento.tsx`,
    `ui/src/features/workflow-activity/react-flow-current-activity-card-import.tsx`,
    `ui/src/features/header/dashboard-export-dialog.tsx`, and
    `ui/src/features/export/build-factory-export-filename.ts` now align with
    the Infinite You rename
  - the header stream/export/current controls now use the requested
    pulsating-dot, share-style, and play-style semantics
- merged PR `#86` satisfied the remaining header button-convergence seam on
  `main`:
  - `ui/src/features/header/dashboard-header.tsx` and
    `ui/src/features/header/tick-slider-control.tsx` now route the export and
    return-to-current actions through
    `ui/src/features/header/dashboard-header-action-button.tsx`
  - `ui/src/features/header/dashboard-header.test.tsx` and
    `ui/src/App.stories.tsx` now protect the converged neutral header-action
    treatment through rendered behavior
- merged PR `#87` satisfied the remaining submit-work button-tone seam on
  `main`:
  - `ui/src/features/submit-work/submit-work-card.tsx` now renders the
    `Submit work` CTA through the shared neutral `outline` tone instead of the
    accent-filled default treatment
  - `ui/src/features/submit-work/submit-work-widget.test.tsx` and
    `ui/src/App.stories.tsx` now protect that neutral rendered treatment
- the tracked local ask diff now explicitly includes the array-valued
  non-success output-interface request, but that ask is already materially
  satisfied on `main` through merged PR `#69`
- there is no remaining unowned customer-visible dashboard ask on `main`; the
  next narrow cleanup candidate now comes from the broader backend quality
  lane:
- merged PR `#88` retired the duplicated legacy dispatch compat payload
  aliases from `ui/src/features/timeline/state/timeline/replayWorldStateTypes.ts`
  and merged PR `#89` centralized the remaining replay
  `completionToTraceDispatch` projection duplication without reopening the
  broader legacy compat lane
- merged PR `#90` closed the authored-throttle transition-ID fallback lane on
  `main`:
  - `pkg/petri/inference_throttle_guard.go` no longer carries
    `WatchedTransitionIDs`
  - `pkg/config/config_mapper.go` no longer lowers authored throttle guards
    through transition-ID watch sets
  - throttle tests now assert runtime-observable lane behavior instead of the
    retired fallback shape
- merged PR `#91` closed the last hidden non-success route compatibility seam
  on `main`:
  - `pkg/config/openapi_factory.go` no longer carries the singular
    `onContinue` / `onRejection` / `onFailure` coercion helper
  - `pkg/api/factory_config_smoke_test.go` now rejects singular non-success
    route objects at the generated boundary
  - checked-in factory fixtures across `factory/`, `examples/`, and
    `tests/functional_test/testdata/` now author canonical array-valued
    non-success routes
- merged PR `#92` closed the stale OpenAPI cron post-processing compatibility
  seam on `main`:
  - `pkg/config/factory_config_mapping.go` no longer calls
    `applyOpenAPICronCompatibility`
  - `pkg/config/openapi_factory.go` no longer reparses normalized authored JSON
    through `buildRawOpenAPIWorkstationCronIndex`
  - `pkg/config/openapi_factory_test.go` and
    `tests/functional/runtime_api/api_runtime_config_alignment_smoke_test.go`
    now cover canonical cron loading through the generated boundary and runtime
    config path
- merged PR `#93` closed the supported portable bundled-file portability seam
  on `main`:
  - `pkg/config/factory_config_mapping.go` now strips supported portable inline
    bundled-file content from canonical export while preserving the portable
    transport entries
  - `pkg/config/runtime_config.go`, `pkg/config/layout.go`, and
    `pkg/config/config_validator.go` now treat supported portable bundled files
    as disk-backed during runtime loading, expansion, and validation
  - `pkg/config/portable_bundled_files*.go`, `pkg/service/factory_test.go`,
    and functional portability smoke coverage now protect the supported
    export/import/runtime round-trip behavior
- merged PR `#94` closed the remaining replay-wrapper cleanup seam on `main`:
  - `ui/src/features/timeline/state/timeline/replayCompletion.ts` and
    `ui/src/features/timeline/state/timeline/replayWorldState.ts` no longer
    carry the cast-wrapper-only replay payload helpers
  - replay timeline tests remain the proof path for the live replay behavior
- merged PR `#95` closed the duplicated submit-request shaping drift on `main`:
  - `pkg/factory/work_request.go` now owns the canonical
    `[]interfaces.SubmitRequest -> interfaces.WorkRequest` shaping path
  - `pkg/internal/submission/work_request.go` is now only a pass-through alias
    onto `factory.WorkRequestFromSubmitRequests`
  - functional runtime smoke coverage now protects trace and request-shaping
    parity through the runtime-visible submission path
- merged PR `#96` closed the dead submission-forwarder lane on `main`:
  - `pkg/internal/submission` is gone
  - live callers now import `pkg/factory.WorkRequestFromSubmitRequests`
    directly
  - the old meta note naming that package as the next cleanup candidate is now
    stale
- open PR `#97` (`website-icon-removal`) edits the dashboard header branding
  and timeline slider layout surface that the meta view already treated as
  materially closed through merged PRs `#83`, `#85`, and `#86`; it therefore
  overlaps an already-closed customer-ask lane instead of opening a new clean
  maintainer seam
- there is still no remaining narrow unowned customer-visible ask gap on
  `main`; the next non-overlapping cleanup candidate now comes from the
  broader backend quality lane:
  - `docs/development/deadcode-baseline.txt` still lists the replay
    event-stream file wrapper cluster in `pkg/replay/event_stream_artifact.go`
    (`ArtifactFromEventStreamFile`, `SaveArtifactFromEventStreamFile`, and the
    adjacent-factory hydration helpers) as unreachable dead surface
  - direct repo reads show that cluster is only exercised by
    `tests/functional/replay_contracts/replay_event_stream_artifact_smoke_long_test.go`
    and is otherwise absent from live runtime, API, and CLI callers
  - that makes it the next narrow non-overlapping cleanup seam: collapse or
    relocate the test-only wrapper layer while preserving replay artifact
    conversion behavior through behavioral replay smoke coverage
- the remaining ask surface beyond that is broader program work:
  - the general standards-migration checklist ask is still open in
    `factory/logs/meta/asks.md`
  - the website `90%` coverage target is still open in
    `factory/logs/meta/asks.md`
  - the manual QA and systems-quality documentation asks are still open in
    `factory/logs/meta/asks.md`

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract; it predates `to-complete`, `consume`,
  and the current `executor-slot` capacity of `10`
- replay outcome counts remain unchanged in the sample:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## recent repo movement

- recent merged PRs on `main` now include:
  - `#96` `remove-dead-submission-work-request-forwarder-package`, merged on
    `2026-05-04T18:22:54Z`
  - `#95` `dedupe-submit-request-shaping-between-production-and-functional-runtime-tests`,
    merged on `2026-05-04T17:34:28Z`
  - `#94` `retire-replay-dispatch-payload-cast-wrappers`, merged on
    `2026-05-04T17:21:31Z`
  - `#93` `canonicalize-portable-bundled-files-without-inline-content`,
    merged on `2026-05-04T15:53:47Z`
  - `#92` `retire-openapi-cron-compatibility-patch`, merged on
    `2026-05-04T14:33:28Z`
  - `#91` `retire-singular-workstation-route-compatibility`, merged on
    `2026-05-04T13:28:02Z`
  - `#90` `retire-authored-throttle-transition-id-fallback`, merged on
    `2026-05-04T12:32:50Z`
  - `#89` `centralize-replay-trace-dispatch-projection`, merged on
    `2026-05-04T11:25:59Z`
  - `#88` `retire-replay-timeline-legacy-compat-duplication`, merged on
    `2026-05-04T10:20:32Z`
  - `#87` `normalize-submit-work-button-treatment`, merged on
    `2026-05-04T09:17:47Z`
  - `#86` `normalize-dashboard-header-button-treatment`, merged on
    `2026-05-04T08:36:11Z`
  - `#85` `align-dashboard-branding-and-header-iconography`, merged on
    `2026-05-04T08:00:00Z`
  - `#69` `workstation-non-success-route-arrays`, merged into `main` before the
    current refresh and now represented by `HEAD`
  - `#84` `align-work-outcome-chart-axis-labels-and-margins`, merged on
    `2026-05-04T06:31:54Z`
  - `#83` `simplify-dashboard-header-toolbar-verbosity`, merged on
    `2026-05-04T05:53:10Z`
  - `#82` `trim-ralph-starter-input-readme-contract`, merged on
    `2026-05-04T04:20:52Z`
  - `#81` `trim-starter-input-readme-contract`, merged on
    `2026-05-04T03:18:53Z`
  - `#80` `align-default-starter-task-input-contract`, merged on
    `2026-05-04T02:36:26Z`
  - `#79` `retire-filewatcher-default-channel-fallback`, merged on
    `2026-05-04T01:27:11Z`
  - `#78` `remove-list-work-legacy-pagination-shim`, merged on
    `2026-05-04T00:28:40Z`
- `gh pr list --state open` currently reports one open PR:
  - `#97` `website-icon-removal`, opened on `2026-05-05T20:59:43Z`

## theory of mind

- the authoritative world model still comes from live git state plus the
  checked-in workflow contract, not from replay fixtures alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- ignored queue residue can become stale within a single merge cycle, so the
  meta loop has to reconcile local inbox files against the newest merged PRs
  before dispatching anything new
- merged PR state can flip between the start of a refresh and the end of it;
  open-PR assumptions need to be revalidated after `git pull` and before queue
  writes
- once a customer-visible ask lands, the right follow-up is usually not another
  broad lane but the smallest remaining seam that preserves the ask's public
  intent without reopening merged work
- once a portability lane looks closed, re-check export mapping, runtime load,
  validation, and service/functional tests together before retiring the idea;
  PR `#93` only became safe to close after all four layers moved to the same
  supported disk-backed ownership model
- once a narrow cleanup merges, the next best seam can still hide in boundary
  normalizers and fixtures rather than in the obvious runtime path; after
  PR `#90`, the stale throttle note was gone on `main` but the array-route ask
  still had a hidden compatibility layer in the OpenAPI factory boundary
- when a public JSON shape moves from singular objects to arrays, re-check
  import/load normalization helpers and checked-in factory fixtures; type and
  schema updates alone do not prove the old authored shape is actually rejected
- after a boundary migration lands, re-check for follow-on post-processing
  patches that still reparse the authored JSON out-of-band; once the generated
  model and mapper both carry the canonical field directly, those patches are
  often the next dead compatibility seam
- for import/export asks, verify both canonical `factory.json` flattening and
  expanded authored layout writes before declaring the lane closed; stripping a
  field during `WriteExpandedFactoryLayout` does not mean `FlattenFactoryConfig`
  and runtime loading have stopped rehydrating it back into the exported JSON
- after broad replay legacy-compat cleanups merge, scan for any remaining
  one-line cast wrappers or alias-only helpers on the live replay path before
  inventing a larger follow-up; those seams are often the next lowest-risk
  simplifications
- when production request-shaping and functional-test request-shaping both
  exist, compare chaining-trace propagation and batch metadata field-by-field
  before assuming the duplicate helper is harmless; this repo already allowed a
  test-only copy to drift away from the live submit path
- once a dedupe PR lands, re-scan the old owner package for alias-only
  wrappers; this repo now shows that a merged centralization can leave a whole
  package behind as dead forwarding surface even after the behavioral drift is
  fixed
- once a cleanup seam is recorded in the checked-in worldview, re-validate it
  against live `main` before dispatching; merged PR `#96` proved the meta view
  can go stale within a single refresh cycle
- when an open PR touches a surface already marked closed in the worldview,
  compare the PR diff against the canonical ask text before dispatching any
  sibling work; PR `#97` is a good example of overlap hiding behind a new
  branch name
- deadcode-baseline entries that are only reachable from one long functional
  smoke are strong candidates for the next narrow cleanup idea, especially when
  the live runtime, API, and CLI paths have no direct callers
