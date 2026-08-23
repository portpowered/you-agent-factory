package javascriptcontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const documentationFixture = "before prose\n\n" + AgentRunFieldsStartMarker + "\nold generated fields\n" + AgentRunFieldsEndMarker + "\n\nafter prose\n"

func TestProjectJavaScriptWorkflowReferenceProjectsRuntimeFields(t *testing.T) {
	projected, err := ProjectJavaScriptWorkflowReference([]byte(documentationFixture), factoryruntime.JavaScriptChildFieldDescriptors())
	if err != nil {
		t.Fatalf("ProjectJavaScriptWorkflowReference() error = %v", err)
	}
	text := string(projected)
	for _, want := range []string{
		"| `prompt` | `string` | required |",
		"| `executorProvider` | `string` | optional |",
		"| `resourceId` | `string` | optional |",
		"| `skipPermissions` | `boolean` | optional |",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("projected reference does not contain %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "before prose\n") || !strings.Contains(text, "\nafter prose\n") {
		t.Fatal("projection changed hand-authored prose outside the generated region")
	}
	if strings.Contains(text, "old generated fields") {
		t.Fatal("projection retained stale generated field content")
	}
}

func TestProjectRuntimeAndDocumentationOutputsSupportForwardFieldEvolution(t *testing.T) {
	fields := append(factoryruntime.JavaScriptChildFieldDescriptors(), factoryruntime.JavaScriptChildFieldDescriptor{
		Name: "futureField", JSONType: "string",
	})

	catalog, err := ProjectRuntimeCatalog([]byte(minimalCatalog), fields)
	if err != nil {
		t.Fatalf("ProjectRuntimeCatalog() error = %v", err)
	}
	if !strings.Contains(string(catalog), `"futureField"`) {
		t.Fatal("projected catalog omitted forward-evolved futureField")
	}

	documentation, err := ProjectJavaScriptWorkflowReference([]byte(documentationFixture), fields)
	if err != nil {
		t.Fatalf("ProjectJavaScriptWorkflowReference() error = %v", err)
	}
	if !strings.Contains(string(documentation), "| `futureField` | `string` | optional |") {
		t.Fatalf("projected documentation omitted forward-evolved futureField:\n%s", documentation)
	}
}

func TestProjectJavaScriptWorkflowReferenceIsDeterministic(t *testing.T) {
	fields := factoryruntime.JavaScriptChildFieldDescriptors()
	first, err := ProjectJavaScriptWorkflowReference([]byte(documentationFixture), fields)
	if err != nil {
		t.Fatalf("first projection error = %v", err)
	}
	second, err := ProjectJavaScriptWorkflowReference([]byte(documentationFixture), fields)
	if err != nil {
		t.Fatalf("second projection error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("repeated documentation projection changed bytes")
	}
}

func TestProjectJavaScriptWorkflowReferenceRejectsInvalidMarkers(t *testing.T) {
	tests := []struct {
		name       string
		document   string
		wantErrors []string
	}{
		{
			name:       "missing end marker",
			document:   "before\n" + AgentRunFieldsStartMarker + "\nold\n",
			wantErrors: []string{"exactly one", AgentRunFieldsEndMarker, "make contracts-generate"},
		},
		{
			name:       "reversed markers",
			document:   "before\n" + AgentRunFieldsEndMarker + "\nold\n" + AgentRunFieldsStartMarker + "\n",
			wantErrors: []string{"invalid generated agent.run marker order"},
		},
		{
			name:       "marker is not on its own line",
			document:   "before " + AgentRunFieldsStartMarker + "\nold\n" + AgentRunFieldsEndMarker + "\n",
			wantErrors: []string{"invalid generated agent.run start marker line", AgentRunFieldsStartMarker},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ProjectJavaScriptWorkflowReference([]byte(test.document), factoryruntime.JavaScriptChildFieldDescriptors())
			if err == nil {
				t.Fatal("projection succeeded for invalid marker input")
			}
			for _, want := range test.wantErrors {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want substring %q", err, want)
				}
			}
		})
	}
}

func TestGenerateJavaScriptWorkflowReferenceWritesOnlyProjectedRegion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(JavaScriptWorkflowReferencePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create docs directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(documentationFixture), 0o644); err != nil {
		t.Fatalf("write docs fixture: %v", err)
	}

	if err := GenerateJavaScriptWorkflowReference(root); err != nil {
		t.Fatalf("GenerateJavaScriptWorkflowReference() error = %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated docs: %v", err)
	}
	if err := GenerateJavaScriptWorkflowReference(root); err != nil {
		t.Fatalf("second GenerateJavaScriptWorkflowReference() error = %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read second generated docs: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("repeated file generation changed bytes")
	}
	if !strings.Contains(string(first), "| `resourceId` | `string` | optional |") {
		t.Fatal("file generation did not project the runtime resourceId field")
	}
}
