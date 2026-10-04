package registeredgetterpointers

import owner "m/pkg/registeredgetters"

type Other struct{}

func (*Other) Lookup() owner.Port                   { return nil }
func selectOwner(s *owner.Service) *owner.Service   { return s }
func selectOther(s *Other) *Other                   { return s }
func pair(s *owner.Service) (*owner.Service, error) { return s, nil }
func Dereference(s *owner.Service)                  { (*s).Lookup() }                                                // want "service-getter-locator.*Service.*Lookup"
func Parenthesized(s *owner.Service)                { (*(s)).Lookup() }                                              // want "service-getter-locator.*Service.*Lookup"
func Address(s *owner.Service)                      { (&*s).Lookup() }                                               // want "service-getter-locator.*Service.*Lookup"
func HelperDereference(s *owner.Service)            { (*selectOwner(s)).Lookup() }                                   // want "service-getter-locator.*Service.*Lookup"
func HelperAddress(s *owner.Service)                { (&*selectOwner(s)).Lookup() }                                  // want "service-getter-locator.*Service.*Lookup"
func ValueAlias(s *owner.Service)                   { selected := *selectOwner(s); selected.Lookup() }               // want "service-getter-locator.*Service.*Lookup"
func AddressAlias(s *owner.Service)                 { selected := *selectOwner(s); (&selected).Lookup() }            // want "service-getter-locator.*Service.*Lookup"
func Tuple(s *owner.Service)                        { selected, _ := pair(s); (*selected).Lookup() }                 // want "service-getter-locator.*Service.*Lookup"
func MutatedTuple(s *owner.Service)                 { selected, _ := pair(s); selected = nil; (*selected).Lookup() } // want "service-getter-locator.*Service.*Lookup"
func MethodValue(s *owner.Service)                  { lookup := (*selectOwner(s)).Lookup; lookup() }                 // want "service-getter-locator.*Service.*Lookup"
func Deferred(s *owner.Service)                     { defer (*selectOwner(s)).Lookup() }                             // want "service-getter-locator.*Service.*Lookup"
func Closure(s *owner.Service)                      { func() { (*selectOwner(s)).Lookup() }() }                      // want "service-getter-locator.*Service.*Lookup"
func Controls(s *owner.Service, other *Other) {
	(*other).Lookup()
	(*selectOther(other)).Lookup()
	selected := *selectOther(other)
	(&selected).Lookup()
	_ = s
	{
		s := other
		(*s).Lookup()
	}
}
