package contractinventory

import (
	"bytes"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
)

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
