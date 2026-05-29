// Package factory implements factory inspection command behavior.
package factory

import (
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
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const queryCurrentRequestTimeout = 10 * time.Second
const queryCurrentErrorBodyPreviewLimit = 200

// ErrCurrentFactoryNotFound reports that the running service could not resolve
// a current factory.
var ErrCurrentFactoryNotFound = errors.New("current factory not found")

// QueryCurrentConfig holds parameters for querying the current factory.
type QueryCurrentConfig struct {
	Port      int
	SessionID string
}

// QueryConfig holds parameters for the factory query command.
type QueryConfig struct {
	Port        int
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
		Port:        cfg.Port,
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
		Port:      cfg.Port,
		SessionID: cfg.SessionID,
	})
}

type queryCurrentOptions struct {
	Port        int
	SessionID   string
	Verbose     bool
	Diagnostics io.Writer
}

func queryCurrent(cfg queryCurrentOptions) (factoryapi.Factory, error) {
	endpoint := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", cfg.Port),
		Path:   sessionpath.CurrentFactoryPath(cfg.SessionID),
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"factory query request endpointPath=%s endpoint=%s port=%d session=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Port,
		clidiag.SessionLabel(cfg.SessionID),
	)

	client := &http.Client{Timeout: queryCurrentRequestTimeout}
	started := time.Now()
	resp, err := client.Get(endpoint.String())
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory query response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, time.Since(started).Milliseconds())
		return factoryapi.Factory{}, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("read current factory response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory query response endpointPath=%s status=%d durationMillis=%d responseBytes=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds(), len(body))
		return factoryapi.Factory{}, queryCurrentError(resp.StatusCode, body)
	}

	var result factoryapi.Factory
	if err := json.Unmarshal(body, &result); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse current factory response: %w", err)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory query response endpointPath=%s status=%d durationMillis=%d responseBytes=%d factoryKind=%s factoryName=%q", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds(), len(body), currentFactoryKind(result), result.Name)
	return result, nil
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
