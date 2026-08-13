package cli

import (
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

// ListHumanApprovalsConfig holds parameters for the read-only pending approval
// list command.
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

// ShowHumanApprovalConfig holds parameters for the read-only pending approval
// detail command.
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

// NewListHumanApprovals binds the approval-list operation to one HTTP
// protocol owned by the CLI composition root.
func NewListHumanApprovals(transport clihttp.Protocol) func(ListHumanApprovalsConfig) error {
	return func(cfg ListHumanApprovalsConfig) error {
		cfg.HTTP = transport
		return ListHumanApprovals(cfg)
	}
}

// NewShowHumanApproval binds the approval-detail operation to one HTTP
// protocol owned by the CLI composition root.
func NewShowHumanApproval(transport clihttp.Protocol) func(ShowHumanApprovalConfig) error {
	return func(cfg ShowHumanApprovalConfig) error {
		cfg.HTTP = transport
		return ShowHumanApproval(cfg)
	}
}

// ListHumanApprovals reads the session-scoped pending approval projection.
func ListHumanApprovals(cfg ListHumanApprovalsConfig) error {
	if err := validateHumanApprovalListConfig(cfg); err != nil {
		return err
	}
	endpoint, err := approvalEndpoint(cfg.Server, sessionpath.HumanApprovalsCollectionPath(cfg.SessionID))
	if err != nil {
		return err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug,
		"human approval list request endpointPath=%s endpoint=%s session=%s",
		endpoint.Path, endpoint.String(), clidiag.SessionLabel(cfg.SessionID))

	var payload json.RawMessage
	response, err := cfg.HTTP.GetJSON(cfg.Context, endpoint.String(), &payload)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug,
			"human approval list response endpointPath=%s error=unreachable durationMillis=%d",
			endpoint.Path, response.Duration.Milliseconds())
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
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug,
		"human approval list response endpointPath=%s status=%d durationMillis=%d resultCount=%d",
		endpoint.Path, resp.StatusCode, response.Duration.Milliseconds(), len(result.Approvals))
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

// ShowHumanApproval reads one pending approval by stable identity.
func ShowHumanApproval(cfg ShowHumanApprovalConfig) error {
	if err := validateHumanApprovalShowConfig(cfg); err != nil {
		return err
	}
	endpoint, err := approvalEndpoint(cfg.Server, sessionpath.HumanApprovalItemPath(cfg.SessionID, cfg.ApprovalID))
	if err != nil {
		return err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug,
		"human approval show request endpointPath=%s endpoint=%s session=%s approvalId=%s",
		endpoint.Path, endpoint.String(), clidiag.SessionLabel(cfg.SessionID), cfg.ApprovalID)

	var payload json.RawMessage
	response, err := cfg.HTTP.GetJSON(cfg.Context, endpoint.String(), &payload)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug,
			"human approval show response endpointPath=%s error=unreachable durationMillis=%d",
			endpoint.Path, response.Duration.Milliseconds())
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
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose || cfg.Debug,
		"human approval show response endpointPath=%s status=%d durationMillis=%d approvalId=%s",
		endpoint.Path, resp.StatusCode, response.Duration.Milliseconds(), result.ApprovalId)
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
	clidiag.Printf(diagnostics, verbose,
		"human approval response endpointPath=%s status=%d durationMillis=%d",
		endpointPath, response.StatusCode, duration.Milliseconds())
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		return fmt.Errorf("%s failed (%d): %s", operation, response.StatusCode, apiError.Message)
	}
	return fmt.Errorf("%s failed (%d)", operation, response.StatusCode)
}

func renderHumanApproval(output io.Writer, approval factoryapi.HumanApproval) error {
	rows := []struct {
		label string
		value string
	}{
		{label: "Approval ID", value: approval.ApprovalId},
		{label: "Session ID", value: approval.SessionId},
		{label: "Dispatch ID", value: approval.DispatchId},
		{label: "Workstation ID", value: approval.WorkstationId},
		{label: "Workstation", value: approval.WorkstationName},
		{label: "Description", value: optionalHumanApprovalString(approval.Description)},
		{label: "Status", value: string(approval.Status)},
		{label: "Decisions", value: joinHumanApprovalDecisions(approval.Decisions)},
		{label: "Work IDs", value: strings.Join(approval.WorkIds, ", ")},
	}
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
