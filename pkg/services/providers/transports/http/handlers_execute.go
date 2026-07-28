package http

import (
	"net/http"
)

// ExecuteHTTP decodes one execute HTTP request, invokes the accepted Providers
// root, and encodes the adapter-owned success response shape.
func (a *Adapter) ExecuteHTTP(w http.ResponseWriter, r *http.Request, providerID string) {
	response, err := a.Execute(r.Context(), ExecuteInput{
		ProviderID: providerID,
		Body:       r.Body,
	})
	if err != nil {
		a.writeExecuteOrInternalError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}
