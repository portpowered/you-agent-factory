package wire

import (
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/internal/service"
)

// NewService constructs the inert live-runtime capability from explicit
// runtime-bound effects.
func NewService(dependencies liveruntime.Dependencies) (liveruntime.Service, error) {
	return internalservice.New(dependencies)
}
