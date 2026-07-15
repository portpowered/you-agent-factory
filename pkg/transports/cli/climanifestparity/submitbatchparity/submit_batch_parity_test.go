package submitbatchparity_test

import (
	"bytes"
	"encoding/json"
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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestGeneratedVsLegacyParityMatrix_SubmitBatch(t *testing.T) {
	legacyRoot, generatedRoot, err := cli.NewRunSubmitFamilyParityRoots(cli.RootCommandOptions{})
	if err != nil {
		t.Fatalf("NewRunSubmitFamilyParityRoots() error = %v", err)
	}
	legacyBatch := mustFindBatch(t, legacyRoot)
	generatedBatch := mustFindBatch(t, generatedRoot)
	assertNoMismatches(t, climanifestparity.CompareConstructorIdentityParity("you.submit.batch", legacyBatch, generatedBatch))

	if mismatches, compareErr := climanifestparity.CompareConstructorHelpParity(
		"you.submit.batch", legacyRoot, generatedRoot, "you submit batch",
	); compareErr != nil {
		t.Fatalf("CompareConstructorHelpParity() error = %v", compareErr)
	} else {
		assertNoMismatches(t, mismatches)
	}
	if mismatches, compareErr := climanifestparity.CompareConstructorCompletionInventoryParity(
		"you.submit.batch", "you submit batch", legacyRoot, generatedRoot,
	); compareErr != nil {
		t.Fatalf("CompareConstructorCompletionInventoryParity() error = %v", compareErr)
	} else {
		assertNoMismatches(t, mismatches)
	}
}

func TestGeneratedVsLegacySubmitBatchInputAndDryRunParity(t *testing.T) {
	flagPath := writeBatchFile(t, validBatchJSON("batch-flag", "flag-work"))
	positionalPath := writeBatchFile(t, validBatchJSON("batch-positional", "positional-work"))
	inline := validBatchJSON("batch-inline", "inline-work")

	tests := []struct {
		name       string
		argv       []string
		stdin      string
		stdinIsTTY bool
		want       string
		wantErr    string
	}{
		{name: "file flag wins over positional and stdin", argv: []string{"submit", "batch", "--dry-run", "--file", flagPath, positionalPath}, stdin: validBatchJSON("batch-stdin", "stdin-work"), want: "requestId: batch-flag"},
		{name: "positional file suppresses stdin", argv: []string{"submit", "batch", "--dry-run", positionalPath}, stdin: validBatchJSON("batch-stdin", "stdin-work"), want: "requestId: batch-positional"},
		{name: "inline JSON", argv: []string{"submit", "batch", "--dry-run", inline}, want: "batchSource: inline"},
		{name: "explicit stdin", argv: []string{"submit", "batch", "--dry-run", "-"}, stdin: validBatchJSON("batch-explicit-stdin", "stdin-work"), want: "requestId: batch-explicit-stdin"},
		{name: "implicit piped stdin", argv: []string{"submit", "batch", "--dry-run"}, stdin: validBatchJSON("batch-implicit-stdin", "stdin-work"), want: "requestId: batch-implicit-stdin"},
		{name: "double dash preserves inline positional", argv: []string{"submit", "batch", "--dry-run", "--", inline}, want: "requestId: batch-inline"},
		{name: "no input on TTY", argv: []string{"submit", "batch", "--dry-run"}, stdinIsTTY: true, wantErr: "batch input required"},
		{name: "malformed inline JSON", argv: []string{"submit", "batch", "--dry-run", "{bad"}, wantErr: "parse inline JSON"},
		{name: "conflicting positionals", argv: []string{"submit", "batch", "--dry-run", positionalPath, flagPath}, wantErr: "at most one positional"},
		{name: "deprecated port", argv: []string{"submit", "batch", "--port", "9090", "--dry-run", inline}, wantErr: "--port is no longer supported"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			legacy, generated := executeBatchPair(t, tc.argv, tc.stdin, tc.stdinIsTTY, submitcli.SubmitBatch)
			assertBatchParity(t, legacy, generated)
			if tc.wantErr != "" {
				if legacy.err == nil || !strings.Contains(legacy.err.Error(), tc.wantErr) {
					t.Fatalf("legacy error = %v, want substring %q", legacy.err, tc.wantErr)
				}
				return
			}
			if legacy.err != nil {
				t.Fatalf("legacy error = %v", legacy.err)
			}
			if !strings.Contains(legacy.stdout, tc.want) || !strings.Contains(legacy.stdout, "dry-run: no request sent") {
				t.Fatalf("legacy stdout = %q, want %q and dry-run summary", legacy.stdout, tc.want)
			}
		})
	}
}

