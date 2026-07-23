// Package modelproviders provides inert, read-only access to the published
// model-provider catalog and its JSON Schemas.
package modelproviders

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	generated "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

//go:embed generated/catalog.json
var catalogJSON []byte

//go:embed generated/provider-manifest.schema.json
var providerManifestSchemaJSON []byte

//go:embed generated/provider-catalog.schema.json
var providerCatalogSchemaJSON []byte

// CatalogJSON returns a detached copy of the published Provider Catalog bytes.
func CatalogJSON() []byte {
	return bytes.Clone(catalogJSON)
}

// ProviderManifestSchemaJSON returns a detached copy of the published Provider
// Manifest JSON Schema bytes.
func ProviderManifestSchemaJSON() []byte {
	return bytes.Clone(providerManifestSchemaJSON)
}

// ProviderCatalogSchemaJSON returns a detached copy of the published Provider
// Catalog JSON Schema bytes.
func ProviderCatalogSchemaJSON() []byte {
	return bytes.Clone(providerCatalogSchemaJSON)
}

// Catalog parses and returns a fresh Provider Catalog value. Every call owns
// its returned slices and pointers, so callers cannot mutate later results.
func Catalog() (generated.ProviderCatalog, error) {
	var catalog generated.ProviderCatalog
	if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
		return generated.ProviderCatalog{}, fmt.Errorf("parse embedded provider catalog: %w", err)
	}
	return catalog, nil
}
