package wire

import (
	"context"
	"fmt"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func validateExternalInvocationCapabilities(
	ctx context.Context,
	provider string,
	integration Integration,
	request InvocationRequest,
) error {
	maximum := integration.MaximumCapabilities()
	required := []Capability{CapabilityPromptSubmission}
	if request.SkipPermissions {
		required = append(required, CapabilityPermissionBypass)
	}
	for _, capability := range required {
		if maximum.Has(capability) {
			continue
		}
		return externalCapabilityMismatch(provider, capability)
	}

	negotiated, err := integration.Capabilities(ctx, request)
	if err != nil {
		return providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindDependency,
			Message: fmt.Sprintf(
				"provider %q capability negotiation failed",
				provider,
			),
		}
	}
	for _, capability := range negotiated.Values() {
		if maximum.Has(capability) {
			continue
		}
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindCapabilityMismatch,
			Message: fmt.Sprintf("provider %q returned capabilities outside its maximum", provider),
		}
	}
	for _, capability := range required {
		if negotiated.Has(capability) {
			continue
		}
		return externalCapabilityMismatch(provider, capability)
	}
	return nil
}

func externalCapabilityMismatch(provider string, capability Capability) error {
	return providers.ExecuteFailure{
		Kind: providers.ExecuteFailureKindCapabilityMismatch,
		Message: fmt.Sprintf(
			"provider %q does not support capability %q",
			provider,
			providers.Capability(capability),
		),
	}
}
