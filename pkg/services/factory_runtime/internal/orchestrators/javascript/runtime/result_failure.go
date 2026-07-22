package workflowruntime

import (
	"strings"

	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime/runtimecontract"
)

func invalidResultFailure(validation workflowresult.ResultValidation) Outcome {
	issue := validation.Issues[0]
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(issue.Code)
	b.WriteString("] ")
	b.WriteString(strings.TrimSpace(issue.Message))
	if path := strings.TrimSpace(issue.Path); path != "" && path != "$" {
		b.WriteString(" at ")
		b.WriteString(path)
	}
	return Outcome{
		OK: false,
		Failure: Failure{
			Code:    CodeInvalidResult,
			Message: b.String(),
		},
	}
}
