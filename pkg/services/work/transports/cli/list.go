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
	Context           context.Context
	Server            string
	SessionID         string
	StateName         string
	StateType         string
	Name              string
	WorkTypeName      string
	TraceID           string
	Terminal          bool
	NonTerminal       bool
	IncludeSuperseded bool
	SortBy            string
	MaxResults        int
	NextToken         string
	Counts            bool
	JSON              bool
	Verbose           bool
	Debug             bool
	Output            io.Writer
	Diagnostics       io.Writer
	HTTP              clihttp.Protocol
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
	if err := validateListConfig(cfg, service); err != nil {
		return err
	}

	prepared, err := service.listPrepare.PrepareListRequest(cfg.Context, workdomain.ListOptions{
		StateName:         cfg.StateName,
		StateType:         cfg.StateType,
		Name:              cfg.Name,
		WorkTypeName:      cfg.WorkTypeName,
		TraceID:           cfg.TraceID,
		Terminal:          cfg.Terminal,
		NonTerminal:       cfg.NonTerminal,
		IncludeSuperseded: cfg.IncludeSuperseded,
		SortBy:            cfg.SortBy,
		MaxResults:        cfg.MaxResults,
		NextToken:         cfg.NextToken,
		Counts:            cfg.Counts,
	})
	if err != nil {
		return listConfigError(err)
	}
	result, err := fetchAllListPages(cfg, prepared)
	if err != nil {
		return err
	}
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(result)
	}
	return renderListResult(cfg.Output, result)
}

func validateListConfig(cfg ListConfig, service *service) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	if service == nil || service.listPrepare == nil {
		return fmt.Errorf("Work list request preparation is required")
	}
	return nil
}

func fetchAllListPages(
	cfg ListConfig,
	prepared workdomain.PreparedListRequest,
) (factoryapi.ListWorkResponse, error) {
	options := prepared.Options
	seenTokens := make(map[string]struct{})
	if options.NextToken != "" {
		seenTokens[options.NextToken] = struct{}{}
	}

	var aggregate factoryapi.ListWorkResponse
	for pageNumber := 1; ; pageNumber++ {
		if err := cfg.Context.Err(); err != nil {
			return factoryapi.ListWorkResponse{}, err
		}

		page, err := fetchListPage(cfg, prepared, options, pageNumber)
		if err != nil {
			return factoryapi.ListWorkResponse{}, err
		}
		if pageNumber == 1 {
			aggregate = page
		} else {
			aggregate.Results = append(aggregate.Results, page.Results...)
			if aggregate.Counts == nil {
				aggregate.Counts = page.Counts
			}
			aggregate.PaginationContext = page.PaginationContext
		}

		nextToken := listPageNextToken(page)
		if nextToken == "" {
			return aggregate, nil
		}
		if err := validateListPageContinuation(nextToken); err != nil {
			return factoryapi.ListWorkResponse{}, fmt.Errorf(
				"work list pagination failed after page %d: %w",
				pageNumber,
				err,
			)
		}
		if _, repeated := seenTokens[nextToken]; repeated {
			return factoryapi.ListWorkResponse{}, fmt.Errorf(
				"work list pagination did not advance after page %d: repeated continuation token",
				pageNumber,
			)
		}
		seenTokens[nextToken] = struct{}{}
		options.NextToken = nextToken
	}
}

func fetchListPage(
	cfg ListConfig,
	prepared workdomain.PreparedListRequest,
	options workdomain.ListOptions,
	pageNumber int,
) (factoryapi.ListWorkResponse, error) {
	prepared.Options = options
	request, err := buildListRequest(cfg, prepared)
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	endpoint := request.endpoint
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work list request page=%d endpointPath=%s server=%s session=%s filters=%s maxResults=%d nextTokenPresent=%t",
		pageNumber,
		endpoint.Path,
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		request.filterSummary,
		options.MaxResults,
		options.NextToken != "",
	)

	var responsePayload json.RawMessage
	response, err := cfg.HTTP.GetJSON(cfg.Context, endpoint.String(), &responsePayload)
	if err != nil {
		return factoryapi.ListWorkResponse{}, handleListPageRequestError(cfg, endpoint, options, pageNumber, response, err)
	}
	resp := response.HTTP
	if resp == nil {
		logListPageFailure(cfg, endpoint, options, pageNumber, response, 0, "invalid-response")
		return factoryapi.ListWorkResponse{}, listPageEmptyResponseError(pageNumber)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return factoryapi.ListWorkResponse{}, listPageStatusError(cfg, endpoint, options, pageNumber, response, resp)
	}
	result, err := decodeListWorkResponse(responsePayload)
	if err != nil {
		logListPageFailure(cfg, endpoint, options, pageNumber, response, resp.StatusCode, "decode")
		return factoryapi.ListWorkResponse{}, listPageDecodeError(pageNumber, err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work list response page=%d endpointPath=%s status=%d durationMillis=%d resultCount=%d nextTokenPresent=%t",
		pageNumber,
		endpoint.Path,
		resp.StatusCode,
		response.Duration.Milliseconds(),
		len(result.Results),
		listPageNextToken(result) != "",
	)
	return result, nil
}

