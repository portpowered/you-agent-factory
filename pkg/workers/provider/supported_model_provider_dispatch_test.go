package provider

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestSupportedModelProviders_BuildCommandRequest_UsesCLICommand(t *testing.T) {
	for _, provider := range interfaces.SupportedModelProviders() {
		t.Run(string(provider), func(t *testing.T) {
			behavior := providerBehaviorFor(string(provider), logging.NoopLogger{})
			req := interfaces.ProviderInferenceRequest{
				ModelProvider: string(provider),
				UserMessage:   "run dispatch verification",
			}

			args, err := behavior.BuildArgs(req, false)
			if err != nil {
				t.Fatalf("BuildArgs: %v", err)
			}

			commandReq := behavior.BuildCommandRequest(req, args)
			if commandReq.Command != string(provider) {
				t.Fatalf("command = %q, want %q", commandReq.Command, provider)
			}
		})
	}
}
