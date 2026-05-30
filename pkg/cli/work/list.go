// Package work implements work inspection command behavior.
package work

import (
	"context"
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
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const listRequestTimeout = 10 * time.Second

// ListConfig holds parameters for the work list command.
type ListConfig struct {
	Server       string
	SessionID    string
	StateName    string
	StateType    string
	Name         string
	WorkTypeName string
	TraceID      string
	SortBy       string
	MaxResults   int
	NextToken    string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// List requests available work from a running factory via HTTP.
func List(cfg ListConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if err := validateListConfig(cfg); err != nil {
		return err
	}

	endpoint, err := listEndpoint(cfg)
	if err != nil {
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work list request endpointPath=%s endpoint=%s server=%s session=%s filters=%s maxResults=%d nextTokenPresent=%t",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		listFilterSummary(cfg),
		cfg.MaxResults,
		cfg.NextToken != "",
	)

	client := &http.Client{Timeout: listRequestTimeout}
	started := time.Now()
	var result factoryapi.ListWorkResponse
	resp, err := clihttp.GetJSON(
		context.Background(),
		client,
		endpoint.String(),
		&result,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpoint.Path,
			LogLabel:     "work list",
		},
	)
	if err != nil {
		return fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("list work failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("list work failed (%d)", resp.StatusCode)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work list response endpointPath=%s status=%d durationMillis=%d resultCount=%d nextTokenPresent=%t",
		endpoint.Path,
		resp.StatusCode,
		time.Since(started).Milliseconds(),
		len(result.Results),
		result.PaginationContext != nil && result.PaginationContext.NextToken != nil && *result.PaginationContext.NextToken != "",
	)
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(result)
	}
	return renderListResult(cfg.Output, result)
}

func validateListConfig(cfg ListConfig) error {
	if cfg.StateType != "" && !validWorkStateType(cfg.StateType) {
		return fmt.Errorf("--state-type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED")
	}
	if cfg.SortBy != "" && cfg.SortBy != string(factoryapi.SortByStateType) {
		return fmt.Errorf("--sort-by must be state.type")
	}
	return nil
}

func listFilterSummary(cfg ListConfig) string {
	parts := make([]string, 0, 6)
	if cfg.StateName != "" {
		parts = append(parts, "state.name")
	}
	if cfg.StateType != "" {
		parts = append(parts, "state.type")
	}
	if cfg.Name != "" {
		parts = append(parts, "name")
	}
	if cfg.WorkTypeName != "" {
		parts = append(parts, "workTypeName")
	}
	if cfg.TraceID != "" {
		parts = append(parts, "traceId")
	}
	if cfg.SortBy != "" {
		parts = append(parts, "sortBy")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

func listEndpoint(cfg ListConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("/work", cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse work list endpoint: %w", err)
	}
	query := endpoint.Query()
	setListQueryParam(query, "state.name", cfg.StateName)
	setListQueryParam(query, "state.type", cfg.StateType)
	setListQueryParam(query, "name", cfg.Name)
	setListQueryParam(query, "workTypeName", cfg.WorkTypeName)
	setListQueryParam(query, "traceId", cfg.TraceID)
	setListQueryParam(query, "sortBy", cfg.SortBy)
	if cfg.MaxResults > 0 {
		query.Set("maxResults", fmt.Sprintf("%d", cfg.MaxResults))
	}
	setListQueryParam(query, "nextToken", cfg.NextToken)
	endpoint.RawQuery = query.Encode()
	return *endpoint, nil
}

func setListQueryParam(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

func renderListResult(output io.Writer, result factoryapi.ListWorkResponse) error {
	if len(result.Results) == 0 {
		_, err := fmt.Fprintln(output, "No work found.")
		return err
	}

	if _, err := fmt.Fprintln(output, "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tRELATIONS"); err != nil {
		return err
	}
	for _, work := range result.Results {
		stateName, stateType := workStateColumns(work.State)
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			stringValue(work.WorkId),
			work.Name,
			stringValue(work.WorkTypeName),
			stateName,
			stateType,
			formatWorkRelations(work.Relations),
		); err != nil {
			return err
		}
	}
	return nil
}

func workStateColumns(state *factoryapi.WorkState) (string, string) {
	if state == nil {
		return "", ""
	}
	return state.Name, string(state.Type)
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
