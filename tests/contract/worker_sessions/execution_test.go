package workersessions_test

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/internal/directexample"
)

func TestPublishedExecutionRequestSatisfiesPublicSchema(t *testing.T) {
	t.Parallel()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	spec, err := loader.LoadFromFile(testutil.MustRepoPath(t, "api/openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	schema := spec.Components.Schemas["WorkerSessionStartRequest"]
	if schema == nil || schema.Value == nil {
		t.Fatal("public WorkerSessionStartRequest schema is missing")
	}
	if err := schema.Value.VisitJSON(directexample.Document(t)); err != nil {
		t.Fatalf("published request violates public schema: %v", err)
	}
}
