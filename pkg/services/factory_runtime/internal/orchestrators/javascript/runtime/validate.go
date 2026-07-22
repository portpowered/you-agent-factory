package workflowruntime

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/validation"
)

func validatePreExecution(req Request) *workflowvalidation.Issue {
	wrapped := wrapWorkflowSource(req.Source)
	result := workflowvalidation.Validate(workflowvalidation.Request{
		Source:    wrapped,
		SourceRef: req.SourceRef,
		Metadata:  req.Metadata,
	})
	if !result.HasIssues() {
		return nil
	}
	issue := result.Issues[0]
	if issue.Line > 1 {
		issue.Line--
	}
	return &issue
}

func preExecutionFailure(req Request, issue workflowvalidation.Issue) Outcome {
	return Outcome{
		OK: false,
		Failure: Failure{
			Code:    preExecutionFailureCode(issue),
			Message: formatPreExecutionMessage(req, issue),
		},
	}
}

func preExecutionFailureCode(issue workflowvalidation.Issue) string {
	if issue.Code == workflowvalidation.CodeForbiddenHostAccess {
		return CodeDeniedCapability
	}
	return CodePreExecutionInvalid
}

func formatPreExecutionMessage(req Request, issue workflowvalidation.Issue) string {
	path := strings.TrimSpace(issue.Path)
	if path == "" {
		path = strings.TrimSpace(req.SourceRef)
	}
	if path == "" {
		path = "workflow source"
	}

	core := strings.TrimSpace(issue.Message)
	if issue.Code == workflowvalidation.CodeSyntaxError {
		core = syntaxErrorCoreMessage(core)
	}

	var b strings.Builder
	b.WriteString(path)
	b.WriteString(": [")
	b.WriteString(issue.Code)
	b.WriteString("] ")
	b.WriteString(core)
	if issue.Line > 0 {
		b.WriteString(issue.LocationSuffix())
	}
	return b.String()
}

func syntaxErrorCoreMessage(message string) string {
	const prefix = "workflow source has a JavaScript syntax error"
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, prefix) {
		return trimmed
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	if remainder == "" {
		return trimmed
	}
	if strings.HasPrefix(remainder, "(") {
		if idx := strings.Index(remainder, "):"); idx >= 0 {
			return strings.TrimSpace(remainder[idx+2:])
		}
		if idx := strings.Index(remainder, ": "); idx >= 0 {
			return strings.TrimSpace(remainder[idx+2:])
		}
	}
	if strings.HasPrefix(remainder, ": ") {
		return strings.TrimSpace(strings.TrimPrefix(remainder, ": "))
	}
	return trimmed
}

func invalidArgsFailure(err error) Outcome {
	return Outcome{
		OK: false,
		Failure: Failure{
			Code:    CodePreExecutionInvalid,
			Message: fmt.Sprintf("workflow args: %s", err.Error()),
		},
	}
}
