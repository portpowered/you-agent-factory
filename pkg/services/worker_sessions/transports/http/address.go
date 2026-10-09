package http

import (
	"encoding/json"
	"errors"
	"net/http"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Ambiguity diagnostics translate only detached identity facts. Provider data
// and source snapshots cannot leak through a failed address resolution.
func writeAmbiguousAddressError(w http.ResponseWriter, err error, phase string) bool {
	var ambiguous *workersessions.AmbiguousAddressError
	if !errors.As(err, &ambiguous) || ambiguous == nil {
		return false
	}
	details := factoryapi.WorkerSessionAddressDetails{}
	for _, candidate := range workersessions.NewAmbiguousAddressError(ambiguous.Candidates).Candidates {
		details.Candidates = append(details.Candidates, factoryapi.WorkerSessionAddressCandidate{
			FactorySessionId: candidate.FactorySessionID,
			WorkerSessionId:  candidate.WorkerSessionID,
			WorkId:           candidate.WorkID,
			State:            factoryapi.WorkerSessionAddressCandidateState(candidate.State),
		})
	}
	payload := struct {
		factoryapi.ErrorResponse
		Phase string `json:"phase,omitempty"`
	}{ErrorResponse: factoryapi.ErrorResponse{
		Message: ambiguous.Error(), Family: factoryapi.ErrorFamilyConflict,
		Code: factoryapi.ErrorResponseCodeWORKERSESSIONAMBIGUOUS, Details: &details,
	}, Phase: phase}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(payload)
	return true
}
