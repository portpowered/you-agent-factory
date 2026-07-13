// Package work implements work inspection command behavior.
package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workquery"
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
	JSON         bool
	Verbose      bool
	Debug        bool
	Output       io.Writer
	Diagnostics  io.Writer
}

// List requests available work from a running factory via HTTP.
func List(cfg ListConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	request, err := buildListRequest(cfg)
	if err != nil {
		return listConfigError(err)
	}
	endpoint := request.endpoint
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work list request endpointPath=%s endpoint=%s server=%s session=%s filters=%s maxResults=%d nextTokenPresent=%t",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		request.filterSummary,
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

func listConfigError(err error) error {
	var validationErr *workquery.ValidationError
	if !errors.As(err, &validationErr) {
		return err
	}
	switch validationErr.Field {
	case workquery.FilterStateType:
		return fmt.Errorf("--state-type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED")
	case "sortBy":
		return fmt.Errorf("--sort-by must be state.type")
	default:
		return err
	}
}

type listRequest struct {
	endpoint      url.URL
	filterSummary string
}

func buildListRequest(cfg ListConfig) (listRequest, error) {
	query, err := workquery.NormalizeList(workquery.ListOptions{
		Filters: map[string]string{
			workquery.FilterStateName:    cfg.StateName,
			workquery.FilterStateType:    cfg.StateType,
			workquery.FilterName:         cfg.Name,
			workquery.FilterWorkTypeName: cfg.WorkTypeName,
			workquery.FilterTraceID:      cfg.TraceID,
		},
		SortBy:     cfg.SortBy,
		MaxResults: cfg.MaxResults,
		NextToken:  cfg.NextToken,
	})
	if err != nil {
		return listRequest{}, err
	}

	endpointPath := sessionpath.WorkCollectionPath(cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return listRequest{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return listRequest{}, fmt.Errorf("parse work list endpoint: %w", err)
	}
	endpoint.RawQuery = query.Values().Encode()
	return listRequest{endpoint: *endpoint, filterSummary: query.FilterSummary()}, nil
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

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
