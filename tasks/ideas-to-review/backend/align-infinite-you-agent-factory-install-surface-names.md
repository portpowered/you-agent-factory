# Align Infinite You And Agent Factory Install Surface Names

## Problem

The repository currently exposes conflicting names across installation surfaces:

- product-facing docs use `infinite-you`
- the Go module path is `github.com/portpowered/agent-factory`
- packaged release artifacts and CLI help use `agent-factory`
- `go install ./cmd/factory` installs a `factory` binary because the entrypoint lives under `cmd/factory`

This mismatch makes consumer installation guidance harder to trust and forces
release tests plus docs to explain exceptions instead of presenting one clear
command contract.

## Why It Matters

- Consumers can install one name and then be told to run another.
- README, CLI help, release artifacts, and `go install` guidance are harder to keep aligned.
- Future install-surface work will keep tripping on the same naming drift unless there is one canonical product, module, and binary contract.

## Suggested Direction

- Decide the canonical public binary name and module path for the product.
- Define whether `cmd/factory` remains the long-term public install path or becomes a compatibility shim.
- Update release packaging, Homebrew metadata, installer docs, and CLI command docs to one naming contract.
- Add a repo-owned guard that fails when README/install docs, CLI help, and release artifact names drift away from that chosen contract.
