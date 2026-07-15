package submitparity_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/spf13/cobra"
)

func TestGeneratedVsLegacyParityMatrix_Submit(t *testing.T) {
	legacyRoot, generatedRoot, err := cli.NewRunSubmitFamilyParityRoots(cli.RootCommandOptions{})
	if err != nil {
		t.Fatalf("NewRunSubmitFamilyParityRoots() error = %v", err)
	}
	legacySubmit := mustFindSubmit(t, legacyRoot)
	generatedSubmit := mustFindSubmit(t, generatedRoot)
	assertNoMismatches(t, climanifestparity.CompareConstructorIdentityParity("you.submit", legacySubmit, generatedSubmit))

	if mismatches, compareErr := climanifestparity.CompareConstructorHelpParity(
		"you.submit", legacyRoot, generatedRoot, "you submit",
	); compareErr != nil {
		t.Fatalf("CompareConstructorHelpParity() error = %v", compareErr)
	} else {
		assertNoMismatches(t, mismatches)
	}
	if mismatches, compareErr := climanifestparity.CompareConstructorCompletionInventoryParity(
		"you.submit", "you submit", legacyRoot, generatedRoot,
	); compareErr != nil {
		t.Fatalf("CompareConstructorCompletionInventoryParity() error = %v", compareErr)
	} else {
		assertNoMismatches(t, mismatches)
	}
}

