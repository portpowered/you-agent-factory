package acceptance

import (
	"context"
	"sync/atomic"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

type unsupportedContinuationProvider struct {
	*testutil.MockProvider
	continuationCalls     atomic.Int32
	continuationReference atomic.Value
}

func (provider *unsupportedContinuationProvider) ContinueReference(
	_ context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	provider.continuationCalls.Add(1)
	provider.continuationReference.Store(request.Reference)
	return providers.ContinueReferenceResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

var _ providers.Service = (*unsupportedContinuationProvider)(nil)
