package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ListConfig holds parameters for the Worker Sessions list command.
type ListConfig struct {
	Context       context.Context
	Server        string
	SessionID     string
	WorkID        string
	Scope         string
	States        []string
	Limit         int
	LimitSet      bool
	MaxResults    int
	MaxResultsSet bool
	NextToken     string
	OutputFormat  string
	JSON          bool
	Verbose       bool
	Debug         bool
	Output        io.Writer
	Diagnostics   io.Writer
	HTTP          clihttp.Protocol
}

// NewList returns the composition-facing list operation bound to one HTTP
// protocol.
func NewList(transport clihttp.Protocol) func(ListConfig) error {
	return func(config ListConfig) error {
		config.HTTP = transport
		return list(config)
	}
}

func list(config ListConfig) error {
	config.WorkID = strings.TrimSpace(config.WorkID)
	config.Scope = strings.TrimSpace(config.Scope)
	config.NextToken = strings.TrimSpace(config.NextToken)
	for index := range config.States {
		config.States[index] = strings.TrimSpace(config.States[index])
	}
	jsonOutput := config.JSON || strings.EqualFold(strings.TrimSpace(config.OutputFormat), "json")
	if err := validateListConfig(config); err != nil {
		return emitCLIError(config, jsonOutput, err)
	}

	format, err := normalizeOutputFormat(config.OutputFormat)
	if err != nil {
		return emitCLIError(config, jsonOutput, err)
	}
	jsonOutput = config.JSON || format == "json"
	endpoint, err := workerSessionsEndpoint(config.Server, config.SessionID, config.WorkID, config.Scope, config.States, config.Limit, config.LimitSet, config.MaxResults, config.NextToken)
	if err != nil {
		return err
	}
	clidiag.Printf(
		config.Diagnostics,
		config.Verbose || config.Debug,
		"worker sessions list request endpointPath=%s endpoint=%s server=%s session=%s workID=%s scope=%s stateCount=%d",
		endpoint.Path,
		endpoint.String(),
		config.Server,
		clidiag.SessionLabel(config.SessionID),
		config.WorkID,
		config.Scope,
		len(config.States),
	)

	var result factoryapi.ListWorkerSessionsResponse
	response, requestErr := config.HTTP.GetJSON(config.Context, endpoint.String(), &result)
	if requestErr != nil {
		clidiag.Printf(
			config.Diagnostics,
			config.Verbose || config.Debug,
			"worker sessions list response endpointPath=%s error=unreachable durationMillis=%d",
			endpoint.Path,
			response.Duration.Milliseconds(),
		)
		return emitCLIError(config, jsonOutput, newCLIError(
			"FACTORY_UNREACHABLE",
			fmt.Sprintf("factory not reachable at %s", endpoint.String()),
			requestErr,
		))
	}
	if response.HTTP == nil {
		return emitCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_LIST_FAILED", "worker session list returned no HTTP response", nil))
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusOK {
		return emitCLIError(config, jsonOutput, workerSessionsHTTPError(response.HTTP, response.HTTP.StatusCode, strings.TrimSpace(config.WorkID) == ""))
	}
	if result.Sessions == nil {
		result.Sessions = []factoryapi.WorkerSessionObservation{}
	}
	clidiag.Printf(
		config.Diagnostics,
		config.Verbose || config.Debug,
		"worker sessions list response endpointPath=%s status=%d durationMillis=%d resultCount=%d",
		endpoint.Path,
		response.HTTP.StatusCode,
		response.Duration.Milliseconds(),
		len(result.Sessions),
	)
	if jsonOutput {
		return encodeListJSON(config.Output, result)
	}
	return renderList(config.Output, result)
}

func validateListConfig(config ListConfig) error {
	if config.Context == nil {
		return fmt.Errorf("context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if config.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	if err := validateListBounds(config); err != nil {
		return err
	}
	if err := validateListScopeAndStates(config); err != nil {
		return err
	}
	if strings.TrimSpace(config.WorkID) != "" && scopedListHasTopLevelFilters(config) {
		return newCLIError(
			"WORKER_SESSION_SCOPED_FILTER_UNSUPPORTED",
			"--state, --limit, --max-results, and --next-token require fleet-wide listing; omit --work-id or use the Work-scoped list without those filters",
			nil,
		)
	}
	return nil
}

func validateListBounds(config ListConfig) error {
	if config.MaxResults < 0 {
		return newCLIError("MAX_RESULTS_INVALID", "--max-results must not be negative", nil)
	}
	if config.LimitSet && config.Limit <= 0 {
		return newCLIError("LIMIT_INVALID", "--limit must be positive", nil)
	}
	if !config.LimitSet && config.Limit < 0 {
		return newCLIError("LIMIT_INVALID", "--limit must be positive", nil)
	}
	return nil
}

func validateListScopeAndStates(config ListConfig) error {
	if config.Scope != "" && config.Scope != "direct" && config.Scope != "factory" && config.Scope != "all" {
		return newCLIError("WORKER_SESSION_SCOPE_INVALID", fmt.Sprintf("unsupported --scope value %q; supported values are direct, factory, and all", config.Scope), nil)
	}
	for _, state := range config.States {
		if !validWorkerSessionState(state) {
			return newCLIError("WORKER_SESSION_STATE_INVALID", fmt.Sprintf("unsupported --state value %q", state), nil)
		}
	}
	return nil
}

func scopedListHasTopLevelFilters(config ListConfig) bool {
	return len(config.States) > 0 || config.LimitSet || config.Limit != 0 || config.MaxResultsSet || config.MaxResults != 0 || strings.TrimSpace(config.NextToken) != ""
}

func validWorkerSessionState(state string) bool {
	switch state {
	case "RESERVED", "STARTING", "RUNNING", "PAUSED", "COMPLETED", "FAILED", "CANCELED", "TERMINATED":
		return true
	default:
		return false
	}
}

// listJSONResponse makes nullable observation fields explicit for CLI JSON.
// The generated REST models use omitempty for optional references, while the
// CLI contract promises stable keys whose unavailable values are null.
type listJSONResponse struct {
	Sessions          []listJSONObservation         `json:"sessions"`
	PaginationContext *factoryapi.PaginationContext `json:"paginationContext,omitempty"`
}

type listJSONObservation struct {
	AttemptID                string                                              `json:"attemptId"`
	Direct                   bool                                                `json:"direct"`
	DurationBasis            factoryapi.WorkerSessionObservationDurationBasis    `json:"durationBasis"`
	DurationMillis           *int64                                              `json:"durationMillis"`
	EndedAt                  *time.Time                                          `json:"endedAt"`
	FactorySessionID         *string                                             `json:"factorySessionId"`
	Failure                  *factoryapi.WorkerSessionFailure                    `json:"failure"`
	Model                    *string                                             `json:"model"`
	Parse                    factoryapi.WorkerSessionParseDiagnostics            `json:"parse"`
	ProviderSession          *factoryapi.WorkerSessionProviderSessionRef         `json:"providerSession"`
	ProviderSessionAvailable bool                                                `json:"providerSessionAvailable"`
	ReasoningEffort          *string                                             `json:"reasoningEffort"`
	RecordingHealth          *factoryapi.WorkerSessionObservationRecordingHealth `json:"recordingHealth"`
	RecordingHealthReason    *string                                             `json:"recordingHealthReason"`
	StartedAt                *time.Time                                          `json:"startedAt"`
	State                    factoryapi.WorkerSessionObservationState            `json:"state"`
	ConfirmationState        factoryapi.ConfirmationState                        `json:"confirmationState"`
	TokenUsage               *listJSONTokenUsage                                 `json:"tokenUsage"`
	TurnUsage                *listJSONTurnUsage                                  `json:"turnUsage,omitempty"`
	Transcript               factoryapi.WorkerSessionObservationTranscript       `json:"transcript"`
	TurnID                   *string                                             `json:"turnId"`
	WorkID                   *string                                             `json:"workId"`
	WorkIDs                  []string                                            `json:"workIds"`
	WorkName                 *string                                             `json:"workName"`
	WorkerSessionID          string                                              `json:"workerSessionId"`
}

type listJSONTokenUsage struct {
	CacheWriteTokens      *int `json:"cacheWriteTokens"`
	CachedInputTokens     *int `json:"cachedInputTokens"`
	InputTokens           *int `json:"inputTokens"`
	OutputTokens          *int `json:"outputTokens"`
	ReasoningOutputTokens *int `json:"reasoningOutputTokens"`
	TotalTokens           *int `json:"totalTokens"`
}

type listJSONTurnUsage struct {
	TurnCount          int `json:"turnCount"`
	FinalContextTokens int `json:"finalContextTokens"`
	PeakContextTokens  int `json:"peakContextTokens"`
}

func encodeListJSON(output io.Writer, result factoryapi.ListWorkerSessionsResponse) error {
	sessions := make([]listJSONObservation, 0, len(result.Sessions))
	for _, session := range result.Sessions {
		sessions = append(sessions, observationJSON(session))
	}
	return json.NewEncoder(output).Encode(listJSONResponse{Sessions: sessions, PaginationContext: result.PaginationContext})
}

func workerSessionsEndpoint(server, sessionID, workID, scope string, states []string, limit int, limitSet bool, maxResults int, nextToken string) (url.URL, error) {
	if strings.TrimSpace(workID) == "" {
		return topLevelWorkerSessionsEndpoint(server, scope, states, limit, limitSet, maxResults, nextToken)
	}
	endpointURL, err := cliserver.RequestURL(server, sessionpath.WorkerSessionsCollectionPath(sessionID))
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse Worker Sessions list endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("workId", workID)
	endpoint.RawQuery = query.Encode()
	return *endpoint, nil
}

func topLevelWorkerSessionsEndpoint(server, scope string, states []string, limit int, limitSet bool, maxResults int, nextToken string) (url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, sessionpath.TopLevelWorkerSessionsCollectionPath())
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse top-level Worker Sessions list endpoint: %w", err)
	}
	query := endpoint.Query()
	if scope != "" {
		query.Set("scope", scope)
	}
	for _, state := range states {
		query.Add("state", state)
	}
	if limitSet || limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	} else if maxResults > 0 {
		query.Set("maxResults", strconv.Itoa(maxResults))
	}
	if nextToken != "" {
		query.Set("nextToken", nextToken)
	}
	endpoint.RawQuery = query.Encode()
	return *endpoint, nil
}

