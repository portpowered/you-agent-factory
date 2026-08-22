package commandregistry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestResolvedMoveRunEMapsStableInputsIntoFreshRequests(t *testing.T) {
	var requests []workcli.MoveConfig
	handler := commandregistry.ResolvedMoveRunE(commandregistry.ResolvedMoveBinding{
		MoveWork: func(cfg workcli.MoveConfig) error {
			requests = append(requests, cfg)
			return nil
		},
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return cmd.ErrOrStderr()
		},
	})

	executeResolvedMove(t, handler, []string{
		"--server", "https://factory.example", "--json", "--debug",
		"work", "move", "--session", "session-alpha",
		"--request-id", "move-request-1", "work-1", "complete",
	}, io.Discard, io.Discard, context.Background())
	executeResolvedMove(t, handler, []string{
		"work", "move", "work-2", "review",
	}, io.Discard, io.Discard, context.Background())

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	assertResolvedMoveConfig(t, requests[0], resolvedMoveConfigValues{
		server: "https://factory.example", sessionID: "session-alpha",
		workID: "work-1", stateName: "complete", requestID: "move-request-1",
		json: true, verbose: true, debug: true,
	})
	assertResolvedMoveConfig(t, requests[1], resolvedMoveConfigValues{
		server: "http://localhost:7437", workID: "work-2", stateName: "review",
	})
}

func TestResolvedWorkMovePublicCommandPreservesRoutesBodiesAndOutputs(t *testing.T) {
	var requests []resolvedMoveRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, resolvedMoveRequest{
			method: r.Method,
			path:   r.URL.EscapedPath(),
			body:   readRequestBody(t, r),
		})
		stateName := "init"
		if r.Method == http.MethodPost {
			stateName = "complete"
		}
		writeResolvedWork(t, w, "work/review", stateName)
	}))
	defer server.Close()

	run := resolvedMoveTransportHandler(t)
	var human bytes.Buffer
	executeResolvedMove(t, run, []string{
		"--server", server.URL, "work", "move", "work/review", "complete",
	}, &human, io.Discard, context.Background())
	assertResolvedMoveHumanOutput(t, human.String())

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	executeResolvedMove(t, run, []string{
		"--server", server.URL, "--json", "--verbose",
		"work", "move", "--session", "session/beta",
		"--request-id", "move-request-1", "work/review", "complete",
	}, &output, &diagnostics, context.Background())

	assertResolvedMoveRequests(t, requests)
	var result workcli.MoveSuccessResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("JSON output = %q: %v", output.String(), err)
	}
	assertResolvedMoveJSONAndDiagnostics(
		t, result, output.String(), diagnostics.String(),
	)
}

func TestResolvedWorkMovePublicCommandRejectsInvalidArgumentsBeforeHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing both", wantErr: "requires at least 2 arg(s)"},
		{name: "missing state", args: []string{"work-1"}, wantErr: "requires at least 2 arg(s)"},
		{name: "extra", args: []string{"work-1", "complete", "extra"}, wantErr: "accepts at most 2 arg(s)"},
		{name: "blank work", args: []string{" ", "complete"}, wantErr: "work id is required"},
		{name: "blank state", args: []string{"work-1", " "}, wantErr: "state name is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append(
				[]string{"--server", server.URL, "work", "move"},
				test.args...,
			)
			err := executeResolvedMoveError(
				t, resolvedMoveTransportHandler(t), args,
				io.Discard, io.Discard, context.Background(),
			)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Execute() error = %v, want %q", err, test.wantErr)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want zero for invalid arguments", requests)
	}
}

