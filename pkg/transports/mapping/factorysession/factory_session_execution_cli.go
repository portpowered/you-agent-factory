package factorysession

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
)

// CLIStartInput carries CLI-resolved durable Factory Session execution fields
// before shared normalization.
type CLIStartInput struct {
	RequestID       string
	Source          factorysessionexecution.Source
	Args            map[string]any
	RequestedPolicy map[string]any
	Wait            *factorysessionexecution.WaitOptions
	Runtime         *factorysessionexecution.RuntimeOptions
}

// StartRequestFromCLI maps one CLI-resolved durable execution request into the
// shared service contract.
func StartRequestFromCLI(input CLIStartInput) (factorysessionexecution.StartRequest, error) {
	return factorysessionexecution.NormalizeStartRequest(factorysessionexecution.StartRequest{
		RequestID:       input.RequestID,
		Source:          input.Source,
		Args:            input.Args,
		RequestedPolicy: input.RequestedPolicy,
		Wait:            input.Wait,
		Runtime:         input.Runtime,
	})
}

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
	return factorysessionexecution.NormalizeEventReconnectRequest(factorysessionexecution.EventReconnectRequest{
		AfterEventID:  input.AfterEventID,
		AfterSequence: input.AfterSequence,
	})
}

// ResultRequestFromCLI maps one CLI-resolved durable result read into the shared
// service contract.
func ResultRequestFromCLI(input CLIResultInput) (factorysessionexecution.ResultRequest, error) {
	req := factorysessionexecution.ResultRequest{
		IncludeArtifacts: input.IncludeArtifacts,
	}
	if trimmed := strings.TrimSpace(input.Mode); trimmed != "" {
		req.Mode = factorysessionexecution.ResultMode(trimmed)
	}
	return factorysessionexecution.NormalizeResultRequest(req)
}
