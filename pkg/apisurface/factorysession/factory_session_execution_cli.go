package factorysession

import (
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// CLIStartInput carries CLI-resolved durable Factory Session execution fields
// before shared normalization.
type CLIStartInput struct {
	RequestID       string
	Source          factorysessionexecution.Source
	Args            map[string]any
	RequestedPolicy map[string]any
	Wait            *factorysessionexecution.WaitOptions
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
	})
}
