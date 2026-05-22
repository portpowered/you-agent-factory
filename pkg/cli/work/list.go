// Package work implements work inspection command behavior.
package work

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const listRequestTimeout = 10 * time.Second

// ListConfig holds parameters for the work list command.
type ListConfig struct {
	Port       int
	SessionID  string
	StateName  string
	StateType  string
	SortBy     string
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
	if cfg.SortBy != "" && cfg.SortBy != string(factoryapi.SortByStateType) {
		return fmt.Errorf("--sort-by must be state.type")
	}

	endpoint := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", cfg.Port),
		Path:   sessionpath.ScopedPath("/work", cfg.SessionID),
	}
	query := endpoint.Query()
	if cfg.StateName != "" {
		query.Set("state.name", cfg.StateName)
	}
	if cfg.StateType != "" {
		query.Set("state.type", cfg.StateType)
	}
	if cfg.SortBy != "" {
		query.Set("sortBy", cfg.SortBy)
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

	if _, err := fmt.Fprintln(cfg.Output, "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\tRELATIONS"); err != nil {
		return err
	}
	for _, work := range result.Results {
		stateName := ""
		stateType := ""
		if work.State != nil {
			stateName = work.State.Name
			stateType = string(work.State.Type)
		}
		if _, err := fmt.Fprintf(
			cfg.Output,
			"%s\t%s\t%s\t%s\t%s\n",
			stringValue(work.WorkId),
			work.Name,
			stateName,
			stateType,
			formatWorkRelations(work.Relations),
		); err != nil {
			return err
		}
	}
	return nil
}

func formatWorkRelations(relations *[]factoryapi.Relation) string {
	if relations == nil || len(*relations) == 0 {
		return "none"
	}

	sorted := make([]factoryapi.Relation, len(*relations))
	copy(sorted, *relations)
	sort.Slice(sorted, func(i, j int) bool {
		left := sorted[i]
		right := sorted[j]
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.TargetWorkName != right.TargetWorkName {
			return left.TargetWorkName < right.TargetWorkName
		}
		if stringValue(left.TargetWorkId) != stringValue(right.TargetWorkId) {
			return stringValue(left.TargetWorkId) < stringValue(right.TargetWorkId)
		}
		return stringValue(left.RequiredState) < stringValue(right.RequiredState)
	})

	parts := make([]string, 0, len(sorted))
	for _, relation := range sorted {
		parts = append(parts, formatRelationSummary(relation))
	}
	return strings.Join(parts, "; ")
}

func formatRelationSummary(relation factoryapi.Relation) string {
	var builder strings.Builder
	builder.WriteString(string(relation.Type))
	builder.WriteString(": ")
	builder.WriteString(relation.TargetWorkName)
	if relation.TargetWorkId != nil && *relation.TargetWorkId != "" {
		builder.WriteString(" [")
		builder.WriteString(*relation.TargetWorkId)
		builder.WriteString("]")
	}
	if relation.RequiredState != nil && *relation.RequiredState != "" {
		builder.WriteString(" (requires ")
		builder.WriteString(*relation.RequiredState)
		builder.WriteString(")")
	}
	return builder.String()
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
