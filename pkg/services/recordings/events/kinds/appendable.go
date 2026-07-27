package factoryeventkinds

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

var publicEmittableKindSet map[factorycontracts.FactoryEventType]struct{}

func init() {
	kinds := PublicEmittableFactoryEventKinds()
	publicEmittableKindSet = make(map[factorycontracts.FactoryEventType]struct{}, len(kinds))
	for _, entry := range kinds {
		publicEmittableKindSet[entry.Kind] = struct{}{}
	}
}

// IsPublicEmittableFactoryEventKind reports whether kind belongs to the
// runtime public emittable inventory and may be appended as a canonical ledger
// fact.
func IsPublicEmittableFactoryEventKind(kind factorycontracts.FactoryEventType) bool {
	_, ok := publicEmittableKindSet[kind]
	return ok
}
