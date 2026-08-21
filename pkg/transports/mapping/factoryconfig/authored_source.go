package factoryconfig

import (
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// DecodeAuthoredFactoryAPI decodes supplied authored factory.json bytes
// without flattening or applying usage-aware taxonomy projection.
func DecodeAuthoredFactoryAPI(data []byte) (factoryapi.Factory, error) {
	factory, _, err := DecodeAuthoredFactoryAPIWithDiagnostics(data)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return factory, nil
}

// DecodeAuthoredFactoryAPIWithDiagnostics decodes authored Factory bytes using
// the same compatibility boundary as runtime loading.
func DecodeAuthoredFactoryAPIWithDiagnostics(
	data []byte,
) (factoryapi.Factory, FactoryDecodeDiagnostics, error) {
	// Validation intentionally preserves authored taxonomy spellings for its
	// inspection output. Runtime loading applies the compatibility normalizers;
	// both boundaries share the same tolerant generated-model decode and path
	// collection.
	factory, err := decodeAuthoredFactoryBoundary(data)
	if err != nil {
		return factoryapi.Factory{}, FactoryDecodeDiagnostics{}, fmt.Errorf("parse factory config: %w", err)
	}
	paths, err := collectUnknownFactoryJSONPaths(data)
	if err != nil {
		return factoryapi.Factory{}, FactoryDecodeDiagnostics{}, fmt.Errorf("parse factory config: %w", err)
	}
	diagnostics := FactoryDecodeDiagnostics{IgnoredJSONPaths: paths}
	return factory, diagnostics, nil
}
