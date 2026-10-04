package registeredguards

type Service struct{}
type State struct{}
type Other struct{}

func NewDirect(p any) *Service {
	if p == nil { // want "required-dependency-guard.*NewDirect->.*NewDirect"
	}
	return nil
}
func NewReverse(p any) *Other { _ = nil != p; return nil } // want "required-dependency-guard.*NewReverse->.*NewReverse"
func NewAlias(p any) *Other {
	a := p
	b := a
	if b == nil { // want "required-dependency-guard.*NewAlias->.*NewAlias"
	}
	return nil
}
func NewDeclared(p any) *Other {
	var a = p
	if a != nil { // want "required-dependency-guard.*NewDeclared->.*NewDeclared"
	}
	return nil
}
func NewGrouped(p, optional any) *Other {
	if p == nil { // want "required-dependency-guard.*NewGrouped->.*NewGrouped"
	}
	if optional == nil {
	}
	return nil
}
func NewShadow(p any) *Other {
	_ = p
	{
		p := (*int)(nil)
		if p == nil {
		}
	}
	return nil
}
func NewOptional(p any) *Other {
	var optional any
	if optional == nil {
	}
	return nil
}
func NewMutation(p any) *Other {
	p = nil
	if p == nil { // want "unresolved-required-dependency-guard.*NewMutation->.*NewMutation"
	}
	return nil
}
func NewAliasMutation(p any) *Other {
	a := p
	a = nil
	if a == nil { // want "unresolved-required-dependency-guard.*NewAliasMutation->.*NewAliasMutation"
	}
	return nil
}
func NewAssertion(p any) *Other {
	_, ok := p.(int)
	if !ok { // want "required-dependency-assertion-guard.*NewAssertion->.*NewAssertion"
	}
	return nil
}
func NewAssertionValue(p any) *Other {
	value, ok := p.(*int)
	_ = ok
	if value == nil { // want "required-dependency-guard.*NewAssertionValue->.*NewAssertionValue"
	}
	return nil
}
func NewAssertionAlias(p any) *Other {
	_, ok := p.(int)
	var alias = ok
	if true != alias { // want "required-dependency-assertion-guard.*NewAssertionAlias->.*NewAssertionAlias"
	}
	return nil
}
func NewAssertionMutation(p any) *Other {
	_, ok := p.(int)
	ok = false
	if ok { // want "unresolved-required-dependency-guard.*NewAssertionMutation->.*NewAssertionMutation"
	}
	return nil
}
func NewAssertionObserve(p any) *Other {
	_, ok := p.(int)
	_ = ok
	if func() bool { _ = ok; return true }() {
	}
	return nil
}
func NewAssertionHelper(p any) *Other {
	_, ok := p.(int)
	if observe(ok) { // want "unresolved-required-dependency-guard.*NewAssertionHelper->.*NewAssertionHelper"
	}
	return nil
}
func observe(bool) bool { return true }
func NewClosure(p any) *Other {
	defer func() {
		a := p
		if a == nil { // want "required-dependency-guard.*NewClosure->.*NewClosure"
		}
	}()
	return nil
}
func NewFor(p any) *Other {
	_, ok := p.(int)
	for ok { // want "required-dependency-assertion-guard.*NewFor->.*NewFor"
		break
	}
	return nil
}
func NewBoolAssertion(p any) *Other {
	value, ok := p.(bool)
	_ = ok
	if value {
	}
	return nil
}
func NewState() *State { return nil }
func (s *Service) Direct() {
	if s == nil { // want "required-receiver-guard"
	}
}
func (s *Service) Alias() {
	self := s
	if self != nil { // want "required-receiver-guard"
	}
}
func (s *Service) Mutation() {
	s = nil
	if s == nil { // want "unresolved-required-dependency-guard"
	}
}
func (s *Service) Shadow() {
	{
		s := (*State)(nil)
		if s == nil {
		}
	}
}
func (s *State) Optional() {
	if s == nil {
	}
}