func normalizeOutputFormat(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "human":
		return "human", nil
	case "json":
		return "json", nil
	default:
		return "", newCLIError("OUTPUT_UNSUPPORTED", fmt.Sprintf("unsupported --output value %q; supported values are human and json", value), nil)
	}
}

type CLIError struct {
	Code    string
	Message string
	// Phase carries the typed interrupt boundary for interrupt-specific JSON
	// diagnostics. Other Worker Sessions operations leave it empty.
	Phase string
	// Family and Response preserve a server-owned list failure when one crossed
	// the HTTP boundary. Local command failures leave Response nil.
	Family   factoryapi.ErrorFamily
	Response *factoryapi.ErrorResponse
	Cause    error
}

func (err *CLIError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message == "" {
		return err.Code
	}
	return fmt.Sprintf("%s: %s", err.Code, err.Message)
}

func (err *CLIError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *CLIError) CLIErrorCode() string {
	if err == nil {
		return ""
	}
	return err.Code
}

func (err *CLIError) CLIErrorMessage() string {
	if err == nil {
		return ""
	}
	return err.Message
}

func (err *CLIError) CLIErrorFamily() factoryapi.ErrorFamily {
	if err == nil {
		return ""
	}
	if err.Response != nil {
		return err.Response.Family
	}
	return err.Family
}

