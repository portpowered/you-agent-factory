package javascriptcontract

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestCheckGeneratedOutputsPassesAlignedOutputsDeterministicallyWithoutWriting(t *testing.T) {
	root := generatedCheckFixture(t)
	before := generatedCheckTree(t, root)

	first, err := CheckGeneratedOutputs(root)
	if err != nil {
		t.Fatalf("first CheckGeneratedOutputs() error = %v", err)
	}
	second, err := CheckGeneratedOutputs(root)
	if err != nil {
		t.Fatalf("second CheckGeneratedOutputs() error = %v", err)
	}
	if len(first) != 0 || len(second) != 0 {
		t.Fatalf("aligned diagnostics = first=%+v second=%+v, want none", first, second)
	}
	if after := generatedCheckTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("CheckGeneratedOutputs() changed aligned outputs")
	}
}

func TestCheckGeneratedOutputsReportsMissingCanonicalFieldWithoutWriting(t *testing.T) {
	root := generatedCheckFixture(t)
	path := filepath.Join(root, filepath.FromSlash(RuntimeCatalogPath))
	mutateCatalog(t, path, func(properties map[string]any) {
		delete(properties, "resourceId")
	})
	before := generatedCheckTree(t, root)

	first, err := CheckGeneratedOutputs(root)
	if err != nil {
		t.Fatalf("first CheckGeneratedOutputs() error = %v", err)
	}
	second, err := CheckGeneratedOutputs(root)
	if err != nil {
		t.Fatalf("second CheckGeneratedOutputs() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("diagnostics are not deterministic: first=%+v second=%+v", first, second)
	}
	assertFieldDiagnostic(t, first, RuntimeCatalogPath, "resourceId", "missing")
	if !strings.Contains(first[0].Message, "make contracts-generate") {
		t.Fatalf("diagnostic message = %q, want regeneration command", first[0].Message)
	}
	if after := generatedCheckTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("CheckGeneratedOutputs() rewrote missing-field fixture")
	}
}

func TestCheckGeneratedOutputsReportsExtraStagedFieldWithoutWriting(t *testing.T) {
	root := generatedCheckFixture(t)
	path := filepath.Join(root, filepath.FromSlash(StagedRuntimeCatalogPath))
	mutateCatalog(t, path, func(properties map[string]any) {
		properties["futureField"] = map[string]any{"type": "string"}
	})
	before := generatedCheckTree(t, root)

	diagnostics, err := CheckGeneratedOutputs(root)
	if err != nil {
		t.Fatalf("CheckGeneratedOutputs() error = %v", err)
	}
	assertFieldDiagnostic(t, diagnostics, StagedRuntimeCatalogPath, "futureField", "extra")
	if after := generatedCheckTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("CheckGeneratedOutputs() rewrote extra-field fixture")
	}
}

func TestCheckGeneratedOutputsReportsStaleDocumentationFieldWithoutWriting(t *testing.T) {
	root := generatedCheckFixture(t)
	path := filepath.Join(root, filepath.FromSlash(JavaScriptWorkflowReferencePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read documentation: %v", err)
	}
	updated := bytes.Replace(payload, []byte("| `resourceId` | `string` | optional | — |\n"), nil, 1)
	if bytes.Equal(payload, updated) {
		t.Fatal("documentation fixture did not contain resourceId row")
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write documentation: %v", err)
	}
	before := generatedCheckTree(t, root)

	diagnostics, err := CheckGeneratedOutputs(root)
	if err != nil {
		t.Fatalf("CheckGeneratedOutputs() error = %v", err)
	}
	assertFieldDiagnostic(t, diagnostics, JavaScriptWorkflowReferencePath, "resourceId", "missing")
	if after := generatedCheckTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("CheckGeneratedOutputs() rewrote documentation fixture")
	}
}

func TestCheckGeneratedOutputsRejectsInvalidDocumentationProjection(t *testing.T) {
	root := generatedCheckFixture(t)
	path := filepath.Join(root, filepath.FromSlash(JavaScriptWorkflowReferencePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read documentation: %v", err)
	}
	updated := bytes.Replace(payload, []byte("| `resourceId` | `string` | optional | — |\n"), []byte("| resourceId | `string` | optional | — |\n"), 1)
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write documentation: %v", err)
	}

	_, err = CheckGeneratedOutputs(root)
	if err == nil || !strings.Contains(err.Error(), JavaScriptWorkflowReferencePath) || !strings.Contains(err.Error(), "make contracts-generate") {
		t.Fatalf("CheckGeneratedOutputs() error = %v, want actionable documentation error", err)
	}
}

func generatedCheckFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	catalog, err := GenerateRuntimeCatalog([]byte(minimalCatalog))
	if err != nil {
		t.Fatalf("GenerateRuntimeCatalog() = %v", err)
	}
	documentation, err := ProjectJavaScriptWorkflowReference(documentationFixtureBytes(), factoryruntime.JavaScriptChildFieldDescriptors())
	if err != nil {
		t.Fatalf("ProjectJavaScriptWorkflowReference() = %v", err)
	}
	writeGeneratedCheckFixture(t, root, RuntimeCatalogPath, catalog)
	writeGeneratedCheckFixture(t, root, StagedRuntimeCatalogPath, catalog)
	writeGeneratedCheckFixture(t, root, JavaScriptWorkflowReferencePath, documentation)
	return root
}

func documentationFixtureBytes() []byte {
	return []byte("before prose\n\n" + AgentRunFieldsStartMarker + "\nold generated fields\n" + AgentRunFieldsEndMarker + "\n\nafter prose\n")
}

func mutateCatalog(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	sharedSchemas := document["sharedSchemas"].(map[string]any)
	agentRun := sharedSchemas["javascript.schema.agent_run_spec"].(map[string]any)
	schema := agentRun["schema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	mutate(properties)
	updated, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode catalog: %v", err)
	}
	updated = append(updated, '\n')
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}

func writeGeneratedCheckFixture(t *testing.T, root, path string, payload []byte) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func generatedCheckTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	paths := []string{RuntimeCatalogPath, StagedRuntimeCatalogPath, JavaScriptWorkflowReferencePath}
	tree := make(map[string][]byte, len(paths))
	for _, path := range paths {
		payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read fixture %s: %v", path, err)
		}
		tree[path] = append([]byte(nil), payload...)
	}
	return tree
}

func assertFieldDiagnostic(t *testing.T, diagnostics []Diagnostic, path, field, issue string) {
	t.Helper()
	code := "javascript.agent_run.field." + issue
	for _, diagnostic := range diagnostics {
		if diagnostic.Path == path && diagnostic.Field == field && diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("diagnostics = %+v, want %s for %s/%s", diagnostics, code, path, field)
}
