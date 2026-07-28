package capture

import (
	"errors"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// NewJSONDecoder binds a representation decoder to the Factory Definition
// snapshot contract.
func NewJSONDecoder[T any](
	decodeBoundary func([]byte) (T, error),
) factorydefinitions.FactorySnapshotJSONDecoder {
	return func(data []byte) (*factorydefinitions.FactorySnapshot, error) {
		return DecodeJSON(data, decodeBoundary)
	}
}

// DecodeJSON validates one representation boundary and captures a detached
// Factory Definition snapshot.
func DecodeJSON[T any](
	data []byte,
	decodeBoundary func([]byte) (T, error),
) (*factorydefinitions.FactorySnapshot, error) {
	if decodeBoundary == nil {
		return nil, errors.New("Factory boundary JSON decoder is required")
	}
	boundary, err := decodeBoundary(data)
	if err != nil {
		return nil, err
	}
	return factorydefinitions.NewFactorySnapshot(boundary)
}