func handleListPageRequestError(
	cfg ListConfig,
	endpoint url.URL,
	options workdomain.ListOptions,
	pageNumber int,
	response clihttp.Response,
	requestErr error,
) error {
	if ctxErr := cfg.Context.Err(); ctxErr != nil {
		logListPageFailure(cfg, endpoint, options, pageNumber, response, 0, "context-canceled")
		return newListCancellationError(pageNumber, ctxErr)
	}
	if errors.Is(requestErr, context.Canceled) || errors.Is(requestErr, context.DeadlineExceeded) {
		logListPageFailure(cfg, endpoint, options, pageNumber, response, 0, "context-canceled")
		return newListCancellationError(pageNumber, requestErr)
	}
	if response.HTTP != nil && response.HTTP.StatusCode == http.StatusOK {
		closeListResponse(response)
		logListPageFailure(cfg, endpoint, options, pageNumber, response, response.HTTP.StatusCode, "decode")
		return listPageDecodeError(pageNumber, requestErr)
	}
	logListPageFailure(cfg, endpoint, options, pageNumber, response, 0, "unreachable")
	return newListTransportError(pageNumber, safeListEndpoint(endpoint), requestErr)
}

func listPageEmptyResponseError(pageNumber int) error {
	if pageNumber == 1 {
		return fmt.Errorf("list work failed: HTTP response was empty")
	}
	return fmt.Errorf("work list page %d failed: HTTP response was empty", pageNumber)
}

func listPageStatusError(
	cfg ListConfig,
	endpoint url.URL,
	options workdomain.ListOptions,
	pageNumber int,
	response clihttp.Response,
	resp *http.Response,
) error {
	logListPageFailure(cfg, endpoint, options, pageNumber, response, resp.StatusCode, "status")
	if errResp, ok := clihttp.DecodeAPIError(resp); ok {
		if pageNumber == 1 {
			return fmt.Errorf("list work failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("work list page %d failed (%d): %s", pageNumber, resp.StatusCode, errResp.Message)
	}
	if pageNumber == 1 {
		return fmt.Errorf("list work failed (%d)", resp.StatusCode)
	}
	return fmt.Errorf("work list page %d failed (%d)", pageNumber, resp.StatusCode)
}

func listPageDecodeError(pageNumber int, decodeErr error) error {
	if pageNumber == 1 {
		return fmt.Errorf("decode work list response: %w", decodeErr)
	}
	return fmt.Errorf("work list page %d response decode: %w", pageNumber, decodeErr)
}

func logListPageFailure(
	cfg ListConfig,
	endpoint url.URL,
	options workdomain.ListOptions,
	pageNumber int,
	response clihttp.Response,
	status int,
	kind string,
) {
	if status > 0 {
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"work list response page=%d endpointPath=%s nextTokenPresent=%t status=%d durationMillis=%d error=%s",
			pageNumber,
			endpoint.Path,
			options.NextToken != "",
			status,
			response.Duration.Milliseconds(),
			kind,
		)
		return
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work list response page=%d endpointPath=%s nextTokenPresent=%t error=%s durationMillis=%d",
		pageNumber,
		endpoint.Path,
		options.NextToken != "",
		kind,
		response.Duration.Milliseconds(),
	)
}

func listPageNextToken(result factoryapi.ListWorkResponse) string {
	if result.PaginationContext == nil || result.PaginationContext.NextToken == nil {
		return ""
	}
	return *result.PaginationContext.NextToken
}

func validateListPageContinuation(token string) error {
	if token != "" && strings.TrimSpace(token) == "" {
		return fmt.Errorf("continuation token was blank")
	}
	return nil
}

func closeListResponse(response clihttp.Response) {
	if response.HTTP == nil || response.HTTP.Body == nil {
		return
	}
	_ = response.HTTP.Body.Close()
}

func safeListEndpoint(endpoint url.URL) string {
	endpoint.RawQuery = ""
	endpoint.ForceQuery = false
	endpoint.Fragment = ""
	return endpoint.String()
}

type listTransportError struct {
	message string
	cause   error
}

func newListTransportError(pageNumber int, endpoint string, cause error) error {
	message := fmt.Sprintf("factory not reachable at %s: transport request failed", endpoint)
	if pageNumber > 1 {
		message = fmt.Sprintf("work list page %d: %s", pageNumber, message)
	}
	return &listTransportError{message: message, cause: cause}
}

func (err *listTransportError) Error() string {
	if err == nil {
		return ""
	}
	return err.message
}

func (err *listTransportError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

type listCancellationError struct {
	page  int
	cause error
}

func newListCancellationError(pageNumber int, cause error) error {
	return &listCancellationError{page: pageNumber, cause: cause}
}

func (err *listCancellationError) Error() string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("work list page %d canceled", err.page)
}

func (err *listCancellationError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
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
	if options.IncludeSuperseded {
		values.Set(workdomain.FilterIncludeSuperseded, "true")
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
		if supersededBy := stringValue(work.SupersededBy); supersededBy != "" {
			if _, err := fmt.Fprintf(output, "  Superseded by: %s\n", supersededBy); err != nil {
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
