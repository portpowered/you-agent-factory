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