func TestGeneratedVsLegacySubmitSuccessParity(t *testing.T) {
	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("ship the release\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		globalFlags []string
		localFlags  []string
		wantPath    string
		wantJSON    bool
	}{
		{
			name:     "default session human output",
			wantPath: "/factory-sessions/~default/work",
		},
		{
			name:        "explicit session JSON output and diagnostics",
			globalFlags: []string{"--json", "--verbose", "--debug"},
			localFlags:  []string{"--session", "session-beta"},
			wantPath:    "/factory-sessions/session-beta/work",
			wantJSON:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var requests []requestObservation
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests = append(requests, requestObservation{method: r.Method, path: r.URL.Path, body: string(body)})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"traceId":"trace-submit","workId":"work-123","name":"request-name","workTypeName":"tasks"}`)
			}))
			defer server.Close()

			argv := append([]string{}, tc.globalFlags...)
			argv = append(argv, "--server", server.URL, "submit", "--name", "request-name", "--work-type-name", "tasks", "--payload", payloadPath)
			argv = append(argv, tc.localFlags...)
			legacy, generated := executeSubmitPair(t, argv, func(cfg submitcli.SubmitConfig) error {
				return submitcli.Submit(cfg)
			})
			assertSubmitParity(t, legacy, generated)
			if !legacy.called || !generated.called {
				t.Fatalf("submit handler calls legacy=%t generated=%t, want both true", legacy.called, generated.called)
			}
			if len(requests) != 2 {
				t.Fatalf("HTTP request count = %d, want 2", len(requests))
			}
			if !reflect.DeepEqual(requests[0], requests[1]) {
				t.Fatalf("HTTP request mismatch:\nlegacy=%+v\ngenerated=%+v", requests[0], requests[1])
			}
			if requests[0].method != http.MethodPost || requests[0].path != tc.wantPath {
				t.Fatalf("HTTP target = %s %s, want POST %s", requests[0].method, requests[0].path, tc.wantPath)
			}
			assertSubmitRequestBody(t, requests[0].body)
			assertSuccessOutput(t, legacy.stdout, tc.wantJSON, tc.wantPath)
		})
	}
}

func TestGeneratedVsLegacySubmitValidationAndFailureParity(t *testing.T) {
	tests := []struct {
		name        string
		argv        []string
		errContains string
		wantCalled  bool
	}{
		{name: "missing name", argv: []string{"submit", "--work-type-name", "tasks", "--payload", "request.md"}, errContains: "--name is required", wantCalled: true},
		{name: "missing work type", argv: []string{"submit", "--name", "request-name", "--payload", "request.md"}, errContains: "--work-type-name is required", wantCalled: true},
		{name: "missing payload", argv: []string{"submit", "--name", "request-name", "--work-type-name", "tasks"}, errContains: "--payload is required", wantCalled: true},
		{name: "removed work type ID", argv: []string{"submit", "--work-type-id", "tasks"}, errContains: "unknown flag: --work-type-id"},
		{name: "deprecated port", argv: []string{"submit", "--port", "9090"}, errContains: "--port is no longer supported"},
		{name: "invalid global boolean", argv: []string{"--json=maybe", "submit"}, errContains: "invalid argument", wantCalled: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			legacy, generated := executeSubmitPair(t, tc.argv, submitcli.Submit)
			assertSubmitParity(t, legacy, generated)
			if legacy.err == nil || !strings.Contains(legacy.err.Error(), tc.errContains) {
				t.Fatalf("legacy error = %v, want substring %q", legacy.err, tc.errContains)
			}
			if legacy.called != tc.wantCalled || generated.called != tc.wantCalled {
				t.Fatalf("handler calls legacy=%t generated=%t, want %t", legacy.called, generated.called, tc.wantCalled)
			}
		})
	}

	t.Run("service failure", func(t *testing.T) {
		serviceErr := errors.New("submit service unavailable")
		argv := []string{"--json", "submit", "--name", "request-name", "--work-type-name", "tasks", "--payload", "request.md", "--session", "session-failure"}
		legacy, generated := executeSubmitPair(t, argv, func(submitcli.SubmitConfig) error { return serviceErr })
		assertSubmitParity(t, legacy, generated)
		if !legacy.called || legacy.err == nil || !strings.Contains(legacy.err.Error(), serviceErr.Error()) {
			t.Fatalf("legacy failure observation = %+v", legacy)
		}
	})
}

type requestObservation struct {
	method string
	path   string
	body   string
}

type submitConfigObservation struct {
	Name, WorkTypeName, Payload, Server, SessionID string
	JSON, Verbose, Debug                           bool
	OutputSet, DiagnosticsSet                      bool
}

type submitExecutionObservation struct {
	config submitConfigObservation
	called bool
	stdout string
	stderr string
	err    error
}

func executeSubmitPair(
	t *testing.T,
	argv []string,
	submitHandler func(submitcli.SubmitConfig) error,
) (submitExecutionObservation, submitExecutionObservation) {
	t.Helper()
	var configs []submitConfigObservation
	options := cli.RootCommandOptions{SubmitWork: func(cfg submitcli.SubmitConfig) error {
		configs = append(configs, observeSubmitConfig(cfg))
		return submitHandler(cfg)
	}}
	legacyRoot, generatedRoot, err := cli.NewRunSubmitFamilyParityRoots(options)
	if err != nil {
		t.Fatalf("NewRunSubmitFamilyParityRoots() error = %v", err)
	}
	legacy := executeSubmitRoot(legacyRoot, argv)
	if len(configs) == 1 {
		legacy.called = true
		legacy.config = configs[0]
	}
	generated := executeSubmitRoot(generatedRoot, argv)
	if len(configs) == 2 {
		generated.called = true
		generated.config = configs[1]
	}
	return legacy, generated
}

func executeSubmitRoot(root *cobra.Command, argv []string) submitExecutionObservation {
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	return submitExecutionObservation{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func observeSubmitConfig(cfg submitcli.SubmitConfig) submitConfigObservation {
	return submitConfigObservation{
		Name: cfg.Name, WorkTypeName: cfg.WorkTypeName, Payload: cfg.Payload,
		Server: cfg.Server, SessionID: cfg.SessionID, JSON: cfg.JSON,
		Verbose: cfg.Verbose, Debug: cfg.Debug,
		OutputSet: cfg.Output != nil, DiagnosticsSet: cfg.Diagnostics != nil,
	}
}

func assertSubmitParity(t *testing.T, legacy, generated submitExecutionObservation) {
	t.Helper()
	if legacy.called != generated.called || !reflect.DeepEqual(legacy.config, generated.config) {
		t.Fatalf("submit call/config mismatch:\nlegacy=%+v\ngenerated=%+v", legacy, generated)
	}
	if legacy.stdout != generated.stdout {
		t.Fatalf("stdout mismatch:\nlegacy=%q\ngenerated=%q", legacy.stdout, generated.stdout)
	}
	if normalizeDiagnostics(legacy.stderr) != normalizeDiagnostics(generated.stderr) {
		t.Fatalf("stderr mismatch:\nlegacy=%q\ngenerated=%q", legacy.stderr, generated.stderr)
	}
	if errorText(legacy.err) != errorText(generated.err) {
		t.Fatalf("error mismatch:\nlegacy=%q\ngenerated=%q", legacy.err, generated.err)
	}
}

func assertSubmitRequestBody(t *testing.T, body string) {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if request["name"] != "request-name" || request["workTypeName"] != "tasks" || request["payload"] != "ship the release\n" {
		t.Fatalf("request body = %#v", request)
	}
}

func assertSuccessOutput(t *testing.T, output string, jsonOutput bool, endpointPath string) {
	t.Helper()
	if !jsonOutput {
		for _, want := range []string{"Submitted: request-name (tasks)", "traceId: trace-submit", "workId: work-123"} {
			if !strings.Contains(output, want) {
				t.Fatalf("human output %q missing %q", output, want)
			}
		}
		return
	}
	var result submitcli.SubmitSuccessResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode JSON output: %v", err)
	}
	if result.EndpointPath != endpointPath || result.SessionID != "session-beta" || result.TraceID != "trace-submit" {
		t.Fatalf("JSON output = %+v", result)
	}
}

func mustFindSubmit(t *testing.T, root *cobra.Command) *cobra.Command {
	t.Helper()
	cmd, err := climanifestparity.FindCommandByPath(root, "you submit")
	if err != nil {
		t.Fatal(err)
	}
	return cmd
}

func normalizeDiagnostics(value string) string {
	fields := strings.Fields(value)
	for index, field := range fields {
		if strings.HasPrefix(field, "durationMillis=") {
			fields[index] = "durationMillis=<elapsed>"
		}
	}
	return strings.Join(fields, " ")
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func assertNoMismatches(t *testing.T, mismatches []climanifestparity.Mismatch) {
	t.Helper()
	if len(mismatches) > 0 {
		t.Fatalf("generated vs legacy parity drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}
}
