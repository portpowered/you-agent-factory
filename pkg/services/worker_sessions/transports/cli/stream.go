package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
)

// StreamConfig holds parameters for the Worker Sessions stream command.
type StreamConfig struct {
	Context      context.Context
	Server       string
	SessionID    string
	Provider     string
	Kind         string
	ID           string
	OutputFormat string
	JSON         bool
	ReplayOnly   bool
	Verbose      bool
	Debug        bool
	Output       io.Writer
	Diagnostics  io.Writer
	HTTP         clihttp.Protocol
}

// NewStream returns the composition-facing stream operation bound to one HTTP
// protocol. The HTTP response body remains open until a terminal frame or a
// source failure is observed.
func NewStream(transport clihttp.Protocol) StreamOperation {
	return func(config StreamConfig) error {
		config.HTTP = transport
		return stream(config)
	}
}

type streamJSONFrame struct {
	Delivery        string                 `json:"delivery"`
	WorkerSessionID string                 `json:"workerSessionId"`
	ProviderSession *streamProviderSession `json:"providerSession"`
	WorkIDs         []string               `json:"workIds"`
	Event           *streamJSONEvent       `json:"event"`
	ErrorCode       *string                `json:"errorCode"`
	ErrorMessage    *string                `json:"errorMessage"`
	ReplaySummary   *streamReplaySummary   `json:"replaySummary,omitempty"`
}

type streamReplaySummary struct {
	Kind          string `json:"kind"`
	Complete      bool   `json:"complete"`
	Reason        string `json:"reason"`
	EventsEmitted int64  `json:"eventsEmitted"`
}

type streamProviderSession struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type streamJSONEvent struct {
	Position       uint64          `json:"position"`
	SourceType     string          `json:"sourceType"`
	SourceID       string          `json:"sourceId"`
	SourceSequence uint64          `json:"sourceSequence"`
	SourceEventID  string          `json:"sourceEventId"`
	SchemaID       string          `json:"schemaId"`
	Payload        json.RawMessage `json:"payload"`
}

