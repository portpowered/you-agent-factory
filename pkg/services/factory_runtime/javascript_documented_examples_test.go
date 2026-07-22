package factory_test

import (
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestJavaScriptValidation_DocumentedAgentRunExamples(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(testutil.MustRepoPath(t, "docs/reference/orchestrators.md"))
	if err != nil {
		t.Fatalf("read orchestrator documentation: %v", err)
	}

	valid := documentedJavaScriptSource(t, string(doc), "agent-run-valid")
	if result := factoryWorkflowDefinitions.Validate(factory.WorkflowValidationRequest{Source: valid}); len(result.Issues) != 0 {
		t.Fatalf("documented valid agent.run example issues = %#v", result.Issues)
	}

	invalid := documentedJavaScriptSource(t, string(doc), "agent-run-invalid")
	result := factoryWorkflowDefinitions.Validate(factory.WorkflowValidationRequest{Source: invalid})
	if len(result.Issues) != 1 {
		t.Fatalf("documented invalid agent.run example issues = %#v, want one", result.Issues)
	}
	const want = `agent.run() does not support field "writableRoots"`
	if result.Issues[0].Message != want {
		t.Fatalf("documented invalid agent.run diagnostic = %q, want %q", result.Issues[0].Message, want)
	}
	for _, secret := range []string{"/example/not-a-real-path", "Review the proposed change"} {
		if strings.Contains(result.Issues[0].Message, secret) {
			t.Fatalf("documented invalid agent.run diagnostic exposed example data %q", secret)
		}
	}
}

func documentedJavaScriptSource(t *testing.T, doc, name string) string {
	t.Helper()

	doc = strings.ReplaceAll(doc, "\r\n", "\n")
	startMarker := "```javascript " + name + "\n"
	start := strings.Index(doc, startMarker)
	if start < 0 {
		t.Fatalf("packaged agents docs missing %s example", name)
	}
	remainder := doc[start+len(startMarker):]
	end := strings.Index(remainder, "\n```")
	if end < 0 {
		t.Fatalf("packaged agents docs has unterminated %s example", name)
	}
	return remainder[:end]
}
