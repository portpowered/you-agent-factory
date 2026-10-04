package registeredgetterconsumer

import owner "m/pkg/registeredgetters"

type Alias = owner.Service
type Promoted struct{ *owner.Service }
type Shadowed struct{ *owner.Service }

func (s *Shadowed) Lookup() owner.Port { return nil }
func Direct(s *owner.Service)          { _ = s.Lookup() }                                   // want "service-getter-locator.*Service.*Lookup"
func Pair(s *owner.Service)            { _, _ = s.Pair() }                                  // want "service-getter-locator.*Service.*Pair"
func Explicit(s *owner.Service)        { _ = s.Explicit() }                                 // want "service-getter-locator.*Service.*Explicit"
func Named(s *owner.Service)           { _ = s.Named() }                                    // want "unresolved-service-getter-locator.*Service.*Named"
func NamedAlias(s *owner.Service)      { _ = s.NamedAlias() }                               // want "unresolved-service-getter-locator.*Service.*NamedAlias"
func Mutated(s *owner.Service)         { _ = s.Mutated() }                                  // want "unresolved-service-getter-locator.*Service.*Mutated"
func FieldMutated(s *owner.Service)    { _ = s.FieldMutated() }                             // want "unresolved-service-getter-locator.*Service.*FieldMutated"
func Identity(s *owner.Service)        { _ = s.Identity() }                                 // want "service-getter-locator.*Service.*Identity"
func Ambiguous(s *owner.Service)       { _ = s.Ambiguous() }                                // want "unresolved-service-getter-locator.*Service.*Ambiguous"
func Tuple(s *owner.Service)           { _, _ = s.Tuple() }                                 // want "unresolved-service-getter-locator.*Service.*Tuple"
func AliasReturn(s *owner.Service)     { _ = s.Alias() }                                    // want "service-getter-locator.*Service.*Alias"
func PromotedCall(s *Promoted)         { _ = s.Lookup() }                                   // want "service-getter-locator.*Service.*Lookup"
func Aliased(s *Alias)                 { _ = s.Lookup() }                                   // want "service-getter-locator.*Service.*Lookup"
func MethodValue(s *owner.Service)     { lookup := s.Lookup; alias := lookup; _ = alias() } // want "service-getter-locator.*Service.*Lookup"
func MethodExpr(s *owner.Service)      { _ = (*owner.Service).Lookup(s) }                   // want "service-getter-locator.*Service.*Lookup"
func Deferred(s *owner.Service)        { defer s.Lookup() }                                 // want "service-getter-locator.*Service.*Lookup"
func Closure(s *owner.Service)         { func() { _ = s.Lookup() }() }                      // want "service-getter-locator.*Service.*Lookup"
func Unused(s *owner.Service)          { _ = s.Lookup }                                     // want "unresolved-service-getter-reference.*Service.*Lookup"
func Escaped(s *owner.Service)         { consume(s.Lookup) }                                // want "unresolved-service-getter-reference.*Service.*Lookup"
func consume(func() owner.Port)        {}
func Reassigned(s *owner.Service) {
	lookup := s.Lookup // want "unresolved-service-getter-reference.*Service.*Lookup"
	lookup = func() owner.Port { return nil }
	_ = lookup()
}
func Allowed(s *owner.Service, state *owner.State, shadow *Shadowed) {
	_ = s.Optional()
	_ = s.View(1)
	_ = s.Data()
	_ = s.Nested()
	_ = state.Lookup()
	_ = shadow.Lookup()
}

func receiver[T any](s *owner.Service) *owner.Service        { return s }
func tupleReceiver(s *owner.Service) (*owner.Service, error) { return s, nil }
func GenericReceiver(s *owner.Service)                       { _ = receiver[int](s).Lookup() } // want "service-getter-locator.*Service.*Lookup"
func TupleReceiver(s *owner.Service) {
	selected, _ := tupleReceiver(s)
	_ = selected.Lookup() // want "service-getter-locator.*Service.*Lookup"
}
func HelperAlias(s *owner.Service) {
	selectOwner := receiver[int]
	_ = selectOwner(s).Lookup() // want "service-getter-locator.*Service.*Lookup"
}
func Derived(s *owner.Service) { _ = s.Derived() } // want "service-getter-locator.*Service.*Derived"
