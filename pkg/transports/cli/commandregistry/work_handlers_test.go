package commandregistry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	cligenerated "github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestNewWorkRegistryRegistersContractedRunnableIDs(t *testing.T) {
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	for _, commandID := range []string{
		"you.work.list",
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		if _, lookupErr := registry.Lookup(commandID); lookupErr != nil {
			t.Fatalf("Lookup(%q) error = %v", commandID, lookupErr)
		}
	}
}

func TestNewWorkRegistryRejectsMissingHandlers(t *testing.T) {
	cases := []struct {
		name     string
		handlers commandregistry.WorkHandlers
	}{
		{
			name: "missing list",
			handlers: commandregistry.WorkHandlers{
				ShowRunE:      noopRunE,
				MoveRunE:      noopRunE,
				VisualizeRunE: noopRunE,
			},
		},
		{
			name: "missing show",
			handlers: commandregistry.WorkHandlers{
				ListRunE:      noopRunE,
				MoveRunE:      noopRunE,
				VisualizeRunE: noopRunE,
			},
		},
		{
			name: "missing move",
			handlers: commandregistry.WorkHandlers{
				ListRunE:      noopRunE,
				ShowRunE:      noopRunE,
				VisualizeRunE: noopRunE,
			},
		},
		{
			name: "missing visualize",
			handlers: commandregistry.WorkHandlers{
				ListRunE: noopRunE,
				ShowRunE: noopRunE,
				MoveRunE: noopRunE,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := commandregistry.NewWorkRegistry(tc.handlers); err == nil {
				t.Fatal("NewWorkRegistry() missing handler = nil, want error")
			}
		})
	}
}

func TestListRunEUsesHandwrittenServicePath(t *testing.T) {
	var gotMethod string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	listCfg := workcli.ListConfig{Context: context.Background()}
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: commandregistry.ListRunE(commandregistry.ListBinding{
			Config:   &listCfg,
			Server:   stringPtr(srv.URL),
			ListWork: workcli.NewList(testHTTPProtocol(t), commandRegistryListPreparation{}),
		}),
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "list"}
	if err := registry.AttachRunE(cmd, "you.work.list"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("HTTP method = %q, want GET for listWorkBySessionId binding", gotMethod)
	}
	if gotPath != "/factory-sessions/~default/work" {
		t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work", gotPath)
	}
}

type commandRegistryListPreparation struct{}

func (commandRegistryListPreparation) PrepareListRequest(
	_ context.Context,
	options workservice.ListOptions,
) (workservice.PreparedListRequest, error) {
	return workservice.PreparedListRequest{Options: options, FilterSummary: "test"}, nil
}

func TestShowRunEUsesHandwrittenServicePath(t *testing.T) {
	var gotMethod string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Work{
			Name:   "Review PRD",
			WorkId: stringPtr("work-review-1"),
			State: &factoryapi.WorkState{
				Name: "review",
				Type: factoryapi.WorkStateTypePROCESSING,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	showCfg := workcli.ShowConfig{Context: context.Background()}
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: noopRunE,
		ShowRunE: commandregistry.ShowRunE(commandregistry.ShowBinding{
			Config:   &showCfg,
			Server:   stringPtr(srv.URL),
			ShowWork: workcli.NewShow(testHTTPProtocol(t)),
		}),
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "show"}
	if err := registry.AttachRunE(cmd, "you.work.show"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"work-review-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("HTTP method = %q, want GET for getWorkBySessionId binding", gotMethod)
	}
	if gotPath != "/factory-sessions/~default/work/work-review-1" {
		t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work/work-review-1", gotPath)
	}
}

func TestMoveRunEUsesHandwrittenServicePath(t *testing.T) {
	var gotMoveMethod string
	var gotMovePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			writeWorkJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
		case r.Method == http.MethodPost:
			gotMoveMethod = r.Method
			gotMovePath = r.URL.Path
			writeWorkJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	moveCfg := workcli.MoveConfig{Context: context.Background()}
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: noopRunE,
		ShowRunE: noopRunE,
		MoveRunE: commandregistry.MoveRunE(commandregistry.MoveBinding{
			Config:   &moveCfg,
			Server:   stringPtr(srv.URL),
			MoveWork: workcli.NewMove(testHTTPProtocol(t)),
		}),
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "move"}
	if err := registry.AttachRunE(cmd, "you.work.move"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"work-move-1", "complete"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotMoveMethod != http.MethodPost {
		t.Fatalf("HTTP method = %q, want POST for moveWorkBySessionId binding", gotMoveMethod)
	}
	if gotMovePath != "/factory-sessions/~default/work/work-move-1/move" {
		t.Fatalf("HTTP path = %q, want /factory-sessions/~default/work/work-move-1/move", gotMovePath)
	}
}

func TestVisualizeRunERemainsLocalReadOnly(t *testing.T) {
	path := writeWorkHandlerBatchFile(t, `{
  "requestId": "visualize-registry-test",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"}
  ]
}`)
	format := "mermaid"

	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: noopRunE,
		ShowRunE: noopRunE,
		MoveRunE: noopRunE,
		VisualizeRunE: commandregistry.VisualizeRunE(commandregistry.VisualizeBinding{
			Format: &format,
			Visualize: func(cfg workcli.VisualizeConfig) error {
				_, err := io.WriteString(cfg.Output, "flowchart TD\n")
				return err
			},
		}),
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}

	cmd := &cobra.Command{Use: "visualize"}
	if err := registry.AttachRunE(cmd, "you.work.visualize"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(out.String(), "flowchart TD\n") {
		t.Fatalf("output missing flowchart header:\n%s", out.String())
	}
}

func TestListRunEWritesDiagnosticsToConfiguredWriter(t *testing.T) {
	var diagnostic bytes.Buffer
	listCfg := workcli.ListConfig{Context: context.Background()}
	runE := commandregistry.ListRunE(commandregistry.ListBinding{
		Config: &listCfg,
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer {
			return &diagnostic
		},
		ListWork: func(cfg workcli.ListConfig) error {
			if cfg.Diagnostics != &diagnostic {
				t.Fatalf("diagnostics writer = %T, want *bytes.Buffer", cfg.Diagnostics)
			}
			return nil
		},
	})
	cmd := &cobra.Command{Use: "list"}
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}
}

func writeWorkJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
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

func TestRunnableWorkCommandIDsFromGeneratedManifest(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableWorkCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableWorkCommandIDs() error = %v", err)
	}
	want := []string{
		"you.work.list",
		"you.work.move",
		"you.work.show",
		"you.work.visualize",
	}
	if len(ids) != len(want) {
		t.Fatalf("runnable IDs = %#v, want %#v", ids, want)
	}
	for i, commandID := range want {
		if ids[i] != commandID {
			t.Fatalf("runnable IDs[%d] = %q, want %q", i, ids[i], commandID)
		}
	}
}

func TestVerifyWorkRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyWorkRunnableCoverage() missing list handler = nil, want error")
	}
}

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

func TestVerifyWorkRunnableCoverageAcceptsCompleteRegistry(t *testing.T) {
	manifest, err := cligenerated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	for _, commandID := range []string{
		"you.work.list",
		"you.work.show",
		"you.work.move",
		"you.work.visualize",
	} {
		if err := registry.Register(commandID, noopRunE); err != nil {
			t.Fatalf("Register(%q) error = %v", commandID, err)
		}
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		t.Fatalf("VerifyWorkRunnableCoverage() error = %v", err)
	}
}
