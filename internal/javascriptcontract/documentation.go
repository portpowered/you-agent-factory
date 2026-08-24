package javascriptcontract

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const (
	// JavaScriptWorkflowReferencePath is the canonical source for the packaged
	// you docs javascript-workflows topic.
	JavaScriptWorkflowReferencePath = "docs/reference/javascript-workflows.md"

	// AgentRunFieldsStartMarker and AgentRunFieldsEndMarker bound the generated
	// field table. All prose outside these markers remains hand-authored.
	AgentRunFieldsStartMarker = "<!-- BEGIN GENERATED: javascript.agent.run.fields -->"
	AgentRunFieldsEndMarker   = "<!-- END GENERATED: javascript.agent.run.fields -->"
)

// GenerateJavaScriptWorkflowReference projects the runtime-owned agent.run
// descriptor into the canonical packaged reference topic.
func GenerateJavaScriptWorkflowReference(repositoryRoot string) error {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(JavaScriptWorkflowReferencePath))
	document, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", JavaScriptWorkflowReferencePath, err)
	}
	projected, err := ProjectJavaScriptWorkflowReference(document, factoryruntime.JavaScriptChildFieldDescriptors())
	if err != nil {
		return err
	}
	if bytes.Equal(document, projected) {
		return nil
	}
	if err := os.WriteFile(path, projected, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", JavaScriptWorkflowReferencePath, err)
	}
	return nil
}

// ProjectJavaScriptWorkflowReference replaces only the bounded agent.run
// field table in a reference document. The explicit fields parameter keeps
// forward-evolution tests independent from filesystem state while production
// generation supplies the runtime-owned descriptor.
func ProjectJavaScriptWorkflowReference(document []byte, fields []factoryruntime.JavaScriptChildFieldDescriptor) ([]byte, error) {
	if err := validateFields(fields); err != nil {
		return nil, err
	}
	for _, field := range fields {
		if strings.ContainsAny(field.Name, "|`\r\n") {
			return nil, fmt.Errorf("%s cannot render runtime descriptor field %q in the generated Markdown table", JavaScriptWorkflowReferencePath, field.Name)
		}
		for _, allowed := range field.Enum {
			if strings.ContainsAny(allowed, "|`\r\n") {
				return nil, fmt.Errorf("%s cannot render enum value %q for runtime descriptor field %q in the generated Markdown table", JavaScriptWorkflowReferencePath, allowed, field.Name)
			}
		}
	}
	payload := renderAgentRunFields(fields)
	return replaceGeneratedAgentRunRegion(document, payload)
}

func renderAgentRunFields(fields []factoryruntime.JavaScriptChildFieldDescriptor) []byte {
	var table strings.Builder
	table.WriteString("### `agent.run` request fields\n\n")
	table.WriteString("| Field | JSON type | Requiredness | Allowed values |\n")
	table.WriteString("|-------|-----------|--------------|----------------|\n")
	for _, field := range fields {
		requiredness := "optional"
		if field.Required {
			requiredness = "required"
		}
		allowed := "—"
		if len(field.Enum) > 0 {
			allowed = strings.Join(field.Enum, ", ")
		}
		fmt.Fprintf(&table, "| `%s` | `%s` | %s | %s |\n", field.Name, field.JSONType, requiredness, allowed)
	}
	return []byte(strings.TrimSuffix(table.String(), "\n"))
}

func replaceGeneratedAgentRunRegion(document, payload []byte) ([]byte, error) {
	if count := bytes.Count(document, []byte(AgentRunFieldsStartMarker)); count != 1 {
		return nil, fmt.Errorf("%s must contain exactly one %s marker; found %d; restore the generated region before running make contracts-generate", JavaScriptWorkflowReferencePath, AgentRunFieldsStartMarker, count)
	}
	if count := bytes.Count(document, []byte(AgentRunFieldsEndMarker)); count != 1 {
		return nil, fmt.Errorf("%s must contain exactly one %s marker; found %d; restore the generated region before running make contracts-generate", JavaScriptWorkflowReferencePath, AgentRunFieldsEndMarker, count)
	}
	startMarker := bytes.Index(document, []byte(AgentRunFieldsStartMarker))
	endMarker := bytes.Index(document, []byte(AgentRunFieldsEndMarker))
	if endMarker <= startMarker {
		return nil, fmt.Errorf("%s has an invalid generated agent.run marker order; %s must precede %s", JavaScriptWorkflowReferencePath, AgentRunFieldsStartMarker, AgentRunFieldsEndMarker)
	}
	startLineStart, startLineEnd := markdownLineBounds(document, startMarker)
	endLineStart, endLineEnd := markdownLineBounds(document, endMarker)
	if string(document[startLineStart:startLineEnd]) != AgentRunFieldsStartMarker {
		return nil, fmt.Errorf("%s has an invalid generated agent.run start marker line; keep %s on its own line", JavaScriptWorkflowReferencePath, AgentRunFieldsStartMarker)
	}
	if string(document[endLineStart:endLineEnd]) != AgentRunFieldsEndMarker {
		return nil, fmt.Errorf("%s has an invalid generated agent.run end marker line; keep %s on its own line", JavaScriptWorkflowReferencePath, AgentRunFieldsEndMarker)
	}
	lineEnding := []byte("\n")
	if startLineEnd < len(document) && document[startLineEnd] == '\r' {
		lineEnding = []byte("\r\n")
	}
	replacement := make([]byte, 0, len(AgentRunFieldsStartMarker)+len(lineEnding)*2+len(payload)+len(AgentRunFieldsEndMarker))
	replacement = append(replacement, AgentRunFieldsStartMarker...)
	replacement = append(replacement, lineEnding...)
	replacement = append(replacement, payload...)
	replacement = append(replacement, lineEnding...)
	replacement = append(replacement, AgentRunFieldsEndMarker...)
	projected := make([]byte, 0, len(document)-(endLineEnd-startLineStart)+len(replacement))
	projected = append(projected, document[:startLineStart]...)
	projected = append(projected, replacement...)
	projected = append(projected, document[endLineEnd:]...)
	return projected, nil
}

func markdownLineBounds(document []byte, position int) (start, end int) {
	start = bytes.LastIndexByte(document[:position], '\n') + 1
	lineEnd := bytes.IndexByte(document[position:], '\n')
	if lineEnd < 0 {
		return start, len(document)
	}
	end = position + lineEnd
	if end > start && document[end-1] == '\r' {
		return start, end - 1
	}
	return start, end
}
