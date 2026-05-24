package factory

import "github.com/portpowered/infinite-you/pkg/interfaces"

// CloneRuntimeTags returns a detached copy of runtime tag metadata while
// preserving nil for absent input.
func CloneRuntimeTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(tags))
	for key, value := range tags {
		cloned[key] = value
	}
	return cloned
}

// CloneRuntimeRelations returns a detached copy of runtime relations while
// preserving nil for absent input.
func CloneRuntimeRelations(relations []interfaces.Relation) []interfaces.Relation {
	if len(relations) == 0 {
		return nil
	}

	cloned := make([]interfaces.Relation, len(relations))
	copy(cloned, relations)
	return cloned
}

// CloneRuntimePayload returns a detached copy of runtime payload bytes while
// preserving nil for absent input.
func CloneRuntimePayload(payload []byte) []byte {
	if len(payload) == 0 {
		return nil
	}

	return append([]byte(nil), payload...)
}
