package root

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

func TestProcessUsageFailuresPreserveCobraDetailsAndHelpHints(t *testing.T) {
	t.Parallel()

	process, buildErr := BuildProcess(context.Background(), serviceedges.Edges{})
	if buildErr != nil {
		t.Fatalf("BuildProcess() error = %v", buildErr)
	}
	tests := []struct {
		name         string
		args         []string
		wantError    string
		wantHelpPath string
	}{
		{
			name:         "unknown top-level flag",
			args:         []string{"you", "--definitely-unknown"},
			wantError:    "unknown flag: --definitely-unknown",
			wantHelpPath: "you --help",
		},
		{
			name:         "unknown subcommand",
			args:         []string{"you", "definitely-not-a-command"},
			wantError:    `unknown command "definitely-not-a-command" for "you"`,
			wantHelpPath: "you --help",
		},
		{
			name:         "bad subcommand flag",
			args:         []string{"you", "session", "show", "--definitely-unknown"},
			wantError:    "unknown flag: --definitely-unknown",
			wantHelpPath: "you session show --help",
		},
		{
			name:         "missing required argument",
			args:         []string{"you", "work", "show"},
			wantError:    "requires at least 1 arg(s), only received 0",
			wantHelpPath: "you work show --help",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			var stdout, stderr bytes.Buffer
			err := process.Execute(Input{
				Args:             test.args,
				Env:              homeEnvironment(home),
				Stdin:            strings.NewReader(""),
				Stdout:           &stdout,
				Stderr:           &stderr,
				Context:          context.Background(),
				WorkingDirectory: home,
			})
			if err == nil {
				t.Fatal("Process.Execute() error = nil, want usage failure")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Process.Execute() error = %q, want %q", err, test.wantError)
			}
			if !strings.Contains(stderr.String(), "Error: "+test.wantError) {
				t.Fatalf("stderr = %q, want Cobra error %q", stderr.String(), test.wantError)
			}
			if !strings.Contains(stderr.String(), "Run '"+test.wantHelpPath+"' for usage.") {
				t.Fatalf("stderr = %q, want help hint for %q", stderr.String(), test.wantHelpPath)
			}
			if strings.Contains(stderr.String(), "CLI_COMMAND_FAILED") ||
				strings.Contains(stderr.String(), "INTERNAL_SERVER_ERROR") {
				t.Fatalf("stderr mislabeled usage failure: %q", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty usage failure output", stdout.String())
			}
		})
	}
}
func TestProcessLocalRunFailuresPreserveSubmittedInputs(t *testing.T) {
	t.Parallel()

	process, buildErr := BuildProcess(context.Background(), serviceedges.Edges{})
	if buildErr != nil {
		t.Fatalf("BuildProcess() error = %v", buildErr)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })

	tests := []struct {
		name        string
		args        []string
		wantCode    string
		wantMessage []string
	}{
		{
			name:        "missing replay path",
			args:        []string{"you", "run", "--replay", "./does-not-exist.json"},
			wantCode:    clidiag.LocalInputFailureCode,
			wantMessage: []string{"--replay", "./does-not-exist.json"},
		},
		{
			name:        "missing resume path",
			args:        []string{"you", "run", "--resume", "./does-not-exist.recording.json"},
			wantCode:    clidiag.LocalInputFailureCode,
			wantMessage: []string{"--resume", "./does-not-exist.recording.json"},
		},
		{
			name:        "resume replay conflict",
			args:        []string{"you", "run", "--resume", "X", "--replay", "X"},
			wantCode:    clidiag.FlagConflictFailureCode,
			wantMessage: []string{"--resume", "--replay"},
		},
		{
			name:        "resume no-record conflict",
			args:        []string{"you", "run", "--resume", "X", "--no-record"},
			wantCode:    clidiag.FlagConflictFailureCode,
			wantMessage: []string{"--resume", "--no-record"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			var stdout, stderr bytes.Buffer
			err := process.Execute(Input{
				Args:             test.args,
				Env:              homeEnvironment(home),
				Stdout:           &stdout,
				Stderr:           &stderr,
				Context:          context.Background(),
				WorkingDirectory: home,
			})
			if err == nil {
				t.Fatal("Process.Execute() error = nil, want local pre-flight failure")
			}
			var response struct {
				Code    string `json:"code"`
				Family  string `json:"family"`
				Message string `json:"message"`
			}
			if decodeErr := json.Unmarshal(stderr.Bytes(), &response); decodeErr != nil {
				t.Fatalf("stderr is not one local ErrorResponse: %v\n%s", decodeErr, stderr.String())
			}
			if response.Code != test.wantCode || response.Family != "BAD_REQUEST" {
				t.Fatalf("local response = %#v, want code %q and BAD_REQUEST family", response, test.wantCode)
			}
			for _, marker := range test.wantMessage {
				if !strings.Contains(response.Message, marker) {
					t.Fatalf("local response message = %q, want marker %q", response.Message, marker)
				}
			}
			if strings.Contains(stderr.String(), "CLI_COMMAND_FAILED") ||
				strings.Contains(stderr.String(), "INTERNAL_SERVER_ERROR") ||
				strings.Contains(stderr.String(), "no such file") {
				t.Fatalf("local failure lost context or leaked fallback/cause: %q", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty local failure output", stdout.String())
			}
		})
	}
}

