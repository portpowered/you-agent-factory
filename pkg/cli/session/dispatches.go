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
		line := fmt.Sprintf(
			"- %s %s %s",
			dispatch.Id,
			dispatch.Status,
			dispatch.DispatchKind,
		)
		if dispatch.Label != nil && strings.TrimSpace(*dispatch.Label) != "" {
			line += " label=" + strings.TrimSpace(*dispatch.Label)
		}
		if dispatch.OutputArtifactIds != nil && len(*dispatch.OutputArtifactIds) > 0 {
			line += " artifacts=" + strings.Join(*dispatch.OutputArtifactIds, ",")
		}
		if _, err := fmt.Fprintf(output, "%s\n", line); err != nil {
			return err
		}
	}
	return nil
}