func (err *CLIError) CLIErrorResponse() factoryapi.ErrorResponse {
	if err == nil {
		return factoryapi.ErrorResponse{}
	}
	if err.Response != nil {
		return *err.Response
	}
	return factoryapi.ErrorResponse{
		Code:    factoryapi.ErrorResponseCode(err.Code),
		Family:  err.Family,
		Message: err.Message,
	}
}

func newCLIError(code, message string, cause error) *CLIError {
	return &CLIError{Code: code, Message: message, Cause: cause}
}

func emitCLIError(config ListConfig, jsonOutput bool, err error) error {
	if !jsonOutput || err == nil {
		return err
	}
	output := config.Output
	centralDiagnostics := clidiag.CentralDiagnosticsEnabled(config.Context)
	if centralDiagnostics {
		output = config.Diagnostics
	}
	if output == nil {
		return err
	}
	if clidiag.WriteFailure(output, err) {
		return err
	}
	payload := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: cliErrorCode(err), Message: cliErrorMessage(err)}
	if encodeErr := json.NewEncoder(output).Encode(payload); encodeErr != nil {
		return errors.Join(err, encodeErr)
	}
	if centralDiagnostics {
		clidiag.MarkDiagnosticRendered(output)
	}
	return err
}

func cliErrorCode(err error) string {
	var typed *CLIError
	if errors.As(err, &typed) && typed.Code != "" {
		return typed.Code
	}
	return "WORKER_SESSION_LIST_FAILED"
}