func TestProcessFailureDisclosurePolicySeparatesDefaultVerboseAndDebug(t *testing.T) {
	t.Parallel()

	t.Run("local failure", func(t *testing.T) {
		testProcessLocalFailureDisclosure(t)
	})
	t.Run("HTTP failure", func(t *testing.T) {
		testProcessHTTPFailureDisclosure(t)
	})
}

type cliDisclosureMode struct {
	name  string
	flags []string
}

func cliDisclosureModes() []cliDisclosureMode {
	return []cliDisclosureMode{
		{name: "default"},
		{name: "verbose", flags: []string{"--verbose"}},
		{name: "debug", flags: []string{"--debug"}},
	}
}

func executeDisclosureFailure(t *testing.T, process *initializerapplication.Process, args []string) string {
	t.Helper()

	var stdout, stderr bytes.Buffer
	home := t.TempDir()
	if err := process.Execute(Input{
		Args:             args,
		Env:              homeEnvironment(home),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: home,
	}); err == nil {
		t.Fatalf("Process.Execute(%v) error = nil, want command failure", args)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty failure output", stdout.String())
	}
	return stderr.String()
}

func testProcessLocalFailureDisclosure(t *testing.T) {
	t.Helper()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })

	missingPath := "./debug-missing-replay.json"
	outputs := make(map[string]string)
	for _, mode := range cliDisclosureModes() {
		args := append([]string{"you"}, mode.flags...)
		args = append(args, "run", "--replay", missingPath)
		outputs[mode.name] = executeDisclosureFailure(t, process, args)
		if !strings.Contains(outputs[mode.name], clidiag.LocalInputFailureCode) ||
			!strings.Contains(outputs[mode.name], missingPath) {
			t.Fatalf("%s stderr = %q, want local path diagnostic", mode.name, outputs[mode.name])
		}
		if mode.name != "debug" && strings.Contains(outputs[mode.name], "no such file") {
			t.Fatalf("%s stderr leaked filesystem cause: %q", mode.name, outputs[mode.name])
		}
	}
	if strings.Contains(outputs["default"], "debug:") || strings.Contains(outputs["verbose"], "debug:") {
		t.Fatalf("non-debug output unexpectedly included debug diagnostics: default=%q verbose=%q", outputs["default"], outputs["verbose"])
	}
	if !strings.Contains(outputs["debug"], "debug: cause[0]=") {
		t.Fatalf("debug local output = %q, want ordered cause detail", outputs["debug"])
	}
}

