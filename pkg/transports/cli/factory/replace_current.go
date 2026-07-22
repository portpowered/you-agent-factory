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
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
	Context     context.Context
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Output      io.Writer
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func NewReplaceCurrent(transport clihttp.Protocol) func(ReplaceCurrentConfig) error {
	return func(cfg ReplaceCurrentConfig) error { cfg.HTTP = transport; return ReplaceCurrent(cfg) }
}

// ReplaceCurrent reads the session current factory and persists it with PUT.
func ReplaceCurrent(cfg ReplaceCurrentConfig) error {
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}

	saved, err := replaceCurrentFactory(replaceCurrentOptions{
		Context:     cfg.Context,
		Server:      cfg.Server,
		SessionID:   cfg.SessionID,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		HTTP:        cfg.HTTP,
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
	Context     context.Context
	Server      string
	SessionID   string
	Verbose     bool
	Diagnostics io.Writer
	labels      replaceCurrentLabels
	HTTP        clihttp.Protocol
}

func replaceCurrentFactory(cfg replaceCurrentOptions) (factoryapi.Factory, error) {
	labels := cfg.labels
	if labels.logLabel == "" {
		labels = replaceCurrentOwningLabels
	}

	current, err := queryCurrent(queryCurrentOptions{
		Context:     cfg.Context,
		Server:      cfg.Server,
		SessionID:   cfg.SessionID,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		HTTP:        cfg.HTTP,
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

	var saved factoryapi.Factory
	response, err := cfg.HTTP.PutJSON(
		cfg.Context,
		endpoint.String(),
		bytes.NewReader(payload),
		&saved,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "%s response endpointPath=%s error=unreachable durationMillis=%d", labels.logLabel, endpoint.Path, response.Duration.Milliseconds())
		return factoryapi.Factory{}, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("read %s response: %w", labels.failureLabel, err)
		}
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "%s response endpointPath=%s status=%d durationMillis=%d responseBytes=%d", labels.logLabel, endpoint.Path, resp.StatusCode, response.Duration.Milliseconds(), len(body))
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
		response.Duration.Milliseconds(),
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
