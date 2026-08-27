# Factory Sessions execution inventory

This versioned inventory reconciles the accepted v2 evidence with the current
checkout without changing Factory Sessions execution code or tests.

## Count units

The accepted v2 artifact contains 285 root-package terminal test-event
identities. A fresh current root `go test -json` capture contains the same
285 identities; the canonical sorted-list SHA-256 is
`817a564b21a1047b026d05bd9851b15dc1a952f2751fb40ec37c648961c57ec0` for both sets.

The package declaration inventory is a different unit. Top-level
`go test -list '^Test'` declarations are counted separately for the four
scoped packages:

| Package set | Planning | Current | Difference |
| --- | ---: | ---: | ---: |
| Root execution | 173 | 190 | +17 |
| Fixtures | 106 | 106 | 0 |
| Recording/replay | 18 | 18 | 0 |
| Runtime persistence | 7 | 8 | +1 |
| **Total declarations** | **304** | **322** | **+18** |

Terminal-event counts and declaration counts must not be subtracted. Therefore
the accepted 285-event artifact cannot prove the PRD's requested 19-addition
scalar. The executable declaration comparison has zero planning-only names and
exactly 18 current-only names, each mapped in the JSON artifact.

## Exact evidence

- Accepted source: read-only
  `unit-test-optimization-c01-wire-timeout-witness` at
  `docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-1-replacement.v2.json`,
  run commit `ba8ef900ee29347295ac7657742fd1aab42f064c`.
- Planning declarations: `git grep -n -E '^func Test[A-Za-z0-9_]+\\('`
  at `270965c35b70b20cb3e5d1e91598d313d775b043`, classified by direct package.
- Current declarations: `go test -list '^Test'` for each scoped package at
  `5748eca459935e78fa9d93aa370fa4bfbf00b4d2`.
- Current root terminal events: `go test -json -count=1
  ./pkg/services/factory_sessions/internal/execution`, exit 0, 285 unique
  terminal identities.

The JSON file contains the full sorted accepted/current root event lists, all
304/322 declaration names per package, canonical hashes, the empty
planning-only set, and the 18 source/behavior mappings.

## Evidence boundary

The focused characterization and three serial pre-change timing samples remain
recorded in `progress.txt`; this artifact does not claim post-change
equivalence, race safety, shuffle convergence, or performance improvement.
Stories 002–004 are routed to later work after inventory review.
