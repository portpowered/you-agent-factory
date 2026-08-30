package agent_test

import (
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type agentSharedScenarioSpec struct {
	name        string
	model       string
	provider    modelprovider.Provider
	inputMarker string
	output      string
	inputMode   agentSharedInputMode
	behavior    agentSharedScenarioBehavior
	failure     factoryapi.WorkFailureType
	message     string
}

func agentSharedScenarioSpecs() []agentSharedScenarioSpec {
	cases := agentSharedSuccessSpecs()
	return append(cases, agentSharedAdverseSpecs()...)
}

func agentSharedSuccessSpecs() []agentSharedScenarioSpec {
	return []agentSharedScenarioSpec{
		{
			name:        "Codex",
			model:       "converged-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "converged agent payload",
			output:      "converged agent response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
		{
			name:        "Registered",
			model:       "registered-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "registered agent payload",
			output:      "registered agent response COMPLETE",
			inputMode:   agentSharedJSONPayloadInput,
		},
		{
			name:        "RuntimeRoot",
			model:       "runtime-root-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "runtime provider root",
			output:      "functional-runtime-provider-output COMPLETE",
			inputMode:   agentSharedJSONSeedInput,
			behavior:    agentSharedHeldSuccess,
		},
		{
			name:        "Claude",
			model:       "claude-agent-model",
			provider:    modelprovider.ProviderClaude,
			inputMarker: "claude agent payload",
			output:      "claude agent response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
	}
}

func agentSharedAdverseSpecs() []agentSharedScenarioSpec {
	return []agentSharedScenarioSpec{
		{
			name:     "Invalid",
			model:    "invalid-agent-model",
			provider: modelprovider.Provider("unknown-provider"),
		},
		{
			name:        "Empty",
			model:       "empty-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "empty recovery payload",
			output:      "empty recovery response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
		{
			name:        "Minimum",
			model:       "minimum-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "minimum agent payload",
			output:      "minimum agent response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
		{
			name:        "Failure",
			model:       "failure-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "controlled failure payload",
			inputMode:   agentSharedTextInput,
			behavior:    agentSharedFailure,
			failure:     factoryapi.WorkFailureTypeAuthFailure,
			message:     agentFailureMessage,
		},
		{
			name:        "Timeout",
			model:       "timeout-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "controlled timeout payload",
			inputMode:   agentSharedTextInput,
			behavior:    agentSharedTimeout,
			failure:     factoryapi.WorkFailureTypeTimeout,
			message:     agentTimeoutMessage,
		},
		{
			name:        "Cancel",
			model:       "cancel-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "controlled cancellation payload",
			inputMode:   agentSharedTextInput,
			behavior:    agentSharedCancel,
			message:     agentCancellationMessage,
		},
		{
			name:        "Recovery",
			model:       "recovery-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "fresh recovery payload",
			output:      "fresh recovery response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
	}
}
