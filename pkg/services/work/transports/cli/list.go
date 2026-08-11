package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ListConfig holds parameters for the work list command.
type ListConfig struct {
	Context      context.Context
	Server       string
	SessionID    string
	StateName    string
	StateType    string
	Name         string
	WorkTypeName string
	TraceID      string
	Terminal     bool
	NonTerminal  bool
	SortBy       string
	MaxResults   int
	NextToken    string
	Counts       bool
	JSON         bool
	Verbose      bool
	Debug        bool
	Output       io.Writer
	Diagnostics  io.Writer
	HTTP         clihttp.Protocol
}

func NewList(
	transport clihttp.Protocol,
	prepare workdomain.ListRequestPreparation,
) func(ListConfig) error {
	svc := New(Config{ListPrepare: prepare})
	return func(cfg ListConfig) error {
		cfg.HTTP = transport
		return svc.List(cfg)
	}
}

// List requests available work from a running factory via HTTP.
func List(prepare workdomain.ListRequestPreparation, cfg ListConfig) error {
	return New(Config{ListPrepare: prepare}).List(cfg)
}

func (service *service) List(cfg ListConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	if service.listPrepare == nil {
		return fmt.Errorf("Work list request preparation is required")
	}

	prepared, err := service.listPrepare.PrepareListRequest(cfg.Context, workdomain.ListOptions{
		StateName:    cfg.StateName,
		StateType:    cfg.StateType,
		Name:         cfg.Name,
		WorkTypeName: cfg.WorkTypeName,
		TraceID:      cfg.TraceID,
		Terminal:     cfg.Terminal,
		NonTerminal:  cfg.NonTerminal,
		SortBy:       cfg.SortBy,
		MaxResults:   cfg.MaxResults,
		NextToken:    cfg.NextToken,
		Counts:       cfg.Counts,
	})
	if err != nil {
		return listConfigError(err)
	}
	request, err := buildListRequest(cfg, prepared)
	if err != nil {
		return err
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
		prepared.Options.MaxResults,
		prepared.Options.NextToken != "",
	)

	var responsePayload json.RawMessage
	response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		endpoint.String(),
		&responsePayload,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work list response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work list response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
			return fmt.Errorf("list work failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work list response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		return fmt.Errorf("list work failed (%d)", resp.StatusCode)
	}
	result, err := decodeListWorkResponse(responsePayload)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work list response endpointPath=%s status=%d durationMillis=%d error=decode", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		return fmt.Errorf("decode work list response: %w", err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work list response endpointPath=%s status=%d durationMillis=%d resultCount=%d nextTokenPresent=%t",
		endpoint.Path,
		resp.StatusCode,
		response.Duration.Milliseconds(),
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
	var validationErr *workdomain.ValidationError
	if !errors.As(err, &validationErr) {
		return err
	}
	switch validationErr.Field {
	case workdomain.FilterStateType:
		return fmt.Errorf("--state-type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED")
	case workdomain.FilterTerminal:
		return fmt.Errorf("--terminal and --non-terminal cannot both be set")
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

func buildListRequest(cfg ListConfig, prepared workdomain.PreparedListRequest) (listRequest, error) {
	endpointPath := sessionpath.WorkCollectionPath(cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return listRequest{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return listRequest{}, fmt.Errorf("parse work list endpoint: %w", err)
	}
	endpoint.RawQuery = listQueryValues(prepared.Options).Encode()
	return listRequest{endpoint: *endpoint, filterSummary: prepared.FilterSummary}, nil
}

func listQueryValues(options workdomain.ListOptions) url.Values {
	values := make(url.Values)
	for key, value := range map[string]string{
		workdomain.FilterStateName:    options.StateName,
		workdomain.FilterStateType:    options.StateType,
		workdomain.FilterName:         options.Name,
		workdomain.FilterWorkTypeName: options.WorkTypeName,
		workdomain.FilterTraceID:      options.TraceID,
		"sortBy":                      options.SortBy,
	} {
		if value != "" {
			values.Set(key, value)
		}
	}
	if options.MaxResults > 0 {
		values.Set("maxResults", fmt.Sprintf("%d", options.MaxResults))
	}
	if options.NextToken != "" {
		values.Set("nextToken", options.NextToken)
	}
	if options.Terminal {
		values.Set(workdomain.FilterTerminal, "true")
	}
	if options.NonTerminal {
		values.Set(workdomain.FilterNonTerminal, "true")
	}
	if options.Counts {
		values.Set("counts", "true")
	}
	return values
}

func renderListResult(output io.Writer, result factoryapi.ListWorkResponse) error {
	if result.Counts != nil {
		if _, err := fmt.Fprintf(output, "Total: %d\n", result.Counts.Total); err != nil {
			return err
		}
	}
	if len(result.Results) == 0 {
		_, err := fmt.Fprintln(output, "No work found.")
		return err
	}

	if _, err := fmt.Fprintln(output, "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS"); err != nil {
		return err
	}
	for _, work := range result.Results {
		stateName, stateType := workStateColumns(work.State)
		structuredResult, err := formatStructuredResult(work.StructuredResult, work.StructuredResult != nil)
		if err != nil {
			return fmt.Errorf("format structured result for Work %q: %w", stringValue(work.WorkId), err)
		}
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			stringValue(work.WorkId),
			work.Name,
			stringValue(work.WorkTypeName),
			stateName,
			stateType,
			structuredResult,
			formatWorkRelations(work.Relations),
		); err != nil {
			return err
		}
		if work.ExpectedArtifacts != nil && len(*work.ExpectedArtifacts) > 0 {
			if _, err := fmt.Fprintf(output, "  Artifacts: %s\n", formatExpectedArtifactSummary(*work.ExpectedArtifacts)); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeListWorkResponse(data []byte) (factoryapi.ListWorkResponse, error) {
	var result factoryapi.ListWorkResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return factoryapi.ListWorkResponse{}, err
	}

	var envelope struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	if len(envelope.Results) != len(result.Results) {
		return factoryapi.ListWorkResponse{}, fmt.Errorf("results length %d does not match decoded work count %d", len(envelope.Results), len(result.Results))
	}
	for index, rawWork := range envelope.Results {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawWork, &fields); err != nil {
			return factoryapi.ListWorkResponse{}, fmt.Errorf("results[%d]: %w", index, err)
		}
		structuredResult, ok := fields["structuredResult"]
		if ok && bytes.Equal(bytes.TrimSpace(structuredResult), []byte("null")) {
			// The generated API type uses interface{} and otherwise collapses
			// explicit JSON null into the same nil value as an omitted field.
			result.Results[index].StructuredResult = json.RawMessage("null")
		}
	}
	return result, nil
}

func formatStructuredResult(value any, present bool) (string, error) {
	data, err := structuredResultJSON(value, present)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func structuredResultJSON(value any, present bool) ([]byte, error) {
	if !present {
		return nil, nil
	}
	if value == nil {
		return []byte("null"), nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return nil, err
		}
		return compact.Bytes(), nil
	}
	return json.Marshal(value)
}

func formatExpectedArtifactSummary(artifacts []factoryapi.WorkExpectedArtifact) string {
	parts := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		status := string(artifact.Verification)
		if artifact.Verification == factoryapi.WorkExpectedArtifactVerificationFailed && artifact.Reason != nil {
			status += ": " + string(*artifact.Reason)
		}
		parts = append(parts, fmt.Sprintf("%s=%s [%s]", artifact.Name, artifact.Pattern, status))
	}
	return strings.Join(parts, "; ")
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
