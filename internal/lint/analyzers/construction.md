# Construction analysis

`RegisteredConstruction` and `DurableConstruction` run in the shared
golangci module plugin. `make golangci` invokes the pinned built-in linters and
strict shared analyzers in ordinary and complete-tag configurations, including
baseline growth. The temporary `make repolint` dependency retains compiler
ownership checking until its analyzer migration. T20/T29 consumers use these analyzers. The former
`internal/contractguard` construction Scan APIs and durable command are removed.

## Metadata and staged policy

`RepositoryConstructionRegistry()` supplies immutable metadata from
`construction_registry.go`. Each compilation unit validates the declarations
it owns using `pass.Pkg` and `pass.TypesInfo`. Symbols identify the original
import path, receiver type without a pointer, and declaration name. Required
parameter indices count individual parameters, including grouped names;
parameter type expressions use qualified names. Constructor results require
matching construction-kind classifications in the same capability set.

For compiled handwritten references and declarations, constructor names
(`New`, `Build`, `Create`, `Ensure`, `Open`, `Provide`) returning an already
classified behavior/effect also inherit that policy without another constructor
entry. Compiler identities preserve generic origins and named aliases. Typed
classified behavior/effect parameters become required inputs; explicit
constructor metadata takes precedence. A multi-result constructor inherits
enforcement if any classified result is enforced. This does not classify a new
owner or turn domain/state/resource results into services.

Allowances name one caller, callee, and repository-relative file, plus a
semantic kind, owner, and reason. Malformed, duplicate, stale, and mismatched
metadata fails as `construction-metadata`. A focused-provider allowance permits
only synchronous construction in the approved body. Constructors invoked by
`defer`, `go`, or closures remain prohibited. Arguments evaluated before a
deferred or asynchronous operation retain synchronous ownership.

The `chat-target-catalog` capability set remains report-only. The analyzer
returns `[]ConstructionFinding` with mode, set, caller, callee, file, line, and
rule. Report observations do not emit blocking diagnostics or enter the
baseline. Enforced findings and conservative unresolved debt use the shared
exact `baseline.txt`; stale entries fail and the `baselinegrowth` analyzer rejects
growth. No owner is enabled by this migration.

## Bounded typed rules

| Rule | Observation |
| --- | --- |
| `registered-construction` | Classified behavior/effect construction outside an exact allowance, indirect provider construction, or a proved provider recursion path. |
| `unresolved-construction-reference` | Escaped, unused, stored, or mutated constructor references. |
| `required-dependency-bag` | Required records or containers that contain classified collaborators, including nested/embedded fields and map keys. Explicit domain/state/resource classifications remain allowed. |
| `required-dependency-guard` | Nil comparisons of required constructor parameters or their traced storage/helper origins. |
| `required-receiver-guard` | Nil comparisons of classified behavior/effect receivers and their aliases. |
| `required-dependency-assertion-guard` | Branches on required collaborator assertion status; merely observing status remains allowed. |
| `unresolved-required-dependency-guard` | Guards with mutated, conflicting, recursive, tuple, or opaque required provenance. |
| `service-getter-locator` | Calls to zero-argument getters returning classified collaborators from required storage. Parameterized views and optional/domain/state/resource results remain allowed. |
| `unresolved-service-getter-locator` | Calls to getters with ambiguous, named, tuple-helper, or mutated required return provenance. |
| `unresolved-service-getter-reference` | Escaped, unused, or mutated getter references. |
| `unresolved-focused-provider-dispatch` | Opaque callback, function-field, interface method, or unsupported callable-return execution on a focused provider path. |

Compiler objects preserve aliases, dot imports, generic function origins,
promoted methods, method values/expressions, pointer receivers, and shadowing.
Object facts carry classified ancestry and getter observations across imports;
dependency source is not indexed. Required storage follows keyed/positional
result literals, grouped/embedded fields, assignments, and local owner aliases.
Same-package helper parameter requiredness reaches a finite fixed point. Helper
return summaries skip nested closure returns and retain unresolved origins
instead of selecting an arbitrary return value.

Provider recursion follows compiled same-package helpers, concrete methods,
immutable function aliases, and invoked closures. Uncalled closures, unused
helpers, local shadows, and cycles that do not return to the provider do not
prove provider recursion. Recursion takes precedence over dispatch debt.
Bounded callable-return summaries require one unnamed literal function result
and the same declaration or authored closure in every explicit return. Named,
tuple, mixed, cyclic, mutated, escaped, or imported return identities retain
dispatch debt. Interface method aliases, including promoted methods, retain
interface dispatch debt; concrete method aliases preserve declaration identity.

`DurableConstruction` preserves the existing six durable rule families and
exact file/test exemptions: runtime construction, persistence construction,
persistence booleans, application composition, canonical events, and live-child
provider execution. Generated source and non-transport test doubles remain
excluded. Transport tests retain the legacy composition exception.

## Scope and remaining enforcement

Type-dependent rules analyze compiled handwritten `cmd`, `internal`, and `pkg`
source. Generated files and registered-construction `_test.go` fixtures remain
excluded; compiled `servertests` helpers remain in scope. The canonical tags
are `functionallong,backendconformance,factoryartifact,managed_process_integration`.
Excluded import edges alone may use `parser.ImportsOnly`; no excluded call-body
proof is claimed. Native platform execution owns platform-dependent bodies.

Unlisted constructor discovery is bounded by the existing constructor-name
vocabulary and exact named result classifications. Arbitrarily named factories,
structural/erased results, distinct defined-type ancestry, callable variables,
and opaque cross-package factories are not proved by this inference. Parameter
requiredness is inferred only for exact classified named collaborators, not
unclassified callbacks or containers. These limits remain inventory obligations;
they authorize no new baseline or exemption. Repository coverage and allowance
retirement remain incomplete while the registry has only a report-only owner.

Compiler-invalid historical examples are rejected before lint analysis:
initialization cycles, ambiguous selectors, recursive aliases, and undefined
interface embeddings have explicit compiler fixtures. They are not successful
analyzer cases. Static fixtures do not prove runtime execution, session
isolation, replay, shutdown, retention, or performance.

T20 still owns the full N/X matrix, arbitrary callable/field/callback and
cross-package dispatch, remaining named/tuple/storage/assertion provenance,
effect ancestry, topology rules, and progressive owner enablement. T29 owns
final repository scope and removal of migration allowances. Independent
G-LOOPBACK and Project validation own integrated acceptance; a clean initial
report is not full T20/T29 or Project acceptance.
