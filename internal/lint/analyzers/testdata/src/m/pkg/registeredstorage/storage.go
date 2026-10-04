package registeredstorage

type Port interface{ Execute() }
type Keyed struct{ port, optional any }
type Positional struct{ first, port any }
type Embedded struct{ Port }
type Assigned struct{ port any }
type Replaced struct{ port any }
type Mutated struct{ port any }
type Other struct{ port any }
type Optional struct{ port any }
type State struct{ port any }
type Shadowed struct{ port any }
type Asserted struct{ port *int }
type Holder struct{ *Keyed }

func (s *Holder) Guard() {
	if s.port == nil {
	}
}

func NewAsserted(p any) *Asserted {
	value, ok := p.(*int)
	_ = ok
	return &Asserted{port: value}
}
func (s *Asserted) Guard() {
	if s.port == nil { // want "required-dependency-guard.*Guard->.*NewAsserted"
	}
}

func NewKeyed(p any) *Keyed { return &Keyed{port: p} }
func (s *Keyed) Guard() {
	if s.port == nil { // want "required-dependency-guard.*Guard->.*NewKeyed"
	}
}
func (s *Keyed) Optional() {
	if s.optional == nil {
	}
}
func (s *Keyed) Alias() {
	self := s
	dep := self.port
	if dep != nil { // want "required-dependency-guard.*Alias->.*NewKeyed"
	}
}
func (s *Keyed) Mutation() {
	s.port = nil
	if s.port == nil { // want "unresolved-required-dependency-guard.*Mutation->.*NewKeyed"
	}
}
func (s *Keyed) Address() {
	_ = &s.port
	if s.port == nil { // want "unresolved-required-dependency-guard.*Address->.*NewKeyed"
	}
}
func (s *Keyed) AliasMutation() {
	self := s
	self = nil
	if self.port == nil { // want "unresolved-required-dependency-guard.*AliasMutation->.*NewKeyed"
	}
}
func (s *Keyed) Assertion() {
	_, ok := s.port.(Port)
	if !ok { // want "required-dependency-assertion-guard.*Assertion->.*NewKeyed"
	}
}
func (s *Keyed) Shadow() {
	{
		s := &Other{}
		if s.port == nil {
		}
	}
}
func (s *Keyed) Other(other *Keyed) {
	if other.port == nil {
	}
}
func (s *Keyed) Closure() {
	defer func() {
		if s.port == nil { // want "required-dependency-guard.*Closure->.*NewKeyed"
		}
	}()
}

func NewPositional(p any) *Positional { alias := p; return &Positional{nil, alias} }
func (s *Positional) Guard() {
	if s.port == nil { // want "required-dependency-guard.*Guard->.*NewPositional"
	}
}
func (s *Positional) Optional() {
	if s.first == nil {
	}
}
func NewEmbedded(p Port) *Embedded { return &Embedded{p} }
func (s *Embedded) Guard() {
	if s.Port == nil { // want "required-dependency-guard.*Guard->.*NewEmbedded"
	}
}
func NewAssigned(p any) *Assigned {
	s := new(Assigned)
	alias := s
	alias.port = p
	return s
}
func (s *Assigned) Guard() {
	if s.port == nil { // want "required-dependency-guard.*Guard->.*NewAssigned"
	}
}
func NewReplaced(p any) *Replaced {
	s := &Replaced{port: p}
	s.port = nil
	return s
}
func (s *Replaced) Guard() {
	if s.port == nil { // want "unresolved-required-dependency-guard.*Guard->.*NewReplaced"
	}
}
func NewMutated(p any) *Mutated {
	var s Mutated
	p = nil
	s.port = p
	return &s
}
func (s *Mutated) Guard() {
	if s.port == nil { // want "unresolved-required-dependency-guard.*Guard->.*NewMutated"
	}
}
func NewOptional(p any) *Optional { return &Optional{port: nil} }
func (s *Optional) Guard() {
	if s.port == nil {
	}
}
func NewState(p any) *State { return &State{port: p} }
func (s *State) Guard() {
	if s.port == nil { // want "required-dependency-guard.*Guard->.*NewState"
	}
}
func (s *State) Receiver() {
	if s == nil {
	}
}
func NewShadowed(p any) *Shadowed {
	s := &Shadowed{}
	{
		s := &Other{}
		s.port = p
	}
	return s
}
func (s *Shadowed) Guard() {
	if s.port == nil {
	}
}
