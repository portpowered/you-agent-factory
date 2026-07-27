package commandregistry_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestResolvedListRunEMapsStableInputsIntoFreshRequests(t *testing.T) {
	var requests []workcli.ListConfig
	handler := commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{
		ListWork: func(cfg workcli.ListConfig) error {
			requests = append(requests, cfg)
			return nil
		},
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return cmd.ErrOrStderr()
		},
	})

	executeResolvedList(t, handler, []string{
		"--server", "https://factory.example", "--json", "--debug",
		"work", "list",
		"--session", "session-alpha",
		"--state-name", "review",
		"--state-type", "PROCESSING",
		"--name", "prd",
		"--work-type-name", "story",
		"--trace-id", "trace-1",
		"--sort-by", "state.type",
		"--max-results", "7",
		"--next-token", base64.StdEncoding.EncodeToString([]byte("cursor-1")),
	}, io.Discard, io.Discard, context.Background())
	executeResolvedList(
		t,
		handler,
		[]string{"work", "list"},
		io.Discard,
		io.Discard,
		context.Background(),
	)

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	assertResolvedListConfig(t, requests[0], resolvedListConfigValues{
		server: "https://factory.example", sessionID: "session-alpha",
		stateName: "review", stateType: "PROCESSING", name: "prd",
		workTypeName: "story", traceID: "trace-1", sortBy: "state.type",
		maxResults: 7, nextToken: base64.StdEncoding.EncodeToString([]byte("cursor-1")),
		json: true, verbose: true, debug: true,
	})
	assertResolvedListConfig(t, requests[1], resolvedListConfigValues{
		server: "http://localhost:7437",
	})
}

func TestResolvedWorkListPublicCommandPreservesRoutesOutputsAndDiagnostics(t *testing.T) {
	nextToken := base64.StdEncoding.EncodeToString([]byte("next"))
	var requests []struct {
		path  string
		query url.Values
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, struct {
			path  string
			query url.Values
		}{path: r.URL.EscapedPath(), query: r.URL.Query()})
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Review PRD",
				WorkId:       stringPtr("work-1"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
			}},
			PaginationContext: &factoryapi.PaginationContext{
				MaxResults: 1,
				NextToken:  &nextToken,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	run := resolvedListTransportHandler(t)
	var human bytes.Buffer
	executeResolvedList(t, run, []string{
		"--server", server.URL, "work", "list",
	}, &human, io.Discard, context.Background())
	if got := human.String(); !strings.Contains(got, "work-1\tReview PRD\tstory\treview\tPROCESSING") {
		t.Fatalf("human output = %q", got)
	}

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	executeResolvedList(t, run, []string{
		"--server", server.URL, "--json", "--verbose",
		"work", "list",
		"--session", "session/beta",
		"--state-name", "in review",
		"--state-type", "PROCESSING",
		"--name", "plan & review",
		"--work-type-name", "story/type",
		"--trace-id", "trace+1",
		"--sort-by", "state.type",
		"--max-results", "1",
		"--next-token", nextToken,
	}, &output, &diagnostics, context.Background())

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if requests[0].path != "/factory-sessions/~default/work" {
		t.Fatalf("default path = %q", requests[0].path)
	}
	sessionRequest := requests[1]
	if sessionRequest.path != "/factory-sessions/session%2Fbeta/work" {
		t.Fatalf("session path = %q", sessionRequest.path)
	}
	assertResolvedListQuery(t, sessionRequest.query, map[string]string{
		"state.name": "in review", "state.type": "PROCESSING",
		"name": "plan & review", "workTypeName": "story/type",
		"traceId": "trace+1", "sortBy": "state.type",
		"maxResults": "1", "nextToken": nextToken,
	})
	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("JSON output = %q: %v", output.String(), err)
	}
	assertResolvedListResponse(t, response, nextToken)
	if strings.Contains(output.String(), "work list request") {
		t.Fatalf("stdout contains diagnostics: %q", output.String())
	}
	if got := diagnostics.String(); !strings.Contains(got, "work list request") ||
		!strings.Contains(got, "filters=state.name,state.type,name,workTypeName,traceId,sortBy") {
		t.Fatalf("diagnostics = %q", got)
	}
}

func TestResolvedWorkListPublicCommandPreservesEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	executeResolvedList(
		t,
		resolvedListTransportHandler(t),
		[]string{"--server", server.URL, "work", "list"},
		&output,
		io.Discard,
		context.Background(),
	)
	if got := output.String(); got != "No work found.\n" {
		t.Fatalf("empty output = %q", got)
	}
}

func TestResolvedWorkListPublicCommandRejectsInvalidInputsBeforeHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	tests := []struct {
		name    string
		args    []string
		wantErr string
		prepare error
	}{
		{
			name: "state type", args: []string{"--state-type", "UNKNOWN"},
			wantErr: "--state-type must be one of",
			prepare: &workservice.ValidationError{
				Field:   workservice.FilterStateType,
				Message: "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED",
			},
		},
		{
			name: "sort", args: []string{"--sort-by", "name"},
			wantErr: "--sort-by must be state.type",
			prepare: &workservice.ValidationError{
				Field: "sortBy", Message: "sortBy must be state.type",
			},
		},
		{
			name: "maximum", args: []string{"--max-results", "-1"},
			wantErr: "maxResults must be zero or greater",
			prepare: &workservice.ValidationError{
				Field: "maxResults", Message: "maxResults must be zero or greater",
			},
		},
		{
			name: "cursor", args: []string{"--next-token", "not-base64"},
			wantErr: "nextToken must be valid standard base64",
			prepare: &workservice.ValidationError{
				Field: "nextToken", Message: "nextToken must be valid standard base64 for a non-empty cursor",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--server", server.URL, "work", "list"}, test.args...)
			err := executeResolvedListError(
				t,
				resolvedListTransportHandlerWithPreparation(
					t,
					resolvedListPreparation{err: test.prepare},
				),
				args,
				io.Discard,
				io.Discard,
				context.Background(),
			)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Execute() error = %v, want %q", err, test.wantErr)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want zero for invalid inputs", requests)
	}
}

