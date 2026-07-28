// Package cli defines the Factory Runtime service-owned CLI adapter.
package cli

import (
	"context"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// Service exposes Factory Runtime CLI command operations to Cobra composition.
type Service interface {
	NormalizeInvocationOutputMode(raw string) (string, error)
	ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput bool) error
	ValidateInvocationOutputMode(req ValidateInvocationOutputModeRequest) error
	MapCurrentFactoryFailure(err error) error
	MapServerFailure(err error) error
	MapInvocationFailure(err error) error
	MapRuntimeRootFailure(err error) error
	ObserveRuntime(ctx context.Context, req factoryruntime.ObserveRequest) error
	CountTokenStates(snap *factoryruntime.PetriMarkingSnapshot) (wip, completed, failed int)
	FormatDuration(d time.Duration) string
}

// Config carries accepted Runtime-root collaborators for adapter construction.
type Config struct {
	Runtime factoryruntime.Service
}

type service struct {
	runtime factoryruntime.Service
}

// New constructs the Factory Runtime CLI service from the accepted Runtime root
// and any other Runtime-root collaborators required by adapter-owned behavior.
func New(cfg Config) Service {
	return &service{runtime: cfg.Runtime}
}

func (service *service) NormalizeInvocationOutputMode(raw string) (string, error) {
	return normalizeInvocationOutputMode(raw)
}

func (service *service) ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput bool) error {
	return validateInvocationOutputSelection(quiet, jsonOutput, explicitOutput)
}

func (service *service) ValidateInvocationOutputMode(req ValidateInvocationOutputModeRequest) error {
	return validateInvocationOutputMode(req)
}

func (service *service) MapCurrentFactoryFailure(err error) error {
	return mapCurrentFactoryFailure(err)
}

func (service *service) MapServerFailure(err error) error {
	return mapServerFailure(err)
}

func (service *service) MapInvocationFailure(err error) error {
	return mapInvocationFailure(err)
}

func (service *service) MapRuntimeRootFailure(err error) error {
	return mapRuntimeRootFailure(err)
}

func (service *service) ObserveRuntime(
	ctx context.Context,
	req factoryruntime.ObserveRequest,
) error {
	if service.runtime == nil {
		return &InvocationError{
			Code:    InvocationErrorCodeFailed,
			Message: "factory runtime root is required",
		}
	}
	_, err := service.runtime.Observe(ctx, req)
	return service.MapRuntimeRootFailure(err)
}

func (service *service) CountTokenStates(snap *factoryruntime.PetriMarkingSnapshot) (wip, completed, failed int) {
	return countTokenStates(snap)
}

func (service *service) FormatDuration(d time.Duration) string {
	return formatDuration(d)
}
