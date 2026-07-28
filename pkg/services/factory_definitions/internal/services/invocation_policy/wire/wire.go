// Package wire constructs the Factory Definitions invocation_policy subservice
// from the parent-private nested owner.
package wire

import (
	"fmt"

	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicyserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/service"
)

// NewService constructs the private invocation_policy subservice. Callers must
// supply Dependencies; this constructor does not select host adapters or take
// Wire/root construction ownership.
func NewService(deps invocationpolicyservice.Dependencies) (invocationpolicyservice.Service, error) {
	_ = deps
	service := invocationpolicyserviceimpl.New()
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions invocation_policy: implementation rejected its dependencies")
	}
	return service, nil
}
