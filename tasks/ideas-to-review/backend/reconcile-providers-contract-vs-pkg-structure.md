# Reconcile `providers/contract` taxonomy with `pkg-structure`

## Resolution

The taxonomy is now aligned on the provider-owned destination:

- `tests/functional/README.md`, the `internal/testlanes` functional-package
  policy, and
  `make functional-boundary-check` treat `tests/functional/providers/contract/`
  as the dedicated home for provider-neutral extension scenarios (and forbid
  new root-level aggregate `providers/*_test.go` files).
- `make pkg-structure` / `allowedFunctionalDomains` explicitly approve
  `providers` when the source is below a subsection, including
  `providers/contract`.

Root-level `providers/*.go` files remain deletion-only aggregate debt, so the
two gates still distinguish dedicated provider destinations from the legacy
aggregate package.

## Verification

The structure-gate and functional-boundary tests cover
`tests/functional/providers/contract/...` as an accepted dedicated destination
and continue to reject new root-level aggregate provider tests.
