// Package work implements work inspection command behavior.
package work

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
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const showRequestTimeout = 10 * time.Second

// ShowConfig holds parameters for the work show command.
type ShowConfig struct {
	Server      string
	SessionID   string
	WorkID      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// Show requests one work item from a running factory via HTTP.
func Show(cfg ShowConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
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

	client := &http.Client{Timeout: showRequestTimeout}
	started := time.Now()
	var work factoryapi.Work
	resp, err := clihttp.GetJSON(
		context.Background(),
		client,
		endpoint.String(),
		&work,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpoint.Path,
			LogLabel:     "work show",
		},
	)
	if err != nil {
		return fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("work %q not found: %s", cfg.WorkID, errResp.Message)
		}
		return fmt.Errorf("work %q not found", cfg.WorkID)
	}
	if resp.StatusCode != http.StatusOK {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("get work failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("get work failed (%d)", resp.StatusCode)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work show response endpointPath=%s status=%d durationMillis=%d workId=%s",
		endpoint.Path,
		resp.StatusCode,
		time.Since(started).Milliseconds(),
		stringValue(work.WorkId),
	)
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(work)
	}
	return renderShowResult(cfg.Output, work)
}

func showEndpoint(cfg ShowConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("/work/"+url.PathEscape(cfg.WorkID), cfg.SessionID)
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
	return nil
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
