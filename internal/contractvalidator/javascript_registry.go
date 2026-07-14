package contractvalidator

const runtimeManifestSchemaID = "https://schemas.portpowered.com/you/contracts/javascript/runtime-manifest.schema.json"

// JavaScriptRegistry registers the JavaScript runtime-manifest schema and its valid fixtures.
func JavaScriptRegistry() Registry {
	const (
		documentationID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
		deprecationsID  = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
	)
	return NewRegistry(Entry{
		Family:        "javascript",
		FormatVersion: "1.0.0",
		Schemas: []Schema{
			{ID: documentationID, Path: "contracts/common/documentation.schema.json"},
			{ID: deprecationsID, Path: "contracts/common/deprecations.schema.json"},
			{ID: runtimeManifestSchemaID, Path: "contracts/javascript/runtime-manifest.schema.json"},
		},
		Documents: []Document{
			{Path: "contracts/testdata/javascript/valid-nested-namespace.json", SchemaID: runtimeManifestSchemaID},
			{Path: "contracts/testdata/javascript/valid-value.json", SchemaID: runtimeManifestSchemaID},
		},
	})
}