func TestResolvedWorkMovePublicCommandPreservesFailuresAndCancellation(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    string
	}{
		{
			name: "not found", statusCode: http.StatusNotFound,
			body:    `{"message":"work not found"}`,
			wantErr: `work "work-1" not found: work not found`,
		},
		{
			name: "active dispatch", statusCode: http.StatusBadRequest,
			body:    `{"message":"work is in an active dispatch"}`,
			wantErr: "move work failed (400): work is in an active dispatch",
		},
		{
			name: "invalid target state", statusCode: http.StatusBadRequest,
			body:    `{"message":"authored state does not exist"}`,
			wantErr: "move work failed (400): authored state does not exist",
		},
		{
			name: "server failure", statusCode: http.StatusInternalServerError,
			body:    `{"message":"factory failed"}`,
			wantErr: "move work failed (500): factory failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if test.name != "not found" && r.Method == http.MethodGet {
					writeResolvedWork(t, w, "work-1", "init")
					return
				}
				w.WriteHeader(test.statusCode)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			err := executeResolvedMoveError(
				t, resolvedMoveTransportHandler(t),
				[]string{"--server", server.URL, "work", "move", "work-1", "complete"},
				io.Discard, io.Discard, context.Background(),
			)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Execute() error = %v, want %q", err, test.wantErr)
			}
		})
	}

	t.Run("cancellation", func(t *testing.T) {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			requests++
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := executeResolvedMoveError(
			t, resolvedMoveTransportHandler(t),
			[]string{"--server", server.URL, "work", "move", "work-1", "complete"},
			io.Discard, io.Discard, ctx,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error = %v, want context canceled", err)
		}
		if requests != 0 {
			t.Fatalf("HTTP requests = %d, want zero after cancellation", requests)
		}
	})

	t.Run("output writer", func(t *testing.T) {
		server := newResolvedMoveServer(t, nil)
		defer server.Close()
		writeErr := errors.New("write failed")
		err := executeResolvedMoveError(
			t, resolvedMoveTransportHandler(t),
			[]string{"--server", server.URL, "work", "move", "work-1", "complete"},
			errorWriter{err: writeErr}, io.Discard, context.Background(),
		)
		if !errors.Is(err, writeErr) {
			t.Fatalf("Execute() error = %v, want %v", err, writeErr)
		}
	})
}

func TestResolvedWorkMovePublicCommandPreservesRequestIdConflict(t *testing.T) {
	var posts, successes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeResolvedWork(t, w, "work-1", "init")
			return
		}
		posts++
		var body factoryapi.MoveWorkRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode move body: %v", err)
		}
		if body.RequestId == nil || *body.RequestId != "retry-key" {
			t.Fatalf("request id = %#v, want retry-key", body.RequestId)
		}
		if posts == 1 {
			successes++
			writeResolvedWork(t, w, "work-1", "complete")
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"message":"Operator move request was already applied."}`)
	}))
	defer server.Close()

	args := []string{
		"--server", server.URL, "work", "move",
		"--request-id", "retry-key", "work-1", "complete",
	}
	executeResolvedMove(
		t, resolvedMoveTransportHandler(t), args,
		io.Discard, io.Discard, context.Background(),
	)
	err := executeResolvedMoveError(
		t, resolvedMoveTransportHandler(t), args,
		io.Discard, io.Discard, context.Background(),
	)
	if err == nil || !strings.Contains(err.Error(), "move work failed (409)") ||
		!strings.Contains(err.Error(), "already applied") {
		t.Fatalf("second Execute() error = %v, want applied conflict", err)
	}
	if posts != 2 || successes != 1 {
		t.Fatalf("posts = %d, successes = %d; want 2 attempts and one mutation", posts, successes)
	}
}

func TestResolvedVisualizeRunEMapsStableInputsIntoFreshRequests(t *testing.T) {
	var requests []workcli.VisualizeConfig
	handler := commandregistry.ResolvedVisualizeRunE(commandregistry.ResolvedVisualizeBinding{
		VisualizeWork: func(cfg workcli.VisualizeConfig) error {
			requests = append(requests, cfg)
			return nil
		},
	})
	executeResolvedVisualize(
		t, handler, []string{"work", "render", "first.json"},
		io.Discard, context.Background(),
	)
	executeResolvedVisualize(
		t, handler,
		[]string{"work", "render", "--format", "markdown-mermaid", "second.json"},
		io.Discard, context.Background(),
	)
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if requests[0].BatchFile != "first.json" || requests[0].Format != "mermaid" ||
		requests[1].BatchFile != "second.json" ||
		requests[1].Format != "markdown-mermaid" {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].Output == nil || requests[0].Context == nil ||
		requests[1].Output == nil || requests[1].Context == nil {
		t.Fatalf("requests lack invocation-local context/output: %#v", requests)
	}
}

func TestResolvedWorkVisualizePublicCommandPreservesLocalBehavior(t *testing.T) {
	path := writeWorkHandlerBatchFile(t, `{
  "requestId":"resolved-visualize","type":"FACTORY_REQUEST_BATCH",
  "works":[{"name":"alpha","workTypeName":"task"},{"name":"beta","workTypeName":"task"}],
  "relations":[{"type":"DEPENDS_ON","sourceWorkName":"beta","targetWorkName":"alpha"}]
}`)
	httpRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		httpRequests++
	}))
	defer server.Close()
	handler := resolvedVisualizeTransportHandler(func(request workservice.VisualizationRequest) (string, error) {
		switch strings.ToLower(request.Format) {
		case "mermaid":
			return "flowchart TD\n  beta --> alpha\n", nil
		case "markdown-mermaid":
			return "# Work Dependency Graph\n\n```mermaid\nflowchart TD\n  beta --> alpha\n```\n", nil
		default:
			return "", fmt.Errorf("unsupported format %q", strings.ToLower(request.Format))
		}
	})

	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{"default mermaid", []string{"--server", server.URL, "work", "render", path},
			[]string{"flowchart TD", "beta --> alpha"}},
		{"format spelling", []string{"work", "render", "--format", "MERMAID", path},
			[]string{"flowchart TD", "beta --> alpha"}},
		{"markdown", []string{"work", "render", "--format", "markdown-mermaid", path},
			[]string{"# Work Dependency Graph", "```mermaid", "beta --> alpha"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			executeResolvedVisualize(
				t, handler, test.args, &output, context.Background(),
			)
			for _, want := range test.want {
				if !strings.Contains(output.String(), want) {
					t.Fatalf("output = %q, want %q", output.String(), want)
				}
			}
		})
	}
	if httpRequests != 0 {
		t.Fatalf("HTTP requests = %d, want zero", httpRequests)
	}
}

