package registeredgetterlisted // want "stale baseline entry.*service-getter-locator.*Removed"

import owner "m/pkg/registeredgetters"

func Direct(s *owner.Service)    { _ = s.Lookup() }
func Named(s *owner.Service)     { _ = s.Named() }
func Reference(s *owner.Service) { _ = s.Lookup }
func Removed()                   {}