func stream(config StreamConfig) error {
	config.Provider = strings.TrimSpace(config.Provider)
	config.Kind = strings.TrimSpace(config.Kind)
	config.ID = strings.TrimSpace(config.ID)
	jsonOutput := config.JSON || strings.EqualFold(strings.TrimSpace(config.OutputFormat), "json")
	if err := validateStreamConfig(config); err != nil {
		return emitStreamCLIError(config, jsonOutput, err)
	}
	format, err := normalizeOutputFormat(config.OutputFormat)
	if err != nil {
		return emitStreamCLIError(config, jsonOutput, err)
	}
	jsonOutput = config.JSON || format == "json"
	endpoint, err := workerSessionEventsEndpoint(config.Server, config.SessionID, config.Provider, config.Kind, config.ID, config.ReplayOnly)
	if err != nil {
		return emitStreamCLIError(config, jsonOutput, err)
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions stream request endpointPath=%s endpoint=%s server=%s session=%s provider=%s kind=%s id=%s",
		endpoint.Path, endpoint.String(), config.Server, clidiag.SessionLabel(config.SessionID), config.Provider, config.Kind, config.ID)

	request, err := http.NewRequestWithContext(config.Context, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return emitStreamCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_STREAM_FAILED", "failed to build Worker Session event stream request", err))
	}
	request.Header.Set("Accept", "text/event-stream")
	response, requestErr := config.HTTP.Execute(request)
	if requestErr != nil {
		return emitStreamCLIError(config, jsonOutput, streamTransportError(config, requestErr))
	}
	if response.HTTP == nil {
		return emitStreamCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_STREAM_FAILED", "worker session stream returned no HTTP response", nil))
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusOK {
		return emitStreamCLIError(config, jsonOutput, workerSessionStreamHTTPError(response.HTTP, response.HTTP.StatusCode))
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions stream response endpointPath=%s status=%d durationMillis=%d",
		endpoint.Path, response.HTTP.StatusCode, response.Duration.Milliseconds())
	return consumeStreamResponse(config, jsonOutput, response)
}

func consumeStreamResponse(config StreamConfig, jsonOutput bool, response clihttp.Response) error {
	reader := bufio.NewReader(response.HTTP.Body)
	started := false
	for {
		payload, atEOF, readErr := readSSEData(reader)
		if readErr != nil {
			return streamReadError(config, jsonOutput, started, readErr)
		}
		var done bool
		var err error
		started, done, err = consumeStreamPayload(config, jsonOutput, started, payload, atEOF)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

func streamReadError(config StreamConfig, jsonOutput, started bool, readErr error) error {
	if errors.Is(readErr, io.EOF) {
		return streamFailureAfterOpen(config, jsonOutput, started, "WORKER_SESSION_STREAM_CLOSED", "Worker Session event stream closed before terminal", nil)
	}
	if errors.Is(readErr, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled) {
		return streamFailureAfterOpen(config, jsonOutput, started, "WORKER_SESSION_STREAM_INTERRUPTED", "worker session stream interrupted", context.Canceled)
	}
	return streamFailureAfterOpen(config, jsonOutput, started, "WORKER_SESSION_STREAM_FAILED", "failed to read Worker Session event stream", readErr)
}

func consumeStreamPayload(config StreamConfig, jsonOutput, started bool, payload []byte, atEOF bool) (bool, bool, error) {
	if len(payload) == 0 {
		if atEOF {
			return started, false, streamFailureAfterOpen(config, jsonOutput, started, "WORKER_SESSION_STREAM_CLOSED", "Worker Session event stream closed before terminal", nil)
		}
		return started, false, nil
	}
	var frame streamJSONFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return started, false, streamFailureAfterOpen(config, jsonOutput, started, "WORKER_SESSION_STREAM_FAILED", "Worker Session event stream returned invalid JSON", err)
	}
	started = true
	if err := writeStreamFrame(config.Output, jsonOutput, frame); err != nil {
		return started, false, err
	}
	if frame.Delivery == "REPLAY_SUMMARY" {
		if frame.ReplaySummary == nil {
			return started, false, newCLIError("WORKER_SESSION_STREAM_FAILED", "Worker Session replay summary is unavailable", nil)
		}
		return started, true, nil
	}
	if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
		return started, !config.ReplayOnly, nil
	}
	if frame.Delivery == "SOURCE_FAILURE" {
		code := stringValue(frame.ErrorCode, "WORKER_SESSION_STREAM_FAILED")
		message := stringValue(frame.ErrorMessage, "Worker Session event stream failed")
		return started, false, newCLIError(code, message, nil)
	}
	if atEOF {
		return started, false, streamFailureAfterOpen(config, jsonOutput, started, "WORKER_SESSION_STREAM_CLOSED", "Worker Session event stream closed before terminal", nil)
	}
	return started, false, nil
}

func writeStreamFrame(output io.Writer, jsonOutput bool, frame streamJSONFrame) error {
	if frame.Delivery == "REPLAY_SUMMARY" {
		if frame.ReplaySummary == nil {
			return newCLIError("WORKER_SESSION_STREAM_FAILED", "Worker Session replay summary is unavailable", nil)
		}
		return writeStreamReplaySummary(output, jsonOutput, *frame.ReplaySummary)
	}
	if jsonOutput {
		return json.NewEncoder(output).Encode(frame)
	}
	return renderStreamFrame(output, frame)
}

func validateStreamConfig(config StreamConfig) error {
	if config.Context == nil {
		return fmt.Errorf("context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if config.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
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

func workerSessionEventsEndpoint(server, sessionID, provider, kind, id string, replayOnly bool) (url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, sessionpath.WorkerSessionsEventsPath(sessionID))
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse Worker Sessions stream endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("provider", provider)
	query.Set("kind", kind)
	query.Set("id", id)
	if replayOnly {
		query.Set("replayOnly", "true")
	}
	endpoint.RawQuery = query.Encode()
	return *endpoint, nil
}

func streamTransportError(config StreamConfig, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled) {
		return newCLIError("WORKER_SESSION_STREAM_INTERRUPTED", "worker session stream interrupted", context.Canceled)
	}
	return newCLIError("FACTORY_UNREACHABLE", fmt.Sprintf("factory not reachable at %s", config.Server), err)
}

func workerSessionStreamHTTPError(response *http.Response, status int) error {
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		code := strings.TrimSpace(string(apiError.Code))
		switch code {
		case "NOT_FOUND":
			code = "WORKER_SESSION_NOT_FOUND"
		case "PROJECTION_UNAVAILABLE":
			code = "WORKER_SESSION_PROJECTION_UNAVAILABLE"
		case "WORKER_SESSION_STREAM_UNAVAILABLE":
			code = "WORKER_SESSION_STREAM_UNAVAILABLE"
		default:
			if code == "" {
				code = "WORKER_SESSION_STREAM_FAILED"
			}
		}
		return newCLIError(code, apiError.Message, nil)
	}
	if status == http.StatusNotFound {
		return newCLIError("WORKER_SESSION_NOT_FOUND", "worker session not found", nil)
	}
	return newCLIError("WORKER_SESSION_STREAM_FAILED", fmt.Sprintf("worker session stream failed (%d)", status), nil)
}

func readSSEData(reader *bufio.Reader) ([]byte, bool, error) {
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		}
		if line == "" {
			if len(data) > 0 {
				return []byte(strings.Join(data, "\n")), err == io.EOF, nil
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
		if err != nil {
			if err == io.EOF && len(data) > 0 {
				return []byte(strings.Join(data, "\n")), true, nil
			}
			return nil, true, err
		}
	}
}

func streamFailureAfterOpen(config StreamConfig, jsonOutput, started bool, code, message string, cause error) error {
	err := newCLIError(code, message, cause)
	if !started {
		return emitStreamCLIError(config, jsonOutput, err)
	}
	return err
}

func renderStreamFrame(output io.Writer, frame streamJSONFrame) error {
	provider, kind, id := "-", "-", "-"
	if frame.ProviderSession != nil {
		provider, kind, id = frame.ProviderSession.Provider, frame.ProviderSession.Kind, frame.ProviderSession.ID
	}
	fields := []string{
		"delivery=" + streamStringOrDash(frame.Delivery),
		"workerSession=" + streamStringOrDash(frame.WorkerSessionID),
		"provider=" + streamStringOrDash(provider),
		"kind=" + streamStringOrDash(kind),
		"providerSession=" + streamStringOrDash(id),
		"workIds=" + joinStreamValues(frame.WorkIDs),
	}
	if frame.Event != nil {
		fields = append(fields,
			fmt.Sprintf("position=%d", frame.Event.Position),
			"sourceType="+streamStringOrDash(frame.Event.SourceType),
			"sourceId="+streamStringOrDash(frame.Event.SourceID),
			fmt.Sprintf("sourceSequence=%d", frame.Event.SourceSequence),
			"sourceEventId="+streamStringOrDash(frame.Event.SourceEventID),
			"schemaId="+streamStringOrDash(frame.Event.SchemaID),
			"payload="+compactStreamPayload(frame.Event.Payload),
		)
	}
	if frame.ErrorCode != nil {
		fields = append(fields, "errorCode="+streamStringOrDash(*frame.ErrorCode))
	}
	if frame.ErrorMessage != nil {
		fields = append(fields, "errorMessage="+streamStringOrDash(*frame.ErrorMessage))
	}
	if frame.ReplaySummary != nil {
		fields = append(fields,
			"kind="+streamStringOrDash(frame.ReplaySummary.Kind),
			fmt.Sprintf("complete=%t", frame.ReplaySummary.Complete),
			"reason="+streamStringOrDash(frame.ReplaySummary.Reason),
			fmt.Sprintf("eventsEmitted=%d", frame.ReplaySummary.EventsEmitted),
		)
	}
	_, err := fmt.Fprintln(output, strings.Join(fields, " "))
	return err
}

func writeStreamReplaySummary(output io.Writer, jsonOutput bool, summary streamReplaySummary) error {
	if jsonOutput {
		return json.NewEncoder(output).Encode(summary)
	}
	fields := []string{
		"kind=" + streamStringOrDash(summary.Kind),
		fmt.Sprintf("complete=%t", summary.Complete),
		"reason=" + streamStringOrDash(summary.Reason),
		fmt.Sprintf("eventsEmitted=%d", summary.EventsEmitted),
	}
	_, err := fmt.Fprintln(output, strings.Join(fields, " "))
	return err
}

func compactStreamPayload(payload json.RawMessage) string {
	value := strings.Join(strings.Fields(string(payload)), " ")
	if value == "" {
		return "null"
	}
	const maxPayload = 512
	if len(value) > maxPayload {
		return value[:maxPayload] + "..."
	}
	return value
}

func joinStreamValues(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}

func streamStringOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func stringValue(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}

func emitStreamCLIError(config StreamConfig, jsonOutput bool, err error) error {
	if !jsonOutput || err == nil || config.Output == nil {
		return err
	}
	payload := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: cliErrorCode(err), Message: cliErrorMessage(err)}
	if encodeErr := json.NewEncoder(config.Output).Encode(payload); encodeErr != nil {
		return errors.Join(err, encodeErr)
	}
	return err
}
