package cli

import (
	"errors"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func ambiguityCLIError(err error, phase string) *CLIError {
	var ambiguous *workersessions.AmbiguousAddressError
	if !errors.As(err, &ambiguous) || ambiguous == nil {
		return nil
	}
	details := &factoryapi.WorkerSessionAddressDetails{}
	for _, candidate := range workersessions.NewAmbiguousAddressError(ambiguous.Candidates).Candidates {
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
