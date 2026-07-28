package factorydefinition

import (
	"context"
	"fmt"

	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ValidateUpsertNamedFactoryRequest rejects named-factory upsert payloads whose
// name or topology fail pre-persist validation.
func (s *Service) ValidateUpsertNamedFactoryRequest(ctx context.Context, name string, snapshot *factorydefinitions.FactorySnapshot) error {
	if err := namedfactorypath.ValidateName(name); err != nil {
		return fmt.Errorf("%w: %w", factorydefinitions.ErrInvalidNamedFactoryName, err)
	}
	if name == factorydefinitions.DefaultCurrentFactoryName {
		return fmt.Errorf("%w: %q is reserved for current-factory readback", factorydefinitions.ErrInvalidNamedFactoryName, name)
	}
	return s.ValidateEditableFactoryTopology(ctx, snapshot)
}

// ValidateEditableFactoryTopology rejects editable factory definitions whose
// topology fails pre-persist validation.
func (s *Service) ValidateEditableFactoryTopology(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	if s == nil {
		return fmt.Errorf("factory definition service is required")
	}
	if s.host == nil {
		return fmt.Errorf("factory definition host is required")
	}
	return s.host.ValidateEditableFactorySnapshot(ctx, snapshot)
}
