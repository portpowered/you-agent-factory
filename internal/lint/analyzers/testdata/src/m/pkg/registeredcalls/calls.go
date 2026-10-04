package registeredcalls

import owner "m/pkg/registeredowner"

func Direct() { owner.New(nil) } // want "registered-construction"
func Generic() { owner.Generic[int](nil) } // want "registered-construction"
func Deferred() { defer owner.New(nil) } // want "registered-construction"
func Closure() { func() { owner.New(nil) }() } // want "registered-construction"
func ClosureReference() { func() { _ = owner.New }() } // want "unresolved-construction-reference"
func Aliased() {
	create := owner.New
	alias := create
	alias(nil) // want "registered-construction"
}
func Unused() { _ = owner.New } // want "unresolved-construction-reference"
func Escaped() any { return owner.New } // want "unresolved-construction-reference"
func Mutated() {
	create := owner.New // want "unresolved-construction-reference"
	create = func(owner.Dependency) *owner.Service { return nil }
	create(nil)
}
func Shadowed() {
	owner := struct { New func(any) }{New: func(any) {}}
	owner.New(nil)
}
func Domain() { owner.Value() }
func AliasMethod() {
	owner.Alias{}.Construct(nil) // want "registered-construction"
}
func PromotedMethod() {
	owner.Embedded{}.Construct(nil) // want "registered-construction"
}
func MethodValue() {
	method := owner.Maker{}.Construct
	method(nil) // want "registered-construction"
}
