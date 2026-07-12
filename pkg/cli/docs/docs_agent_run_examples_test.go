package docs

import (
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/childcontract"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

func TestMarkdown_AgentRunExamplesMatchExecutableValidation(t *testing.T) {
	t.Parallel()

	doc, err := Markdown("orchestrators")
	if err != nil {
		t.Fatalf("Markdown(orchestrators) error = %v", err)
	}
	if got, want := documentedAgentRunFields(doc), childcontract.SupportedFields(); !reflect.DeepEqual(got, want) {
		t.Fatalf("documented agent.run fields = %v, want executable fields %v", got, want)
	}

	valid := documentedJavaScriptExample(t, doc, "agent-run-valid")
	if result := workflowvalidation.Validate(workflowvalidation.Request{Source: valid}); len(result.Issues) != 0 {
		t.Fatalf("documented valid agent.run example issues = %#v", result.Issues)
	}

	invalid := documentedJavaScriptExample(t, doc, "agent-run-invalid")
	result := workflowvalidation.Validate(workflowvalidation.Request{Source: invalid})
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
