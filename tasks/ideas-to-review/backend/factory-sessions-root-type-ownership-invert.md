# Invert Factory Sessions execution type ownership without import cycles

## Problem

Closing `type X = internal/execution.X` and `type Y = internal/contracts.Y`
re-exports from the Factory Sessions root is required so peers treat plain root
contracts as the source of truth. A naive invert fails:

`factory_sessions` → `internal/execution` → `internal/contracts` → `factory_sessions`

when contracts are changed to alias from the root while the root still imports
execution for type aliases.

## Why it matters

CTR-SES story work repeatedly hits this wall: opening/binding can publish plain
root vocabulary, but full re-export closure cannot land without either keeping
aliases or completing a coordinated ownership invert.

## Proposed direction

1. Move durable execution request/result/error type bodies onto the
   `factory_sessions` root (or a root-owned types file with no import of
   `internal/execution`).
2. Change `internal/execution` to consume root types (aliases or direct imports).
3. Only then invert `internal/contracts` to alias from the root.
4. Keep function helpers that must stay in execution callable without making
   nested packages the peer-facing contract source.

## Non-goals

- Broad IMP-SES nested capability rewrites.
- Transport/OpenAPI redesign beyond compile fixes forced by type ownership.
