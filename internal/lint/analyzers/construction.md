# Construction analysis

`RegisteredConstruction` and `DurableConstruction` run in the shared
golangci module plugin. `make golangci` invokes the pinned built-in linters and
strict shared analyzers in ordinary and complete-tag configurations, including
baseline growth and compiler ownership. T20/T29 consumers use these analyzers. The former
`internal/contractguard` construction Scan APIs and durable command are removed.

## Metadata and composition

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

Composition uses compiler-resolved free-function identities in `pkg/wire` or
an exact owning service's `wire` package, with a constructor/provider name.
Methods, ordinary operations and package initializers remain prohibited.
Only synchronous, acyclic construction with resolved dispatch is lawful;
`defer`, `go`, closures and unresolved callable dispatch still fail. Arguments
evaluated before a deferred or asynchronous operation retain synchronous
ownership. Exact `github.com/google/wire.NewSet` and `Build` declarations may
retain direct provider references at those composition boundaries; shadowed
or unrelated callables receive no such treatment. No caller/callee/file
migration allowance schema remains.

The `chat-target-catalog`, `events`, and populated `repository` capability sets enforce. The analyzer returns
`[]ConstructionFinding` with mode, set, caller, callee, file, line and rule.
Report mode remains available to controlled analyzer fixtures, without enabling
an owner or entering the baseline. Enforced findings and conservative unresolved debt use the shared
exact `baseline.txt`; stale entries fail and the `baselinegrowth` analyzer rejects
growth. Other owner classifications remain final-enforcement obligations; this
checker increment does not prove repository coverage. The repository set currently
classifies Provider Sessions' public Service, private inspectionService and HTTP
Adapter/Handler. Its explicit New/NewService inputs require the captured reader;
NewAdapter requires the root; NewHandler requires the adapter and Zap logger.
Adding this set does not classify the remaining repository owners.

## Bounded typed rules

| Rule | Observation |
| --- | --- |
| `registered-construction` | Classified behavior/effect construction outside typed composition, indirect provider construction, or a proved provider recursion path. |
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
are `integration,functionallong,backendconformance,factoryartifact,managed_process_integration`.
Excluded import edges alone may use `parser.ImportsOnly`; no excluded call-body
proof is claimed. Native platform execution owns platform-dependent bodies.

Unlisted constructor discovery is bounded by the existing constructor-name
vocabulary and exact named result classifications. Arbitrarily named factories,
structural/erased results, distinct defined-type ancestry, callable variables,
and opaque cross-package factories are not proved by this inference. Parameter
requiredness is inferred only for exact classified named collaborators, not
unclassified callbacks or containers. These limits remain inventory obligations;
they authorize no new baseline or exemption. Repository coverage remains incomplete: Chat and Events classifications do
not cover the remaining owner, Platform effect and transport capabilities. Composition identity does not prove upstream graph reachability,
unused-provider absence, or imported helper bodies. Those properties retain
their existing Wire, deadcode and independent review gates.

The maintained `ci-smoke` cohort uses the production registry with minimal,
compiler-valid declarations for the Chat catalog and Events. Its
ordinary and complete-tag clean/seeded/recovered plugin invocations distinguish lawful owning Wire
construction and scoped map allocation from same-owner construction,
cross-owner construction, and an unlisted constructor returning a classified
type. Events also checks required logger guards independently of the legitimate
non-positive retention default and topic-state allocation. These fixtures prove
diagnostics and qualified symbols for those sets; they do not prove that
unclassified repository owners conform.

Events fixtures also trace an unlisted constructor's required `events.Service`
parameter into stored peer state. Seeds exercise the parameter guard, stored
guard, zero-argument getter call and escaped getter reference. A parameterized
peer view remains lawful, including through an imported embedded Store. The
consumer's promoted getter call and escape must retain the original qualified
Store method identity through compiler object facts. The fixture adds no production constructor or owner
classification; it proves the existing registry's typed guard/getter rules.

