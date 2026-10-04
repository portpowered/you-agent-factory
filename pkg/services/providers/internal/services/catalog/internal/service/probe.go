package service

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

// IdentityProbe preserves static catalog facts without performing readiness I/O.
func IdentityProbe(_ context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
	return descriptor.Clone(), nil
}

// NewProbeOperation completes the readiness projection over a supplied query.
// Native failures become bounded, safe facts; context cancellation remains an error.
func NewProbeOperation(query catalog.ProbeQuery) catalog.ProbeOperation {
	return func(ctx context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
		if err := ctx.Err(); err != nil {
			return providers.Descriptor{}, err
		}
		facts, err := query(ctx, descriptor.Clone())
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return providers.Descriptor{}, ctxErr
			}
			facts = probeFailureFacts(descriptor)
		}
		return mergeProbeFacts(descriptor, facts), nil
	}
}
