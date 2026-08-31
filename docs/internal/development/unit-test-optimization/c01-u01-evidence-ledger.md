# U01 bounded hosted evidence ledger

Status: `BLOCKED` with complete-delivery disposition under the operator
amendment. The authorized hosted attempt produced twelve completed unit
observations across both requested job settings and both diagnostic/control
modes. Every completed observation preserved the 445 timing package rows and
80.7% aggregate coverage, but every one failed the inherited timing corpus
invariants. No valid cohort, median, infrastructure attribution, or package
classification is admitted.

Canonical inventory: [`c01-cost-attribution-inventory.json`](c01-cost-attribution-inventory.json)

## Scope and provenance

- Source head: `4d13d577ce699ea80ff9643b2221bbd2f178bd09`
- Workflow change: commit `4d13d577ce` adds only bounded `workflow_dispatch`
  inputs for unit jobs `1`/`4`, diagnostic/control mode, and sample identity;
  it does not change the tested package selection, coverage floor, or required
  checks.
- Runner/toolchain: `ubuntu-latest`, Go `go1.25.0`, attempt `1` for every
  completed observation.
- Invocation: `make test-unit-coverage` with `UNIT_COVERAGE_JOBS` set to the
  requested value. Diagnostic rows additionally set
  `UNIT_COVERAGE_BUILD_DIAGNOSTICS` to the artifact path; controls leave it
  empty.
- Artifact name: `unit-coverage-diagnostics`; raw artifacts remain in GitHub
  Actions and are not committed.

## Completed observations

