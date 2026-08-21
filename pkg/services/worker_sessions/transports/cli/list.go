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
	Context      context.Context
	Server       string
	SessionID    string
	WorkID       string
	Scope        string
	States       []string
	MaxResults   int
	NextToken    string
	OutputFormat string
	JSON         bool
	Verbose      bool
	Debug        bool
	Output       io.Writer
	Diagnostics  io.Writer
	HTTP         clihttp.Protocol
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
	endpoint, err := workerSessionsEndpoint(config.Server, config.SessionID, config.WorkID, config.Scope, config.States, config.MaxResults, config.NextToken)
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
	if config.MaxResults < 0 {
		return newCLIError("MAX_RESULTS_INVALID", "--max-results must not be negative", nil)
	}
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
	AttemptID                string                                           `json:"attemptId"`
	Direct                   bool                                             `json:"direct"`
	DurationBasis            factoryapi.WorkerSessionObservationDurationBasis `json:"durationBasis"`
	DurationMillis           *int64                                           `json:"durationMillis"`
	EndedAt                  *time.Time                                       `json:"endedAt"`
	Failure                  *factoryapi.WorkerSessionFailure                 `json:"failure"`
	Model                    *string                                          `json:"model"`
	Parse                    factoryapi.WorkerSessionParseDiagnostics         `json:"parse"`
	ProviderSession          *factoryapi.WorkerSessionProviderSessionRef      `json:"providerSession"`
	ProviderSessionAvailable bool                                             `json:"providerSessionAvailable"`
	ReasoningEffort          *string                                          `json:"reasoningEffort"`
	StartedAt                *time.Time                                       `json:"startedAt"`
	State                    factoryapi.WorkerSessionObservationState         `json:"state"`
	TokenUsage               *listJSONTokenUsage                              `json:"tokenUsage"`
	Transcript               factoryapi.WorkerSessionObservationTranscript    `json:"transcript"`
	TurnID                   *string                                          `json:"turnId"`
	WorkIDs                  []string                                         `json:"workIds"`
	WorkerSessionID          string                                           `json:"workerSessionId"`
}

type listJSONTokenUsage struct {
	CacheWriteTokens      *int `json:"cacheWriteTokens"`
	CachedInputTokens     *int `json:"cachedInputTokens"`
	InputTokens           *int `json:"inputTokens"`
	OutputTokens          *int `json:"outputTokens"`
	ReasoningOutputTokens *int `json:"reasoningOutputTokens"`
	TotalTokens           *int `json:"totalTokens"`
}

func encodeListJSON(output io.Writer, result factoryapi.ListWorkerSessionsResponse) error {
	sessions := make([]listJSONObservation, 0, len(result.Sessions))
	for _, session := range result.Sessions {
		sessions = append(sessions, observationJSON(session))
	}
	return json.NewEncoder(output).Encode(listJSONResponse{Sessions: sessions, PaginationContext: result.PaginationContext})
}

func workerSessionsEndpoint(server, sessionID, workID, scope string, states []string, maxResults int, nextToken string) (url.URL, error) {
	if strings.TrimSpace(workID) == "" {
		return topLevelWorkerSessionsEndpoint(server, scope, states, maxResults, nextToken)
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

func topLevelWorkerSessionsEndpoint(server, scope string, states []string, maxResults int, nextToken string) (url.URL, error) {
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
	if maxResults > 0 {
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
	Cause error
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
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		code := strings.TrimSpace(string(apiError.Code))
		if status == http.StatusNotFound {
			if topLevel {
				code = "WORKER_SESSION_NOT_FOUND"
			} else {
				code = "WORK_NOT_FOUND"
			}
		}
		if code == "" {
			code = "WORKER_SESSION_LIST_FAILED"
		}
		return newCLIError(code, apiError.Message, nil)
	}
	code := "WORKER_SESSION_LIST_FAILED"
	if status == http.StatusNotFound {
		if topLevel {
			code = "WORKER_SESSION_NOT_FOUND"
		} else {
			code = "WORK_NOT_FOUND"
		}
	}
	return newCLIError(code, fmt.Sprintf("worker session list failed (%d)", status), nil)
}

func renderList(output io.Writer, result factoryapi.ListWorkerSessionsResponse) error {
	if len(result.Sessions) == 0 {
		_, err := fmt.Fprintln(output, "No worker sessions found.")
		return err
	}
	if _, err := fmt.Fprintln(output, "DIRECT\tPROVIDER\tKIND\tSESSION ID\tWORK ID\tATTEMPT\tSTATE\tTOKENS\tDURATION\tFAILURE\tMODEL\tREASONING EFFORT"); err != nil {
		return err
	}
	for _, session := range result.Sessions {
		provider, kind, sessionID := "-", "-", "-"
		if session.ProviderSession != nil && session.ProviderSessionAvailable {
			provider = session.ProviderSession.Provider
			kind = session.ProviderSession.Kind
			sessionID = session.ProviderSession.Id
		}
		workIDs := "-"
		if len(session.WorkIds) > 0 {
			workIDs = strings.Join(session.WorkIds, ",")
		}
		attempt := session.AttemptId
		if attempt == "" {
			attempt = "-"
		}
		state := string(session.State)
		if state == "" {
			state = "-"
		}
		failure := "-"
		if session.Failure != nil {
			if class := safeAgentRunFailureClass(session.Failure.AgentRunFailureClass); class != nil {
				failure = *class
			} else if session.Failure.Kind != "" {
				failure = session.Failure.Kind
			}
		}
		if _, err := fmt.Fprintf(
			output,
			"%t\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			session.Direct,
			provider,
			kind,
			sessionID,
			workIDs,
			attempt,
			state,
			formatTokens(session.TokenUsage),
			formatDuration(session.DurationMillis),
			failure,
			stringOrDash(session.Model),
			stringOrDash(session.ReasoningEffort),
		); err != nil {
			return err
		}
	}
	return nil
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

func formatTokens(usage *factoryapi.ProviderSessionTokenUsage) string {
	if usage == nil || usage.TotalTokens == nil {
		return "-"
	}
	return strconv.Itoa(*usage.TotalTokens)
}

func formatDuration(millis *int64) string {
	if millis == nil {
		return "-"
	}
	return (time.Duration(*millis) * time.Millisecond).String()
}
