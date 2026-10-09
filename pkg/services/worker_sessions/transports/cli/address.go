package cli

import (
	"encoding/json"
	"errors"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Only ambiguity diagnostics expose candidate details; general server error
// details may contain arbitrary JSON or private upstream data.
func remoteAddressDetails(response factoryapi.ErrorResponse) *factoryapi.WorkerSessionAddressDetails {
	if response.Code != factoryapi.ErrorResponseCodeWORKERSESSIONAMBIGUOUS || response.Details == nil {
		return nil
	}
	encoded, err := json.Marshal(response.Details)
	if err != nil {
		return nil
	}
	var details factoryapi.WorkerSessionAddressDetails
	if err := json.Unmarshal(encoded, &details); err != nil {
		return nil
	}
	return &details
}

func ambiguityCLIError(err error, phase string) *CLIError {
	var ambiguous *workersessions.AmbiguousAddressError
	if !errors.As(err, &ambiguous) || ambiguous == nil {
		return nil
	}
	details := &factoryapi.WorkerSessionAddressDetails{}
	for _, candidate := range ambiguous.Clone().Candidates {
		details.Candidates = append(details.Candidates, factoryapi.WorkerSessionAddressCandidate{
			FactorySessionId: candidate.FactorySessionID,
			WorkerSessionId:  candidate.WorkerSessionID,
			WorkId:           candidate.WorkID,
			State:            factoryapi.WorkerSessionAddressCandidateState(candidate.State),
		})
	}
	return &CLIError{Code: "WORKER_SESSION_AMBIGUOUS", Message: ambiguous.Error(), Phase: phase, Cause: err, Details: details}
}

func cliErrorDetails(err error) *factoryapi.WorkerSessionAddressDetails {
	var typed *CLIError
	if errors.As(err, &typed) && typed != nil {
		return typed.Details
	}
	return nil
}
