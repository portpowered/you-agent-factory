// Package work implements work inspection command behavior.
package work

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const listRequestTimeout = 10 * time.Second

// ListConfig holds parameters for the work list command.
type ListConfig struct {
	Port       int
	StateName  string
	StateType  string
	MaxResults int
	NextToken  string
	JSON       bool
	Output     io.Writer
}

// List requests available work from a running factory via HTTP.
func List(cfg ListConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if cfg.StateType != "" && !validWorkStateType(cfg.StateType) {
		return fmt.Errorf("--state-type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED")
	}

	endpoint := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", cfg.Port),
		Path:   "/work",
	}
	query := endpoint.Query()
	if cfg.StateName != "" {
		query.Set("state.name", cfg.StateName)
	}
	if cfg.StateType != "" {
		query.Set("state.type", cfg.StateType)
	}
	if cfg.MaxResults > 0 {
		query.Set("maxResults", fmt.Sprintf("%d", cfg.MaxResults))
	}
	if cfg.NextToken != "" {
		query.Set("nextToken", cfg.NextToken)
	}
	endpoint.RawQuery = query.Encode()

	client := &http.Client{Timeout: listRequestTimeout}
	resp, err := client.Get(endpoint.String())
	if err != nil {
		return fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp factoryapi.ErrorResponse
		if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("list work failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("list work failed (%d)", resp.StatusCode)
	}

	var result factoryapi.ListWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(result)
	}

	if len(result.Results) == 0 {
		_, err = fmt.Fprintln(cfg.Output, "No work found.")
		return err
	}

	for _, work := range result.Results {
		stateName := ""
		stateType := ""
		if work.State != nil {
			stateName = work.State.Name
			stateType = string(work.State.Type)
		}
		if _, err := fmt.Fprintf(cfg.Output, "%s\t%s\t%s\t%s\n", stringValue(work.WorkId), work.Name, stateName, stateType); err != nil {
			return err
		}
	}
	return nil
}

func validWorkStateType(stateType string) bool {
	switch factoryapi.WorkStateType(stateType) {
	case factoryapi.WorkStateTypeINITIAL,
		factoryapi.WorkStateTypePROCESSING,
		factoryapi.WorkStateTypeTERMINAL,
		factoryapi.WorkStateTypeFAILED:
		return true
	default:
		return false
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
