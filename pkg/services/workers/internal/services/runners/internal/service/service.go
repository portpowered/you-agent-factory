package service

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
)

type service struct {
	bindings map[string]runners.Binding
}

var _ runners.Service = (*service)(nil)

// New validates and snapshots every registration before publishing a usable
// registry. Any invalid entry rejects the complete input.
func New(registrations []runners.Registration) (runners.Service, error) {
	bindings := make(map[string]runners.Binding, len(registrations))
	for index, registration := range registrations {
		binding, err := validateRegistration(index, registration)
		if err != nil {
			return nil, err
		}
		if _, exists := bindings[binding.Identity]; exists {
			return nil, fmt.Errorf(
				"%w: identity %q",
				workers.ErrDuplicateRunnerRegistration,
				binding.Identity,
			)
		}
		bindings[binding.Identity] = binding
	}
	return &service{bindings: bindings}, nil
}

func (s *service) Resolve(
	request runners.ResolutionRequest,
) (runners.Binding, error) {
	if strings.TrimSpace(request.Identity) == "" {
		return runners.Binding{}, workers.ErrMissingRunnerSelection
	}
	if s == nil {
		return runners.Binding{}, fmt.Errorf(
			"%w: %q",
			workers.ErrUnknownRunnerSelection,
			request.Identity,
		)
	}
	binding, ok := s.bindings[request.Identity]
	if !ok {
		return runners.Binding{}, fmt.Errorf(
			"%w: %q",
			workers.ErrUnknownRunnerSelection,
			request.Identity,
		)
	}
	supported := supportedOptionalCapabilities(binding.Metadata)
	for _, required := range request.RequiredCapabilities {
		if supported[required] {
			continue
		}
		return runners.Binding{}, &workers.UnsupportedRunnerCapabilityError{
			RunnerID:   binding.Identity,
			Capability: required,
		}
	}
	binding.Metadata = cloneMetadata(binding.Metadata)
	return binding, nil
}

func validateRegistration(
	index int,
	registration runners.Registration,
) (runners.Binding, error) {
	if err := validateIdentity(registration.Identity); err != nil {
		return runners.Binding{}, fmt.Errorf(
			"%w: registration %d identity: %v",
			workers.ErrInvalidRunnerRegistration,
			index,
			err,
		)
	}
	if registration.Metadata.ID != registration.Identity {
		return runners.Binding{}, fmt.Errorf(
			"%w: registration %d identity %q does not match metadata identity %q",
			workers.ErrConflictingRunnerRegistration,
			index,
			registration.Identity,
			registration.Metadata.ID,
		)
	}
	if err := validateMetadata(registration.Metadata); err != nil {
		return runners.Binding{}, fmt.Errorf(
			"%w: registration %d metadata: %v",
			workers.ErrInvalidRunnerRegistration,
			index,
			err,
		)
	}
	if nilRunner(registration.Runner) {
		return runners.Binding{}, fmt.Errorf(
			"%w: registration %d runner implementation is nil",
			workers.ErrInvalidRunnerRegistration,
			index,
		)
	}
	return runners.Binding{
		Identity: registration.Identity,
		Metadata: cloneMetadata(registration.Metadata),
		Runner:   registration.Runner,
	}, nil
}

func validateIdentity(identity string) error {
	if identity != strings.TrimSpace(identity) ||
		identity != workerrunner.NormalizeRunnerID(identity) {
		return fmt.Errorf("must use canonical lowercase form without surrounding whitespace")
	}
	if err := workers.ProviderIdentity(identity).Validate(); err != nil {
		return err
	}
	return nil
}

func nilRunner(runner workers.Runner) bool {
	if runner == nil {
		return true
	}
	value := reflect.ValueOf(runner)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func cloneMetadata(metadata workers.RunnerMetadata) workers.RunnerMetadata {
	metadata.Capabilities.Baseline = append(
		[]workers.RunnerBaselineCapability(nil),
		metadata.Capabilities.Baseline...,
	)
	metadata.Capabilities.Optional = append(
		[]workers.RunnerOptionalCapabilitySupport(nil),
		metadata.Capabilities.Optional...,
	)
	return metadata
}

func supportedOptionalCapabilities(
	metadata workers.RunnerMetadata,
) map[workers.RunnerOptionalCapability]bool {
	supported := make(
		map[workers.RunnerOptionalCapability]bool,
		len(metadata.Capabilities.Optional),
	)
	for _, capability := range metadata.Capabilities.Optional {
		supported[capability.Capability] =
			capability.Status == workers.RunnerOptionalCapabilityStatusSupported
	}
	return supported
}
