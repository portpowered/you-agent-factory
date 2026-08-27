# Unit-lane v2 baseline

This is the accepted pre-optimization baseline for
`unit-test-optimization-c01-wire-timeout-witness-003`. The frozen reference
base is `9e19e26e0fb6df47cfdd4c4d4469ce712aae04ff`. The counterfactual tree
adds only the measurement commits `bb83727ff313077751465c1aefbecfd8136e1372`
and `ba8ef900ee29347295ac7657742fd1aab42f064c`; `git diff` confirms that no
`pkg/wire` file is present in that change.

## Procedure and identity

The capacity and lane settings were recorded with:

```text
make -s print-go-parallelism YOU_LOGICAL_CPUS=4 YOU_EXPECTED_CONCURRENT_LANES=4 UNIT_DEFAULT_JOBS=2
```

The three serial captures used the canonical `make test-unit-fresh` entry
point, `UNIT_DEFAULT_JOBS=2`, `computedLaneBudget=2`, `-count=1`, and unique
v2 timing output paths. They ran locally in Ubuntu 24.04.1 WSL2 with Go
1.25.0, Linux/amd64, and the CPU identity recorded in
[baseline-make-environment.v2.json](baseline-make-environment.v2.json).
No environment invalidations were declared.

## Accepted samples

| Ordinal | Capture | Commit | Exit | Unit-lane wall (s) | Package elapsed sum (s) | Packages | Tests | Cache / outcome |
| ---: | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | [replacement timing](baseline-make-run-1-replacement.v2.json) | `ba8ef900ee29347295ac7657742fd1aab42f064c` | 0 | 222.006 | 94.512 | 444/444 | 18,122 | 0 cached, 0 unknown, 444 pass |
| 2 | [timing](baseline-make-run-2.v2.json) | `ba8ef900ee29347295ac7657742fd1aab42f064c` | 0 | 239.612 | 114.687 | 444/444 | 18,122 | 0 cached, 0 unknown, 444 pass |
| 3 | [timing](baseline-make-run-3.v2.json) | `ba8ef900ee29347295ac7657742fd1aab42f064c` | 0 | 258.271 | 111.206 | 444/444 | 18,122 | 0 cached, 0 unknown, 444 pass |

The delivery retains each machine-readable v2 timing document. Raw
stdout/stderr and status captures are intentionally excluded from this lane's
delivery diff under the operator evidence policy; they are not inputs to the
baseline checker. The accepted reference median is `239.612s`, recomputed from
exactly these three walls.
The checked-in budget instance contains the complete sorted 444-package and
18,122-test inventories derived from the v2 documents.

The exact baseline validation was:

```text
go run ./cmd/unitlanebudget -mode baseline -samples docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-1-replacement.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-2.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-3.v2.json
```

It exited 0 with `Result: pass`, `Inventory: 444 packages, 18122 tests`, and
`Cache: 0 cached, 0 unknown`.

## Retained invalid attempt

The first attempt remains as the machine-readable
`baseline-make-run-1.v2.json` document. Its raw stdout/stderr and status
captures are intentionally not retained in this delivery. It used the earlier
measurement commit, exited 1, and stopped at 320/446 packages because the
discovery code included the Linux-inactive `filesystemreplace` package. The
invalid timing document records the failure and the single explicit
replacement; no invalid artifact was silently replaced or deleted.

## Evidence boundary

This baseline proves the canonical Make entry point, complete active-Linux
package/test inventory, uncached execution, identity, and pre-optimization
timing distribution. It does not prove the preserved Wire behavior, final
improvement/variance thresholds, targeted race evidence, hosted CI, or
clean-room validation; those remain owned by later stories and gates.
