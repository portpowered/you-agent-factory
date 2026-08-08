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

// ShowConfig holds parameters for the Worker Sessions show command.
type ShowConfig struct {
	Context      context.Context
	Server       string
	SessionID    string
	Provider     string
	Kind         string
	ID           string
	OutputFormat string
	JSON         bool
	Verbose      bool
	Debug        bool
	Output       io.Writer
	Diagnostics  io.Writer
	HTTP         clihttp.Protocol
}

// NewShow returns the composition-facing show operation bound to one HTTP
// protocol.
func NewShow(transport clihttp.Protocol) ShowOperation {
	return func(config ShowConfig) error {
		config.HTTP = transport
		return show(config)
	}
}

func show(config ShowConfig) error {
	config.Provider = strings.TrimSpace(config.Provider)
	config.Kind = strings.TrimSpace(config.Kind)
	config.ID = strings.TrimSpace(config.ID)
	if err := validateShowConfig(config); err != nil {
		return emitShowCLIError(config, config.JSON || strings.EqualFold(config.OutputFormat, "json"), err)
	}
	format, err := normalizeOutputFormat(config.OutputFormat)
	if err != nil {
		return emitShowCLIError(config, config.JSON, err)
	}
	jsonOutput := config.JSON || format == "json"
	endpoint, err := workerSessionDetailEndpoint(config.Server, config.SessionID, config.Provider, config.Kind, config.ID)
	if err != nil {
		return emitShowCLIError(config, jsonOutput, err)
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions show request endpointPath=%s endpoint=%s server=%s session=%s provider=%s kind=%s id=%s",
		endpoint.Path, endpoint.String(), config.Server, clidiag.SessionLabel(config.SessionID), config.Provider, config.Kind, config.ID)

	var observation factoryapi.WorkerSessionObservation
	response, requestErr := config.HTTP.GetJSON(config.Context, endpoint.String(), &observation)
	if requestErr != nil {
		return emitShowCLIError(config, jsonOutput, newCLIError(
			"FACTORY_UNREACHABLE", fmt.Sprintf("factory not reachable at %s", endpoint.String()), requestErr,
		))
	}
	if response.HTTP == nil {
		return emitShowCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_SHOW_FAILED", "worker session show returned no HTTP response", nil))
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusOK {
		return emitShowCLIError(config, jsonOutput, workerSessionShowHTTPError(response.HTTP, response.HTTP.StatusCode))
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions show response endpointPath=%s status=%d durationMillis=%d workerSessionID=%s",
		endpoint.Path, response.HTTP.StatusCode, response.Duration.Milliseconds(), observation.WorkerSessionId)
	if jsonOutput {
		return encodeObservationJSON(config.Output, observation)
	}
	return renderShow(config.Output, observation)
}

func validateShowConfig(config ShowConfig) error {
	if config.Context == nil {
		return fmt.Errorf("context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if config.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	config.Provider = strings.TrimSpace(config.Provider)
	config.Kind = strings.TrimSpace(config.Kind)
	config.ID = strings.TrimSpace(config.ID)
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

func workerSessionDetailEndpoint(server, sessionID, provider, kind, id string) (url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, sessionpath.WorkerSessionsDetailPath(sessionID))
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse Worker Sessions show endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("provider", provider)
	query.Set("kind", kind)
	query.Set("id", id)
	endpoint.RawQuery = query.Encode()
	return *endpoint, nil
}

func workerSessionShowHTTPError(response *http.Response, status int) error {
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		code := strings.TrimSpace(string(apiError.Code))
		switch code {
		case "NOT_FOUND":
			code = "WORKER_SESSION_NOT_FOUND"
		case "PROJECTION_UNAVAILABLE":
			code = "WORKER_SESSION_PROJECTION_UNAVAILABLE"
		}
		if code == "" {
			code = "WORKER_SESSION_SHOW_FAILED"
		}
		return newCLIError(code, apiError.Message, nil)
	}
	if status == http.StatusNotFound {
		return newCLIError("WORKER_SESSION_NOT_FOUND", "worker session not found", nil)
	}
	return newCLIError("WORKER_SESSION_SHOW_FAILED", fmt.Sprintf("worker session show failed (%d)", status), nil)
}

func emitShowCLIError(config ShowConfig, jsonOutput bool, err error) error {
	if !jsonOutput || err == nil || config.Output == nil {
		return err
	}
	payload := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: cliErrorCodeWithFallback(err, "WORKER_SESSION_SHOW_FAILED"), Message: cliErrorMessage(err)}
	if encodeErr := json.NewEncoder(config.Output).Encode(payload); encodeErr != nil {
		return errors.Join(err, encodeErr)
	}
	return err
}

func encodeObservationJSON(output io.Writer, observation factoryapi.WorkerSessionObservation) error {
	return json.NewEncoder(output).Encode(observationJSON(observation))
}

func observationJSON(session factoryapi.WorkerSessionObservation) listJSONObservation {
	var tokenUsage *listJSONTokenUsage
	if session.TokenUsage != nil {
		tokenUsage = &listJSONTokenUsage{
			CacheWriteTokens: session.TokenUsage.CacheWriteTokens, CachedInputTokens: session.TokenUsage.CachedInputTokens,
			InputTokens: session.TokenUsage.InputTokens, OutputTokens: session.TokenUsage.OutputTokens,
			ReasoningOutputTokens: session.TokenUsage.ReasoningOutputTokens, TotalTokens: session.TokenUsage.TotalTokens,
		}
	}
	return listJSONObservation{
		AttemptID: session.AttemptId, DurationBasis: session.DurationBasis, DurationMillis: session.DurationMillis,
		EndedAt: session.EndedAt, Failure: session.Failure, Parse: session.Parse,
		ProviderSession: session.ProviderSession, ProviderSessionAvailable: session.ProviderSessionAvailable,
		StartedAt: session.StartedAt, State: session.State, TokenUsage: tokenUsage,
		Transcript: session.Transcript, TurnID: session.TurnId, WorkIDs: session.WorkIds,
		WorkerSessionID: session.WorkerSessionId,
	}
}

func renderShow(output io.Writer, session factoryapi.WorkerSessionObservation) error {
	if err := writeShowFields(output, session); err != nil {
		return err
	}
	if err := writeTokenUsage(output, session.TokenUsage); err != nil {
		return err
	}
	if err := writeFailure(output, session.Failure); err != nil {
		return err
	}
	return writeParseDiagnostics(output, session.Parse)
}

func writeShowFields(output io.Writer, session factoryapi.WorkerSessionObservation) error {
	provider, kind, id := "-", "-", "-"
	if session.ProviderSession != nil && session.ProviderSessionAvailable {
		provider, kind, id = session.ProviderSession.Provider, session.ProviderSession.Kind, session.ProviderSession.Id
	}
	fields := []struct{ label, value string }{
		{"Worker Session ID", session.WorkerSessionId}, {"Provider", provider}, {"Kind", kind}, {"Provider Session ID", id},
		{"Work IDs", joinOrDash(session.WorkIds)}, {"Turn ID", stringOrDash(session.TurnId)}, {"Attempt ID", session.AttemptId},
		{"State", stringOrDashPtr(string(session.State))}, {"Started", formatTime(session.StartedAt)}, {"Ended", formatTime(session.EndedAt)},
		{"Duration", formatDuration(session.DurationMillis)}, {"Duration basis", stringOrDashPtr(string(session.DurationBasis))},
		{"Transcript", stringOrDashPtr(string(session.Transcript))},
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", field.label, field.value); err != nil {
			return err
		}
	}
	return nil
}

