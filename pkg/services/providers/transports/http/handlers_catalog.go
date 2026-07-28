package http

import (
	"net/http"
)

// ListProvidersHTTP decodes one list-providers HTTP request, invokes the
// accepted Providers root, and encodes the adapter-owned success response shape.
func (a *Adapter) ListProvidersHTTP(w http.ResponseWriter, r *http.Request) {
	response, err := a.ListProviders(r.Context())
	if err != nil {
		a.writeCatalogOrInternalError(w, err, catalogListFailedMessage)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// GetProviderHTTP decodes one get-provider HTTP request, invokes the accepted
// Providers root, and encodes the adapter-owned success response shape.
func (a *Adapter) GetProviderHTTP(w http.ResponseWriter, r *http.Request, providerID string) {
	response, err := a.GetProvider(r.Context(), GetProviderInput{ProviderID: providerID})
	if err != nil {
		a.writeCatalogOrInternalError(w, err, catalogGetFailedMessage)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}
