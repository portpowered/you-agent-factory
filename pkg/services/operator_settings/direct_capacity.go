package operatorsettings

import "fmt"

// DefaultDirectSessionCapacity is the finite host bound when the saved key is absent.
const DefaultDirectSessionCapacity int64 = 16

func resolveDirectSessionCapacity(authored *int64) (int64, error) {
	if authored == nil {
		return DefaultDirectSessionCapacity, nil
	}
	if *authored < 1 {
		return 0, fmt.Errorf("%w: defaults.directSessionCapacity must be a positive integer", ErrDocumentMalformed)
	}
	return *authored, nil
}

func cloneCapacity(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// CloneCapacity detaches the saved capacity during document conversion.
func (defaults Defaults) CloneCapacity() *int64 { return cloneCapacity(defaults.DirectSessionCapacity) }

// CloneCapacity detaches the saved capacity during document conversion.
func (defaults DocumentDefaults) CloneCapacity() *int64 {
	return cloneCapacity(defaults.DirectSessionCapacity)
}

// ResolveDirectSessionCapacity resolves this saved file-only default.
func (defaults Defaults) ResolveDirectSessionCapacity() (int64, error) {
	return resolveDirectSessionCapacity(defaults.DirectSessionCapacity)
}

// ResolveDirectSessionCapacity resolves the detached saved document default.
func (defaults DocumentDefaults) ResolveDirectSessionCapacity() (int64, error) {
	return resolveDirectSessionCapacity(defaults.DirectSessionCapacity)
}
