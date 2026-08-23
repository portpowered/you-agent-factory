package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

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

	var responsePayload json.RawMessage
	response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		endpoint.String(),
		&responsePayload,
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
			return clihttp.NewAPIError(
				resp.StatusCode,
				errResp,
				fmt.Sprintf("work %q not found: %s", cfg.WorkID, errResp.Message),
				nil,
			)
		}
		return fmt.Errorf("work %q not found", cfg.WorkID)
	}
	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work show response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return clihttp.NewAPIError(
				resp.StatusCode,
				errResp,
				fmt.Sprintf("get work failed (%d): %s", resp.StatusCode, errResp.Message),
				nil,
			)
		}
		return fmt.Errorf("get work failed (%d)", resp.StatusCode)
	}
	work, err := decodeWorkResponse(responsePayload)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work show response endpointPath=%s status=%d durationMillis=%d error=decode", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		return fmt.Errorf("decode work response: %w", err)
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

func decodeWorkResponse(data []byte) (factoryapi.Work, error) {
	var work factoryapi.Work
	if err := json.Unmarshal(data, &work); err != nil {
		return factoryapi.Work{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return factoryapi.Work{}, err
	}
	structuredResult, ok := fields["structuredResult"]
	if ok && bytes.Equal(bytes.TrimSpace(structuredResult), []byte("null")) {
		work.StructuredResult = json.RawMessage("null")
	}
	return work, nil
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
	if err := writeWorkFailureDetail(output, work.FailureDetail); err != nil {
		return err
	}
	return nil
}

func writeWorkFailureDetail(output io.Writer, detail *factoryapi.FailureDetail) error {
	if detail == nil {
		return nil
	}
	if _, err := fmt.Fprintf(output, "Failure reason:\t%s\n", detail.Reason); err != nil {
		return err
	}
	_, err := fmt.Fprintf(output, "Failure message:\t%s\n", detail.Message)
	return err
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

// ListHumanApprovalsConfig holds parameters for the read-only pending approval list command.
type ListHumanApprovalsConfig struct {
	Context     context.Context
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

// ShowHumanApprovalConfig holds parameters for the read-only pending approval detail command.
type ShowHumanApprovalConfig struct {
	Context     context.Context
	Server      string
	SessionID   string
	ApprovalID  string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func NewListHumanApprovals(transport clihttp.Protocol) func(ListHumanApprovalsConfig) error {
	return func(cfg ListHumanApprovalsConfig) error { cfg.HTTP = transport; return ListHumanApprovals(cfg) }
}

func NewShowHumanApproval(transport clihttp.Protocol) func(ShowHumanApprovalConfig) error {
	return func(cfg ShowHumanApprovalConfig) error { cfg.HTTP = transport; return ShowHumanApproval(cfg) }
}

func ListHumanApprovals(cfg ListHumanApprovalsConfig) error {
	if err := validateHumanApprovalListConfig(cfg); err != nil {
		return err
	}
	endpoint, err := approvalEndpoint(cfg.Server, sessionpath.HumanApprovalsCollectionPath(cfg.SessionID))
	if err != nil {
		return err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug, "human approval list request endpointPath=%s endpoint=%s session=%s", endpoint.Path, endpoint.String(), clidiag.SessionLabel(cfg.SessionID))
	var payload json.RawMessage
	response, err := cfg.HTTP.GetJSON(cfg.Context, endpoint.String(), &payload)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug, "human approval list response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return humanApprovalHTTPError("list human approvals", resp, cfg.Diagnostics, cfg.Verbose || cfg.Debug, endpoint.Path, response.Duration)
	}
	var result factoryapi.ListHumanApprovalsResponse
	if err := json.Unmarshal(payload, &result); err != nil {
		return fmt.Errorf("decode human approval list response: %w", err)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug, "human approval list response endpointPath=%s status=%d durationMillis=%d resultCount=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds(), len(result.Approvals))
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	for _, approval := range result.Approvals {
		if err := renderHumanApproval(cfg.Output, approval); err != nil {
			return err
		}
		printHumanApprovalNextCommand(cfg.Diagnostics, !cfg.JSON, cfg.SessionID, approval.ApprovalId)
	}
	return nil
}

func ShowHumanApproval(cfg ShowHumanApprovalConfig) error {
	if err := validateHumanApprovalShowConfig(cfg); err != nil {
		return err
	}
	endpoint, err := approvalEndpoint(cfg.Server, sessionpath.HumanApprovalItemPath(cfg.SessionID, cfg.ApprovalID))
	if err != nil {
		return err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug, "human approval show request endpointPath=%s endpoint=%s session=%s approvalId=%s", endpoint.Path, endpoint.String(), clidiag.SessionLabel(cfg.SessionID), cfg.ApprovalID)
	var payload json.RawMessage
	response, err := cfg.HTTP.GetJSON(cfg.Context, endpoint.String(), &payload)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug, "human approval show response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		if apiError, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("human approval %q not found: %s", cfg.ApprovalID, apiError.Message)
		}
		return fmt.Errorf("human approval %q not found", cfg.ApprovalID)
	}
	if resp.StatusCode != http.StatusOK {
		return humanApprovalHTTPError("get human approval", resp, cfg.Diagnostics, cfg.Verbose || cfg.Debug, endpoint.Path, response.Duration)
	}
	var result factoryapi.HumanApproval
	if err := json.Unmarshal(payload, &result); err != nil {
		return fmt.Errorf("decode human approval response: %w", err)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug, "human approval show response endpointPath=%s status=%d durationMillis=%d approvalId=%s", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds(), result.ApprovalId)
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	if err := renderHumanApproval(cfg.Output, result); err != nil {
		return err
	}
	printHumanApprovalNextCommand(cfg.Diagnostics, !cfg.JSON, cfg.SessionID, result.ApprovalId)
	return nil
}

func validateHumanApprovalListConfig(cfg ListHumanApprovalsConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	return nil
}

func validateHumanApprovalShowConfig(cfg ShowHumanApprovalConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	if strings.TrimSpace(cfg.ApprovalID) == "" {
		return fmt.Errorf("approval id is required")
	}
	return nil
}

func approvalEndpoint(server, path string) (url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, path)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse human approval endpoint: %w", err)
	}
	return *endpoint, nil
}

