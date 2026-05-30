package submit

import (
	"fmt"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	cliProgramName           = "you"
	batchSuccessMaxWorkLines = 10
)

func printBatchSuccessHuman(w io.Writer, req interfaces.WorkRequest, result factoryapi.UpsertWorkRequestResponse) error {
	if _, err := fmt.Fprintf(w, "requestId: %s\n", result.RequestId); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "traceId: %s\n", result.TraceId); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "work count: %d\n", len(result.Works)); err != nil {
		return err
	}
	if relationCount := len(req.Relations); relationCount > 0 {
		if _, err := fmt.Fprintf(w, "relationCount: %d\n", relationCount); err != nil {
			return err
		}
	}

	works := result.Works
	shown := len(works)
	if shown > batchSuccessMaxWorkLines {
		shown = batchSuccessMaxWorkLines
	}
	for i := 0; i < shown; i++ {
		if err := printBatchSuccessWorkLine(w, works[i]); err != nil {
			return err
		}
	}
	if remaining := len(works) - shown; remaining > 0 {
		_, err := fmt.Fprintf(w, "... and %d more work(s)\n", remaining)
		return err
	}
	return nil
}

func printBatchSuccessWorkLine(w io.Writer, work factoryapi.UpsertWorkRequestSubmittedWork) error {
	line := fmt.Sprintf("  %s (%s)", work.Name, work.WorkTypeName)
	if workID := strings.TrimSpace(work.WorkId); workID != "" {
		line += " workId=" + workID
	}
	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "  %s\n", batchSuccessWorkHint(work))
	return err
}

func batchSuccessWorkHint(work factoryapi.UpsertWorkRequestSubmittedWork) string {
	if workID := strings.TrimSpace(work.WorkId); workID != "" {
		return cliProgramName + " work show " + workID
	}
	return cliProgramName + " work list --name " + work.Name
}
