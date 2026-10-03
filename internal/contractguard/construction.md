# Qualified construction metadata

`ConstructionRegistry` is immutable input to the static checkers. Each scan
builds a fresh authored-source index; it never constructs a service, loads a
runtime graph, or consults a mutable registry. The package-boundary, durable
construction, and logging commands consume the same validated observations
while retaining their existing rules and diagnostics.

Required-dependency nil comparisons are reported in registered constructors
and methods whose fields are initialized from those parameters through keyed
or positional result literals and explicit field assignments. Positional fields
follow authored declaration order, including grouped and embedded fields.
Assignment targets resolve explicit types, allocations and local owner aliases.
Conflicting writes or reassigned storage retain unresolved provenance rather than
selecting an arbitrary branch. Local declaration aliases and closures retain that provenance;
shadowed variables and optional domain fields do not acquire requiredness.
Same-package authored helper calls propagate required arguments to helper
parameters until the finite object graph reaches a fixed point, so guards in
called helper bodies are observed at their own qualified operation. Single-result
helper returns preserve provenance through aliases, chains, and type assertions.
Nested closure returns do not become returns of their enclosing helper. Mixed
origins, recursion in return summaries, named returns, and unavailable function
values produce explicit debt when their result guards a required dependency.
Parallel assignments retain the corresponding value's provenance; a comma-ok
assertion's value retains provenance separately from its boolean status.
Conditions in `if` and `for` that branch on a required collaborator's assertion
status are reported separately, including negation, boolean comparisons and
local status aliases. Merely observing assertion status remains allowed.
Nil checks of registered behavior/effect receivers and their aliases are
reported; unrelated domain and scoped-state receivers remain optional. Receiver
and field origins also propagate to called same-package helper parameters.
Reassignment of a traced parameter, alias, or method field makes a guard an
`unresolved-required-dependency-guard` observation. This debt blocks an enforced
set, just like a proved guard, and remains nonblocking in report mode.

Direct calls resolve dot-imported authored functions and instantiated generic
functions while preserving local shadowing. Immutable local constructor values,
alias chains, deferred calls and calls inside closures resolve to their exact
declarations. All uses must be direct calls or transfers to other covered local
aliases; escaped, unused, reassigned and package-stored values retain reference
debt. Same-package helper values also retain their return/argument provenance.
Methods resolve explicit receiver types, allocations, local receiver aliases and
method expressions. Imported and local type aliases follow authored alias chains;
defined types do not inherit the original type's method identity. Method values
use the same local-value rules.

Promoted methods follow authored struct embeddings in breadth-first selector
order. The finding names the original method declaration, including local and
imported aliases, embedded pointers/values, nested wrappers, and defined struct
types that retain embedded fields. Own methods and fields hide deeper methods;
same-depth field/method collisions and multiple paths to the same method remain
ambiguous. Path multiplicity saturates at two, and previously searched depths
are discarded, so recursive and diamond embeddings terminate without selecting
an arbitrary path. Imported private methods are not callable through promotion.
Opaque, generic and interface embeddings still require classification/debt
before readiness for implementation selection. Authored named interface method
sets, including local/imported aliases, defined interfaces and nested interface
embeddings, establish whether a selector competes at the struct embedding's
depth. An empty or unrelated interface does not hide a concrete getter;
a matching interface method blocks selection of a competing or deeper concrete
method. Cyclic or opaque interface embeddings remain unresolved.
Embedding predeclared `any` or `error` in an authored interface preserves its
known method set, including aliases, defined interfaces and imported contracts.
`error` contributes only `Error`; `any` contributes no selectors. Authored types
named `any` or `error` take precedence over the predeclared interfaces. This
does not establish implementation identity or classify direct predeclared
struct embeddings.
Inline interface embeddings and parenthesized interface expressions retain
their flattened method sets. Aliases to interface literals retain the authored
declaration for selector lookup rather than requiring a named target. Imported
inline methods preserve their declaring package's visibility; a private method
cannot compete with a caller package's private getter. Opaque and cyclic inline
embeddings remain unresolved, just like their named counterparts.
An opaque deeper embedding cannot hide a proved shallower
selector. Promotion establishes method identity, not requiredness of wrapper
storage, interface implementation selection or generic/pointer method-set proof.

Concrete helper result signatures also establish a method receiver's
declared type, including imported helpers, authored result aliases, named results,
immutable helper values and method chains. Tuple assignments and grouped local
declarations select the exact declared result position, including grouped/named
results and aliases of the selected receiver. Reassignments, closure writes and
range writes to these receiver bindings preserve their declared Go type and
qualified method identity. They do not prove instance equivalence. Helper
function values still require immutable bindings. Generic helpers are covered only when
their result is a concrete indexed type. This establishes qualified method
identity, not helper-return equivalence or required storage provenance; constructor
field tracing still uses its separate allocation/alias rules. Interface
implementation selection and opaque function values remain uncovered.
Address and dereference expressions preserve the known receiver declaration,
including helper results, tuple bindings and local aliases. This proves method
identity only; it does not prove pointer validity, instance equivalence, or the
method set of an arbitrary pointer depth or interface implementation.

