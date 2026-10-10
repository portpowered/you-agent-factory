package interfacestorage

type Port interface{ Execute() }
type Service interface{ Execute() }
type implementation struct{ required, optional Port }
type unrelated struct{ optional Port }

// Returning a concrete implementation as an interface preserves its storage
// provenance without classifying every implementation of that interface.
func New(required Port, optional Port) Service {
	alias := required
	return build(alias, optional)
}
func build(required Port, optional Port) Service {
	value := &implementation{required: required, optional: optional}
	return value
}
func (s *implementation) Execute() {
	self := s
	if self == nil { // want "required-receiver-guard.*implementation.*Execute->.*New"
	}
	if self.required == nil { // want "required-dependency-guard.*implementation.*Execute->.*New"
	}
	if s.optional != nil {
		s.optional.Execute()
	}
}
func (s *implementation) Dependency() Port   { return s.required } // want Dependency:"construction-getter=service-getter-locator"
func (s *implementation) Optional() Port     { return s.optional }
func (s *implementation) View(_ string) Port { return s.required }
func (s *implementation) Mutated() {
	s.required = nil
	if s.required == nil { // want "unresolved-required-dependency-guard.*Mutated->.*New"
	}
}
func consume(s *implementation) {
	_ = s.Dependency() // want "service-getter-locator.*consume->.*implementation.*Dependency"
	_ = s.Dependency   // want "unresolved-service-getter-reference.*consume->.*implementation.*Dependency"
	_ = s.Optional()
	_ = s.View("scope")
}
func (s *unrelated) Execute() {
	if s == nil || s.optional == nil {
	}
}

// An uncalled closure does not supply a constructor result identity.
func closureOnly() Service {
	_ = func() Service { return &unrelated{} }
	return nil
}