func writeTokenUsage(output io.Writer, usage *factoryapi.ProviderSessionTokenUsage) error {
	if usage == nil {
		_, err := fmt.Fprintln(output, "Token usage:\tunavailable")
		return err
	}
	_, err := fmt.Fprintf(output, "Token usage:\tinput=%s cached-input=%s cache-write=%s output=%s reasoning=%s total=%s\n",
		intOrDash(usage.InputTokens), intOrDash(usage.CachedInputTokens), intOrDash(usage.CacheWriteTokens),
		intOrDash(usage.OutputTokens), intOrDash(usage.ReasoningOutputTokens), intOrDash(usage.TotalTokens))
	return err
}

func writeFailure(output io.Writer, failure *factoryapi.WorkerSessionFailure) error {
	if failure == nil {
		_, err := fmt.Fprintln(output, "Failure:\tunavailable")
		return err
	}
	if _, err := fmt.Fprintf(output, "Failure:\tkind=%s detail=%s provider-kind=%s continuation-kind=%s continuation-outcome=%s\n",
		stringOrDashPtr(failure.Kind), stringOrDashPtr(failure.Detail), stringOrDash(failure.ProviderFailureKind),
		stringOrDash(failure.ProviderContinuationFailureKind), stringOrDash(failure.ProviderContinuationOutcome)); err != nil {
		return err
	}
	return nil
}

func writeParseDiagnostics(output io.Writer, parse factoryapi.WorkerSessionParseDiagnostics) error {
	if _, err := fmt.Fprintf(output, "Parse diagnostics:\tevents=%d malformed=%d unknown=%d errors=%d\n",
		parse.EventCount, parse.MalformedLineCount, parse.UnknownEventCount, len(parse.Errors)); err != nil {
		return err
	}
	for index, diagnostic := range parse.Errors {
		if _, err := fmt.Fprintf(output, "Parse error %d:\tcode=%s line=%d message=%s\n",
			index+1, stringOrDashPtr(diagnostic.Code), diagnostic.LineNumber, stringOrDashPtr(diagnostic.Message)); err != nil {
			return err
		}
	}
	return nil
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}

func stringOrDash(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "-"
	}
	return *value
}

func stringOrDashPtr(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func intOrDash(value *int) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *value)
}

func formatTime(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func cliErrorCodeWithFallback(err error, fallback string) string {
	code := cliErrorCode(err)
	if code == "WORKER_SESSION_LIST_FAILED" {
		return fallback
	}
	return code
}
