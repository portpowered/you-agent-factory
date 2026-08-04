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
		mode          string
		want          providers.ExecuteFailureKind
		wantSessionID string
	}{
		{mode: "version", want: providers.ExecuteFailureKindMisconfigured},
		{mode: "init-fail", want: providers.ExecuteFailureKindUnknown},
		{mode: "malformed", want: providers.ExecuteFailureKindUnknown},
		{mode: "eof", want: providers.ExecuteFailureKindUnknown},
		{mode: "fail", want: providers.ExecuteFailureKindUnknown, wantSessionID: "acp-session-service-1"},
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
			if test.wantSessionID != "" {
				if failure.SessionRef == nil || failure.SessionRef.Provider != providers.ID("cursor-acp") || failure.SessionRef.Kind != providers.SessionIDKind || failure.SessionRef.ID != test.wantSessionID {
					t.Fatalf("ExecuteFailure.SessionRef = %#v, want cursor-acp/%s/%s", failure.SessionRef, providers.SessionIDKind, test.wantSessionID)
				}
			}
			if starts.Load() == 0 {
				t.Fatal("ACP protocol failure did not start the Agent process")
			}
		})
	}
}

// TestPromptCancelledStopReasonMapsToExecuteFailureKindCanceled proves the
// established ACP cancellation normalization independent of any real
// concurrent control call: whenever session/prompt returns
// StopReasonCancelled (the ACP protocol's outcome for a session/cancel
// notification the agent honored, whether triggered through
// ControlAttempt or otherwise), daemon.execute reports the same
// ExecuteFailureKindCanceled every other cancellation path in this service
// already normalizes to.
func TestPromptCancelledStopReasonMapsToExecuteFailureKindCanceled(t *testing.T) {
	var starts atomic.Int32
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}, protocolHelperCommandFactory(&starts), availableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

	_, err = serviceValue.Execute(context.Background(), "cursor-acp", providers.ExecuteRequest{
		Provider:           "cursor-acp",
		AttemptID:          "attempt-cancelled-turn",
		Model:              "test-model",
		UserMessage:        "cancelled turn",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: append(os.Environ(), protocolHelperEnvironment+"=cancelled-turn"),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
	}
	if failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("ExecuteFailure.Kind = %q, want %q", failure.Kind, providers.ExecuteFailureKindCanceled)
	}
}

// TestContinuationResumesExactSessionThroughSessionLoad proves a request
// carrying ResumeSession reaches the ACP peer through session/load with the
// exact opaque session id forwarded unchanged, and the returned result
// preserves that exact id - never a new one session/new would have minted.
// The helper peer in "resume" mode does not implement session/new at all, so
// any regression back to unconditionally starting a fresh session fails this
// test instead of silently substituting a different Provider Session.
func TestContinuationResumesExactSessionThroughSessionLoad(t *testing.T) {
	var starts atomic.Int32
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}, protocolHelperCommandFactory(&starts), availableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

	result, err := serviceValue.Execute(context.Background(), "cursor-acp", providers.ExecuteRequest{
		Provider:           "cursor-acp",
		AttemptID:          "attempt-resume",
		UserMessage:        "continue the prior turn",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: append(os.Environ(), protocolHelperEnvironment+"=resume"),
		ResumeSession: &providers.SessionRef{
			Provider: "cursor-acp",
			Kind:     providers.SessionIDKind,
			ID:       "resume-target-session",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil - the peer only implements session/load in resume mode", err)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "resume-target-session" {
		t.Fatalf("SessionRef = %#v, want the exact resumed session id unchanged", result.SessionRef)
	}
}

// TestContinuationSessionLoadFailureDoesNotFallBackToFreshSession proves a
// session/load ResourceNotFound failure (a stale or unknown session) is
// classified as ExecuteFailureKindSessionNotFound - the typed vocabulary
// Continue translates into the closed stale continuation failure - instead
// of the generic RPC failure kind, and is reported as-is instead of silently
// starting a fresh session. The helper peer in "resume-not-found" mode also
// does not implement session/new, so a regression to a fresh-session
// fallback would fail this test rather than masking the failure with an
// unrelated new session.
func TestContinuationSessionLoadFailureDoesNotFallBackToFreshSession(t *testing.T) {
	var starts atomic.Int32
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}, protocolHelperCommandFactory(&starts), availableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

	_, err = serviceValue.Execute(context.Background(), "cursor-acp", providers.ExecuteRequest{
		Provider:           "cursor-acp",
		AttemptID:          "attempt-resume-not-found",
		UserMessage:        "continue the prior turn",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: append(os.Environ(), protocolHelperEnvironment+"=resume-not-found"),
		ResumeSession: &providers.SessionRef{
			Provider: "cursor-acp",
			Kind:     providers.SessionIDKind,
			ID:       "stale-session",
		},
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure - a session/load failure must be reported, not silently retried as a fresh session", err, err)
	}
	if failure.Kind != providers.ExecuteFailureKindSessionNotFound {
		t.Fatalf("ExecuteFailure.Kind = %q, want %q", failure.Kind, providers.ExecuteFailureKindSessionNotFound)
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP daemon starts = %d, want exactly 1 - a stale reference must not start a second daemon or fresh-session attempt", starts.Load())
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
			if mode == "resume" || mode == "resume-not-found" {
				return fmt.Errorf("unexpected session/new during a continuation - the continued attempt must resume through session/load instead of starting a fresh session")
			}
			if err := writeRPCResult(writer, request.ID, `{"sessionId":"acp-session-service-1","configOptions":[]}`); err != nil {
				return err
			}
		case "session/load":
			if mode == "resume-not-found" {
				// -32002 is the ACP schema's ErrorCodeResourceNotFound - the
				// real code a conformant agent returns for an unrecognized
				// session/load id.
				return writeRPCError(writer, request.ID, -32002, "no rollout found for that session")
			}
			if err := writeRPCResult(writer, request.ID, `{"configOptions":[]}`); err != nil {
				return err
			}
		case "session/prompt":
			if mode == "fail" {
				return writeRPCError(writer, request.ID, -32603, "Internal error")
			}
			if mode == "cancelled-turn" {
				return writeRPCResult(writer, request.ID, `{"stopReason":"cancelled"}`)
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
