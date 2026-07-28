package internal

import (
	"context"
	"fmt"
	"sort"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

type service struct {
	descriptors map[providers.ID]catalog.Descriptor
	aliases     map[providers.ID]providers.ID
	locator     platformprocess.ExecutableLocator
}

func New(descriptors []catalog.Descriptor, locator platformprocess.ExecutableLocator) (catalog.Service, error) {
	if locator == nil {
		return nil, fmt.Errorf("provider executable locator is required")
	}
	indexed := make(map[providers.ID]catalog.Descriptor, len(descriptors))
	aliases := make(map[providers.ID]providers.ID)
	for index, descriptor := range descriptors {
		if err := descriptor.Provider.ID.Validate(); err != nil {
			return nil, fmt.Errorf("provider descriptor %d: %w", index, err)
		}
		if descriptor.Provider.ExecutionKind == "" {
			return nil, fmt.Errorf("provider descriptor %q: execution kind is required", descriptor.Provider.ID)
		}
		if _, exists := indexed[descriptor.Provider.ID]; exists {
			return nil, fmt.Errorf("duplicate provider identity %q", descriptor.Provider.ID)
		}
		descriptor.Provider = descriptor.Provider.Clone()
		descriptor.Aliases = append([]providers.ID(nil), descriptor.Aliases...)
		indexed[descriptor.Provider.ID] = descriptor
		for _, alias := range descriptor.Aliases {
			if err := alias.Validate(); err != nil {
				return nil, fmt.Errorf("provider descriptor %q alias: %w", descriptor.Provider.ID, err)
			}
			if _, exists := indexed[alias]; exists {
				return nil, fmt.Errorf("provider alias %q collides with an identity", alias)
			}
			if owner, exists := aliases[alias]; exists {
				return nil, fmt.Errorf("provider alias %q is duplicated by %q and %q", alias, owner, descriptor.Provider.ID)
			}
			aliases[alias] = descriptor.Provider.ID
		}
	}
	for alias := range aliases {
		if _, exists := indexed[alias]; exists {
			return nil, fmt.Errorf("provider alias %q collides with an identity", alias)
		}
	}
	return &service{descriptors: indexed, aliases: aliases, locator: locator}, nil
}

func (s *service) List(ctx context.Context) ([]catalog.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(s.descriptors))
	for id := range s.descriptors {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	result := make([]catalog.Descriptor, 0, len(ids))
	for _, value := range ids {
		descriptor := s.withAvailability(s.descriptors[providers.ID(value)])
		descriptor.Provider = descriptor.Provider.Clone()
		result = append(result, descriptor)
	}
	return result, nil
}

func (s *service) Get(ctx context.Context, id providers.ID) (catalog.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return catalog.Descriptor{}, err
	}
	canonical := id
	if resolved, ok := s.aliases[id]; ok {
		canonical = resolved
	}
	descriptor, ok := s.descriptors[canonical]
	if !ok {
		return catalog.Descriptor{}, fmt.Errorf("%w: %q", providers.ErrUnknownProvider, id)
	}
	descriptor = s.withAvailability(descriptor)
	descriptor.Provider = descriptor.Provider.Clone()
	return descriptor, nil
}

func (s *service) withAvailability(descriptor catalog.Descriptor) catalog.Descriptor {
	command := strings.TrimSpace(descriptor.Provider.Command)
	if command == "" {
		descriptor.Provider.Availability = providers.Availability{
			State: providers.AvailabilityUnavailable, Detail: "provider command is not configured",
		}
		return descriptor
	}
	if _, err := s.locator.LookPath(command); err != nil {
		descriptor.Provider.Availability = providers.Availability{
			State: providers.AvailabilityUnavailable, Detail: fmt.Sprintf("executable %q was not found", command),
		}
		return descriptor
	}
	descriptor.Provider.Availability = providers.Availability{State: providers.AvailabilityAvailable}
	return descriptor
}
