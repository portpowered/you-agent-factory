package submit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	cliProgramName           = "you"
	batchSuccessMaxWorkLines = 10
)

// BatchSubmitJSONWork is one accepted work entry in structured batch submit output.
type BatchSubmitJSONWork struct {
	Name         string `json:"name"`
	WorkTypeName string `json:"workTypeName"`
	WorkID       string `json:"workId,omitempty"`
}

// BatchSubmitJSONResult is the structured stdout payload for `you submit batch --json`.
type BatchSubmitJSONResult struct {
	DryRun        bool                  `json:"dryRun,omitempty"`
	RequestID     string                `json:"requestId"`
	TraceID       string                `json:"traceId,omitempty"`
	WorkCount     int                   `json:"workCount"`
	RelationCount int                   `json:"relationCount"`
	SessionID     string                `json:"sessionId"`
	EndpointPath  string                `json:"endpointPath"`
	BatchSource   string                `json:"batchSource"`
	WorkNames     []string              `json:"workNames,omitempty"`
	Works         []BatchSubmitJSONWork `json:"works,omitempty"`
}

func batchSubmitEndpointPath(sessionID, requestID string) string {
	return sessionpath.ScopedPath("/work-requests/"+url.PathEscape(requestID), sessionID)
}

func batchSubmitSessionID(sessionID string) string {
	return clidiag.SessionLabel(sessionID)
}

func batchTraceIDFromRequest(req interfaces.WorkRequest) string {
	if req.CurrentChainingTraceID != "" {
		return req.CurrentChainingTraceID
	}
	for _, work := range req.Works {
		if work.TraceID != "" {
			return work.TraceID
		}
	}
	return ""
}

func printBatchDryRunJSON(w io.Writer, sessionID, endpointPath, batchSource string, req interfaces.WorkRequest) error {
	names := make([]string, 0, len(req.Works))
	for _, work := range req.Works {
		names = append(names, work.Name)
	}
	payload := BatchSubmitJSONResult{
		DryRun:        true,
		RequestID:     req.RequestID,
		WorkCount:     len(req.Works),
		RelationCount: len(req.Relations),
		SessionID:     batchSubmitSessionID(sessionID),
		EndpointPath:  endpointPath,
		BatchSource:   batchSource,
		WorkNames:     names,
	}
	if traceID := batchTraceIDFromRequest(req); traceID != "" {
		payload.TraceID = traceID
	}
	return json.NewEncoder(w).Encode(payload)
}

func printBatchSuccessJSON(w io.Writer, sessionID, endpointPath, batchSource string, req interfaces.WorkRequest, result factoryapi.UpsertWorkRequestResponse) error {
	payload := BatchSubmitJSONResult{
		RequestID:     result.RequestId,
		TraceID:       result.TraceId,
		WorkCount:     len(result.Works),
		RelationCount: len(req.Relations),
		SessionID:     batchSubmitSessionID(sessionID),
		EndpointPath:  endpointPath,
		BatchSource:   batchSource,
		Works:         batchSubmitJSONWorks(result.Works),
	}
	return json.NewEncoder(w).Encode(payload)
}

func batchSubmitJSONWorks(works []factoryapi.UpsertWorkRequestSubmittedWork) []BatchSubmitJSONWork {
	out := make([]BatchSubmitJSONWork, 0, len(works))
	for _, work := range works {
		item := BatchSubmitJSONWork{
			Name:         work.Name,
			WorkTypeName: work.WorkTypeName,
		}
		if workID := strings.TrimSpace(work.WorkId); workID != "" {
			item.WorkID = workID
		}
		out = append(out, item)
	}
	return out
}

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
