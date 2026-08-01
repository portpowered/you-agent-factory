# Reconcile coverage baselines after Go package moves

Go package moves and service-boundary reorganizations can leave the unit and
functional coverage manifests out of sync with the packages measured by the
coverage profile. The resulting CI failures are easy to misattribute to the
current PR and can span unrelated service families.

Recent CI evidence includes:

- stale package paths that must be reordered or replaced after a move;
- newly measurable packages missing from a manifest;
- zero-coverage moved root packages that need an explicit measurement
  exception or focused tests; and
- small real floor regressions mixed with the path drift.

Define a package-move reconciliation workflow that derives or validates the
manifest taxonomy against the measured profile, distinguishes path drift from
real coverage regressions, and preserves the policy that real floor regressions
must be fixed with tests rather than hidden by lowering minima. Add focused
checks so a structural package change reports the affected old/new paths and
the required manifest action before a feature PR reaches the full CI gate.