func cliErrorMessage(err error) string {
	var typed *CLIError
	if errors.As(err, &typed) && typed.Message != "" {
		return typed.Message
	}
	return err.Error()
}

func workerSessionsHTTPError(response *http.Response, status int, topLevel bool) error {
	if response != nil && response.Body != nil {
		defer response.Body.Close()
		if apiError, ok := clihttp.DecodeAPIError(response); ok || apiError.Code != "" || apiError.Family != "" {
			code := strings.TrimSpace(string(apiError.Code))
			if code == "" {
				code = listHTTPFallbackCode(status, topLevel)
			}
			message := strings.TrimSpace(apiError.Message)
			if message == "" {
				message = fmt.Sprintf("worker session list failed (%d)", status)
			}
			apiError.Code = factoryapi.ErrorResponseCode(code)
			apiError.Message = message
			return &CLIError{
				Code:     code,
				Message:  message,
				Family:   apiError.Family,
				Response: &apiError,
			}
		}
	}
	code := listHTTPFallbackCode(status, topLevel)
	return newCLIError(code, fmt.Sprintf("worker session list failed (%d)", status), nil)
}

func listHTTPFallbackCode(status int, topLevel bool) string {
	code := "WORKER_SESSION_LIST_FAILED"
	if status == http.StatusNotFound {
		if topLevel {
			code = "WORKER_SESSION_NOT_FOUND"
		} else {
			code = "WORK_NOT_FOUND"
		}
	}
	return code
}

func renderList(output io.Writer, result factoryapi.ListWorkerSessionsResponse) error {
	if len(result.Sessions) == 0 {
		_, err := fmt.Fprintln(output, "No worker sessions found.")
		return err
	}
	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "WORK NAME\tWORK ID\tWORKER SESSION ID\tPROVIDER\tKIND\tPROVIDER SESSION ID\tSTATE\tCONFIRMATION STATE\tSTARTED\tDURATION\tEXIT/FAILURE KIND"); err != nil {
		return err
	}
	for _, session := range result.Sessions {
		provider, kind, providerSessionID := "-", "-", "-"
		if session.ProviderSession != nil && session.ProviderSessionAvailable {
			provider = session.ProviderSession.Provider
			kind = session.ProviderSession.Kind
			providerSessionID = session.ProviderSession.Id
		}
		workID := stringOrDash(session.WorkId)
		if workID == "-" && len(session.WorkIds) > 0 {
			workID = strings.Join(session.WorkIds, ",")
		}
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			stringOrDash(session.WorkName),
			workID,
			stringOrDashPtr(session.WorkerSessionId),
			provider,
			kind,
			providerSessionID,
			stringOrDashPtr(string(session.State)),
			workerSessionConfirmationState(session),
			formatTime(session.StartedAt),
			formatDuration(session.DurationMillis),
			listFailureKind(session.Failure),
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

func listFailureKind(failure *factoryapi.WorkerSessionFailure) string {
	if failure == nil {
		return "-"
	}
	if kind := strings.TrimSpace(failure.Kind); kind != "" {
		return kind
	}
	for _, value := range []*string{
		failure.ProviderFailureKind,
		failure.ProviderContinuationFailureKind,
		failure.ProviderContinuationOutcome,
		failure.AgentRunFailureClass,
	} {
		if value != nil && strings.TrimSpace(*value) != "" {
			return *value
		}
	}
	return "-"
}

func safeAgentRunFailureClass(value *string) *string {
	if value == nil {
		return nil
	}
	class := safeAgentRunFailureClassValue(*value)
	if class == "" {
		return nil
	}
	return &class
}

func safeAgentRunFailureClassValue(class string) string {
	switch class {
	case workers.AgentRunFailureClassProvider, workers.AgentRunFailureClassHarness:
		return class
	default:
		return ""
	}
}

func formatDuration(millis *int64) string {
	if millis == nil {
		return "-"
	}
	return (time.Duration(*millis) * time.Millisecond).String()
}
