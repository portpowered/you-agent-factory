package registeredproviders

import owner "m/pkg/registeredowner"

var flag bool

func mutualStep(p owner.Dependency)         { Mutual(p) }
func genericStep[T any](p owner.Dependency) { Generic(p) }

type helper struct{}

func (helper) step(p owner.Dependency) { Method(p) }

var packageNext = func(p owner.Dependency) { plain(p) }
var packageMutable = func(p owner.Dependency) {}

func replace() { packageMutable = func(owner.Dependency) {} }

var packageAddress = func(p owner.Dependency) {}

func invoke(callback func()) { callback() }

type Hook struct{ Run func() }
type Contract interface{ Run() }

func declarationFactory() func(owner.Dependency) { return ReturnedDeclaration }
func closureFactory(p owner.Dependency) func()   { return func() { ReturnedClosure(p) } }
func aliasFactory(p owner.Dependency) func()     { return func() { ReturnedAlias(p) } }
func nestedFactory(p owner.Dependency) func() func() {
	return func() func() { return func() { ReturnedNested(p) } }
}
func sameFactory() func(owner.Dependency) {
	if flag {
		return SameAlternatives
	}
	return (SameAlternatives)
}
func mixedFactory() func(owner.Dependency) {
	if flag {
		return MixedAlternatives
	}
	return plain
}
func plain(owner.Dependency) {}
func distinctFactory() func() {
	if flag {
		return func() {}
	}
	return func() {}
}
func namedFactory() (next func())               { return func() {} }
func tupleFactory() (func(), int)               { return func() {}, 0 }
func cycleFactory() func()                      { return cycleFactory() }
func acyclicFactory() func()                    { return func() {} }
func consume(func())                            {}
func unrelatedStep()                            { unrelatedStep() }
func uncalledFactory(p owner.Dependency) func() { return func() { ReturnedUncalled(p) } }
func nestedReturnFactory(p owner.Dependency) func() {
	_ = func() func() { return func() { NestedReturn(p) } }
	return func() {}
}
func deferredFactory(p owner.Dependency) func() { return func() { ReturnedDeferred(p) } }
func goFactory(p owner.Dependency) func()       { return func() { ReturnedGo(p) } }
func sameClosureFactory(p owner.Dependency) func() {
	next := func() { SameClosure(p) }
	if flag {
		return next
	}
	return (next)
}
func callbackFactory(callback func()) func()      { return callback }
func fieldFactory() func()                        { h := Hook{}; return h.Run }
func mutualFactory() func()                       { return mutualNext() }
func mutualNext() func()                          { return mutualFactory() }
func escapedFactory() func()                      { next := func() {}; consume(next); return next }
func evaluationFactory(p owner.Dependency) func() { UncalledEvaluation(p); return func() {} }

var packageParenWrite = func(owner.Dependency) {}

func replaceParen() { (packageParenWrite) = func(owner.Dependency) {} }

var packageParenAddress = func(owner.Dependency) {}
