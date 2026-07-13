package contractinventory

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
)

func TestRepositoryCompatibilityBaseline_IsPreservedByOpenAPI(t *testing.T) {
	t.Parallel()

	openAPIPath := testpath.MustRepoPathFromCaller(t, 0, "api", "openapi.yaml")
	baselinePath := testpath.MustRepoPathFromCaller(t, 0, "contracts", "testdata", "baseline", "rest-operations.json")

	openAPIData, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read openapi input: %v", err)
	}
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	var compatibilityBaseline Inventory
	if err := json.Unmarshal(normalizeFixtureNewlines(baseline), &compatibilityBaseline); err != nil {
		t.Fatalf("decode compatibility baseline: %v", err)
	}

	inventory, err := ExtractFromOpenAPIYAML(openAPIData)
	if err != nil {
		t.Fatalf("ExtractFromOpenAPIYAML() error = %v", err)
	}
	currentByID := make(map[string]Operation, len(inventory.Operations))
	for _, operation := range inventory.Operations {
		currentByID[operation.OperationID] = operation
	}
	for _, expected := range compatibilityBaseline.Operations {
		actual, ok := currentByID[expected.OperationID]
		if !ok {
			t.Fatalf("compatibility operation %q from %s is missing", expected.OperationID, baselinePath)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("compatibility operation %q changed: got %#v, want %#v", expected.OperationID, actual, expected)
		}
	}
}

func TestRepositoryOpenAPI_RepeatedExtractionsAreByteIdentical(t *testing.T) {
	t.Parallel()

	openAPIPath := testpath.MustRepoPathFromCaller(t, 0, "api", "openapi.yaml")
	openAPIData, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read openapi input: %v", err)
	}

	first, err := ExtractFromOpenAPIYAML(openAPIData)
	if err != nil {
		t.Fatalf("first ExtractFromOpenAPIYAML() error = %v", err)
	}
	second, err := ExtractFromOpenAPIYAML(openAPIData)
	if err != nil {
		t.Fatalf("second ExtractFromOpenAPIYAML() error = %v", err)
	}

	firstJSON, err := MarshalCanonicalJSON(first)
	if err != nil {
		t.Fatalf("first MarshalCanonicalJSON() error = %v", err)
	}
	secondJSON, err := MarshalCanonicalJSON(second)
	if err != nil {
		t.Fatalf("second MarshalCanonicalJSON() error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated repository openapi extraction json differs")
	}
}

func normalizeFixtureNewlines(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}
