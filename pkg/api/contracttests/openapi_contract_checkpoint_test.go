package apicontract_test

import (
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestOpenAPIContract_CheckpointSchemasExposeArtifactMetadataOnly(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	assertSchemaPropertyRef(t, schemas, "FactorySessionJavaScriptCheckpointRef", "artifactRef", "#/components/schemas/FactoryArtifactRef")
	assertSchemaPropertyRef(t, schemas, "FactorySessionResult", "resultArtifactRef", "#/components/schemas/FactoryArtifactRef")
	assertSchemaPropertyRef(t, schemas, "FactorySessionPartialResult", "partialResultArtifactRef", "#/components/schemas/FactoryArtifactRef")
	assertSchemaPropertyRef(t, schemas, "JavaScriptCheckpointRefEventPayload", "artifactRef", "#/components/schemas/FactoryArtifactRef")
	assertEnumValues(t, schemaObject(t, schemas, "FactoryArtifactVisibility"), "FactoryArtifactVisibility", []string{"PUBLIC", "INTERNAL_CHECKPOINT"})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryArtifactKind"), "FactoryArtifactKind", []string{"CHECKPOINT"})
}

func TestGeneratedCheckpointContracts_ArtifactTypesAgreeWithOpenAPI(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionJavaScriptCheckpointRef{}), "ArtifactRef", reflect.TypeOf((*factoryapi.FactoryArtifactRef)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionResult{}), "ResultArtifactRef", reflect.TypeOf((*factoryapi.FactoryArtifactRef)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionPartialResult{}), "PartialResultArtifactRef", reflect.TypeOf((*factoryapi.FactoryArtifactRef)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.JavaScriptCheckpointRefEventPayload{}), "ArtifactRef", reflect.TypeOf(factoryapi.FactoryArtifactRef{}))
}
