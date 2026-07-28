package factoryeventkinds

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

var publicEmittableKindSet map[recordings.FactoryEventType]struct{}

func init() {
	kinds := PublicEmittableFactoryEventKinds()
	publicEmittableKindSet = make(map[recordings.FactoryEventType]struct{}, len(kinds))
	for _, entry := range kinds {
		publicEmittableKindSet[entry.Kind] = struct{}{}
	}
}

// IsPublicEmittableFactoryEventKind reports whether kind belongs to the
// runtime public emittable inventory and may be appended as a canonical ledger
// fact.
func IsPublicEmittableFactoryEventKind(kind recordings.FactoryEventType) bool {
	_, ok := publicEmittableKindSet[kind]
	return ok
}
