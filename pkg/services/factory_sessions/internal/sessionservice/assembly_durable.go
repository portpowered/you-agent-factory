package service

import (
	"fmt"

	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

// BindProcessDurable installs the one process-owned durable execution service
// on the canonical Assembly before it is exposed to callers.
func (a *Assembly) BindProcessDurable(execution durableexecution.Service) error {
	if a == nil || execution == nil {
		return fmt.Errorf("bind Factory Sessions durable execution: assembly and execution are required")
	}
	if current, ok := a.SessionGateway.(*Service); ok && current.durable != nil {
		return fmt.Errorf("bind Factory Sessions durable execution: already bound")
	}
	a.SessionGateway = &Service{durable: execution}
	return nil
}
