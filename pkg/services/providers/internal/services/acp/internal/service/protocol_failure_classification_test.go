package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const protocolHelperEnvironment = "YOU_TEST_ACP_PROTOCOL_HELPER"

// TestACPProtocolFailureHelperProcess is the OS-process peer for direct ACP
// service classification tests. It is not a Factory/process-boundary cell.
func TestACPProtocolFailureHelperProcess(t *testing.T) {
	mode := os.Getenv(protocolHelperEnvironment)
	if mode == "" {
		return
	}
	if err := runProtocolFailurePeer(mode, os.Stdin, os.Stdout, os.Stderr); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	os.Exit(0)
}

func TestProtocolFailuresMapToStableExecuteFailureKinds(t *testing.T) {
	for _, test := range []struct {
		mode string
		want providers.ExecuteFailureKind
	}{
		{mode: "version", want: providers.ExecuteFailureKindMisconfigured},
		{mode: "init-fail", want: providers.ExecuteFailureKindUnknown},
		{mode: "malformed", want: providers.ExecuteFailureKindUnknown},
		{mode: "eof", want: providers.ExecuteFailureKindUnknown},
		{mode: "fail", want: providers.ExecuteFailureKindUnknown},
	} {
		t.Run(test.mode, func(t *testing.T) {
			var starts atomic.Int32
			serviceValue, err := New([]providers.ACPIntegration{{
				ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
			}}, protocolHelperCommandFactory(&starts), availableLocator{})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

			cwd := t.TempDir()
			_, err = serviceValue.Execute(context.Background(), "cursor-acp", providers.ExecuteRequest{
				Provider:           "cursor-acp",
				AttemptID:          "attempt-" + test.mode,
				Model:              "test-model",
				UserMessage:        "classify ACP failure",
				WorkingDirectory:   cwd,
				ProcessEnvironment: append(os.Environ(), protocolHelperEnvironment+"="+test.mode),
			})
			var failure providers.ExecuteFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
			}
			if failure.Kind != test.want {
				t.Fatalf("ExecuteFailure.Kind = %q, want %q (message=%q)", failure.Kind, test.want, failure.Message)
			}
			if starts.Load() == 0 {
				t.Fatal("ACP protocol failure did not start the Agent process")
			}
		})
	}
}

func TestMissingExecutableFailsBeforeStartWithWorkFailureType(t *testing.T) {
	var starts atomic.Int32
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}, protocolHelperCommandFactory(&starts), missingLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

	_, err = serviceValue.Execute(context.Background(), "cursor-acp", providers.ExecuteRequest{
		Provider:         "cursor-acp",
		AttemptID:        "attempt-missing",
		UserMessage:      "missing executable",
		WorkingDirectory: t.TempDir(),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
	}
	if failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf("ExecuteFailure.Kind = %q, want %q", failure.Kind, providers.ExecuteFailureKindDependency)
	}
	if failure.Diagnostics == nil || failure.Diagnostics.Metadata["work-failure-type"] != "missing_executable" {
		t.Fatalf("diagnostics = %#v, want work-failure-type=missing_executable", failure.Diagnostics)
	}
	if starts.Load() != 0 {
		t.Fatalf("ACP starts = %d, want 0 for unavailable executable", starts.Load())
	}
}

func TestInitializeFailureRedactsConfiguredSecretsFromStderr(t *testing.T) {
	var starts atomic.Int32
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}, protocolHelperCommandFactory(&starts), availableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

	_, err = serviceValue.Execute(context.Background(), "cursor-acp", providers.ExecuteRequest{
		Provider:         "cursor-acp",
		AttemptID:        "attempt-stderr",
		UserMessage:      "redact stderr",
		WorkingDirectory: t.TempDir(),
		EnvVars:          map[string]string{"ACP_TEST_API_TOKEN": "super-secret-token"},
		ProcessEnvironment: append(os.Environ(),
			protocolHelperEnvironment+"=stderr",
			"ACP_TEST_API_TOKEN=super-secret-token",
		),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
	}
	if strings.Contains(failure.Message, "super-secret-token") {
		t.Fatalf("ExecuteFailure leaked configured secret: %q", failure.Message)
	}
	if !strings.Contains(failure.Message, "agent diagnostic token=<redacted>") {
		t.Fatalf("ExecuteFailure omitted redacted stderr diagnostic: %q", failure.Message)
	}
	if starts.Load() == 0 {
		t.Fatal("stderr redaction case did not start the Agent process")
	}
}