func TestGeneratedVsLegacySubmitBatchHTTPAndOutputParity(t *testing.T) {
	path := writeBatchFile(t, validBatchJSON("batch-http", "alpha"))
	tests := []struct {
		name        string
		globalFlags []string
		localFlags  []string
		wantPath    string
		wantJSON    bool
	}{
		{name: "default session human output", wantPath: "/factory-sessions/~default/work-requests/batch-http"},
		{name: "explicit session JSON output with silent diagnostics", globalFlags: []string{"--json", "--verbose", "--debug"}, localFlags: []string{"--session", "session-beta"}, wantPath: "/factory-sessions/session-beta/work-requests/batch-http", wantJSON: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var requests []requestObservation
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests = append(requests, requestObservation{method: r.Method, path: r.URL.Path, body: string(body)})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
					RequestId: "batch-http", TraceId: "trace-batch",
					Works: []factoryapi.UpsertWorkRequestSubmittedWork{{Name: "alpha", WorkTypeName: "task", WorkId: "work-alpha"}},
				})
			}))
			defer server.Close()

			argv := append([]string{}, tc.globalFlags...)
			argv = append(argv, "--server", server.URL, "submit", "batch", path)
			argv = append(argv, tc.localFlags...)
			legacy, generated := executeBatchPair(t, argv, "", true, submitcli.SubmitBatch)
			assertBatchParity(t, legacy, generated)
			if legacy.err != nil {
				t.Fatalf("legacy error = %v", legacy.err)
			}
			if len(requests) != 2 || !reflect.DeepEqual(requests[0], requests[1]) {
				t.Fatalf("HTTP requests = %+v, want two identical requests", requests)
			}
			if requests[0].method != http.MethodPut || requests[0].path != tc.wantPath || !strings.Contains(requests[0].body, `"requestId": "batch-http"`) {
				t.Fatalf("HTTP request = %+v, want PUT %s with canonical body", requests[0], tc.wantPath)
			}
			assertSuccessOutput(t, legacy.stdout, tc.wantJSON, tc.wantPath)
			if len(tc.globalFlags) > 0 {
				if legacy.config.DiagnosticsSet || generated.config.DiagnosticsSet {
					t.Fatalf("diagnostic writers legacy=%t generated=%t, want pre-migration nil writers", legacy.config.DiagnosticsSet, generated.config.DiagnosticsSet)
				}
				if legacy.stderr != "" || generated.stderr != "" {
					t.Fatalf("stderr legacy=%q generated=%q, want pre-migration silence", legacy.stderr, generated.stderr)
				}
			}
		})
	}
}

