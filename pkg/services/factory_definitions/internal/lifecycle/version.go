package lifecycle

import (
	"fmt"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// RequireFreshEditableFactoryVersion rejects save requests whose base version
// does not advance the provided current definition version.
func (s *Service) RequireFreshEditableFactoryVersion(
	baseVersion *factorydefinitions.FactoryVersion,
	currentVersion factorydefinitions.FactoryVersion,
) error {
	if s == nil {
		return fmt.Errorf("factory definition service is required")
	}
	if baseVersion == nil {
		return fmt.Errorf("%w: save request must include an advanced factory version", factorydefinitions.ErrFactoryVersionStale)
	}
	if !isEditableFactoryVersionAdvanced(*baseVersion, currentVersion) {
		return fmt.Errorf("%w: submitted version logical=%d physical=%s must advance current logical=%d physical=%s",
			factorydefinitions.ErrFactoryVersionStale,
			baseVersion.Logical,
			baseVersion.Physical.UTC().Format(time.RFC3339Nano),
			currentVersion.Logical,
			currentVersion.Physical.UTC().Format(time.RFC3339Nano),
		)
	}
	return nil
}

// NextEditableFactoryVersion returns the hybrid logical timestamp assigned to a
// successful editable-factory save.
func (s *Service) NextEditableFactoryVersion(
	current *factorydefinitions.FactoryVersion,
	now time.Time,
) factorydefinitions.FactoryVersion {
	physical := now.UTC()
	logical := int64(1)
	if current != nil {
		logical = current.Logical + 1
		if !physical.After(current.Physical.UTC()) {
			physical = current.Physical.UTC().Add(time.Nanosecond)
		}
	}
	return factorydefinitions.FactoryVersion{
		Logical:  logical,
		Physical: physical,
	}
}

func isEditableFactoryVersionAdvanced(candidate, current factorydefinitions.FactoryVersion) bool {
	return candidate.Logical > current.Logical && candidate.Physical.UTC().After(current.Physical.UTC())
}
