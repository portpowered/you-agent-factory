# Walker Contract Guard Inventory

This inventory records every `*_contract_guard_test.go` file in the repository
that currently performs a filesystem walk. It makes the guarded root, intended
validation surface, and current exclusion behavior explicit before the
follow-on cleanup stories align skip policy and diagnostics.

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch `ralph/inventory-remaining-contract-guard-walkers`)
- Owner: Codex branch `ralph/inventory-remaining-contract-guard-walkers`
- Reviewers: Infinite You maintainers
- Packages or subsystems: `pkg/api`, `pkg/config`, `pkg/interfaces`, `pkg/petri`

## Inventory

| Guard file | Walk root | Validation surface | Contract class | Current exclusions | Notes |
| --- | --- | --- | --- | --- | --- |
| `pkg/api/legacy_model_guard_test.go` | module root (`../..`) | All handwritten `.go` files under the module while forbidding deleted legacy replay/event types and generated-type aliases. | `handwritten_module_scan` | `pkg/api/generated`, `ui/dist`, `ui/node_modules`, `ui/storybook-static` | This is the hardened baseline for broad handwritten-source scans because the root includes generated API output and built UI artifacts. |
| `pkg/config/exhaustion_rule_contract_guard_test.go` | `pkg/` (`..`) | Production `.go` files under `pkg/` while keeping retired authored exhaustion identifiers deleted and limiting `petri.TransitionExhaustion` ownership. | `handwritten_pkg_scan` | hidden directories, `pkg/api/generated`; excludes all `_test.go` files from the walked surface | Broad package scan of handwritten production code. The package-root scope still skips hidden metadata so nested worktree or editor artifacts cannot become authored-source inputs later. |
| `pkg/interfaces/runtime_lookup_contract_guard_test.go` | `pkg/` (`..`) in the production guard; temp fixture roots in focused regression tests | All `.go` files under `pkg/` when checking runtime-lookup interface ownership and raw `FactoryDir` / `RuntimeBaseDir` escape hatches. | `pkg_ownership_scan` | hidden directories, `pkg/api/generated` | This remains broader than production-only scans because the contract also forbids test-local escape hatches under `pkg/`, but generated API output and hidden metadata stay outside the ownership surface. |
| `pkg/interfaces/world_view_contract_guard_test.go` | package root (`.`) for boundary-mirror checks; `pkg/` (`..`) for canonical-mirror checks | Package-local `pkg/interfaces/*.go` files for boundary-only mirror names, plus all `.go` files under `pkg/` for retired canonical mirror names. | `mixed_package_and_pkg_scan` | hidden directories for both walk scopes; `pkg/api/generated` on the broader `pkg/` scan; explicit allowlists for the guard file itself | This file contains two walker scopes: a narrow package-local scan and a broader `pkg/` scan used to keep retired canonical mirrors out of the rest of `pkg/`. |
| `pkg/petri/transition_contract_guard_test.go` | module root (`../..`) | All handwritten non-test `.go` files under the module while forbidding retired runtime-owned `petri.Transition` literal fields. | `handwritten_module_scan` | `pkg/api/generated`, `ui/dist`, `ui/node_modules`, `ui/storybook-static`; excludes all `_test.go` files from the walked surface | Shares the same broad-root handwritten-source shape as the API legacy-model guard, but with petri-specific ownership rules. |

## Current Shape

- `handwritten_module_scan` guards walk the full module and therefore must
  classify generated or built artifact directories explicitly.
- `handwritten_pkg_scan` and `pkg_ownership_scan` guards start at `pkg/`, so
  they avoid repository-root metadata by construction, but still need explicit
  hidden-directory and generated-output exclusions for the package tree they
  intentionally inspect.
- `pkg/interfaces/world_view_contract_guard_test.go` is the only targeted file
  with more than one walk scope; future cleanup work needs to preserve the
  distinction between the package-local mirror-name rule and the broader
  `pkg/` canonical-mirror rule.

## Helper Decision

- The repeated directory-exclusion predicates now live in
  `internal/testpath/contract_guard_walkers.go`.
- The shared helper stays intentionally small: it only answers whether a walked
  directory is outside the handwritten-source surface for either module-root or
  `pkg/`-root scans.
- Each guard still owns its own walk root, file filtering, allowed-file
  exceptions, and failure messages at the callsite, so package semantics remain
  local even though the repeated skip predicates no longer drift independently.

## Story-Relevant Conclusions

- The targeted walker inventory is limited to `pkg/api`, `pkg/config`,
  `pkg/interfaces`, and `pkg/petri`; no other `*_contract_guard_test.go` file
  currently uses `filepath.Walk` or `filepath.WalkDir`.
- The broad handwritten-source baselines are the module-root walkers in
  `pkg/api/legacy_model_guard_test.go` and
  `pkg/petri/transition_contract_guard_test.go`.
- The next alignment pass should compare `pkg/config` and `pkg/interfaces`
  walkers against those module-root baselines without forcing unrelated
  exclusions onto package-local ownership scans.

## Closeout

- Date: `2026-04-30`
- Targeted guard set matches the checked-in inventory above: the repository
  walker surface remains limited to `pkg/api/legacy_model_guard_test.go`,
  `pkg/config/exhaustion_rule_contract_guard_test.go`,
  `pkg/interfaces/runtime_lookup_contract_guard_test.go`,
  `pkg/interfaces/world_view_contract_guard_test.go`, and
  `pkg/petri/transition_contract_guard_test.go`.
- Verified package slice:
  `go test ./pkg/api ./pkg/petri ./pkg/interfaces ./pkg/config -count=1 -timeout 300s`
- Verified repository quality gate:
  `make lint`
- Result: the targeted package slice is green, the shared skip-helper inventory
  still matches the guarded handwritten-source surface, and the closeout proof
  stays attached to the same checked-in document maintainers use for future
  walker drift reviews.
