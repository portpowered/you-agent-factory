# Reconcile `providers/contract` taxonomy with `pkg-structure`

## Problem

Two enforced layouts disagree for provider-neutral functional proofs:

- `tests/functional/README.md`, `internal/testlanes` required packages, and
  `make functional-boundary-check` treat `tests/functional/providers/contract/`
  as the dedicated home for provider-neutral extension scenarios (and forbid
  new root-level aggregate `providers/*_test.go` files).
- `make pkg-structure` / `allowedFunctionalDomains` treat `providers` as an
  unclassified catch-all. Existing paths are deletion-only baseline debt;
  **new** Go sources under `providers/**` fail CI even when placed in the
  documented `contract` subsection.

Story 5 hit this after moving the fake-custom E2E into `providers/contract/`
to clear functional-boundary feedback: Lint then failed with
`functional-test-unclassified-domain` on the new test file.

## Interim workaround used

Place new provider-neutral conductor/custom-integration proofs under the
approved domain path `tests/functional/workers/inference/`, and keep
`tests/functional/providers/contract/doc.go` so required provider-package
discovery still succeeds.

## Proposed durable fix

Pick one authoritative taxonomy and align the gates:

1. Promote `providers` (or a narrower approved noun such as
   `provider_integrations`) into `allowedFunctionalDomains` with required
   subsection depth, **or**
2. Retarget README / testlanes / functional-boundary guidance to
   `tests/functional/workers/inference/...` (and related approved
   subsections) and stop advertising `providers/contract` for new work,
   then burn down the providers baseline.

Do not keep both “required destination” and “new files forbidden” signals.
