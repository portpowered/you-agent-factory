package factory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const saveCurrentRequestTimeout = queryCurrentRequestTimeout

// SaveCurrentConfig holds parameters for persisting the live current factory.
type SaveCurrentConfig struct {
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Output      io.Writer
	Diagnostics io.Writer
}

// SaveCurrent reads the session current factory and persists it with PUT.
func SaveCurrent(cfg SaveCurrentConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	saved, err := saveCurrentFactory(saveCurrentOptions{
		Server:      cfg.Server,
		SessionID:   cfg.SessionID,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
	})
	if err != nil {
		return renderSaveCurrentError(err)
	}

	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(saved)
	}

	return renderSaveCurrentSuccess(saved, cfg.SessionID, cfg.Output)
}

type saveCurrentOptions struct {
	Server      string
	SessionID   string
	Verbose     bool
	Diagnostics io.Writer
}

func saveCurrentFactory(cfg saveCurrentOptions) (factoryapi.Factory, error) {
	current, err := queryCurrent(queryCurrentOptions{
		Server:      cfg.Server,
		SessionID:   cfg.SessionID,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
	})
	if err != nil {
		return factoryapi.Factory{}, err
	}

	endpointPath := sessionpath.CurrentFactoryPath(cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse factory save endpoint: %w", err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"factory save request endpointPath=%s endpoint=%s server=%s session=%s factoryName=%q",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		current.Name,
	)

	payload, err := json.Marshal(current)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("encode current factory payload: %w", err)
	}

	client := &http.Client{Timeout: saveCurrentRequestTimeout}
	started := time.Now()
	req, err := http.NewRequest(http.MethodPut, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("build save current factory request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory save response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, time.Since(started).Milliseconds())
		return factoryapi.Factory{}, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("read save current factory response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory save response endpointPath=%s status=%d durationMillis=%d responseBytes=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds(), len(body))
		return factoryapi.Factory{}, saveCurrentError(resp.StatusCode, body)
	}

	var saved factoryapi.Factory
	if err := json.Unmarshal(body, &saved); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse save current factory response: %w", err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"factory save response endpointPath=%s status=%d durationMillis=%d responseBytes=%d factoryName=%q",
		endpoint.Path,
		resp.StatusCode,
		time.Since(started).Milliseconds(),
		len(body),
		saved.Name,
	)
	return saved, nil
}

func renderSaveCurrentSuccess(saved factoryapi.Factory, sessionID string, output io.Writer) error {
	_, err := fmt.Fprintf(
		output,
		"Saved factory %s\nSession: %s\n",
		saved.Name,
		clidiag.SessionLabel(sessionID),
	)
	return err
}

func renderSaveCurrentError(err error) error {
	if errors.Is(err, ErrCurrentFactoryNotFound) {
		return fmt.Errorf("running service has no active current factory; start a factory or activate a named factory: %w", err)
	}
	return err
}

func saveCurrentError(statusCode int, body []byte) error {
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) != nil || errResp.Message == "" {
		return unexpectedSaveCurrentStatusError(statusCode, body)
	}
	return fmt.Errorf("save current factory failed (%d): %s", statusCode, errResp.Message)
}

func unexpectedSaveCurrentStatusError(statusCode int, body []byte) error {
	preview := strings.TrimSpace(string(body))
	if preview == "" {
		return fmt.Errorf("save current factory failed (%d)", statusCode)
	}
	if len(preview) > queryCurrentErrorBodyPreviewLimit {
		preview = preview[:queryCurrentErrorBodyPreviewLimit] + "..."
	}
	return fmt.Errorf("save current factory failed (%d): %s", statusCode, preview)
}