func testProcessHTTPFailureDisclosure(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/factory-sessions/missing" {
			t.Fatalf("request = %s %s, want GET /factory-sessions/missing", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":"NOT_FOUND","family":"NOT_FOUND","message":"factory session not found"}`)
	}))
	defer server.Close()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })

	outputs := make(map[string]string)
	for _, mode := range cliDisclosureModes() {
		args := append([]string{"you"}, mode.flags...)
		args = append(args, "--server", server.URL, "--json", "session", "show", "missing")
		outputs[mode.name] = executeDisclosureFailure(t, process, args)
		if !strings.Contains(outputs[mode.name], `"family":"NOT_FOUND"`) {
			t.Fatalf("%s stderr = %q, want structured server family", mode.name, outputs[mode.name])
		}
	}
	for _, name := range []string{"default", "verbose"} {
		if strings.Contains(outputs[name], "debug:") || strings.Contains(outputs[name], "method=GET") {
			t.Fatalf("%s HTTP output unexpectedly included debug metadata: %q", name, outputs[name])
		}
	}
	debugOutput := outputs["debug"]
	for _, want := range []string{
		"debug: cause[0]=",
		"debug: http method=GET",
		"url=" + server.URL + "/factory-sessions/missing",
		"status=404",
	} {
		if !strings.Contains(debugOutput, want) {
			t.Fatalf("debug HTTP output = %q, want %q", debugOutput, want)
		}
	}
	if strings.Contains(debugOutput, "?") || strings.Contains(debugOutput, "Authorization") {
		t.Fatalf("debug HTTP output exposed unsanitized request detail: %q", debugOutput)
	}
}

func TestProcessRemoteSessionShowPreservesStructuredNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/missing" {
			t.Fatalf("request path = %q, want /factory-sessions/missing", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":"NOT_FOUND","family":"NOT_FOUND","message":"factory session not found"}`)
	}))
	defer server.Close()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	err = process.Execute(Input{
		Args:             []string{"you", "--server", server.URL, "--json", "session", "show", "missing"},
		Env:              homeEnvironment(t.TempDir()),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	})
	if err == nil {
		t.Fatal("Process.Execute(session show) error = nil, want structured 404 failure")
	}
	var response struct {
		Code    string `json:"code"`
		Family  string `json:"family"`
		Message string `json:"message"`
	}
	if decodeErr := json.Unmarshal(stderr.Bytes(), &response); decodeErr != nil {
		t.Fatalf("stderr is not one JSON error response: %v\n%s", decodeErr, stderr.String())
	}
	if response.Code != "NOT_FOUND" || response.Family != "NOT_FOUND" || response.Message != "factory session not found" {
		t.Fatalf("structured response = %#v, want server-provided 404 details", response)
	}
	if strings.Contains(stderr.String(), "CLI_COMMAND_FAILED") || strings.Contains(stderr.String(), "INTERNAL_SERVER_ERROR") {
		t.Fatalf("stderr collapsed structured 404: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty failure output", stdout.String())
	}
}

func TestProcessRemoteWorkShowPreservesStructuredNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/work/missing" {
			t.Fatalf("request path = %q, want /factory-sessions/~default/work/missing", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":"NOT_FOUND","family":"NOT_FOUND","message":"work not found"}`)
	}))
	defer server.Close()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	err = process.Execute(Input{
		Args:             []string{"you", "--server", server.URL, "--json", "work", "show", "missing"},
		Env:              homeEnvironment(t.TempDir()),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	})
	if err == nil {
		t.Fatal("Process.Execute(work show) error = nil, want structured 404 failure")
	}
	var response struct {
		Code    string `json:"code"`
		Family  string `json:"family"`
		Message string `json:"message"`
	}
	if decodeErr := json.Unmarshal(stderr.Bytes(), &response); decodeErr != nil {
		t.Fatalf("stderr is not one JSON error response: %v\n%s", decodeErr, stderr.String())
	}
	if response.Code != "NOT_FOUND" || response.Family != "NOT_FOUND" || response.Message != "work not found" {
		t.Fatalf("structured response = %#v, want server-provided 404 details", response)
	}
	if strings.Contains(stderr.String(), "CLI_COMMAND_FAILED") || strings.Contains(stderr.String(), "INTERNAL_SERVER_ERROR") {
		t.Fatalf("stderr collapsed structured 404: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty failure output", stdout.String())
	}
}
