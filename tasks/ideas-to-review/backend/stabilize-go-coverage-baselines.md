# Stabilize Go coverage baseline verification

`make test-unit-coverage` and `make test-functional-coverage` can fail on
fractional per-package baseline drift even when the selected test suites pass
and the change does not touch Go behavior. The drift has appeared in unrelated
packages across independent CI-lane work, which makes coverage verification a
recurring mergeability risk.

Investigate and remove the source of nondeterminism without weakening the
coverage policy. Preserve the independent unit and functional thresholds and
package-level regression protection. Prefer a deterministic test/coverage
collection boundary, stable package sequencing, or another behavior-preserving
fix backed by repeated focused lane runs.
