// Package resolution defines the parent-private Operator Settings effective
// resolution capability.
package resolution

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Service fulfills the accepted CTR-SET effective-resolution root slice as a
// parent-private owner. Resolution does not mutate the operator document.
type Service interface {
	ResolveEffective(operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error)
}
