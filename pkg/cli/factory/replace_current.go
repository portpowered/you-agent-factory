package factory

import (
	"bytes"
	"context"
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
	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const replaceCurrentRequestTimeout = queryCurrentRequestTimeout

var replaceCurrentOwningLabels = replaceCurrentLabels{
	logLabel:     "factory replace-current",
	failureLabel: "replace current factory",
}

type replaceCurrentLabels struct {
	logLabel     string
	failureLabel string
}

// ReplaceCurrentConfig holds parameters for persisting the live current factory.
type ReplaceCurrentConfig struct {
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Output      io.Writer
	Diagnostics io.Writer
}

// ReplaceCurrent reads the session current factory and persists it with PUT.
func ReplaceCurrent(cfg ReplaceCurrentConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	saved, err := replaceCurrentFactory(replaceCurrentOptions{
		Server:      cfg.Server,
		SessionID:   cfg.SessionID,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		labels:      replaceCurrentOwningLabels,
	})
	if err != nil {
		return renderReplaceCurrentError(err)
	}

	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(saved)
	}

	return renderReplaceCurrentSuccess(saved, cfg.SessionID, cfg.Output)
}

type replaceCurrentOptions struct {
	Server      string
	SessionID   string
	Verbose     bool
	Diagnostics io.Writer
	labels      replaceCurrentLabels
}

func replaceCurrentFactory(cfg replaceCurrentOptions) (factoryapi.Factory, error) {
	labels := cfg.labels
	if labels.logLabel == "" {
		labels = replaceCurrentOwningLabels
	}

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
		return factoryapi.Factory{}, fmt.Errorf("parse %s endpoint: %w", labels.logLabel, err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"%s request endpointPath=%s endpoint=%s server=%s session=%s factoryName=%q",
		labels.logLabel,
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		current.Name,
	)

	requestBody := buildReplaceCurrentFactoryRequest(current)
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("encode %s payload: %w", labels.failureLabel, err)
	}

	client := &http.Client{Timeout: replaceCurrentRequestTimeout}
	started := time.Now()
	var saved factoryapi.Factory
	resp, err := clihttp.PutJSON(
		context.Background(),
		client,
		endpoint.String(),
		bytes.NewReader(payload),
		&saved,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpoint.Path,
			LogLabel:     labels.logLabel,
		},
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("read %s response: %w", labels.failureLabel, err)
		}
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "%s response endpointPath=%s status=%d durationMillis=%d responseBytes=%d", labels.logLabel, endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds(), len(body))
		return factoryapi.Factory{}, replaceCurrentHTTPError(labels.failureLabel, resp.StatusCode, body)
	}

	responseBytes, err := currentFactoryResponseBytes(saved)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"%s response endpointPath=%s status=%d durationMillis=%d responseBytes=%d factoryName=%q",
		labels.logLabel,
		endpoint.Path,
		resp.StatusCode,
		time.Since(started).Milliseconds(),
		responseBytes,
		saved.Name,
	)
	return saved, nil
}

func renderReplaceCurrentSuccess(saved factoryapi.Factory, sessionID string, output io.Writer) error {
	_, err := fmt.Fprintf(
		output,
		"Replaced current factory %s\nSession: %s\n",
		saved.Name,
		clidiag.SessionLabel(sessionID),
	)
	return err
}

func renderReplaceCurrentError(err error) error {
	if errors.Is(err, ErrCurrentFactoryNotFound) {
		return fmt.Errorf("running service has no active current factory; start a factory or activate a named factory: %w", err)
	}
	return err
}

func replaceCurrentHTTPError(failureLabel string, statusCode int, body []byte) error {
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) != nil || errResp.Message == "" {
		return unexpectedReplaceCurrentHTTPError(failureLabel, statusCode, body)
	}
	return fmt.Errorf("%s failed (%d): %s", failureLabel, statusCode, errResp.Message)
}

func buildReplaceCurrentFactoryRequest(current factoryapi.Factory) factoryapi.SaveFactoryForSessionRequest {
	factoryPayload := current
	factoryPayload.Version = advanceFactoryVersionForReplace(current.Version)
	mode := factoryapi.FactorySaveModeReplaceCurrent
	return factoryapi.SaveFactoryForSessionRequest{
		Factory: factoryPayload,
		Mode:    &mode,
	}
}

func advanceFactoryVersionForReplace(current *factoryapi.HybridLogicalTimestamp) *factoryapi.HybridLogicalTimestamp {
	if current == nil {
		return nil
	}
	advanced := *current
	advanced.Logical = apitypes.Int64String(current.Logical.Int64() + 1)
	advanced.Physical = current.Physical.UTC().Add(time.Millisecond)
	return &advanced
}

func unexpectedReplaceCurrentHTTPError(failureLabel string, statusCode int, body []byte) error {
	preview := strings.TrimSpace(string(body))
	if preview == "" {
		return fmt.Errorf("%s failed (%d)", failureLabel, statusCode)
	}
	if len(preview) > queryCurrentErrorBodyPreviewLimit {
		preview = preview[:queryCurrentErrorBodyPreviewLimit] + "..."
	}
	return fmt.Errorf("%s failed (%d): %s", failureLabel, statusCode, preview)
}