func TestResolvedWorkVisualizePublicCommandPreservesFailures(t *testing.T) {
	invalid := writeWorkHandlerBatchFile(t, `{not-json`)
	tests := []struct {
		name      string
		args      []string
		operation workservice.VisualizationOperation
		want      string
	}{
		{"missing argument", []string{"work", "render"}, nil, "requires at least 1 arg"},
		{"extra argument", []string{"work", "render", invalid, "extra"}, nil, "accepts at most 1 arg"},
		{"invalid format", []string{"work", "render", "--format", "svg", invalid},
			visualizationFailure(`unsupported format "svg"`), `unsupported format "svg"`},
		{"unreadable file", []string{"work", "render", filepath.Join(t.TempDir(), "missing.json")},
			visualizationFailure("batch file not found"), "batch file not found"},
		{"invalid content", []string{"work", "render", invalid},
			visualizationFailure("invalid JSON"), "invalid JSON"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := test.operation
			if operation == nil {
				operation = func(workservice.VisualizationRequest) (string, error) {
					t.Fatal("visualization operation called after parser failure")
					return "", nil
				}
			}
			err := executeResolvedVisualizeError(
				t, resolvedVisualizeTransportHandler(operation), test.args,
				io.Discard, context.Background(),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
		})
	}

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := executeResolvedVisualizeError(
			t, resolvedVisualizeTransportHandler(func(workservice.VisualizationRequest) (string, error) {
				t.Fatal("visualization operation called after cancellation")
				return "", nil
			}),
			[]string{"work", "render", invalid}, io.Discard, ctx,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error = %v, want context canceled", err)
		}
	})

	t.Run("output writer", func(t *testing.T) {
		writeErr := errors.New("write failed")
		err := executeResolvedVisualizeError(
			t, resolvedVisualizeTransportHandler(visualizationFailure("invalid JSON")),
			[]string{"work", "render", invalid},
			errorWriter{err: writeErr}, context.Background(),
		)
		if err == nil || errors.Is(err, writeErr) {
			t.Fatalf("invalid input error = %v, want validation before output", err)
		}
		valid := writeWorkHandlerBatchFile(t, `{
		  "requestId":"writer","type":"FACTORY_REQUEST_BATCH",
		  "works":[{"name":"alpha","workTypeName":"task"}]
		}`)
		err = executeResolvedVisualizeError(
			t, resolvedVisualizeTransportHandler(func(workservice.VisualizationRequest) (string, error) {
				return "flowchart TD\n", nil
			}),
			[]string{"work", "render", valid},
			errorWriter{err: writeErr}, context.Background(),
		)
		if !errors.Is(err, writeErr) {
			t.Fatalf("Execute() error = %v, want %v", err, writeErr)
		}
	})
}

