package apicontract_test

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func assertOpenAPI3RefPropertyDescription(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string) *openapi3.Schema {
	t.Helper()
	property, ok := schema.Properties[propertyName]
	if !ok || property == nil || property.Value == nil {
		t.Fatalf("%s.properties.%s is missing", schemaName, propertyName)
	}
	assertOpenAPI3Description(t, schemaName+".properties."+propertyName, property.Value.Description)
	return resolveOpenAPI3ReferencedPropertySchema(t, property.Value, schemaName+".properties."+propertyName)
}

func resolveOpenAPI3ReferencedPropertySchema(t *testing.T, schema *openapi3.Schema, path string) *openapi3.Schema {
	t.Helper()
	if schema == nil {
		t.Fatalf("%s schema is missing", path)
	}
	if len(schema.AllOf) == 1 {
		if schema.AllOf[0].Value != nil {
			return schema.AllOf[0].Value
		}
		if schema.AllOf[0].Ref != "" {
			t.Fatalf("%s allOf[0] ref %q is not resolved", path, schema.AllOf[0].Ref)
		}
	}
	return schema
}
