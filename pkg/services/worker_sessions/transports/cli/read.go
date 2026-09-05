package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ReadConfig holds parameters for the Worker Sessions read command.
type ReadConfig struct {
	Context         context.Context
	Server          string
	SessionID       string
	WorkerSessionID string
	Provider        string
	Kind            string
	ID              string
	OutputFormat    string
	JSON            bool
	Verbose         bool
	Debug           bool
	Output          io.Writer
	Diagnostics     io.Writer
	HTTP            clihttp.Protocol
}

// NewRead returns the composition-facing transcript operation bound to one
// HTTP protocol.
func NewRead(transport clihttp.Protocol) ReadOperation {
	return func(config ReadConfig) error {
		config.HTTP = transport
		return read(config)
	}
}

func read(config ReadConfig) error {
	config.Provider = strings.TrimSpace(config.Provider)
	config.Kind = strings.TrimSpace(config.Kind)
	config.ID = strings.TrimSpace(config.ID)
	config.WorkerSessionID = strings.TrimSpace(config.WorkerSessionID)
	jsonOutput := config.JSON || strings.EqualFold(strings.TrimSpace(config.OutputFormat), "json")
	if err := validateReadConfig(config); err != nil {
		return emitReadCLIError(config, jsonOutput, err)
	}
	format, err := normalizeOutputFormat(config.OutputFormat)
	if err != nil {
		return emitReadCLIError(config, config.JSON, err)
	}
	jsonOutput = config.JSON || format == "json"
	endpoint, err := workerSessionTranscriptEndpoint(config.Server, config.SessionID, config.WorkerSessionID, config.Provider, config.Kind, config.ID)
	if err != nil {
		return emitReadCLIError(config, jsonOutput, err)
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions read request endpointPath=%s endpoint=%s server=%s session=%s workerSessionID=%s provider=%s kind=%s id=%s",
		endpoint.Path, endpoint.String(), config.Server, clidiag.SessionLabel(config.SessionID), config.WorkerSessionID, config.Provider, config.Kind, config.ID)

	var transcript factoryapi.WorkerSessionTranscriptResponse
	response, requestErr := config.HTTP.GetJSON(config.Context, endpoint.String(), &transcript)
	if requestErr != nil {
		if errors.Is(requestErr, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled) {
			return emitReadCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_TRANSCRIPT_INTERRUPTED", "worker session transcript read interrupted", context.Canceled))
		}
		return emitReadCLIError(config, jsonOutput, newCLIError(
			"FACTORY_UNREACHABLE", fmt.Sprintf("factory not reachable at %s", endpoint.String()), requestErr,
		))
	}
	if response.HTTP == nil {
		return emitReadCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_READ_FAILED", "worker session transcript read returned no HTTP response", nil))
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusOK {
		return emitReadCLIError(config, jsonOutput, workerSessionReadHTTPError(response.HTTP, response.HTTP.StatusCode))
	}
	if transcript.Entries == nil {
		transcript.Entries = []factoryapi.ProviderSessionTranscriptEntry{}
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions read response endpointPath=%s status=%d durationMillis=%d entryCount=%d workerSessionID=%s",
		endpoint.Path, response.HTTP.StatusCode, response.Duration.Milliseconds(), len(transcript.Entries), transcript.WorkerSessionId)
	if jsonOutput {
		return encodeReadJSON(config.Output, transcript)
	}
	return renderRead(config.Output, transcript)
}

func validateReadConfig(config ReadConfig) error {
	if config.Context == nil {
		return fmt.Errorf("context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if config.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	config.WorkerSessionID = strings.TrimSpace(config.WorkerSessionID)
	config.Provider = strings.TrimSpace(config.Provider)
	config.Kind = strings.TrimSpace(config.Kind)
	config.ID = strings.TrimSpace(config.ID)
	if config.WorkerSessionID != "" {
		if config.Provider != "" || config.Kind != "" || config.ID != "" {
			return newCLIError("WORKER_SESSION_MODE_CONFLICT", "--worker-session-id cannot be combined with --provider, --kind, or --id", nil)
		}
		return nil
	}
	if config.Provider == "" {
		return newCLIError("PROVIDER_REQUIRED", "--provider is required", nil)
	}
	if config.Kind == "" {
		return newCLIError("SESSION_KIND_REQUIRED", "--kind is required", nil)
	}
	if config.ID == "" {
		return newCLIError("SESSION_ID_REQUIRED", "--id is required", nil)
	}
	if config.Provider != string(providers.IDCodex) && config.Provider != string(providers.IDCursor) {
		return newCLIError("PROVIDER_UNSUPPORTED", fmt.Sprintf("unsupported provider %q", config.Provider), nil)
	}
	if config.Kind != providers.SessionIDKind {
		return newCLIError("SESSION_KIND_UNSUPPORTED", fmt.Sprintf("unsupported session kind %q", config.Kind), nil)
	}
	return nil
}

func workerSessionTranscriptEndpoint(server, sessionID, workerSessionID, provider, kind, id string) (url.URL, error) {
	path := sessionpath.WorkerSessionsTranscriptPath(sessionID)
	if strings.TrimSpace(workerSessionID) != "" {
		path = sessionpath.TopLevelWorkerSessionTranscriptPath(workerSessionID)
	}
	endpointURL, err := cliserver.RequestURL(server, path)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse Worker Sessions read endpoint: %w", err)
	}
	if strings.TrimSpace(workerSessionID) != "" {
		return *endpoint, nil
	}
	query := endpoint.Query()
	query.Set("provider", provider)
	query.Set("kind", kind)
	query.Set("id", id)
	endpoint.RawQuery = query.Encode()
	return *endpoint, nil
}

type readJSONResponse struct {
	AttemptID             string                   `json:"attemptId"`
	Entries               []readJSONEntry          `json:"entries"`
	ProviderSession       *readJSONProviderSession `json:"providerSession"`
	RecordingHealth       string                   `json:"recordingHealth"`
	RecordingHealthReason *string                  `json:"recordingHealthReason"`
	State                 string                   `json:"state"`
	TurnID                *string                  `json:"turnId"`
	WorkIDs               []string                 `json:"workIds"`
	WorkerSessionID       string                   `json:"workerSessionId"`
}

type readJSONProviderSession struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type readJSONEntry struct {
	Arguments        *string    `json:"arguments"`
	CallID           *string    `json:"callId"`
	Encrypted        *bool      `json:"encrypted"`
	EncryptedContent *string    `json:"encryptedContent"`
	LineNumber       *int       `json:"lineNumber"`
	Name             *string    `json:"name"`
	Order            int        `json:"order"`
	Output           *string    `json:"output"`
	SourceType       *string    `json:"sourceType"`
	Status           *string    `json:"status"`
	Summary          *string    `json:"summary"`
	Text             *string    `json:"text"`
	Timestamp        *time.Time `json:"timestamp"`
	TurnIndex        *int       `json:"turnIndex"`
	Type             string     `json:"type"`
}

func encodeReadJSON(output io.Writer, response factoryapi.WorkerSessionTranscriptResponse) error {
	entries := make([]readJSONEntry, len(response.Entries))
	for index, entry := range response.Entries {
		entries[index] = readJSONEntry{
			Arguments: entry.Arguments, CallID: entry.CallId, Encrypted: entry.Encrypted,
			EncryptedContent: entry.EncryptedContent, LineNumber: entry.LineNumber, Name: entry.Name,
			Order: entry.Order, Output: entry.Output, SourceType: entry.SourceType, Status: entry.Status,
			Summary: entry.Summary, Text: entry.Text, Timestamp: entry.Timestamp, TurnIndex: entry.TurnIndex,
			Type: string(entry.Type),
		}
	}
	var providerSession *readJSONProviderSession
	if response.ProviderSession != nil {
		providerSession = &readJSONProviderSession{
			Provider: response.ProviderSession.Provider, Kind: response.ProviderSession.Kind, ID: response.ProviderSession.Id,
		}
	}
	return json.NewEncoder(output).Encode(readJSONResponse{
		AttemptID: response.AttemptId, Entries: entries, ProviderSession: providerSession,
		RecordingHealth: string(response.RecordingHealth), RecordingHealthReason: response.RecordingHealthReason,
		State: response.State, TurnID: response.TurnId, WorkIDs: response.WorkIds, WorkerSessionID: response.WorkerSessionId,
	})
}

func renderRead(output io.Writer, response factoryapi.WorkerSessionTranscriptResponse) error {
	provider := response.ProviderSession
	var providerName, providerKind, providerID string
	if provider != nil {
		providerName, providerKind, providerID = provider.Provider, provider.Kind, provider.Id
	}
	fields := []struct{ label, value string }{
		{"Worker Session ID", response.WorkerSessionId},
		{"Provider", stringOrDashPtr(providerName)},
		{"Kind", stringOrDashPtr(providerKind)},
		{"Provider Session ID", stringOrDashPtr(providerID)},
		{"Work IDs", joinOrDash(response.WorkIds)},
		{"Turn ID", stringOrDash(response.TurnId)},
		{"Attempt ID", stringOrDashPtr(response.AttemptId)},
		{"State", stringOrDashPtr(response.State)},
		{"Recording Health", stringOrDashPtr(string(response.RecordingHealth))},
		{"Recording Health Reason", stringOrDash(response.RecordingHealthReason)},
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", field.label, field.value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(output, "Entries:\t%d\n", len(response.Entries)); err != nil {
		return err
	}
	for index, entry := range response.Entries {
		if _, err := fmt.Fprintf(output, "Entry %d:\ttype=%s order=%d", index+1, stringOrDashPtr(string(entry.Type)), entry.Order); err != nil {
			return err
		}
		for _, field := range []struct{ key, value string }{
			{"role", transcriptRole(entry.Type)}, {"text", compactReadValue(entry.Text)}, {"summary", compactReadValue(entry.Summary)},
			{"tool", compactReadValue(entry.Name)}, {"callId", compactReadValue(entry.CallId)}, {"arguments", compactReadValue(entry.Arguments)},
			{"output", compactReadValue(entry.Output)}, {"status", compactReadValue(entry.Status)}, {"sourceType", compactReadValue(entry.SourceType)},
			{"encrypted", boolReadValue(entry.Encrypted)}, {"encryptedContent", compactReadValue(entry.EncryptedContent)},
		} {
			if _, err := fmt.Fprintf(output, " %s=%s", field.key, field.value); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(output); err != nil {
			return err
		}
	}
	return nil
}

func transcriptRole(value factoryapi.ProviderSessionTranscriptEntryType) string {
	switch string(value) {
	case "user_message":
		return "user"
	case "assistant_message":
		return "assistant"
	case "reasoning":
		return "reasoning-summary"
	case "tool_call", "tool_output":
		return "tool"
	case "system_event":
		return "system"
	default:
		return "-"
	}
}

func compactReadValue(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "-"
	}
	return strings.Join(strings.Fields(*value), " ")
}

func boolReadValue(value *bool) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%t", *value)
}

func workerSessionReadHTTPError(response *http.Response, status int) error {
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		code := strings.TrimSpace(string(apiError.Code))
		switch code {
		case "NOT_FOUND":
			code = "WORKER_SESSION_NOT_FOUND"
		case "WORKER_SESSION_TRANSCRIPT_ACTIVE":
			code = "WORKER_SESSION_TRANSCRIPT_ACTIVE"
		case "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE":
			code = "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"
		case "WORKER_SESSION_TRANSCRIPT_PROJECTION_UNAVAILABLE":
			code = "WORKER_SESSION_TRANSCRIPT_PROJECTION_UNAVAILABLE"
		default:
			if code == "" {
				code = "WORKER_SESSION_READ_FAILED"
			}
		}
		return newCLIError(code, apiError.Message, nil)
	}
	if status == http.StatusNotFound {
		return newCLIError("WORKER_SESSION_NOT_FOUND", "worker session transcript not found", nil)
	}
	if status == http.StatusConflict {
		return newCLIError("WORKER_SESSION_TRANSCRIPT_ACTIVE", "worker session is still active; transcript is not final", nil)
	}
	return newCLIError("WORKER_SESSION_READ_FAILED", fmt.Sprintf("worker session transcript read failed (%d)", status), nil)
}

func emitReadCLIError(config ReadConfig, jsonOutput bool, err error) error {
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
	}{Code: cliErrorCodeWithFallback(err, "WORKER_SESSION_READ_FAILED"), Message: cliErrorMessage(err)}
	if encodeErr := json.NewEncoder(output).Encode(payload); encodeErr != nil {
		return errors.Join(err, encodeErr)
	}
	if centralDiagnostics {
		clidiag.MarkDiagnosticRendered(output)
	}
	return err
}
