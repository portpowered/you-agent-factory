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

	acpsdk "github.com/coder/acp-go-sdk"
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
		{mode: "malformed", want: providers.ExecuteFailureKindDependency},
		{mode: "eof", want: providers.ExecuteFailureKindDependency},
		{mode: "fail", want: providers.ExecuteFailureKindUnknown, wantSessionID: "acp-session-service-1"},
		{mode: "peer-closed", want: providers.ExecuteFailureKindDependency, wantSessionID: "acp-session-service-1"},
		{mode: "server-failure", want: providers.ExecuteFailureKindDependency, wantSessionID: "acp-session-service-1"},
		{mode: "rate-limit", want: providers.ExecuteFailureKindThrottled, wantSessionID: "acp-session-service-1"},
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
				UserMessage:        "classify ACP failure",
				WorkingDirectory:   cwd,
				ProcessEnvironment: protocolHelperProcessEnvironment(test.mode),
			})
			var failure providers.ExecuteFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
			}
			if failure.Kind != test.want {
				t.Fatalf("ExecuteFailure.Kind = %q, want %q (message=%q)", failure.Kind, test.want, failure.Message)
			}
			if test.mode == "rate-limit" && (strings.Contains(failure.Message, "secret") || !strings.Contains(failure.Message, "usage or capacity limits")) {
				t.Fatalf("rate-limit failure message = %q, want fixed safe diagnostic", failure.Message)
			}
			if test.mode == "peer-closed" {
				wantMessage := `ACP provider "cursor-acp" disconnected before responding; retry the request`
				if failure.Message != wantMessage {
					t.Fatalf("peer-closed failure message = %q, want %q", failure.Message, wantMessage)
				}
				if failure.Diagnostics == nil || len(failure.Diagnostics.Progress) < 2 {
					t.Fatalf("peer-closed diagnostics = %#v, want started and failed progress", failure.Diagnostics)
				}
				progress := failure.Diagnostics.Progress
				if progress[0].Phase != "started" || progress[len(progress)-1].Phase != "failed" ||
					progress[len(progress)-1].Detail != wantMessage ||
					progress[len(progress)-1].Metadata["error_code"] != "ACP_PEER_CLOSED" {
					t.Fatalf("peer-closed diagnostics = %#v, want safe message and ACP_PEER_CLOSED", failure.Diagnostics)
				}
			}
			if test.mode == "fail" {
				if failure.Diagnostics == nil || len(failure.Diagnostics.Progress) == 0 {
					t.Fatalf("arbitrary -32603 diagnostics = %#v, want RPC failure", failure.Diagnostics)
				}
				progress := failure.Diagnostics.Progress
				if progress[len(progress)-1].Metadata["error_code"] == "ACP_PEER_CLOSED" ||
					failure.Message == `ACP provider "cursor-acp" disconnected before responding; retry the request` {
					t.Fatalf("arbitrary -32603 failure = %#v, want distinct RPC failure", failure)
				}
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

func TestACPPeerClosedFailureRequiresExactSDKShape(t *testing.T) {
	for _, test := range []struct {
		name string
		err  *acpsdk.RequestError
		want bool
	}{
		{name: "before response", err: &acpsdk.RequestError{Code: -32603, Message: "Internal error", Data: map[string]any{"error": "peer disconnected before response"}}, want: true},
		{name: "before notifications", err: &acpsdk.RequestError{Code: -32603, Message: "Internal error", Data: map[string]any{"error": "peer disconnected while waiting for pre-response notifications"}}, want: true},
		{name: "provider error", err: &acpsdk.RequestError{Code: -32603, Message: "Internal error"}},
		{name: "different message", err: &acpsdk.RequestError{Code: -32603, Message: "Provider error", Data: map[string]any{"error": "peer disconnected before response"}}},
		{name: "different code", err: &acpsdk.RequestError{Code: -32001, Message: "Internal error", Data: map[string]any{"error": "peer disconnected before response"}}},
		{name: "different data", err: &acpsdk.RequestError{Code: -32603, Message: "Internal error", Data: map[string]any{"error": "peer disconnected after response"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isACPPeerClosedFailure(test.err); got != test.want {
				t.Fatalf("isACPPeerClosedFailure(%#v) = %t, want %t", test.err, got, test.want)
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
		UserMessage:        "cancelled turn",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: protocolHelperProcessEnvironment("cancelled-turn"),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
	}
	if failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("ExecuteFailure.Kind = %q, want %q", failure.Kind, providers.ExecuteFailureKindCanceled)
	}
}

func TestDeniedPermissionWithEmptyPromptIsFailure(t *testing.T) {
	cwd := t.TempDir()
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
		AttemptID:          "attempt-denied-permission",
		UserMessage:        "read outside the workspace",
		WorkingDirectory:   cwd,
		ProcessEnvironment: protocolHelperProcessEnvironment("permission-denied-empty"),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
	}
	if failure.Kind != providers.ExecuteFailureKindUnknown || !strings.Contains(failure.Message, "permission request was denied") {
		t.Fatalf("ExecuteFailure = %#v, want denied-permission diagnostic", failure)
	}
	if failure.SessionRef == nil || failure.SessionRef.ID != "acp-session-service-1" {
		t.Fatalf("ExecuteFailure.SessionRef = %#v, want opened ACP session", failure.SessionRef)
	}
}

func TestExplicitUnadvertisedModelFailsBeforePrompt(t *testing.T) {
	cwd := t.TempDir()
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
		AttemptID:          "attempt-unadvertised-model",
		Model:              "muse-spark-1.3-contributor-free",
		UserMessage:        "use the requested model",
		WorkingDirectory:   cwd,
		ProcessEnvironment: protocolHelperProcessEnvironment("model-unadvertised"),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
	}
	if failure.Kind != providers.ExecuteFailureKindInvalidRequest || !strings.Contains(failure.Message, "muse-spark-1.3-contributor-free") {
		t.Fatalf("ExecuteFailure = %#v, want explicit unsupported-model diagnostic", failure)
	}
	if failure.SessionRef == nil || failure.SessionRef.ID != "acp-session-service-1" {
		t.Fatalf("ExecuteFailure.SessionRef = %#v, want opened ACP session", failure.SessionRef)
	}
}

func TestACPExecuteObservesProviderSessionWhileAttemptIsLive(t *testing.T) {
	var starts atomic.Int32
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}, protocolHelperCommandFactory(&starts), availableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

	observed := make(chan providers.SessionRef, 1)
	releaseObservation := make(chan struct{})
	workingDirectory := t.TempDir()
	type executeOutcome struct {
		result providers.ExecuteResult
		err    error
	}
	completed := make(chan executeOutcome, 1)
	go func() {
		result, executeErr := serviceValue.Execute(context.Background(), "cursor-acp", providers.ExecuteRequest{
			Provider: "cursor-acp", AttemptID: "attempt-live-observation", UserMessage: "observe the provider session",
			WorkingDirectory: workingDirectory, ProcessEnvironment: protocolHelperProcessEnvironment("normal"),
			SessionObserver: func(reference providers.SessionRef) {
				observed <- reference
				<-releaseObservation
			},
		})
		completed <- executeOutcome{result: result, err: executeErr}
	}()

	reference := <-observed
	want := providers.SessionRef{Provider: "cursor-acp", Kind: providers.SessionIDKind, ID: "acp-session-service-1"}
	if reference != want {
		t.Fatalf("live SessionObserver reference = %#v, want %#v", reference, want)
	}
	select {
	case outcome := <-completed:
		t.Fatalf("Execute() completed before live observation released: %#v", outcome)
	default:
	}
	close(releaseObservation)
	outcome := <-completed
	if outcome.err != nil {
		t.Fatalf("Execute() error = %v", outcome.err)
	}
	if outcome.result.SessionRef == nil || *outcome.result.SessionRef != want {
		t.Fatalf("Execute().SessionRef = %#v, want %#v", outcome.result.SessionRef, want)
	}
}

// TestContinuationResumesExactSessionThroughSessionLoad proves a request
// carried through the private continuation boundary reaches the ACP peer through session/load with the
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

	reference := providers.SessionRef{
		Provider: "cursor-acp",
		Kind:     providers.SessionIDKind,
		ID:       "resume-target-session",
	}
	result, err := serviceValue.Continue(context.Background(), "cursor-acp", providers.ExecuteRequest{
		Provider:           "cursor-acp",
		AttemptID:          "attempt-resume",
		UserMessage:        "continue the prior turn",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: protocolHelperProcessEnvironment("resume"),
	}, reference)
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

	reference := providers.SessionRef{
		Provider: "cursor-acp",
		Kind:     providers.SessionIDKind,
		ID:       "stale-session",
	}
	_, err = serviceValue.Continue(context.Background(), "cursor-acp", providers.ExecuteRequest{
		Provider:           "cursor-acp",
		AttemptID:          "attempt-resume-not-found",
		UserMessage:        "continue the prior turn",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: protocolHelperProcessEnvironment("resume-not-found"),
	}, reference)
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
		Provider:           "cursor-acp",
		AttemptID:          "attempt-stderr",
		UserMessage:        "redact stderr",
		WorkingDirectory:   t.TempDir(),
		EnvVars:            map[string]string{"ACP_TEST_API_TOKEN": "super-secret-token"},
		ProcessEnvironment: append(protocolHelperProcessEnvironment("stderr"), "ACP_TEST_API_TOKEN=super-secret-token"),
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

func protocolHelperProcessEnvironment(mode string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, protocolHelperEnvironment) {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, protocolHelperEnvironment+"="+mode)
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
	var pendingPromptID json.RawMessage
	for scanner.Scan() {
		if mode == "permission-denied-empty" && strings.Contains(scanner.Text(), `"outcome"`) {
			if !strings.Contains(scanner.Text(), `"optionId":"deny"`) {
				return fmt.Errorf("permission response did not select reject option: %s", scanner.Text())
			}
			if err := writeRPCResult(writer, pendingPromptID, `{"stopReason":"end_turn"}`); err != nil {
				return err
			}
			continue
		}
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
			if mode == "peer-closed" {
				return nil
			}
			if mode == "permission-denied-empty" {
				pendingPromptID = append(json.RawMessage(nil), request.ID...)
				_, err := fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":"permission-1","method":"session/request_permission","params":{"sessionId":"acp-session-service-1","toolCall":{"toolCallId":"tool-1","title":"Read outside workspace"},"options":[{"optionId":"deny","kind":"reject_once","name":"Deny"}]}}`)
				if err != nil {
					return err
				}
				if err := writer.Flush(); err != nil {
					return err
				}
				continue
			}
			if mode == "fail" {
				return writeRPCError(writer, request.ID, -32603, "Internal error")
			}
			if mode == "server-failure" {
				return writeRPCError(writer, request.ID, -32001, "temporarily unavailable")
			}
			if mode == "rate-limit" {
				return writeRPCError(writer, request.ID, -32603, "Internal error: Rate limit exceeded; secret=private")
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
