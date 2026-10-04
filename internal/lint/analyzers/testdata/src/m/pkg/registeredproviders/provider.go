package registeredproviders

import owner "m/pkg/registeredowner"

func Direct(p owner.Dependency)              { owner.New(p) }
func Deferred(p owner.Dependency)            { defer owner.New(p) }        // want "registered-construction.*Deferred.*New"
func Async(p owner.Dependency)               { go owner.New(p) }           // want "registered-construction.*Async.*New"
func ClosureConstruction(p owner.Dependency) { func() { owner.New(p) }() } // want "registered-construction.*ClosureConstruction.*New"
func Argument(p owner.Dependency)            { defer consumeService(owner.New(p)) }
func consumeService(*owner.Service)          {}
func Recursive(p owner.Dependency)           { Recursive(p); owner.New(p) }                          // want "registered-construction.*Recursive.*New"
func Mutual(p owner.Dependency)              { mutualStep(p); owner.New(p) }                         // want "registered-construction.*Mutual.*New"
func Generic(p owner.Dependency)             { genericStep[int](p); owner.New(p) }                   // want "registered-construction.*Generic.*New"
func Method(p owner.Dependency)              { helper{}.step(p); owner.New(p) }                      // want "registered-construction.*Method.*New"
func Closure(p owner.Dependency)             { next := func() { Closure(p) }; next(); owner.New(p) } // want "registered-construction.*Closure.*New"
func PackageClosure(p owner.Dependency)      { packageNext(p); owner.New(p) }
func PackageMutation(p owner.Dependency)     { packageMutable(p); owner.New(p) }                               // want "unresolved-focused-provider-dispatch.*PackageMutation.*New"
func PackageAddress(p owner.Dependency)      { _ = &packageAddress; packageAddress(p); owner.New(p) }          // want "unresolved-focused-provider-dispatch.*PackageAddress.*New"
func Callback(p owner.Dependency)            { invoke(func() {}); owner.New(p) }                               // want "unresolved-focused-provider-dispatch.*Callback.*New"
func Field(p owner.Dependency)               { h := Hook{}; h.Run(); owner.New(p) }                            // want "unresolved-focused-provider-dispatch.*Field.*New"
func Interface(p owner.Dependency)           { var h Contract; h.Run(); owner.New(p) }                         // want "unresolved-focused-provider-dispatch.*Interface.*New"
func Mutable(p owner.Dependency)             { next := func() {}; next = func() {}; next(); owner.New(p) }     // want "unresolved-focused-provider-dispatch.*Mutable.*New"
func ReturnedDeclaration(p owner.Dependency) { declarationFactory()(p); owner.New(p) }                         // want "registered-construction.*ReturnedDeclaration.*New"
func ReturnedClosure(p owner.Dependency)     { closureFactory(p)(); owner.New(p) }                             // want "registered-construction.*ReturnedClosure.*New"
func ReturnedAlias(p owner.Dependency)       { first := aliasFactory(p); next := first; next(); owner.New(p) } // want "registered-construction.*ReturnedAlias.*New"
func ReturnedNested(p owner.Dependency)      { nestedFactory(p)()(); owner.New(p) }                            // want "registered-construction.*ReturnedNested.*New"
func SameAlternatives(p owner.Dependency)    { sameFactory()(p); owner.New(p) }                                // want "registered-construction.*SameAlternatives.*New"
func MixedAlternatives(p owner.Dependency)   { mixedFactory()(p); owner.New(p) }                               // want "unresolved-focused-provider-dispatch.*MixedAlternatives.*New"
func DistinctClosures(p owner.Dependency)    { distinctFactory()(); owner.New(p) }                             // want "unresolved-focused-provider-dispatch.*DistinctClosures.*New"
func NamedReturn(p owner.Dependency)         { namedFactory()(); owner.New(p) }                                // want "unresolved-focused-provider-dispatch.*NamedReturn.*New"
func TupleReturn(p owner.Dependency)         { next, _ := tupleFactory(); next(); owner.New(p) }               // want "unresolved-focused-provider-dispatch.*TupleReturn.*New"
func ReturnCycle(p owner.Dependency)         { cycleFactory()(); owner.New(p) }                                // want "unresolved-focused-provider-dispatch.*ReturnCycle.*New"
func Escaped(p owner.Dependency)             { next := acyclicFactory(); consume(next); next(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Escaped.*New"
func EscapedAlias(p owner.Dependency) {
	next := acyclicFactory()
	alias := next
	consume(alias)
	next()
	owner.New(p) // want "unresolved-focused-provider-dispatch.*EscapedAlias.*New"
}
func Acyclic(p owner.Dependency)          { next := acyclicFactory(); next(); owner.New(p) }
func Uncalled(p owner.Dependency)         { _ = func() { Uncalled(p) }; owner.New(p) }
func Shadowed(p owner.Dependency)         { Shadowed := func(owner.Dependency) {}; Shadowed(p); owner.New(p) }
func UnrelatedCycle(p owner.Dependency)   { unrelatedStep(); owner.New(p) }
func Builtins(p owner.Dependency)         { _ = len([]int{}); _ = int(1); owner.New(p) }
func ReturnedUncalled(p owner.Dependency) { next := uncalledFactory(p); _ = next; owner.New(p) }
func NestedReturn(p owner.Dependency)     { nestedReturnFactory(p)(); owner.New(p) }

type ContractWrapper struct{ Contract }
type MethodWrapper struct{ helper }

func PromotedInterface(p owner.Dependency) { h := ContractWrapper{}; h.Run(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*PromotedInterface.*New"
func PromotedMethod(p owner.Dependency)    { h := MethodWrapper{}; h.step(p); owner.New(p) }
func ReturnedDeferred(p owner.Dependency)  { defer deferredFactory(p)(); owner.New(p) }   // want "registered-construction.*ReturnedDeferred.*New"
func ReturnedGo(p owner.Dependency)        { go goFactory(p)(); owner.New(p) }            // want "registered-construction.*ReturnedGo.*New"
func SameClosure(p owner.Dependency)       { sameClosureFactory(p)(); owner.New(p) }      // want "registered-construction.*SameClosure.*New"
func ReturnedCallback(p owner.Dependency)  { callbackFactory(func() {})(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*ReturnedCallback.*New"
func ReturnedField(p owner.Dependency)     { fieldFactory()(); owner.New(p) }             // want "unresolved-focused-provider-dispatch.*ReturnedField.*New"
func MutualReturnCycle(p owner.Dependency) { mutualFactory()(); owner.New(p) }            // want "unresolved-focused-provider-dispatch.*MutualReturnCycle.*New"
func MutatedResult(p owner.Dependency) {
	next := acyclicFactory()
	next = func() {}
	next()
	owner.New(p) // want "unresolved-focused-provider-dispatch.*MutatedResult.*New"
}
func ReturnedEscaped(p owner.Dependency)    { escapedFactory()(); owner.New(p) }                           // want "unresolved-focused-provider-dispatch.*ReturnedEscaped.*New"
func UncalledEvaluation(p owner.Dependency) { _ = evaluationFactory(p); owner.New(p) }                     // want "registered-construction.*UncalledEvaluation.*New"
func RecursionAndDebt(p owner.Dependency)   { var next func(); next(); RecursionAndDebt(p); owner.New(p) } // want "registered-construction.*RecursionAndDebt.*New"
func PackageParenWrite(p owner.Dependency)  { packageParenWrite(p); owner.New(p) }                         // want "unresolved-focused-provider-dispatch.*PackageParenWrite.*New"
func PackageParenAddress(p owner.Dependency) {
	_ = &(packageParenAddress)
	packageParenAddress(p)
	owner.New(p) // want "unresolved-focused-provider-dispatch.*PackageParenAddress.*New"
}
func ConstructorAlias(p owner.Dependency) {
	construct := owner.New
	alias := (construct)
	alias(p)
}
