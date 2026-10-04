package registeredbaglisted // want "stale baseline entry.*required-dependency-bag.*Removed"

import "m/pkg/registeredbagchild"

type Service struct{}

func New(input registeredbagchild.Bag) *Service { return nil }