func visualizationFailure(message string) workservice.VisualizationOperation {
	return func(workservice.VisualizationRequest) (string, error) {
		return "", errors.New(message)
	}
}

func resolvedVisualizeTransportHandler(
	operation workservice.VisualizationOperation,
) commandregistry.ResolvedWorkRunE {
	return commandregistry.ResolvedVisualizeRunE(commandregistry.ResolvedVisualizeBinding{
		VisualizeWork: workcli.NewVisualize(operation),
	})
}

func executeResolvedVisualize(
	t *testing.T,
	handler commandregistry.ResolvedWorkRunE,
	args []string,
	output io.Writer,
	ctx context.Context,
) {
	t.Helper()
	if err := executeResolvedVisualizeError(t, handler, args, output, ctx); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}
}

func executeResolvedVisualizeError(
	t *testing.T,
	handler commandregistry.ResolvedWorkRunE,
	args []string,
	output io.Writer,
	ctx context.Context,
) error {
	t.Helper()
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
		return nil
	}
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{
		List: noop, Show: noop, Move: noop, Visualize: handler,
	})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}
	root.SetOut(output)
	root.SetErr(io.Discard)
	root.SetContext(ctx)
	root.SetArgs(args)
	return root.Execute()
}

type resolvedMoveConfigValues struct {
	server    string
	sessionID string
	workID    string
	stateName string
	requestID string
	json      bool
	verbose   bool
	debug     bool
}

func assertResolvedMoveConfig(
	t *testing.T,
	cfg workcli.MoveConfig,
	want resolvedMoveConfigValues,
) {
	t.Helper()
	got := resolvedMoveConfigValues{
		server: cfg.Server, sessionID: cfg.SessionID, workID: cfg.WorkID,
		stateName: cfg.StateName, requestID: cfg.RequestID, json: cfg.JSON,
		verbose: cfg.Verbose, debug: cfg.Debug,
	}
	if got != want {
		t.Fatalf("move config = %#v, want %#v", got, want)
	}
}

func assertResolvedMoveHumanOutput(t *testing.T, got string) {
	t.Helper()
	for _, want := range []string{
		"Work ID:\twork/review\n",
		"Previous state:\tinit\n",
		"New state:\tcomplete\n",
		"Session ID:\t~default\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("human output = %q, want %q", got, want)
		}
	}
}

func assertResolvedMoveJSONAndDiagnostics(
	t *testing.T,
	result workcli.MoveSuccessResult,
	output string,
	diagnostics string,
) {
	t.Helper()
	want := workcli.MoveSuccessResult{
		WorkID: "work/review", PreviousState: "init", NewState: "complete",
		SessionID:    "session/beta",
		EndpointPath: "/factory-sessions/session/beta/work/work/review/move",
	}
	if result != want {
		t.Fatalf("JSON result = %#v, want %#v", result, want)
	}
	if strings.Contains(output, "work move request") {
		t.Fatalf("stdout contains diagnostics: %q", output)
	}
	for _, wantDiagnostic := range []string{
		"work move request", "requestIdPresent=true", "work move response",
	} {
		if !strings.Contains(diagnostics, wantDiagnostic) {
			t.Fatalf("diagnostics = %q, want %q", diagnostics, wantDiagnostic)
		}
	}
}

func writeWorkHandlerBatchFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "batch.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}
	return path
}
