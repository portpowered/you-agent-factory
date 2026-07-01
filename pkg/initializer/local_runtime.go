package initializer

import "context"

// LocalRuntimeRunner is the session/runtime seam used by local in-process CLI
// startup without coupling transports to root pkg/service.FactoryService.
type LocalRuntimeRunner interface {
	Run(ctx context.Context) error
}