func TestSafeACPStderrRedactsSensitiveEnvironmentValues(t *testing.T) {
	got := safeACPStderr(
		"agent diagnostic token=super-secret-token path=/tmp/work",
		map[string]string{"ACP_TEST_API_TOKEN": "super-secret-token"},
	)
	if strings.Contains(got, "super-secret-token") {
		t.Fatalf("safeACPStderr leaked secret: %q", got)
	}
	if want := "agent diagnostic token=<redacted> path=/tmp/work"; got != want {
		t.Fatalf("safeACPStderr() = %q, want %q", got, want)
	}
	if got := safeACPStderr("plain", map[string]string{"PATH": "/usr/bin"}); got != "plain" {
		t.Fatalf("safeACPStderr(non-sensitive) = %q, want plain", got)
	}
}

type availableLocator struct{}

func (availableLocator) LookPath(file string) (string, error) { return file, nil }

type missingLocator struct{}

func (missingLocator) LookPath(string) (string, error) {
	return "", errors.New("executable not found")
}

func protocolHelperCommandFactory(starts *atomic.Int32) func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		if name == "cursor-agent" && len(args) == 1 && args[0] == "acp" {
			starts.Add(1)
			return exec.Command(os.Args[0], "-test.run=^TestACPProtocolFailureHelperProcess$")
		}
		return exec.Command(name, args...)
	}
}

func runProtocolFailurePeer(mode string, stdin io.Reader, stdout, stderr io.Writer) error {
	if mode == "malformed" {
		_, err := fmt.Fprintln(stdout, "{not-json")
		return err
	}
	if mode == "eof" {
		return nil
	}
	if mode == "stderr" {
		_, _ = fmt.Fprintln(stderr, "agent diagnostic token="+os.Getenv("ACP_TEST_API_TOKEN"))
	}
	scanner := bufio.NewScanner(stdin)
	writer := bufio.NewWriter(stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode client RPC: %w", err)
		}
		switch request.Method {
		case "initialize":
			if mode == "init-fail" || mode == "stderr" {
				return writeRPCError(writer, request.ID, -32603, "Internal error")
			}
			version := 1
			if mode == "version" {
				version = 999
			}
			if err := writeRPCResult(writer, request.ID, fmt.Sprintf(`{"protocolVersion":%d,"agentCapabilities":{},"authMethods":[]}`, version)); err != nil {
				return err
			}
			if mode == "version" {
				return nil
			}
		case "session/new":
			if err := writeRPCResult(writer, request.ID, `{"sessionId":"acp-session-service-1","configOptions":[]}`); err != nil {
				return err
			}
		case "session/prompt":
			if mode == "fail" {
				return writeRPCError(writer, request.ID, -32603, "Internal error")
			}
			return writeRPCResult(writer, request.ID, `{"stopReason":"end_turn"}`)
		case "$/cancel_request", "session/cancel":
			return nil
		default:
			return fmt.Errorf("unexpected client RPC method %q", request.Method)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read client RPC: %w", err)
	}
	return nil
}

func writeRPCResult(writer *bufio.Writer, id json.RawMessage, result string) error {
	if _, err := fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%s,"result":%s}`+"\n", id, result); err != nil {
		return err
	}
	return writer.Flush()
}

func writeRPCError(writer *bufio.Writer, id json.RawMessage, code int, message string) error {
	if _, err := fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%s,"error":{"code":%d,"message":%q}}`+"\n", id, code, message); err != nil {
		return err
	}
	return writer.Flush()
}
