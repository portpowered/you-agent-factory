// Package http adapts Provider Sessions HTTP operations through the accepted
// Provider Sessions root contract. Request decoding, representation mapping,
// service invocation, error mapping, and response encoding for owned Provider
// Sessions HTTP operations remain here with the owning service.
package http

import (
	"errors"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
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
// identity. HTTP decode/encode for the detail operation is added in later
// adapter stories; this method proves the fake-root injection seam.
func (a *Adapter) Details(provider, kind, id string) (providersessions.Detail, error) {
	if a == nil || a.sessions == nil {
		return providersessions.Detail{}, errors.New("Provider Sessions service is required")
	}
	return a.sessions.Details(provider, kind, id)
}
