// Package factory implements factory inspection command behavior.
package factory

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const queryCurrentRequestTimeout = 10 * time.Second

// ErrCurrentFactoryNotFound reports that the running service could not resolve
// a current factory.
var ErrCurrentFactoryNotFound = errors.New("current factory not found")

// QueryCurrentConfig holds parameters for querying the current factory.
type QueryCurrentConfig struct {
	Port int
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