| Mode | Jobs | Sample | Run | Job | Artifact | Diagnostic wall (s) | Timing wall (s) | Coverage | Timing | Admission |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |
| diagnostic | 1 | 1 | [33356894321](https://github.com/portpowered/you-agent-factory/actions/runs/33356894321) | 99380925320 | 9745671137; `coverage-build-diagnostics.json` | 390.983 | 390.983 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| control | 1 | 1 | [33356915970](https://github.com/portpowered/you-agent-factory/actions/runs/33356915970) | 99381063719 | 9745715186; no diagnostic file | — | 440.585 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| control | 1 | 2 | [33356915983](https://github.com/portpowered/you-agent-factory/actions/runs/33356915983) | 99381031153 | 9745682014; no diagnostic file | — | 481.061 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| diagnostic | 4 | 3 | [33356916239](https://github.com/portpowered/you-agent-factory/actions/runs/33356916239) | 99381102075 | 9745724679; `coverage-build-diagnostics.json` | 305.377 | 305.377 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| control | 4 | 1 | [33356916302](https://github.com/portpowered/you-agent-factory/actions/runs/33356916302) | 99381113336 | 9745738959; no diagnostic file | — | 311.144 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| control | 1 | 3 | [33356916185](https://github.com/portpowered/you-agent-factory/actions/runs/33356916185) | 99381199219 | 9745920874; no diagnostic file | — | 439.731 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| diagnostic | 4 | 1 | [33356916103](https://github.com/portpowered/you-agent-factory/actions/runs/33356916103) | 99381199510 | 9745903639; `coverage-build-diagnostics.json` | 309.098 | 309.098 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| diagnostic | 4 | 2 | [33356916178](https://github.com/portpowered/you-agent-factory/actions/runs/33356916178) | 99381180962 | 9745836442; `coverage-build-diagnostics.json` | 246.872 | 246.872 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| diagnostic | 1 | 2 | [33356916051](https://github.com/portpowered/you-agent-factory/actions/runs/33356916051) | 99381238422 | 9745966479; `coverage-build-diagnostics.json` | 415.174 | 415.174 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| diagnostic | 1 | 3 | [33356916069](https://github.com/portpowered/you-agent-factory/actions/runs/33356916069) | 99381250371 | 9746063004; `coverage-build-diagnostics.json` | 582.640 | 582.640 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| control | 4 | 2 | [33356916310](https://github.com/portpowered/you-agent-factory/actions/runs/33356916310) | 99381242763 | 9745965836; no diagnostic file | — | 308.866 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |
| control | 4 | 3 | [33356916131](https://github.com/portpowered/you-agent-factory/actions/runs/33356916131) | 99381278545 | 9746040698; no diagnostic file | — | 312.564 | 80.7% | incomplete; 11,560 tests; 57 skips | rejected |

All twelve completed timing artifacts report `packageCount=445` and
`expectedPackageCount=445`, `testPassCount=11503`, `testFailCount=0`, and
`testSkipCount=57`. They report `complete=false` with the same capture reason:
`unit timing capture could not read every go test event, so some per-test rows
may be missing`. Their coverage summaries report 480 measured coverage
packages, not the required 445-package evidence universe, and their timing
package-state rows do not expose cached/unknown status. These are direct
reasons for rejection, not a substitute for the missing evidence.

The completed diagnostic observations report `expectedPackages=445`,
`compilerCommands=2087`, `linkerCommands=445`, and `buildActions=2532`.
Their invocation hashes are respectively
`sha256:e39f89958fb59fc2e03287be130b31df125103f09e43e5c1cd1c006c4af2b009`
for jobs 1 and
`sha256:0f4d3d21a7577dbe387bbec48624e064275ad6204f9c547ea5ca7b2c8b54ead0`
for jobs 4, demonstrating that the requested jobs value reached the
authoritative invocation. The setup-go log for the first diagnostic records
the exact key
`setup-go-Linux-x64-ubuntu24-go-1.25.0-70c8dd24106c110416ea09866dce4ff9e81bf705c128680b5f66ebeb8f4fa90b`,
an exact hit, and a cache archive of approximately 0 MB / 7,439 bytes. The
diagnostic JSON itself has empty primary and matched action-cache keys, so the
run does not establish populated Go build-cache content.

All twelve unit jobs completed successfully, but seven enclosing workflows
were canceled before their unrelated checks finished: runs `33356916051`,
`33356916069`, `33356916185`, `33356916103`, `33356916310`, `33356916131`, and
`33356916178`. Their successful unit artifacts are included above; the
canceled workflow conclusions are not samples of the required terminal CI
state. No enclosing workflow conclusion is used as a cohort result.

## Cohort and attribution admission

No jobs=1 or jobs=4 cohort has three valid diagnostic samples, so no median is
published. The completed rows are intentionally not paired by sample identity
for diagnostic-overhead subtraction; no diagnostic overhead is claimed. The
observed wall values also do not support a collapsed-parallelism verdict while
the shared timing and package invariants are invalid.

The admitted attribution table is empty. Its required row shape is published
below so an independent validator can reject malformed future additions:

| Stable identity | Bucket | Measured seconds | Measurement source | Measured-or-inferred | Verdict | Owning lane | Blocked-by |
| --- | --- | ---: | --- | --- | --- | --- | --- |
| *(no admitted records)* | — | — | — | — | — | — | invariant-preserving same-head cohort unavailable |

No bucket is encoded as zero, inferred, or arithmetic residual. The required
five infrastructure buckets (`compile`, `link`, `covdata`, `merge`, and
`evaluate`) remain unmeasured. The expected 445 package classification rows
are likewise empty because the required timing corpus is not admissible; no
package is credited with conversion. The existing Factory Sessions pilot
remains `KEEP-AS-IS` at 10.63 seconds because at most 10.63 seconds is
available on the 649-second path.

## Terminal disposition

The operator amendment authorizes a partial or blocked result with evidence as
complete delivery after a genuine attempt. This ledger therefore closes the
hosted-attribution attempt as `BLOCKED`, records exactly what was obtained,
and leaves corrective instrumentation for a separately owned lane. It does
not claim valid medians, bucket measurements, 445 package verdicts, or a
terminal final-head CI result.
