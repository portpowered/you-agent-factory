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
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

const queryCurrentRequestTimeout = 10 * time.Second

// ErrCurrentFactoryNotFound reports that the running service could not resolve
// a current factory.
var ErrCurrentFactoryNotFound = errors.New("current factory not found")

// QueryCurrentConfig holds parameters for querying the current factory.
type QueryCurrentConfig struct {
	Port int
}

// QueryConfig holds parameters for the factory query command.
type QueryConfig struct {
	Port   int
	JSON   bool
	Output io.Writer
}

// Query prints the active factory from a running factory service.
func Query(cfg QueryConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	current, err := QueryCurrent(QueryCurrentConfig{Port: cfg.Port})
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(current)
	}

	return RenderCurrentFactory(current, cfg.Output)
}

// QueryCurrent requests the active factory from a running factory service.
func QueryCurrent(cfg QueryCurrentConfig) (factoryapi.Factory, error) {
	endpoint := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", cfg.Port),
		Path:   "/factory/~current",
	}

	client := &http.Client{Timeout: queryCurrentRequestTimeout}
	resp, err := client.Get(endpoint.String())
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return factoryapi.Factory{}, queryCurrentError(resp)
	}

	var result factoryapi.Factory
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse current factory response: %w", err)
	}
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

func queryCurrentError(resp *http.Response) error {
	var errResp factoryapi.ErrorResponse
	if json.NewDecoder(resp.Body).Decode(&errResp) != nil || errResp.Message == "" {
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: service returned 404", ErrCurrentFactoryNotFound)
		}
		return fmt.Errorf("query current factory failed (%d)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", ErrCurrentFactoryNotFound, errResp.Message)
	}
	return fmt.Errorf("query current factory failed (%d): %s", resp.StatusCode, errResp.Message)
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
