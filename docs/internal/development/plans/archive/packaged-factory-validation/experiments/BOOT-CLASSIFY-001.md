# BOOT-CLASSIFY-001

## Identity

- Status: `PASSED`
- Factory: `@you/classify`
- Repository base: `d072b6475633ea2aff585eb82d2de0459d3d8b69`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-CLASSIFY-001`
- Model for every role: provider `CODEX`, model `gpt-5.6-terra`
- Current packaged artifact:
  `packages/packaged-factories/generated/factories/classify/factory.yaml`
- Accepted recordings and SHA-256 values:
  - canary `BOOT-CLASSIFY-001-R01.replay.json`:
    `EAE0B6F01C5731C2EC21E9FFFC21A9AA4A5368743C4ADEB24714783DD52BEB4A`
  - representative `BOOT-CLASSIFY-001-R02.replay.json`:
    `550E7B4620A7B6752C878ABBB14CDB614716BE7408C43C1C55C7C8B14357ECD4`

## Canary

The request asked for one read-only README fact. The replay dispatched
`classify-request` and then `execute-small`. The result correctly identified
the product in one sentence, cited the requested file, and completed in 20
seconds.

## Representative workload

The request asked for a multi-step conformance assessment of all immediate
`pkg` families against `docs/architecture/packaged-structure.md`. The replay
dispatched `classify-request` and then `execute-medium`. The result enumerated
the exact six directories, inspected and cited a representative implementation
from every family, and correctly found no discrepancy. It completed in 55
seconds.

Both runs had exactly one classifier and one selected-lane dispatch. The
worktree had no tracked changes or unrequested artifacts; only documented
runtime session state was created.

## Decision

- Canary status: `PASSED`.
- Representative status: `PASSED`.
- Goal status: `MEETS_EXPECTATIONS`.
- The live evidence covers small and medium routing. Deterministic functional
  coverage separately proves all three lanes and invalid-label failure.
