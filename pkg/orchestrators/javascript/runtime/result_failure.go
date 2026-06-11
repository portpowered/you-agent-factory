package workflowruntime

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

func invalidResultFailure(validation workflowresult.Result) Outcome {
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