Required constructor records containing registered behavior/effect collaborators
are reported as `required-dependency-bag`. Authored nested and embedded fields,
pointer/container fields (including map keys), and local/imported aliases and
defined records retain that classification. Recursive domain records terminate
without acquiring collaborator identity. Direct collaborators and records with
explicit domain, state or resource classifications remain accepted. Optional
unregistered parameters do not acquire requiredness from their names or fields.
Diagnostics identify the registered constructor and parameter declaration line,
without exposing parameter names or source text; report mode stays nonblocking.
Generic record instantiations and declarations outside the indexed module still
need bounded classification/debt before whole-set readiness can be claimed.

Zero-argument methods that return a registered behavior/effect collaborator
from traced required storage are service getters. Calling them reports
`service-getter-locator` with the exact getter and enclosing operation, including
imported type aliases, method expressions, immutable method values, deferred
calls and closures. Same-package helper return summaries preserve this origin;
ambiguous summaries or mutated fields report `unresolved-service-getter-locator`.
Multiple explicit returns are checked by collaborator result position, so an
error/domain result cannot confer lookup identity on an optional peer result.
Named declarations with explicit required-field returns retain proved identity.
Writes of required storage to a named result followed by a naked return or an
explicit return of that result retain unresolved lookup debt; grouped results,
parallel assignments and closure writes use exact result objects. Tuple-producing
helper returns with required arguments also retain debt until per-result
summaries exist. A finite monotonic write analysis preserves named-result debt
through local aliases, grouped declarations, later assignments, same-package
helpers and cycles. It follows exact AST objects, so shadows and optional/domain
result paths stay separate. Possible required writes remain debt even across
conflicting assignments; this does not prove control-flow values of named results.
Escaped or unused getter values report `unresolved-service-getter-reference`.
Parameterized views, optional fields, domain/state/resource results and returns
inside nested closures do not become peer getters. This does not yet establish
interface-dispatched getters, complete named/tuple result provenance, opaque or
arbitrary implementation selection; those limits still require
classification/debt before whole-set readiness.

This bounded scan does not yet establish interface-promoted
methods, generic receiver instantiations, package-variable execution provenance,
storage through arbitrary returned owner objects, assertion-status helper returns,
the remaining service-locator forms, recursive providers or complete effect ancestry. Those remaining
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
Focused-provider allowances cover synchronous calls in the provider body,
including immutable constructor aliases and conditional calls. Calls inside
closures, or constructors invoked by `defer` or `go`, remain prohibited even
inside an approved provider. Constructor calls evaluating arguments to a
deferred/asynchronous operation run synchronously and retain the allowance.
Focused-provider allowances also stop applying when resolved authored calls in
the provider package lead back to that provider. The finite call walk follows
cross-file helpers, immutable aliases, generic function instantiation and
concrete methods, including calls scheduled by defer/go. Invoked function
literals and immutable local closure aliases add edges through a finite body
walk; nested invoked closures and helper-owned closures retain the same qualified
provider identity. Uncalled closure bodies, passed callback values and shadowed
bindings do not establish an invocation edge. The resulting finding
identifies the registered constructor and its provider operation. Uncalled
helpers, shadowed declarations and cycles that do not return to the provider do
not establish that construction path. Mutable/opaque closure execution, unresolved callable
values, cross-package paths and noncyclic secondary graphs still need bounded
classification/debt before whole-set readiness. This is syntactic reachability,
not proof that a conditional path executes or that a recursive call terminates.

Called package closure values and immutable alias chains are also followed
within the provider package, across authored files. Closure bodies retain their
declaring source for qualified call resolution. Assignments, range writes or
address escapes anywhere in that package prevent choosing the initializer;
local shadows do not write the package binding. Uncalled package values and
cyclic value aliases do not establish invocation. This is bounded authored-source
reachability, not proof of external mutation safety or initialization order.
Called mutable/escaped package values, cyclic value aliases, opaque declared
function variables and callback parameters now retain
`unresolved-focused-provider-dispatch` at the allowed constructor call. This
debt follows the same bounded helper/closure walk, is nonblocking in report
mode, and blocks an enforced set without claiming a concrete recursive path.
Uncalled callbacks, uninvoked closure bodies, shadowed bindings, builtins and
type conversions do not acquire dispatch debt. A proved recursive path still
reports `registered-construction` even if another path is unresolved.
Field, returned-function, cross-package and arbitrary callback dispatch still
need classification/debt before whole-set readiness; this does not prove
callback argument equivalence or control-flow execution.

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

A clean report for the initial set is not whole-repository enforcement or
runtime behavior evidence. Interface-promoted/generic methods, package-stored values,
unresolved storage/status summaries, topology, and effect ancestry still require
fixtures and analysis before enabling a capability set. Cross-package helper
return equivalence is outside this bounded same-package summary.
