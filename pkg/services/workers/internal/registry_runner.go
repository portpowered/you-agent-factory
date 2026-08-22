package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// registryCapabilityRunner enforces the selection facts exposed through the
// Workers root contract before delegating execution. Provider invocation is
// owned by providers.Service; this adapter intentionally does not translate
// through the former provider conductor protocol.
type registryCapabilityRunner struct {
	next      workers.Runner
	providers providers.Service
}

func (r registryCapabilityRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if err := validateRequestedRunnerCapabilities(ctx, r.providers, request); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	return r.next.Execute(ctx, request)
}

func validateRequestedRunnerCapabilities(
	ctx context.Context,
	providerService providers.Service,
	request workers.RunnerExecutionRequest,
) error {
	if providerService == nil {
		return nil
	}
	metadata, builtIn := workers.BuiltInRunnerMetadata(request.RunnerID)
	if !builtIn {
		resolved, err := providerService.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: request.RunnerID})
		if err != nil {
			return err
		}
		descriptor, err := providerService.GetProvider(ctx, providers.GetProviderRequest{ID: resolved.ID})
		if err != nil {
			return err
		}
		metadata = runnerMetadataFromProvider(descriptor.Provider)
	}
	supported := make(map[workers.RunnerOptionalCapability]bool, len(metadata.Capabilities.Optional))
	for _, capability := range metadata.Capabilities.Optional {
		supported[capability.Capability] = capability.Status == workers.RunnerOptionalCapabilityStatusSupported
	}
	for _, required := range request.RequiredOptionalCapabilities {
		if supported[required] {
			continue
		}
		return fmt.Errorf(
			"%s is not supported by the %s runner in v1",
			strings.ReplaceAll(string(required), "_", " "),
			metadata.ID,
		)
	}
	return nil
}

func runnerMetadataFromProvider(descriptor providers.Descriptor) workers.RunnerMetadata {
	optional := make([]workers.RunnerOptionalCapabilitySupport, 0, 3)
	for _, mapping := range []struct {
		provider providers.Capability
		worker   workers.RunnerOptionalCapability
	}{
		{providers.CapabilityImageInput, workers.RunnerOptionalCapabilityImageInput},
		{providers.CapabilitySessionResume, workers.RunnerOptionalCapabilitySessionResume},
		{providers.CapabilityStructuredOutput, workers.RunnerOptionalCapabilityStructuredOutput},
	} {
		status := workers.RunnerOptionalCapabilityStatusUnsupported
		for _, capability := range descriptor.Capabilities {
			if capability == mapping.provider {
				status = workers.RunnerOptionalCapabilityStatusSupported
				break
			}
		}
		optional = append(optional, workers.RunnerOptionalCapabilitySupport{
			Capability: mapping.worker,
			Status:     status,
		})
	}
	return workers.RunnerMetadata{
		ID:           strings.ToLower(strings.TrimSpace(descriptor.ID.String())),
		DisplayName:  descriptor.DisplayName,
		Capabilities: workers.NewCapabilities(optional...),
	}
}
