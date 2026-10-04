package service

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// IdentityProbe preserves static catalog facts without performing readiness I/O.
func IdentityProbe(_ context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
	return descriptor.Clone(), nil
}