func humanApprovalHTTPError(operation string, response *http.Response, diagnostics io.Writer, verbose bool, endpointPath string, duration time.Duration) error {
	clidiag.Printf(diagnostics, verbose, "human approval response endpointPath=%s status=%d durationMillis=%d", endpointPath, response.StatusCode, duration.Milliseconds())
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		return fmt.Errorf("%s failed (%d): %s", operation, response.StatusCode, apiError.Message)
	}
	return fmt.Errorf("%s failed (%d)", operation, response.StatusCode)
}

func renderHumanApproval(output io.Writer, approval factoryapi.HumanApproval) error {
	rows := []struct{ label, value string }{{"Approval ID", approval.ApprovalId}, {"Session ID", approval.SessionId}, {"Dispatch ID", approval.DispatchId}, {"Workstation ID", approval.WorkstationId}, {"Workstation", approval.WorkstationName}, {"Description", optionalHumanApprovalString(approval.Description)}, {"Status", string(approval.Status)}, {"Decisions", joinHumanApprovalDecisions(approval.Decisions)}, {"Work IDs", strings.Join(approval.WorkIds, ", ")}}
	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	return nil
}

func printHumanApprovalNextCommand(output io.Writer, enabled bool, sessionID, approvalID string) {
	command := fmt.Sprintf("you work approval show %s", approvalID)
	if strings.TrimSpace(sessionID) != "" {
		command += " --session " + sessionID
	}
	clidiag.Printf(output, enabled, "human approval next command: %s", command)
}

func joinHumanApprovalDecisions(decisions []factoryapi.HumanApprovalDecisions) string {
	values := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		values = append(values, string(decision))
	}
	return strings.Join(values, ", ")
}

func optionalHumanApprovalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
