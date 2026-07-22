package factorysession

import factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// CLIResultInput carries CLI-resolved durable Factory Session result read fields
// before shared normalization.
type CLIResultInput struct {
	Mode             string
	IncludeArtifacts bool
}

// CLIEventReconnectInput carries CLI-resolved durable Factory Session event
// reconnect fields before shared normalization.
type CLIEventReconnectInput struct {
	AfterEventID  string
	AfterSequence *int
}

// EventReconnectRequestFromCLI maps one CLI-resolved event reconnect request into
// the shared service contract.
func EventReconnectRequestFromCLI(input CLIEventReconnectInput) (factorysessionexecution.EventReconnectRequest, error) {
	return factorysessionexecution.EventReconnectRequest{
		AfterEventID:  input.AfterEventID,
		AfterSequence: input.AfterSequence,
	}, nil
}

// ResultRequestFromCLI maps one CLI-resolved durable result read into the shared
// service contract.
func ResultRequestFromCLI(input CLIResultInput) (factorysessionexecution.ResultRequest, error) {
	req := factorysessionexecution.ResultRequest{
		IncludeArtifacts: input.IncludeArtifacts,
	}
	if input.Mode != "" {
		req.Mode = factorysessionexecution.ResultMode(input.Mode)
	}
	return req, nil
}
