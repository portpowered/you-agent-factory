# Split World-State Generated Adapters

## Why

The active local maintainer ask is to keep backend files under 1000 lines.
There are currently no handwritten backend Go files over that threshold, but
`pkg/factory/projections/world_state.go` is already at 992 lines and carries an
explicit function-length exception for the generated initial-structure adapter.

This makes `world_state.go` the narrowest implementation-ready cleanup target:

- it is the closest handwritten backend production file to the 1000-line
  threshold
- the oversized area is already identified by a checked-in exception comment
- the file already sits beside sibling projection files, so extracting helper
  ownership matches the existing package structure

## Problem

`pkg/factory/projections/world_state.go` currently mixes three different
responsibilities:

1. world-state reconstruction and reducer orchestration
2. topology and relation lookup helpers
3. generated-to-domain projection adapters for initial structure, work items,
   work content, and relations

The cleanest split seam is the pure helper block centered on
`initialStructureFromGenerated`, plus the adjacent work-item and work-content
converters that only translate generated API payloads into projection-domain
values. The relation helpers can stay in `world_state.go` for now because they
still depend on reducer state and are a weaker first split.

## Requested Change

Split the generated adapter logic out of
`pkg/factory/projections/world_state.go` into focused projection helper files,
while preserving all current runtime behavior.

Implementation guidance:

- keep `ReconstructFactoryWorldState`, the reducer type, and reducer mutation
  methods in `world_state.go`
- move pure generated-to-domain adapter helpers into new sibling files under
  `pkg/factory/projections/`
- remove the now-obsolete function-length exception once the extracted helper
  block is no longer oversized
- do not add new abstraction layers or compatibility wrappers; this should be a
  file-ownership cleanup, not a redesign

Reasonable split targets:

- one file for initial-structure conversion helpers
- one file for work-item and work-content conversion helpers
- keep reducer-state relation lookup helpers where they are unless the follow-up
  needs a second split after the pure adapter extraction

## Guardrails

- preserve the current projection outputs exactly
- preserve nil-versus-empty behavior for content, tags, and optional slices or
  maps
- preserve existing event-order and reducer semantics
- keep package APIs unchanged outside the `projections` package
- do not change generated files

## Verification

Add or update focused tests only where needed, preferring behavior assertions
over shape tests:

- run the existing `pkg/factory/projections` test suite
- keep world-state reconstruction and dashboard-facing projection assertions
  green without changing their observable outputs
- if helper extraction changes any edge-case handling, cover it with direct
  package tests around the extracted pure conversion helpers
- existing coverage already exercises the intended seam through
  `world_state_test.go` and `world_state_support_test.go`, especially the
  initial structure, route-array mapping, resource seeding, and work-item
  reconstruction paths that depend on these generated adapters
