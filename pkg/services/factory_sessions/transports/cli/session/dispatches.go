package session

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

// DispatchesConfig holds parameters for the session dispatches command.
type DispatchesConfig struct {
	Context     context.Context
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	Phase       string
	Status      string
	HTTP        clihttp.Protocol
}

func NewDispatches(transport clihttp.Protocol) func(DispatchesConfig) error {
	return func(cfg DispatchesConfig) error { cfg.HTTP = transport; return Dispatches(cfg) }
}

// Dispatches requests one durable Factory Session dispatch list from a running host via HTTP.
func Dispatches(cfg DispatchesConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	if !isDurableExecutionSessionID(cfg.SessionID) {
		return fmt.Errorf(
			"factory session %q is not a durable Factory Session id; session dispatches requires a dur-sess-* session id",
			strings.TrimSpace(cfg.SessionID),
		)
	}

	endpoint, err := dispatchesEndpoint(cfg)
	if err != nil {
		return err
	}

	var listed factoryapi.ListFactorySessionDispatchesResponse
	response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		endpoint.String(),
		&listed,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session dispatches response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session dispatches response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("factory session %q not found: %s", strings.TrimSpace(cfg.SessionID), errResp.Message)
		}
		return fmt.Errorf("factory session %q not found", strings.TrimSpace(cfg.SessionID))
	}
	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session dispatches response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("list factory session dispatches failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("list factory session dispatches failed (%d)", resp.StatusCode)
	}

	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(listed)
	}
	return renderDispatchesHuman(cfg.Output, listed)
}

func dispatchesEndpoint(cfg DispatchesConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("/dispatches", cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse session dispatches endpoint: %w", err)
	}
	query := endpoint.Query()
	if phase := strings.TrimSpace(cfg.Phase); phase != "" {
		query.Set("phase", phase)
	}
	if status := strings.TrimSpace(cfg.Status); status != "" {
		query.Set("status", status)
	}
	endpoint.RawQuery = query.Encode()
	return *endpoint, nil
}

func renderDispatchesHuman(output io.Writer, result factoryapi.ListFactorySessionDispatchesResponse) error {
	count := len(result.Dispatches)
	if _, err := fmt.Fprintf(
		output,
		"Factory session %s dispatches (%d):\n",
		result.SessionId,
		count,
	); err != nil {
		return err
	}
	for _, dispatch := range result.Dispatches {
		if err := renderDispatchHuman(output, dispatch); err != nil {
			return err
		}
	}
	return nil
}

func renderDispatchHuman(output io.Writer, dispatch factoryapi.FactorySessionDispatchSummary) error {
	if _, err := fmt.Fprintf(output, "- %s %s %s\n", dispatch.Id, dispatch.Status, dispatch.DispatchKind); err != nil {
		return err
	}
	providerSessions := "none"
	if dispatch.ProviderSessionRefs != nil && len(*dispatch.ProviderSessionRefs) > 0 {
		refs := make([]string, 0, len(*dispatch.ProviderSessionRefs))
		for _, ref := range *dispatch.ProviderSessionRefs {
			refs = append(refs, ref.Id)
		}
		providerSessions = strings.Join(refs, ", ")
	}
	attempt, duration := "unavailable", "unavailable"
	if dispatch.Attempt != nil {
		attempt = fmt.Sprint(*dispatch.Attempt)
	}
	if dispatch.Usage != nil && dispatch.Usage.DurationMillis != nil {
		duration = fmt.Sprintf("%dms", *dispatch.Usage.DurationMillis)
	}
	artifacts, failure := "none", "none"
	if dispatch.OutputArtifactIds != nil && len(*dispatch.OutputArtifactIds) > 0 {
		artifacts = strings.Join(*dispatch.OutputArtifactIds, ", ")
	}
	if dispatch.FailureDetail != nil {
		failure = strings.TrimSpace(dispatch.FailureDetail.Message)
	}
	rows := [][2]string{
		{"Phase", formatOptionalString(dispatch.Phase)}, {"Label", formatOptionalString(dispatch.Label)},
		{"Runner", formatOptionalString(dispatch.RunnerId)}, {"Model", formatOptionalString(dispatch.Model)},
		{"Provider sessions", providerSessions}, {"Attempt", attempt}, {"Duration", duration},
		{"Artifacts", artifacts}, {"Failure", failure},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "  %s:\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	return nil
}
