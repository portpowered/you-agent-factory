package docs

import (
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestMarkdown_AgentRunExamplesMatchExecutableValidation(t *testing.T) {
	t.Parallel()

	doc, err := Markdown("orchestrators")
	if err != nil {
		t.Fatalf("Markdown(orchestrators) error = %v", err)
	}
	if got, want := documentedAgentRunFields(doc), factory.JavaScriptChildSupportedFields(); !reflect.DeepEqual(got, want) {
		t.Fatalf("documented agent.run fields = %v, want executable fields %v", got, want)
	}

	valid := documentedJavaScriptExample(t, doc, "agent-run-valid")
	if !strings.Contains(valid, "agent.run") {
		t.Fatalf("documented valid example = %q, want agent.run call", valid)
	}

	invalid := documentedJavaScriptExample(t, doc, "agent-run-invalid")
	if !strings.Contains(invalid, "writableRoots") {
		t.Fatalf("documented invalid example = %q, want unsupported field fixture", invalid)
	}
}

func TestDocumentedJavaScriptExample_AcceptsCRLF(t *testing.T) {
	t.Parallel()

	doc, err := Markdown("orchestrators")
	if err != nil {
		t.Fatalf("Markdown(orchestrators) error = %v", err)
	}
	doc = strings.ReplaceAll(doc, "\r\n", "\n")
	doc = strings.ReplaceAll(doc, "\n", "\r\n")
	if got := documentedJavaScriptExample(t, doc, "agent-run-valid"); !strings.Contains(got, "agent.run") {
		t.Fatalf("documented valid agent.run example = %q, want agent.run call", got)
	}
}

func documentedAgentRunFields(doc string) []string {
	section := doc[strings.Index(doc, "### Beta JavaScript child-agent contract"):]
	section = section[:strings.Index(section, "This complete example")]
	var fields []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "| `") {
			fields = append(fields, strings.Split(line, "`")[1])
		}
	}
	return fields
}

func documentedJavaScriptExample(t *testing.T, doc, name string) string {
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
