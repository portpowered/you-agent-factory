package factorysession

import (
	"strings"

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

// CLIResultInput carries CLI-resolved durable Factory Session result read fields
// before shared normalization.
type CLIResultInput struct {
	Mode             string
	IncludeArtifacts bool
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
