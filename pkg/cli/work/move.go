// Package work implements work inspection command behavior.
package work

import (
	"bytes"
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

const moveRequestTimeout = 15 * time.Second

// MoveConfig holds parameters for the work move command.
type MoveConfig struct {
	Server      string
	SessionID   string
	WorkID      string
	StateName   string
	RequestID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// MoveSuccessResult is the stable JSON envelope for a successful operator move.
type MoveSuccessResult struct {
	WorkID        string `json:"workId"`
	PreviousState string `json:"previousState"`
	NewState      string `json:"newState"`
	SessionID     string `json:"sessionId"`
	EndpointPath  string `json:"endpointPath"`
}

// Move relocates one work item to another authored state via HTTP.
func Move(cfg MoveConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	workID := strings.TrimSpace(cfg.WorkID)
	if workID == "" {
		return fmt.Errorf("work id is required")
	}
	stateName := strings.TrimSpace(cfg.StateName)
	if stateName == "" {
		return fmt.Errorf("state name is required")
	}
	cfg.WorkID = workID
	cfg.StateName = stateName

	previousState, err := loadWorkStateBeforeMove(cfg)
	if err != nil {
		return err
	}

	moveEndpoint, err := moveEndpoint(cfg)
	if err != nil {
		return err
	}
	requestBody, err := json.Marshal(factoryapi.MoveWorkRequest{
		StateName: stateName,
		RequestId: optionalStringPointer(strings.TrimSpace(cfg.RequestID)),
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work move request endpointPath=%s endpoint=%s server=%s session=%s workId=%s stateName=%s requestIdPresent=%t",
		moveEndpoint.Path,
		moveEndpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		cfg.WorkID,
		cfg.StateName,
		strings.TrimSpace(cfg.RequestID) != "",
	)

	client := &http.Client{Timeout: moveRequestTimeout}
	started := time.Now()
	var moved factoryapi.Work
	resp, err := clihttp.PostJSON(
		context.Background(),
		client,
		moveEndpoint.String(),
		bytes.NewReader(requestBody),
		&moved,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: moveEndpoint.Path,
			LogLabel:     "work move",
		},
	)
	if err != nil {
		return fmt.Errorf("factory not reachable at %s: %w", moveEndpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return moveFailureError(resp.StatusCode, errResp)
		}
		return fmt.Errorf("move work failed (%d)", resp.StatusCode)
	}

	newState := stateNameFromWork(moved)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work move response endpointPath=%s status=%d durationMillis=%d workId=%s previousState=%s newState=%s",
		moveEndpoint.Path,
		resp.StatusCode,
		time.Since(started).Milliseconds(),
		cfg.WorkID,
		previousState,
		newState,
	)

	result := MoveSuccessResult{
		WorkID:        cfg.WorkID,
		PreviousState: previousState,
		NewState:      newState,
		SessionID:     clidiag.SessionLabel(cfg.SessionID),
		EndpointPath:  moveEndpoint.Path,
	}
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(result)
	}
	return renderMoveSuccess(cfg.Output, result)
}

func loadWorkStateBeforeMove(cfg MoveConfig) (string, error) {
	showEndpoint, err := showEndpoint(ShowConfig{
		Server:    cfg.Server,
		SessionID: cfg.SessionID,
		WorkID:    cfg.WorkID,
	})
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: moveRequestTimeout}
	var work factoryapi.Work
	resp, err := clihttp.GetJSON(
		context.Background(),
		client,
		showEndpoint.String(),
		&work,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: showEndpoint.Path,
			LogLabel:     "work move prefetch",
		},
	)
	if err != nil {
		return "", fmt.Errorf("factory not reachable at %s: %w", showEndpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return "", fmt.Errorf("work %q not found: %s", cfg.WorkID, errResp.Message)
		}
		return "", fmt.Errorf("work %q not found", cfg.WorkID)
	}
	if resp.StatusCode != http.StatusOK {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return "", fmt.Errorf("get work failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return "", fmt.Errorf("get work failed (%d)", resp.StatusCode)
	}
	return stateNameFromWork(work), nil
}

func moveEndpoint(cfg MoveConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("/work/"+url.PathEscape(cfg.WorkID)+"/move", cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse work move endpoint: %w", err)
	}
	return *endpoint, nil
}

func moveFailureError(status int, errResp factoryapi.ErrorResponse) error {
	message := strings.TrimSpace(errResp.Message)
	if message == "" {
		message = "request failed"
	}
	switch status {
	case http.StatusNotFound:
		return fmt.Errorf("work move failed (404): %s", message)
	case http.StatusBadRequest:
		return fmt.Errorf("move work failed (400): %s", message)
	case http.StatusConflict:
		return fmt.Errorf("move work failed (409): %s", message)
	default:
		return fmt.Errorf("move work failed (%d): %s", status, message)
	}
}

func renderMoveSuccess(output io.Writer, result MoveSuccessResult) error {
	lines := []struct {
		label string
		value string
	}{
		{label: "Work ID", value: result.WorkID},
		{label: "Previous state", value: result.PreviousState},
		{label: "New state", value: result.NewState},
		{label: "Session ID", value: result.SessionID},
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", line.label, line.value); err != nil {
			return err
		}
	}
	return nil
}

func stateNameFromWork(work factoryapi.Work) string {
	if work.State == nil {
		return ""
	}
	return work.State.Name
}

func optionalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
