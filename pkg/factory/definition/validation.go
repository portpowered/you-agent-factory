package factorydefinition

import (
	"fmt"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// ValidateUpsertNamedFactoryRequest rejects named-factory upsert payloads whose
// name or topology fail pre-persist validation.
func (s *Service) ValidateUpsertNamedFactoryRequest(name string, snapshot *interfaces.FactorySnapshot) error {
	if err := factoryconfig.ValidateNamedFactoryName(name); err != nil {
		return fmt.Errorf("%w: %w", factoryconfig.ErrInvalidNamedFactoryName, err)
	}
	if name == interfaces.DefaultCurrentFactoryName {
		return fmt.Errorf("%w: %q is reserved for current-factory readback", factoryconfig.ErrInvalidNamedFactoryName, name)
	}
	return s.ValidateEditableFactoryTopology(snapshot)
}

// ValidateEditableFactoryTopology rejects editable factory definitions whose
// topology fails pre-persist validation.
func (s *Service) ValidateEditableFactoryTopology(snapshot *interfaces.FactorySnapshot) error {
	if s == nil {
		return fmt.Errorf("factory definition service is required")
	}
	if s.host == nil {
		return fmt.Errorf("factory definition host is required")
	}
	return s.host.ValidateEditableFactorySnapshot(snapshot)
}
