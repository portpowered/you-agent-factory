# Qualified construction metadata

`ConstructionRegistry` is immutable input to the static checkers. Each scan
builds a fresh authored-source index; it never constructs a service, loads a
runtime graph, or consults a mutable registry. The package-boundary, durable
construction, and logging commands consume the same validated observations
while retaining their existing rules and diagnostics.

Required-dependency nil comparisons are reported in registered constructors
and methods whose fields are initialized from those parameters through keyed
result literals. Local declaration aliases and closures retain that provenance;
shadowed variables and optional domain fields do not acquire requiredness.
Reassignment of a traced parameter, alias, or method field makes a guard an
`unresolved-required-dependency-guard` observation. This debt blocks an enforced
set, just like a proved guard, and remains nonblocking in report mode.

Direct calls resolve dot-imported authored functions and instantiated generic
functions while preserving local shadowing. Constructor values passed or stored
for later execution still produce unresolved-reference debt. This bounded scan
does not yet establish helper summaries, imported type aliases, method-valued
constructors, field storage through assignment, type-assertion fallbacks,
dependency bags, service locators, or complete effect ancestry. Those remaining
story-002 cases must be proved before a capability set can claim complete
enforcement coverage.

Symbols identify an import path, optional receiver type (without a pointer),
and declaration name. Required parameter indices count individual parameters,
including grouped declarations, and do not depend on parameter names. Named
types in parameter expressions use their full import paths. Constructor results
list the ordered named result declarations, stripping pointers and omitting
`error`. Each result requires a matching construction-kind classification in
the same capability set. Other result forms require additional analysis before
they can be registered.

Allowances identify one caller declaration, one registered callee, and one
repository-relative file. Validation requires that exact call in authored
source, a recognized semantic kind, an owner, and a rationale. The initial
permanent focused-provider allowance covers the Chat Sessions catalog provider,
whose body directly constructs its implementation. Migration allowances need a
removal owner. Wildcards, directories, stale declarations, duplicate entries,
conflicting classifications, and ambiguous platform declarations are rejected.

The initial repository set is report-only. Its metadata comes from the Chat
Sessions catalog implementation and its authored owner Wire provider. Report
observations do not change existing gate status or enter deletion-only finding
baselines. Enforced observations count separately from prior rules. No owner is
enabled by this increment; readiness and full provenance checking belong to
subsequent rollout work.

The source surface is handwritten `cmd`, `internal`, and `pkg` Go source.
Generated files, `_test.go` outer-edge fixtures, vendor, testdata, hidden
metadata, and build output are excluded. Compiled helpers such as
`pkg/transports/http/servertests` remain production-policy input. All authored
platform variants are indexed; multiple declarations of a registered identity
are classification failures, rather than an arbitrary target selection.

This first scan resolves direct local functions and named import selectors,
retains enclosing operations across closures, and distinguishes lexically
shadowed imports. Registered function references report
`unresolved-construction-reference` debt. It does not yet prove dependency
storage, helper summaries, dot imports, imported type aliases, generic wrappers,
method-call receivers, guards, or effect ancestry. A clean report for the initial
set is not whole-repository enforcement or runtime behavior evidence. Those
limits must be closed before enabling a capability set.
