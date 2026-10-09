package workersessions_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/internal/testutil"
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
	if err := schema.Value.VisitJSON(publishedExecutionDocument(t)); err != nil {
		t.Fatalf("published request violates public schema: %v", err)
	}
}

// Keep this docs fixture in the test compilation rather than the production graph.
func publishedExecutionDocument(t testing.TB) map[string]any {
	t.Helper()
	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/reference/operations.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(strings.ReplaceAll(string(data), "\r\n", "\n"), "Save this complete request as `execution.json`:")
	if !ok {
		t.Fatal("published execution request is missing")
	}
	_, block, ok := strings.Cut(section, "```json\n")
	if !ok {
		t.Fatal("published execution JSON is missing")
	}
	block, _, ok = strings.Cut(block, "\n```")
	if !ok {
		t.Fatal("published execution JSON is incomplete")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(block), &document); err != nil {
		t.Fatalf("published execution JSON: %v", err)
	}
	return document
}
