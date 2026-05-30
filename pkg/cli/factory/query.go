// Package factory implements factory inspection command behavior.
package factory

import (
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
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const queryCurrentRequestTimeout = 10 * time.Second
const queryCurrentErrorBodyPreviewLimit = 200

// ErrCurrentFactoryNotFound reports that the running service could not resolve
// a current factory.
var ErrCurrentFactoryNotFound = errors.New("current factory not found")

// QueryCurrentConfig holds parameters for querying the current factory.
type QueryCurrentConfig struct {
	Server    string
	SessionID string
}

// QueryConfig holds parameters for the factory query command.
type QueryConfig struct {
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// Query prints the active factory from a running factory service.
func Query(cfg QueryConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	current, err := queryCurrent(queryCurrentOptions{
		Server:      cfg.Server,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
	})
	if err != nil {
		return renderQueryCurrentError(err)
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(current)
	}

	return RenderCurrentFactory(current, cfg.Output)
}

// QueryCurrent requests the active factory from a running factory service.
func QueryCurrent(cfg QueryCurrentConfig) (factoryapi.Factory, error) {
	return queryCurrent(queryCurrentOptions{
		Server:    cfg.Server,
		SessionID: cfg.SessionID,
	})
}

type queryCurrentOptions struct {
	Server      string
	SessionID   string
	Verbose     bool
	Diagnostics io.Writer
}

func queryCurrent(cfg queryCurrentOptions) (factoryapi.Factory, error) {
	endpointPath := sessionpath.CurrentFactoryPath(cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse factory query endpoint: %w", err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"factory query request endpointPath=%s endpoint=%s server=%s session=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
	)

	client := &http.Client{Timeout: queryCurrentRequestTimeout}
	started := time.Now()
	var result factoryapi.Factory
	resp, err := clihttp.GetJSON(
		context.Background(),
		client,
		endpoint.String(),
		&result,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpoint.Path,
			LogLabel:     "factory query",
		},
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("read current factory response: %w", err)
		}
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory query response endpointPath=%s status=%d durationMillis=%d responseBytes=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds(), len(body))
		return factoryapi.Factory{}, queryCurrentError(resp.StatusCode, body)
	}

	responseBytes, err := currentFactoryResponseBytes(result)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory query response endpointPath=%s status=%d durationMillis=%d responseBytes=%d factoryKind=%s factoryName=%q", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds(), responseBytes, currentFactoryKind(result), result.Name)
	return result, nil
}

func currentFactoryResponseBytes(result factoryapi.Factory) (int, error) {
	body, err := json.Marshal(result)
	if err != nil {
		return 0, fmt.Errorf("marshal current factory response: %w", err)
	}
	return len(body), nil
}

// RenderCurrentFactory writes a concise human-readable current-factory result.
func RenderCurrentFactory(current factoryapi.Factory, output io.Writer) error {
	if _, err := fmt.Fprintln(output, "NAME\tKIND\tID\tFACTORY DIRECTORY"); err != nil {
		return err
	}
	_, err := fmt.Fprintf(
		output,
		"%s\t%s\t%s\t%s\n",
		current.Name,
		currentFactoryKind(current),
		stringPtrValue(current.Id),
		stringPtrValue(current.FactoryDirectory),
	)
	return err
}

func renderQueryCurrentError(err error) error {
	if errors.Is(err, ErrCurrentFactoryNotFound) {
		return fmt.Errorf("running service has no active current factory; start a factory or activate a named factory: %w", err)
	}
	return err
}

func queryCurrentError(statusCode int, body []byte) error {
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) != nil || errResp.Message == "" {
		if statusCode == http.StatusNotFound {
			return fmt.Errorf("%w: service returned 404", ErrCurrentFactoryNotFound)
		}
		return unexpectedCurrentFactoryStatusError(statusCode, body)
	}
	if statusCode == http.StatusNotFound && errResp.Code == factoryapi.NOTFOUND {
		return fmt.Errorf("%w: %s", ErrCurrentFactoryNotFound, errResp.Message)
	}
	return fmt.Errorf("query current factory failed (%d): %s", statusCode, errResp.Message)
}

func unexpectedCurrentFactoryStatusError(statusCode int, body []byte) error {
	preview := strings.TrimSpace(string(body))
	if preview == "" {
		return fmt.Errorf("query current factory failed (%d)", statusCode)
	}
	if len(preview) > queryCurrentErrorBodyPreviewLimit {
		preview = preview[:queryCurrentErrorBodyPreviewLimit] + "..."
	}
	return fmt.Errorf("query current factory failed (%d): %s", statusCode, preview)
}

func currentFactoryKind(current factoryapi.Factory) string {
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return "default-root"
	}
	return "named"
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
