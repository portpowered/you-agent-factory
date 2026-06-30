package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

func isDurableExecutionSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "dur-sess-")
}

func showDurableSession(cfg ShowConfig) error {
	endpoint, err := durableShowEndpoint(cfg)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: showRequestTimeout}
	var durable factoryapi.FactorySessionDurableReadModel
	resp, err := clihttp.GetJSON(
		context.Background(),
		client,
		endpoint.String(),
		&durable,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpoint.Path,
			LogLabel:     "session show durable",
		},
	)
	if err != nil {
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("factory session %q not found: %s", resolvedSessionID(cfg.SessionID), errResp.Message)
		}
		return fmt.Errorf("factory session %q not found", resolvedSessionID(cfg.SessionID))
	}
	if resp.StatusCode != http.StatusOK {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("get factory session failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("get factory session failed (%d)", resp.StatusCode)
	}

	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(durable)
	}
	return renderDurableShowResult(cfg.Output, durable)
}

func durableShowEndpoint(cfg ShowConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("", cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse durable session show endpoint: %w", err)
	}
	return *endpoint, nil
}

func renderDurableShowResult(output io.Writer, session factoryapi.FactorySessionDurableReadModel) error {
	rows := []struct {
		label string
		value string
	}{
		{label: "Factory session", value: session.SessionId},
		{label: "Lifecycle status", value: string(session.Status)},
		{label: "Orchestrator kind", value: string(session.OrchestratorKind)},
	}
	if session.Dialect != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Workflow dialect", value: strings.TrimSpace(*session.Dialect)})
	}
	if session.Phase != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Phase", value: strings.TrimSpace(*session.Phase)})
	}
	if summary := formatDurableProgressSummary(session.Progress); summary != "" {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Progress", value: summary})
	}
	if session.ResultSummary != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Result status", value: string(session.ResultSummary.ResultStatus)})
	}

	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	if err := writeDurableLifecycleFields(output, session.Lifecycle); err != nil {
		return err
	}
	return nil
}

func formatDurableProgressSummary(progress *factoryapi.FactorySessionDurableProgressCounts) string {
	if progress == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if progress.TotalDispatches != nil {
		parts = append(parts, fmt.Sprintf("total=%d", *progress.TotalDispatches))
	}
	if progress.CompletedDispatches != nil {
		parts = append(parts, fmt.Sprintf("completed=%d", *progress.CompletedDispatches))
	}
	if progress.InFlightDispatches != nil {
		parts = append(parts, fmt.Sprintf("in flight=%d", *progress.InFlightDispatches))
	}
	if progress.FailedDispatches != nil && *progress.FailedDispatches > 0 {
		parts = append(parts, fmt.Sprintf("failed=%d", *progress.FailedDispatches))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func writeDurableLifecycleFields(
	output io.Writer,
	lifecycle *factoryapi.FactorySessionDurableLifecycleTimestamps,
) error {
	if lifecycle == nil {
		return nil
	}
	fields := []struct {
		label string
		value *time.Time
	}{
		{label: "Queued at", value: lifecycle.QueuedAt},
		{label: "Started at", value: lifecycle.StartedAt},
		{label: "Paused at", value: lifecycle.PausedAt},
		{label: "Interrupted at", value: lifecycle.InterruptedAt},
		{label: "Resumed at", value: lifecycle.ResumedAt},
		{label: "Finished at", value: lifecycle.FinishedAt},
	}
	for _, field := range fields {
		if field.value == nil {
			continue
		}
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", field.label, field.value.Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}
