package registeredgetters

type Port interface{ Execute() }
type Derived Port // want Derived:"construction-kind=behavior"
type Domain interface{ Data() }
type Service struct {
	port, optional Port
	domain         Domain
	derived        Derived
}

func New(port Port, derived Derived, domain Domain) *Service {
	return &Service{port: port, derived: derived, domain: domain}
}
func (s *Service) Derived() Derived          { return s.derived }                               // want Derived:"construction-getter=service-getter-locator"
func (s *Service) Lookup() Port              { return s.port }                                  // want Lookup:"construction-getter=service-getter-locator"
func (s *Service) Pair() (error, Port)       { return nil, s.port }                             // want Pair:"construction-getter=service-getter-locator"
func (s *Service) Explicit() (result Port)   { return s.port }                                  // want Explicit:"construction-getter=service-getter-locator"
func (s *Service) Named() (result Port)      { result = s.port; return }                        // want Named:"construction-getter=unresolved-service-getter-locator"
func (s *Service) NamedAlias() (result Port) { result = s.port; alias := result; return alias } // want NamedAlias:"construction-getter=unresolved-service-getter-locator"
func (s *Service) Mutated() Port             { value := s.port; value = nil; return value }     // want Mutated:"construction-getter=unresolved-service-getter-locator"
func (s *Service) FieldMutated() Port        { s.port = nil; return s.port }                    // want FieldMutated:"construction-getter=unresolved-service-getter-locator"
func (s *Service) Identity() Port            { return identity(s.port) }                        // want Identity:"construction-getter=service-getter-locator"
func identity(port Port) Port                { return port }
func (s *Service) Ambiguous() Port           { return mixed(s.port) } // want Ambiguous:"construction-getter=unresolved-service-getter-locator"
func mixed(port Port) Port {
	if flag {
		return port
	}
	return nil
}

var flag bool

func (s *Service) Tuple() (Port, error) { return pair(s.port) } // want Tuple:"construction-getter=unresolved-service-getter-locator"
func pair(port Port) (Port, error)      { return port, nil }
func (s *Service) Alias() Port          { owner := s; renamed := owner.port; return renamed } // want Alias:"construction-getter=service-getter-locator"
func (s *Service) Optional() Port       { return s.optional }
func (s *Service) View(_ int) Port      { return s.port }
func (s *Service) Data() Domain         { return s.domain }
func (s *Service) Nested() Port         { _ = func() Port { return s.port }; return nil }

type State struct{ port Port }

func NewState(port Port) *State { return &State{port: port} }
func (s *State) Lookup() Port   { return s.port }
func Local(s *Service)          { _ = s.Lookup() } // want "service-getter-locator.*Service.*Lookup"

func (s *Service) NamedParenthesized() (result Port) { // want NamedParenthesized:"construction-getter=unresolved-service-getter-locator"
	result = s.port
	return (result)
}
func (s *Service) NamedChain() (result Port) { // want NamedChain:"construction-getter=unresolved-service-getter-locator"
	result = s.port
	alias := result
	second := alias
	return second
}
func (s *Service) NamedDeclared() (result Port) { // want NamedDeclared:"construction-getter=unresolved-service-getter-locator"
	result = s.port
	var alias Port = result
	return alias
}
func (s *Service) NamedGrouped() (result Port) { // want NamedGrouped:"construction-getter=unresolved-service-getter-locator"
	result = s.port
	var other, alias Port = s.optional, result
	_ = other
	return alias
}
func (s *Service) NamedLaterAlias() (result Port) { // want NamedLaterAlias:"construction-getter=unresolved-service-getter-locator"
	result = s.port
	alias := s.optional
	alias = result
	return alias
}
func (s *Service) NamedHelper() (result Port) { // want NamedHelper:"construction-getter=unresolved-service-getter-locator"
	result = s.port
	return identity(result)
}
func (s *Service) NamedCrossResult() (first, second Port) { // want NamedCrossResult:"construction-getter=unresolved-service-getter-locator"
	alias := second
	first = alias
	second = s.port
	return first, s.optional
}
func (s *Service) NamedClosure() (result Port) { // want NamedClosure:"construction-getter=unresolved-service-getter-locator"
	alias := s.optional
	func() { result = s.port; alias = result }()
	return alias
}
func (s *Service) NamedCycle() (result Port) { // want NamedCycle:"construction-getter=unresolved-service-getter-locator"
	result = s.port
	first := result
	second := first
	first = second
	return second
}
func (s *Service) NamedGroupedResult() (first, second Port) { // want NamedGroupedResult:"construction-getter=unresolved-service-getter-locator"
	second = s.port
	return
}
func (s *Service) NamedParallel() (err error, result Port) { // want NamedParallel:"construction-getter=unresolved-service-getter-locator"
	err, result = nil, s.port
	return
}
func (s *Service) NamedTuple() (result Port, err error) { // want NamedTuple:"construction-getter=unresolved-service-getter-locator"
	result, err = pair(s.port)
	return
}
func (s *Service) NamedConflict() (result Port) { // want NamedConflict:"construction-getter=unresolved-service-getter-locator"
	result = s.port
	result = s.optional
	return
}
func (s *Service) DomainSlot() (Port, any) { return s.optional, s.port }
func (s *Service) NamedDomainSlot() (result Port, data any) {
	data = s.port
	alias := result
	return alias, data
}
func (s *Service) NamedOptional() (result Port) {
	result = s.optional
	alias := result
	return alias
}
func (s *Service) NamedShadow() (result Port) {
	{
		result := s.port
		_ = result
	}
	alias := result
	return alias
}
func (s *Service) NamedUnused() (result Port) {
	result = s.port
	alias := s.optional
	return alias
}
func (s *Service) NamedZero() (result Port) { return }
