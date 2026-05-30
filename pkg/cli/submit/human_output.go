package submit

import (
	"fmt"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func writeHumanSubmitSuccess(w io.Writer, result factoryapi.SubmitWorkResponse, fallbackName, fallbackWorkType string) error {
	name := submitResponseString(result.Name, fallbackName)
	workType := submitResponseString(result.WorkTypeName, fallbackWorkType)

	if _, err := fmt.Fprintf(w, "Submitted: %s (%s)\n", name, workType); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "traceId: %s\n", result.TraceId); err != nil {
		return err
	}

	workID := submitResponseString(result.WorkId, "")
	if workID != "" {
		if _, err := fmt.Fprintf(w, "workId: %s\n", workID); err != nil {
			return err
		}
		_, err := fmt.Fprintf(w, "Verify: you work show %s\n", workID)
		return err
	}

	if _, err := fmt.Fprintln(w, "workId was not returned; verify with:"); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "you work list --name %s\n", name)
	return err
}

func submitResponseString(ptr *string, fallback string) string {
	if ptr != nil {
		if trimmed := strings.TrimSpace(*ptr); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(fallback)
}
