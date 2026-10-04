package registeredhelpers

func identity(p any) any { return p }
func chain(p any) any    { return identity(p) }
func asserted(p any) any { return p.(interface{ Execute() }) }
func closureReturn(p any) any {
	_ = func() any { return nil }
	return p
}
func domain(p any) *int { return nil }
func mixed(p any) any {
	if false {
		return nil
	}
	return p
}
func recursive(p any) any { return recursive(p) }
func mutual(p any) any    { return other(p) }
func other(p any) any     { return mutual(p) }
func mutated(p any) any   { p = nil; return p }
func named(p any) (result any) {
	result = p
	return
}
func pair(p any) (any, bool) { return p, true }
func first(p any) any {
	value, _ := pair(p)
	return value
}
func generic[T any](p T) T { return p }

func check(p any) {
	if p == nil { // want "required-dependency-guard.*check->.*New"
	}
}
func checkChain(p any) { nextCheck(p) }
func nextCheck(p any) {
	if p == nil { // want "required-dependency-guard.*nextCheck->.*New"
	}
}
func checkRecursive(p any) {
	if p == nil { // want "required-dependency-guard.*checkRecursive->.*New"
	}
	checkRecursive(p)
}
func checkClosure(p any) {
	defer func() {
		if p == nil { // want "required-dependency-guard.*checkClosure->.*New"
		}
	}()
}
func mutateCheck(p any) {
	p = nil
	mutatedCheck(p)
}
func mutatedCheck(p any) {
	if p == nil { // want "unresolved-required-dependency-guard.*mutatedCheck->.*New"
	}
}
func operationCheck(p any) {
	if p == nil { // want "required-dependency-guard.*operationCheck->.*New"
	}
}
func checkShadow(p any) {
	{
		p := (*int)(nil)
		if p == nil {
		}
	}
}
func checkOptional(p *int) {
	if p == nil {
	}
}
func uncalled(p any) {
	if p == nil {
	}
}