func TestResolvedWorkListPublicCommandPreservesFailuresAndCancellation(t *testing.T) {
	t.Run("server failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"message":"factory failed"}`)
		}))
		defer server.Close()
		err := executeResolvedListError(
			t,
			resolvedListTransportHandler(t),
			[]string{"--server", server.URL, "work", "list"},
			io.Discard,
			io.Discard,
			context.Background(),
		)
		if err == nil || !strings.Contains(err.Error(), "list work failed (500): factory failed") {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			requests++
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := executeResolvedListError(
			t,
			resolvedListTransportHandler(t),
			[]string{"--server", server.URL, "work", "list"},
			io.Discard,
			io.Discard,
			ctx,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error = %v, want context canceled", err)
		}
		if requests != 0 {
			t.Fatalf("HTTP requests = %d, want zero after cancellation", requests)
		}
	})

	t.Run("output writer", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"results":[]}`)
		}))
		defer server.Close()
		writeErr := errors.New("write failed")
		err := executeResolvedListError(
			t,
			resolvedListTransportHandler(t),
			[]string{"--server", server.URL, "work", "list"},
			errorWriter{err: writeErr},
			io.Discard,
			context.Background(),
		)
		if !errors.Is(err, writeErr) {
			t.Fatalf("Execute() error = %v, want %v", err, writeErr)
		}
	})
}

func resolvedListTransportHandler(t *testing.T) commandregistry.ResolvedWorkRunE {
	t.Helper()
	return resolvedListTransportHandlerWithPreparation(t, resolvedListPreparation{})
}

func resolvedListTransportHandlerWithPreparation(
	t *testing.T,
	prepare workservice.ListRequestPreparation,
) commandregistry.ResolvedWorkRunE {
	t.Helper()
	return commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{
		ListWork: workcli.NewList(
			testHTTPProtocol(t),
			prepare,
		),
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return cmd.ErrOrStderr()
		},
	})
}

type resolvedListPreparation struct {
	err error
}

func (prepare resolvedListPreparation) PrepareListRequest(
	ctx context.Context,
	options workservice.ListOptions,
) (workservice.PreparedListRequest, error) {
	if err := ctx.Err(); err != nil {
		return workservice.PreparedListRequest{}, err
	}
	if prepare.err != nil {
		return workservice.PreparedListRequest{}, prepare.err
	}
	return workservice.PreparedListRequest{
		Options:       options,
		FilterSummary: "state.name,state.type,name,workTypeName,traceId,sortBy",
	}, nil
}

func executeResolvedList(
	t *testing.T,
	list commandregistry.ResolvedWorkRunE,
	args []string,
	output io.Writer,
	diagnostics io.Writer,
	ctx context.Context,
) {
	t.Helper()
	if err := executeResolvedListError(t, list, args, output, diagnostics, ctx); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}
}

func executeResolvedListError(
	t *testing.T,
	list commandregistry.ResolvedWorkRunE,
	args []string,
	output io.Writer,
	diagnostics io.Writer,
	ctx context.Context,
) error {
	t.Helper()
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{
		List: list, Show: noop, Move: noop, Visualize: noop,
	})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}
	root.SetOut(output)
	root.SetErr(diagnostics)
	root.SetContext(ctx)
	root.SetArgs(args)
	return root.Execute()
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type resolvedListConfigValues struct {
	server       string
	sessionID    string
	stateName    string
	stateType    string
	name         string
	workTypeName string
	traceID      string
	sortBy       string
	maxResults   int
	nextToken    string
	json         bool
	verbose      bool
	debug        bool
}

func assertResolvedListConfig(
	t *testing.T,
	got workcli.ListConfig,
	want resolvedListConfigValues,
) {
	t.Helper()
	values := resolvedListConfigValues{
		server: got.Server, sessionID: got.SessionID,
		stateName: got.StateName, stateType: got.StateType, name: got.Name,
		workTypeName: got.WorkTypeName, traceID: got.TraceID, sortBy: got.SortBy,
		maxResults: got.MaxResults, nextToken: got.NextToken,
		json: got.JSON, verbose: got.Verbose, debug: got.Debug,
	}
	if values != want {
		t.Fatalf("list config values = %#v, want %#v", values, want)
	}
}

func assertResolvedListQuery(t *testing.T, query url.Values, want map[string]string) {
	t.Helper()
	for key, value := range want {
		if got := query.Get(key); got != value {
			t.Fatalf("query %s = %q, want %q", key, got, value)
		}
		if len(query[key]) != 1 {
			t.Fatalf("query %s values = %v, want one encoded value", key, query[key])
		}
	}
}

func assertResolvedListResponse(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	nextToken string,
) {
	t.Helper()
	if len(response.Results) != 1 {
		t.Fatalf("JSON results = %#v, want one", response.Results)
	}
	if response.PaginationContext == nil || response.PaginationContext.NextToken == nil {
		t.Fatalf("JSON pagination = %#v, want next token", response.PaginationContext)
	}
	if *response.PaginationContext.NextToken != nextToken {
		t.Fatalf("JSON next token = %q, want %q", *response.PaginationContext.NextToken, nextToken)
	}
}

type fixedHTTPClock struct{}

func (fixedHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func testHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, fixedHTTPClock{})
	if err != nil {
		t.Fatalf("build test HTTP protocol: %v", err)
	}
	return protocol
}