func TestGeneratedVsLegacySubmitBatchFailuresOccurBeforeSideEffects(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"message":"batch service unavailable"}`)
	}))
	defer server.Close()

	malformed := []string{"--server", server.URL, "submit", "batch", `{bad`}
	legacy, generated := executeBatchPair(t, malformed, "", true, submitcli.SubmitBatch)
	assertBatchParity(t, legacy, generated)
	if requests != 0 || legacy.err == nil || !strings.Contains(legacy.err.Error(), "parse inline JSON") {
		t.Fatalf("validation result requests=%d error=%v, want no request and parse error", requests, legacy.err)
	}

	path := writeBatchFile(t, validBatchJSON("batch-unavailable", "alpha"))
	argv := []string{"--server", server.URL, "submit", "batch", path}
	legacy, generated = executeBatchPair(t, argv, "", true, submitcli.SubmitBatch)
	assertBatchParity(t, legacy, generated)
	if requests != 2 || legacy.err == nil || !strings.Contains(legacy.err.Error(), "batch service unavailable") {
		t.Fatalf("service result requests=%d error=%v, want two requests and API failure", requests, legacy.err)
	}
}

type requestObservation struct {
	method string
	path   string
	body   string
}

type batchConfigObservation struct {
	FileFlag, Server, SessionID         string
	Args                                []string
	DryRun, JSON, Verbose, Debug        bool
	StdinSet, OutputSet, DiagnosticsSet bool
}

type batchExecutionObservation struct {
	config batchConfigObservation
	called bool
	stdout string
	stderr string
	err    error
}

func executeBatchPair(
	t *testing.T,
	argv []string,
	stdin string,
	stdinIsTTY bool,
	batchHandler func(submitcli.BatchConfig) error,
) (batchExecutionObservation, batchExecutionObservation) {
	t.Helper()
	var configs []batchConfigObservation
	options := cli.RootCommandOptions{SubmitBatch: func(cfg submitcli.BatchConfig) error {
		configs = append(configs, observeBatchConfig(cfg))
		cfg.StdinIsTTY = func() bool { return stdinIsTTY }
		return batchHandler(cfg)
	}}
	legacyRoot, generatedRoot, err := cli.NewRunSubmitFamilyParityRoots(options)
	if err != nil {
		t.Fatalf("NewRunSubmitFamilyParityRoots() error = %v", err)
	}
	legacy := executeBatchRoot(legacyRoot, argv, stdin)
	if len(configs) == 1 {
		legacy.called, legacy.config = true, configs[0]
	}
	generated := executeBatchRoot(generatedRoot, argv, stdin)
	if len(configs) == 2 {
		generated.called, generated.config = true, configs[1]
	}
	return legacy, generated
}

func executeBatchRoot(root *cobra.Command, argv []string, stdin string) batchExecutionObservation {
	var stdout, stderr bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	return batchExecutionObservation{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func observeBatchConfig(cfg submitcli.BatchConfig) batchConfigObservation {
	return batchConfigObservation{
		FileFlag: cfg.FileFlag, Server: cfg.Server, SessionID: cfg.SessionID,
		Args: append([]string(nil), cfg.Args...), DryRun: cfg.DryRun, JSON: cfg.JSON,
		Verbose: cfg.Verbose, Debug: cfg.Debug, StdinSet: cfg.Stdin != nil,
		OutputSet: cfg.Output != nil, DiagnosticsSet: cfg.Diagnostics != nil,
	}
}

func assertBatchParity(t *testing.T, legacy, generated batchExecutionObservation) {
	t.Helper()
	if legacy.called != generated.called || !reflect.DeepEqual(legacy.config, generated.config) {
		t.Fatalf("batch call/config mismatch:\nlegacy=%+v\ngenerated=%+v", legacy, generated)
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

func assertSuccessOutput(t *testing.T, output string, jsonOutput bool, endpointPath string) {
	t.Helper()
	if !jsonOutput {
		for _, want := range []string{"requestId: batch-http", "traceId: trace-batch", "alpha (task) workId=work-alpha"} {
			if !strings.Contains(output, want) {
				t.Fatalf("human output %q missing %q", output, want)
			}
		}
		return
	}
	var result submitcli.BatchSubmitJSONResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode JSON output: %v", err)
	}
	if result.EndpointPath != endpointPath || result.SessionID != "session-beta" || result.TraceID != "trace-batch" || result.BatchSource != "file" {
		t.Fatalf("JSON output = %+v", result)
	}
}

func mustFindBatch(t *testing.T, root *cobra.Command) *cobra.Command {
	t.Helper()
	cmd, err := climanifestparity.FindCommandByPath(root, "you submit batch")
	if err != nil {
		t.Fatal(err)
	}
	return cmd
}

func writeBatchFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func validBatchJSON(requestID, workName string) string {
	return `{"requestId": "` + requestID + `", "type": "FACTORY_REQUEST_BATCH", "works": [{"name": "` + workName + `", "workTypeName": "task", "payload": {"title": "Task"}}]}`
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
