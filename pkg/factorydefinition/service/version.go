package factorydefinition

import (
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

// RequireFreshEditableFactoryVersion rejects save requests whose base version
// does not advance the provided current definition version.
func (s *Service) RequireFreshEditableFactoryVersion(
	baseVersion *factoryapi.HybridLogicalTimestamp,
	currentVersion factoryapi.HybridLogicalTimestamp,
) error {
	if s == nil {
		return fmt.Errorf("factory definition service is required")
	}
	if baseVersion == nil {
		return fmt.Errorf("%w: save request must include an advanced factory version", apisurface.ErrFactoryVersionStale)
	}
	if !isEditableFactoryVersionAdvanced(*baseVersion, currentVersion) {
		return fmt.Errorf("%w: submitted version logical=%d physical=%s must advance current logical=%d physical=%s",
			apisurface.ErrFactoryVersionStale,
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
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	physical := now.UTC()
	logical := int64(1)
	if current != nil {
		logical = current.Logical.Int64() + 1
		if !physical.After(current.Physical.UTC()) {
			physical = current.Physical.UTC().Add(time.Nanosecond)
		}
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(logical),
		Physical: physical,
	}
}

func isEditableFactoryVersionAdvanced(candidate, current factoryapi.HybridLogicalTimestamp) bool {
	return candidate.Logical > current.Logical && candidate.Physical.UTC().After(current.Physical.UTC())
}
