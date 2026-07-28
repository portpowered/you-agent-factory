package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ShowConfig holds parameters for the work show command.
type ShowConfig struct {
	Context     context.Context
	Server      string
	SessionID   string
	WorkID      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func NewShow(transport clihttp.Protocol) func(ShowConfig) error {
	svc := New(Config{})
	return func(cfg ShowConfig) error {
		cfg.HTTP = transport
		return svc.Show(cfg)
	}
}

// Show requests one work item from a running factory via HTTP.
func Show(cfg ShowConfig) error {
	return New(Config{}).Show(cfg)
}

func (service *service) Show(cfg ShowConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	workID := strings.TrimSpace(cfg.WorkID)
	if workID == "" {
		return fmt.Errorf("work id is required")
	}
	cfg.WorkID = workID

	endpoint, err := showEndpoint(cfg)
	if err != nil {
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work show request endpointPath=%s endpoint=%s server=%s session=%s workId=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		cfg.WorkID,
	)

	var work factoryapi.Work
	response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		endpoint.String(),
		&work,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work show response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work show response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("work %q not found: %s", cfg.WorkID, errResp.Message)
		}
		return fmt.Errorf("work %q not found", cfg.WorkID)
	}
	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work show response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("get work failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("get work failed (%d)", resp.StatusCode)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work show response endpointPath=%s status=%d durationMillis=%d workId=%s",
		endpoint.Path,
		resp.StatusCode,
		response.Duration.Milliseconds(),
		stringValue(work.WorkId),
	)
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(work)
	}
	return renderShowResult(cfg.Output, work)
}

func showEndpoint(cfg ShowConfig) (url.URL, error) {
	endpointPath := sessionpath.WorkItemPath(cfg.SessionID, cfg.WorkID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse work show endpoint: %w", err)
	}
	return *endpoint, nil
}

func renderShowResult(output io.Writer, work factoryapi.Work) error {
	stateName, stateType := workStateColumns(work.State)
	rows := []struct {
		label string
		value string
	}{
		{label: "Work ID", value: stringValue(work.WorkId)},
		{label: "Name", value: work.Name},
		{label: "Work type", value: stringValue(work.WorkTypeName)},
		{label: "State name", value: stateName},
		{label: "State type", value: stateType},
		{label: "Trace", value: primaryWorkTrace(work)},
		{label: "Relations", value: formatWorkRelations(work.Relations)},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	if err := writeWorkStopSummary(output, work.StopSummary); err != nil {
		return err
	}
	return nil
}

func primaryWorkTrace(work factoryapi.Work) string {
	if work.CurrentChainingTraceId != nil && strings.TrimSpace(*work.CurrentChainingTraceId) != "" {
		return strings.TrimSpace(*work.CurrentChainingTraceId)
	}
	if work.TraceId != nil && strings.TrimSpace(*work.TraceId) != "" {
		return strings.TrimSpace(*work.TraceId)
	}
	return ""
}

func writeWorkStopSummary(output io.Writer, summary *factoryapi.FactoryStopSummary) error {
	if summary == nil {
		return nil
	}
	fields := workStopSummaryFields(summary)
	if summary.SessionLifecycleStatus != nil {
		fields = append(fields, "lifecycle="+string(*summary.SessionLifecycleStatus))
	}
	if _, err := fmt.Fprintf(output, "Stop summary:\t%s\n", strings.Join(fields, " ")); err != nil {
		return err
	}
	return writeStopSummaryDetailLines(output, summary)
}

func workStopSummaryFields(summary *factoryapi.FactoryStopSummary) []string {
	fields := []string{
		fmt.Sprintf("kind=%s", summary.StopKind),
		fmt.Sprintf("session=%s", summary.SessionId),
	}
	if stateField := trimmedString(summary.WorkState); stateField != "" {
		fields = append(fields, "state="+stateField)
	}
	return fields
}

func writeStopSummaryDetailLines(output io.Writer, summary *factoryapi.FactoryStopSummary) error {
	if err := writeStopDispatchLine(output, summary.LatestDispatch); err != nil {
		return err
	}
	if err := writeOptionalStopSummaryLine(output, "Stop result", summary.LatestResultSummary); err != nil {
		return err
	}
	if err := writeOptionalStopSummaryLine(output, "Recovery surface", summary.SuggestedRecoverySurface); err != nil {
		return err
	}
	return writeOptionalStopSummaryLine(output, "Recovery action", summary.SuggestedRecoveryAction)
}

func writeStopDispatchLine(output io.Writer, dispatch *factoryapi.FactoryStopDispatchSummary) error {
	if dispatch == nil {
		return nil
	}
	dispatchFields := []string{
		dispatch.DispatchId,
		fmt.Sprintf("status=%s", dispatch.Status),
		fmt.Sprintf("kind=%s", dispatch.DispatchKind),
	}
	if workstation := trimmedString(dispatch.WorkstationName); workstation != "" {
		dispatchFields = append(dispatchFields, "workstation="+workstation)
	}
	_, err := fmt.Fprintf(output, "Stop dispatch:\t%s\n", strings.Join(dispatchFields, " "))
	return err
}

func writeOptionalStopSummaryLine(output io.Writer, label string, value *string) error {
	text := trimmedString(value)
	if text == "" {
		return nil
	}
	_, err := fmt.Fprintf(output, "%s:\t%s\n", label, text)
	return err
}

func trimmedString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
