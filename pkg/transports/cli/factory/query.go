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

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ErrCurrentFactoryNotFound reports that the running service could not resolve
// a current factory.
var ErrCurrentFactoryNotFound = errors.New("current factory not found")

// QueryCurrentConfig holds parameters for querying the current factory.
type QueryCurrentConfig struct {
	Context   context.Context
	Server    string
	SessionID string
	HTTP      clihttp.Protocol
}

// QueryConfig holds parameters for the factory show command.
type QueryConfig struct {
	Context     context.Context
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func NewQuery(transport clihttp.Protocol) func(QueryConfig) error {
	return func(cfg QueryConfig) error { cfg.HTTP = transport; return Query(cfg) }
}

func NewQueryCurrent(transport clihttp.Protocol) func(QueryCurrentConfig) (factoryapi.Factory, error) {
	return func(cfg QueryCurrentConfig) (factoryapi.Factory, error) {
		cfg.HTTP = transport
		return QueryCurrent(cfg)
	}
}

// Query prints the active factory from a running factory service.
func Query(cfg QueryConfig) error {
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
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
		Context:   cfg.Context,
		Server:    cfg.Server,
		SessionID: cfg.SessionID,
		HTTP:      cfg.HTTP,
	})
}

type queryCurrentOptions struct {
	Context     context.Context
	Server      string
	SessionID   string
	Verbose     bool
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func queryCurrent(cfg queryCurrentOptions) (factoryapi.Factory, error) {
	if cfg.Context == nil {
		return factoryapi.Factory{}, fmt.Errorf("context is required")
	}
	if cfg.HTTP == nil {
		return factoryapi.Factory{}, fmt.Errorf("CLI HTTP protocol is required")
	}
	endpointPath := sessionpath.CurrentFactoryPath(cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse factory show endpoint: %w", err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"factory show request endpointPath=%s endpoint=%s server=%s session=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
	)

	var result factoryapi.Factory
	response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		endpoint.String(),
		&result,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory show response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return factoryapi.Factory{}, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return factoryapi.Factory{}, clihttp.WithHTTPResponse(resp, fmt.Errorf("read current factory response: %w", err))
		}
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory show response endpointPath=%s status=%d durationMillis=%d responseBytes=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds(), len(body))
		return factoryapi.Factory{}, clihttp.WithHTTPResponse(resp, queryCurrentError(resp.StatusCode, body))
	}

	responseBytes, err := currentFactoryResponseBytes(result)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "factory show response endpointPath=%s status=%d durationMillis=%d responseBytes=%d factoryKind=%s factoryName=%q", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds(), responseBytes, currentFactoryKind(result), result.Name)
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
		return unexpectedCurrentFactoryStatusError(statusCode)
	}
	if statusCode == http.StatusNotFound && errResp.Code == factoryapi.ErrorResponseCodeNOTFOUND {
		return fmt.Errorf("%w: %s", ErrCurrentFactoryNotFound, errResp.Message)
	}
	return fmt.Errorf("query current factory failed (%d): %s", statusCode, errResp.Message)
}

func unexpectedCurrentFactoryStatusError(statusCode int) error {
	return fmt.Errorf("query current factory failed (%d)", statusCode)
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
