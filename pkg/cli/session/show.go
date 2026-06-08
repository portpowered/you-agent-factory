// Package session implements factory-session lifecycle command behavior.
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
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const showRequestTimeout = 10 * time.Second

// ShowConfig holds parameters for the session show command.
type ShowConfig struct {
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// Show requests one live factory session projection from a running host via HTTP.
func Show(cfg ShowConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	endpoint, err := showEndpoint(cfg)
	if err != nil {
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session show request endpointPath=%s endpoint=%s server=%s session=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
	)

	client := &http.Client{Timeout: showRequestTimeout}
	started := time.Now()
	var result factoryapi.FactorySession
	resp, err := clihttp.GetJSON(
		context.Background(),
		client,
		endpoint.String(),
		&result,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpoint.Path,
			LogLabel:     "session show",
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
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session show response endpointPath=%s status=%d durationMillis=%d sessionId=%s orchestratorKind=%s",
		endpoint.Path,
		resp.StatusCode,
		time.Since(started).Milliseconds(),
		result.Id,
		result.Runtime.OrchestratorKind,
	)
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(result)
	}
	return renderShowResult(cfg.Output, result)
}

func showEndpoint(cfg ShowConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("", cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse session show endpoint: %w", err)
	}
	return *endpoint, nil
}

func renderShowResult(output io.Writer, session factoryapi.FactorySession) error {
	rows := []struct {
		label string
		value string
	}{
		{label: "Factory session", value: session.Id},
		{label: "Project", value: session.Project},
		{label: "Folder path", value: session.FolderPath},
		{label: "Factory dir", value: session.FactoryDir},
		{label: "Default session", value: defaultMarker(session.IsDefault)},
		{label: "Target kind", value: string(session.Target.Kind)},
		{label: "Target name", value: targetName(session.Target.Name)},
		{label: "Orchestrator kind", value: string(session.Runtime.OrchestratorKind)},
		{label: "Runtime status", value: string(session.Runtime.Status)},
		{label: "Factory state", value: session.Runtime.Progress.FactoryState},
		{label: "Total tokens", value: fmt.Sprintf("%d", session.Runtime.Progress.TotalTokens)},
		{label: "In-flight dispatches", value: fmt.Sprintf("%d", session.Runtime.Progress.InFlightCount)},
	}
	if session.Runtime.Dialect != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Workflow dialect", value: strings.TrimSpace(*session.Runtime.Dialect)})
	}
	if session.Runtime.SourceRef != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Workflow source ref", value: strings.TrimSpace(*session.Runtime.SourceRef)})
	}
	if session.Runtime.PolicyHash != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Policy hash", value: strings.TrimSpace(*session.Runtime.PolicyHash)})
	}

	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}

	switch session.Runtime.OrchestratorKind {
	case factoryapi.JAVASCRIPT:
		return renderJavaScriptSessionProjection(output, session.Runtime.Javascript)
	default:
		return renderPetriSessionProjection(output, session.Runtime.Petri)
	}
}

func renderPetriSessionProjection(output io.Writer, petri *factoryapi.FactorySessionPetriProjection) error {
	if petri == nil {
		_, err := fmt.Fprintln(output, "Petri projection:\tnone")
		return err
	}
	if _, err := fmt.Fprintf(output, "Petri marking tokens:\t%d\n", len(petri.Marking)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Enabled transitions:\t%d\n", len(petri.EnabledTransitions)); err != nil {
		return err
	}
	for _, transition := range petri.EnabledTransitions {
		if _, err := fmt.Fprintf(
			output,
			"Enabled transition:\t%s (%s)\n",
			transition.TransitionId,
			transition.WorkerType,
		); err != nil {
			return err
		}
	}
	return nil
}

func renderJavaScriptSessionProjection(output io.Writer, javascript *factoryapi.FactorySessionJavaScriptProjection) error {
	if javascript == nil {
		_, err := fmt.Fprintln(output, "Dynamic workflow projection:\tnone")
		return err
	}
	if _, err := fmt.Fprintln(output, "Dynamic workflow:\tJavaScript factory session"); err != nil {
		return err
	}
	if err := writeJavaScriptPhaseFields(output, javascript); err != nil {
		return err
	}
	return writeJavaScriptCheckpointRefs(output, javascript.Checkpoints)
}

func writeJavaScriptPhaseFields(output io.Writer, javascript *factoryapi.FactorySessionJavaScriptProjection) error {
	if javascript.Phase != nil {
		if _, err := fmt.Fprintf(output, "Phase:\t%s\n", strings.TrimSpace(*javascript.Phase)); err != nil {
			return err
		}
	}
	if len(javascript.Phases) > 0 {
		if _, err := fmt.Fprintf(output, "Phases:\t%s\n", strings.Join(javascript.Phases, ", ")); err != nil {
			return err
		}
	}
	if javascript.ArgsDigest != nil {
		if _, err := fmt.Fprintf(output, "Args digest:\t%s\n", strings.TrimSpace(*javascript.ArgsDigest)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(output, "Script status:\t%s\n", javascript.ScriptStatus); err != nil {
		return err
	}
	_, err := fmt.Fprintf(
		output,
		"Child dispatches:\tqueued=%d running=%d completed=%d\n",
		javascript.ChildDispatchCounts.Queued,
		javascript.ChildDispatchCounts.Running,
		javascript.ChildDispatchCounts.Completed,
	)
	return err
}

func writeJavaScriptCheckpointRefs(output io.Writer, checkpoints *[]factoryapi.FactorySessionJavaScriptCheckpointRef) error {
	if checkpoints == nil {
		return nil
	}
	for _, checkpoint := range *checkpoints {
		label := checkpoint.Id
		if checkpoint.Label != nil && strings.TrimSpace(*checkpoint.Label) != "" {
			label = fmt.Sprintf("%s (%s)", checkpoint.Id, strings.TrimSpace(*checkpoint.Label))
		}
		if checkpoint.Summary != nil && strings.TrimSpace(*checkpoint.Summary) != "" {
			label = fmt.Sprintf("%s — %s", label, strings.TrimSpace(*checkpoint.Summary))
		}
		if _, err := fmt.Fprintf(output, "Checkpoint ref:\t%s\n", label); err != nil {
			return err
		}
	}
	return nil
}

func resolvedSessionID(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return sessionpath.DefaultFactorySessionID
	}
	return strings.TrimSpace(sessionID)
}
