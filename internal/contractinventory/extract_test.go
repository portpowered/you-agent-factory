package contractinventory

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestExtractFromOpenAPIYAML_StableOrderingForMultiPathDocument(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "stable-ordering.yaml")
	inventory, err := ExtractFromOpenAPIYAML(data)
	if err != nil {
		t.Fatalf("ExtractFromOpenAPIYAML() error = %v", err)
	}

	if inventory.FormatVersion != FormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, FormatVersion)
	}

	wantOrder := []struct {
		path, method, operationID string
	}{
		{"/alpha", "GET", "alphaGet"},
		{"/alpha", "POST", "alphaPost"},
		{"/beta", "GET", "betaGet"},
	}
	if len(inventory.Operations) != len(wantOrder) {
		t.Fatalf("len(operations) = %d, want %d", len(inventory.Operations), len(wantOrder))
	}
	for i, want := range wantOrder {
		got := inventory.Operations[i]
		if got.Path != want.path || got.Method != want.method || got.OperationID != want.operationID {
			t.Fatalf("operations[%d] = %+v, want path=%q method=%q operationId=%q", i, got, want.path, want.method, want.operationID)
		}
	}

	post := inventory.Operations[1]
	if post.XDocID != "alpha-post-doc" {
		t.Fatalf("alphaPost xDocId = %q, want %q", post.XDocID, "alpha-post-doc")
	}
	if !post.HasSummary || !post.HasDescription {
		t.Fatalf("alphaPost metadata = summary %v description %v, want both true", post.HasSummary, post.HasDescription)
	}
	if !slices.Equal(post.RequestMediaTypes, []string{"application/json"}) {
		t.Fatalf("alphaPost requestMediaTypes = %v, want [application/json]", post.RequestMediaTypes)
	}
	if len(post.Responses) != 1 || post.Responses[0].Status != "201" {
		t.Fatalf("alphaPost responses = %+v, want one 201 response", post.Responses)
	}
	if !slices.Equal(post.Responses[0].MediaTypes, []string{"application/json"}) {
		t.Fatalf("alphaPost response mediaTypes = %v, want [application/json]", post.Responses[0].MediaTypes)
	}

	get := inventory.Operations[0]
	if get.HasSummary || get.HasDescription || get.XDocID != "" {
		t.Fatalf("alphaGet metadata = summary %v description %v xDocId %q, want false/false/empty", get.HasSummary, get.HasDescription, get.XDocID)
	}
	if !slices.Equal(get.RequestMediaTypes, []string{}) {
		t.Fatalf("alphaGet requestMediaTypes = %v, want empty slice", get.RequestMediaTypes)
	}
	if len(get.Responses) != 1 {
		t.Fatalf("alphaGet responses = %+v, want one response", get.Responses)
	}
	if !slices.Equal(get.Responses[0].MediaTypes, []string{"application/json", "text/plain"}) {
		t.Fatalf("alphaGet response mediaTypes = %v, want [application/json text/plain]", get.Responses[0].MediaTypes)
	}
}

func TestExtractFromOpenAPIYAML_MissingOperationIDIncludesMethodAndPath(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "missing-operation-id.yaml")
	_, err := ExtractFromOpenAPIYAML(data)
	if err == nil {
		t.Fatal("ExtractFromOpenAPIYAML() error = nil, want missing operationId failure")
	}
	message := err.Error()
	if !strings.Contains(message, "missing operationId") {
		t.Fatalf("error = %q, want missing operationId diagnostic", message)
	}
	if !strings.Contains(message, "GET") || !strings.Contains(message, "/items") {
		t.Fatalf("error = %q, want GET and /items in diagnostic", message)
	}
}

func TestExtractFromOpenAPIYAML_DuplicateOperationIDIncludesMethodAndPath(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "duplicate-operation-id.yaml")
	_, err := ExtractFromOpenAPIYAML(data)
	if err == nil {
		t.Fatal("ExtractFromOpenAPIYAML() error = nil, want duplicate operationId failure")
	}
	message := err.Error()
	if !strings.Contains(message, "duplicate operationId") || !strings.Contains(message, "sharedId") {
		t.Fatalf("error = %q, want duplicate operationId diagnostic", message)
	}
	for _, want := range []string{"GET", "/alpha", "POST", "/beta"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want %q in diagnostic", message, want)
		}
	}
}

func TestExtractFromOpenAPIYAML_SuccessfulExtractionHasUniqueOperationIDs(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "stable-ordering.yaml")
	inventory, err := ExtractFromOpenAPIYAML(data)
	if err != nil {
		t.Fatalf("ExtractFromOpenAPIYAML() error = %v", err)
	}

	seen := make(map[string]struct{}, len(inventory.Operations))
	for _, operation := range inventory.Operations {
		if operation.OperationID == "" {
			t.Fatalf("operation at %s %s has empty operationId", operation.Method, operation.Path)
		}
		if _, ok := seen[operation.OperationID]; ok {
			t.Fatalf("duplicate operationId %q in successful inventory", operation.OperationID)
		}
		seen[operation.OperationID] = struct{}{}
	}
	if len(seen) != len(inventory.Operations) {
		t.Fatalf("len(unique operationIds) = %d, want %d", len(seen), len(inventory.Operations))
	}
}

func TestMarshalCanonicalJSON_IsByteIdenticalAcrossRepeatedExtractions(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "stable-ordering.yaml")
	first, err := ExtractFromOpenAPIYAML(data)
	if err != nil {
		t.Fatalf("first ExtractFromOpenAPIYAML() error = %v", err)
	}
	second, err := ExtractFromOpenAPIYAML(data)
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
		t.Fatalf("repeated extraction json differs:\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
	if firstJSON[len(firstJSON)-1] != '\n' {
		t.Fatal("canonical json should end with trailing newline")
	}
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	path := filepath.Join(filepath.Dir(file), "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return data
}
