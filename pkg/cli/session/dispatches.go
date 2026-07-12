package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const dispatchesRequestTimeout = 10 * time.Second

// DispatchesConfig holds parameters for the session dispatches command.
type DispatchesConfig struct {
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	Phase       string
	Status      string
}

// Dispatches requests one durable Factory Session dispatch list from a running host via HTTP.
func Dispatches(cfg DispatchesConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
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

	client := &http.Client{Timeout: dispatchesRequestTimeout}
	var listed factoryapi.ListFactorySessionDispatchesResponse
	resp, err := clihttp.GetJSON(
		context.Background(),
		client,
		endpoint.String(),
		&listed,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpoint.Path,
			LogLabel:     "session dispatches",
		},
	)
	if err != nil {
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("factory session %q not found: %s", strings.TrimSpace(cfg.SessionID), errResp.Message)
		}
		return fmt.Errorf("factory session %q not found", strings.TrimSpace(cfg.SessionID))
	}
	if resp.StatusCode != http.StatusOK {
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
