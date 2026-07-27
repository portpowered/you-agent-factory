// Package http adapts Provider Sessions HTTP operations through the accepted
// Provider Sessions root contract. Request decoding, representation mapping,
// service invocation, error mapping, and response encoding for owned Provider
// Sessions HTTP operations remain here with the owning service.
//
// HTTP-PSES owns getProviderSessionDetails only. Root Inspect and Project slices
// stay peer APIs without adapter-owned HTTP mapping in this packet; see
// OwnedHTTPOperationIDs.
package http

import (
	"errors"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Adapter maps Provider Sessions HTTP operations through the accepted root
// contract without importing Provider Sessions internals or owning canonical
// session storage state.
type Adapter struct {
	sessions providersessions.Service
}

// NewAdapter constructs the Provider Sessions HTTP adapter bound to the
// accepted root Service seam.
func NewAdapter(sessions providersessions.Service) *Adapter {
	if sessions == nil {
		return nil
	}
	return &Adapter{sessions: sessions}
}

// Details invokes the Provider Sessions root Details slice for one session
// identity.
func (a *Adapter) Details(provider, kind, id string) (providersessions.Detail, error) {
	if a == nil || a.sessions == nil {
		return providersessions.Detail{}, errors.New("Provider Sessions service is required")
	}
	return a.sessions.Details(provider, kind, id)
}

// GetProviderSessionDetails decodes owned detail HTTP inputs, invokes the root
// Details slice, and encodes the success response shape.
func (a *Adapter) GetProviderSessionDetails(
	params factoryapi.GetProviderSessionDetailsParams,
) (factoryapi.ProviderSessionDetailResponse, error) {
	provider, kind, id, err := decodeDetailsParams(params)
	if err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, err
	}
	detail, err := a.Details(provider, kind, id)
	if err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, err
	}
	return providerSessionDetailToAPI(detail), nil
}
