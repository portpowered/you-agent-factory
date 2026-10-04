package registeredhelpers

type Service struct{ port any }

func New(p any) *Service {
	Identity(p)
	Chain(p)
	Assertion(p)
	ClosureReturn(p)
	Domain(p)
	Mixed(p)
	Recursive(p)
	Mutual(p)
	MutatedHelper(p)
	MutatedArgument(p)
	NamedResult(p)
	FunctionValue(p)
	ReassignedFunctionValue(p)
	Opaque(p, nil)
	TupleResult(p)
	Generic(p)
	check(p)
	checkChain(p)
	checkRecursive(p)
	checkClosure(p)
	checkShadow(p)
	checkOptional(nil)
	mutateCheck(p)
	return &Service{port: chain(p)}
}

func Identity(p any) {
	if identity(p) == nil { // want "required-dependency-guard.*Identity->.*New"
	}
}
func Chain(p any) {
	if chain(p) == nil { // want "required-dependency-guard.*Chain->.*New"
	}
}
func Assertion(p any) {
	if asserted(p) == nil { // want "required-dependency-guard.*Assertion->.*New"
	}
}
func ClosureReturn(p any) {
	if closureReturn(p) == nil { // want "required-dependency-guard.*ClosureReturn->.*New"
	}
}
func Domain(p any) {
	if domain(p) == nil {
	}
}
func Mixed(p any) {
	if mixed(p) == nil { // want "unresolved-required-dependency-guard.*Mixed->.*New"
	}
}
func Recursive(p any) {
	if recursive(p) == nil { // want "unresolved-required-dependency-guard.*Recursive->.*New"
	}
}
func Mutual(p any) {
	if mutual(p) == nil { // want "unresolved-required-dependency-guard.*Mutual->.*New"
	}
}
func MutatedHelper(p any) {
	if mutated(p) == nil { // want "unresolved-required-dependency-guard.*MutatedHelper->.*New"
	}
}
func MutatedArgument(p any) {
	p = nil
	if identity(p) == nil { // want "unresolved-required-dependency-guard.*MutatedArgument->.*New"
	}
}
func NamedResult(p any) {
	if named(p) == nil { // want "unresolved-required-dependency-guard.*NamedResult->.*New"
	}
}
func FunctionValue(p any) {
	f := identity
	if f(p) == nil { // want "required-dependency-guard.*FunctionValue->.*New"
	}
}
func ReassignedFunctionValue(p any) {
	f := identity
	f = identity
	if f(p) == nil { // want "unresolved-required-dependency-guard.*ReassignedFunctionValue->.*New"
	}
}
func Opaque(p any, f func(any) any) {
	if f(p) == nil { // want "unresolved-required-dependency-guard.*Opaque->.*New"
	}
}
func TupleResult(p any) {
	if first(p) == nil { // want "unresolved-required-dependency-guard.*TupleResult->.*New"
	}
}
func Generic(p any) {
	if generic[any](p) == nil { // want "required-dependency-guard.*Generic->.*New"
	}
}
func (s *Service) Operation() {
	alias := chain(s.port)
	if alias == nil { // want "required-dependency-guard.*Operation->.*New"
	}
}
func (s *Service) CheckOperation() { operationCheck(s.port) }
func (s *Service) Shadowed() {
	identity := func(p *int) *int { return p }
	if identity(nil) == nil {
	}
}
