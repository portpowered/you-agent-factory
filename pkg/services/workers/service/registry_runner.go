package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

type registryCapabilityRunner struct {
	next      workers.Runner
	providers *providerregistry.Registry
}

func (r registryCapabilityRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if err := validateRequestedRunnerCapabilities(r.providers, request); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	return r.next.Execute(ctx, request)
}

func validateRequestedRunnerCapabilities(
	providers *providerregistry.Registry,
	request workers.RunnerExecutionRequest,
) error {
	if providers == nil {
		return nil
	}
	metadata, err := providers.RunnerMetadata(request.RunnerID)
	if err != nil {
		return err
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