The same production-registry fixtures distinguish an unlisted generic Store
constructor and a dot-imported constructor call from a local function shadow
with the same name. Generic and dot-imported calls retain the original qualified
constructor identity; the unrelated shadow remains lawful. The plugin smoke
controls seed and recover these calls in both tag configurations. Execution
evidence, rather than fixture presence, determines the plugin proof status.

Final inventory auditing must distinguish retired construction paths from
surviving requiredness guards. Provider Sessions PR #3132 removed the captured
reader and named HTTP requiredness guards; its existing context and identity
validation remains lawful. Operator Settings PR #3133 removed the constructed
root's receiver/effect guards, including LoadDocument, ApplyDocumentUpdate and
ResolveEffective. PR #3137 (merge
`6415826884a338ceac875e8b892b690921ac9bcc`) subsequently removed the private
document receiver, storage/codec/catalog and Providers-query requiredness guards,
and retired `RootBinding`/`NewAdapterFromRoot`. Optional `diagnosticDecoder` and
`preserveUnknown`, request/domain validation, cancellation and resource errors
remain lawful. These source corrections do not classify Settings automatically.

Work still has requiredness findings in `work/internal/service.go`:
`(applicationService).runtime` checks the constructed receiver and injected
`runtimes`; `SubmitFileForSession` checks injected `readSubmittedFile`.
`NewService` stores these completed inputs directly; canonical
`pkg/wire/runtime_inputs.go:provideWorkService` supplies them, with
`provideWorkSubmittedFileReader` selecting the host override or `os.ReadFile`.
The runtime returned by `ResolveWorkRuntime` may legitimately be unavailable
for a session; its resource/domain absence check must remain. Required receiver
and stored-input checks need the original T05 owner's correction. T29 cannot
suppress them, classify them as scoped state or edit production in this lane.

The Provider Sessions smoke extension uses the unchanged full production
registry. Both tag configurations add same-owner and new cross-owner construction,
captured-reader and Zap-logger guards, getter calls/escapes, lawful Wire construction
and a parameterized view. A local Zap declaration is a controlled signature
fixture, not proof of logging behavior. Execution evidence belongs in PR comments.
The S1-S9 full-repository witness remains incomplete while other roles are unclassified.

Receiver/storage requiredness now follows authored concrete return values behind
classified interface results, including immutable aliases and same-package
helpers whose required parameters flow from the constructor. Compiler field
identities connect keyed/positional storage to concrete receivers and getters;
returned identities stay local to the compilation unit and never classify all
interface implementers. Nested closure returns and unrelated implementations
are excluded. The compiler-valid `interfacestorage` fixture protects required
receiver/storage guards, mutations, getter calls/escapes and lawful optional or
parameterized views. The maintained Provider Sessions plugin smoke also seeds
receiver and stored-reader guards behind its public interface return.

Named/bare returns, mutated result aliases, interface conversions, parameter-return
helpers, opaque/imported helper bodies and recursive return summaries still lack
complete concrete-result identity tracking. Existing unresolved dependency rules
remain applicable where an origin is known; absence of a finding on unsupported
return identity is not proof. Multi-result paths use declared assignability to
select matching return slots, not an inventory of every interface implementer.
Build tags/platforms limit bodies to the actual compiler-selected source, and
arbitrary factories or erased/structural types still need explicit classification.
These limits grant no exemption for known production findings.

Events classifies the public `events.Service`, private `service.Store` and
private `topicState` as behavior, behavior and scoped state respectively.
`NewWithRetention` requires its logger at parameter index 1, and the exact
`events/wire.NewService` signature requires logger parameter 0; retention is
domain policy. The owning Wire provider calls it directly. The retired private `New`
wrapper and Wire logger fallback were removed by PR #3126. No construction or
requiredness allowance is introduced. Final owner classifications, terminal
inventory reconciliation and repository-wide enforcement remain T29 work.

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
