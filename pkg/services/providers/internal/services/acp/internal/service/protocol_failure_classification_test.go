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
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
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
			serviceValue := newProtocolFailureTestService(t, &starts)
			failure := executeProtocolFailureCase(t, serviceValue, test.mode)
			if failure.Kind != test.want {
				t.Fatalf("ExecuteFailure.Kind = %q, want %q (message=%q)", failure.Kind, test.want, failure.Message)
			}
			switch test.mode {
			case "rate-limit":
				assertSafeRateLimitDiagnostic(t, failure)
			case "peer-closed":
				assertPeerClosedFailure(t, failure)
			case "fail":
				assertDistinctArbitraryRPCFailure(t, failure)
			}
			if test.wantSessionID != "" {
				assertFailureSessionRef(t, failure, test.wantSessionID)
			}
			if starts.Load() == 0 {
				t.Fatal("ACP protocol failure did not start the Agent process")
			}
		})
	}
}

func newProtocolFailureTestService(t *testing.T, starts *atomic.Int32) acp.ContinuationService {
	t.Helper()
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}, protocolHelperCommandFactory(starts), availableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })
	return serviceValue
}

func executeProtocolFailureCase(t *testing.T, serviceValue acp.ContinuationService, mode string) providers.ExecuteFailure {
	t.Helper()
	_, err := serviceValue.Execute(context.Background(), "cursor-acp", providers.ExecuteRequest{
		Provider:           "cursor-acp",
		AttemptID:          "attempt-" + mode,
		UserMessage:        "classify ACP failure",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: protocolHelperProcessEnvironment(mode),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
	}
	return failure
}

func assertSafeRateLimitDiagnostic(t *testing.T, failure providers.ExecuteFailure) {
	t.Helper()
	if strings.Contains(failure.Message, "secret") || !strings.Contains(failure.Message, "usage or capacity limits") {
		t.Fatalf("rate-limit failure message = %q, want fixed safe diagnostic", failure.Message)
	}
}

func assertPeerClosedFailure(t *testing.T, failure providers.ExecuteFailure) {
	t.Helper()
	wantMessage := `ACP provider "cursor-acp" disconnected before responding; retry the request`
	if failure.Message != wantMessage {
		t.Fatalf("peer-closed failure message = %q, want %q", failure.Message, wantMessage)
	}
	if failure.Diagnostics == nil || len(failure.Diagnostics.Progress) < 2 {
		t.Fatalf("peer-closed diagnostics = %#v, want started and failed progress", failure.Diagnostics)
	}
	progress := failure.Diagnostics.Progress
	last := progress[len(progress)-1]
	if progress[0].Phase != "started" || last.Phase != "failed" ||
		last.Detail != wantMessage ||
		last.Metadata["error_code"] != "ACP_PEER_CLOSED" {
		t.Fatalf("peer-closed diagnostics = %#v, want safe message and ACP_PEER_CLOSED", failure.Diagnostics)
	}
}

func assertDistinctArbitraryRPCFailure(t *testing.T, failure providers.ExecuteFailure) {
	t.Helper()
	if failure.Diagnostics == nil || len(failure.Diagnostics.Progress) == 0 {
		t.Fatalf("arbitrary -32603 diagnostics = %#v, want RPC failure", failure.Diagnostics)
	}
	last := failure.Diagnostics.Progress[len(failure.Diagnostics.Progress)-1]
	if last.Metadata["error_code"] == "ACP_PEER_CLOSED" ||
		failure.Message == `ACP provider "cursor-acp" disconnected before responding; retry the request` {
		t.Fatalf("arbitrary -32603 failure = %#v, want distinct RPC failure", failure)
	}
}

func assertFailureSessionRef(t *testing.T, failure providers.ExecuteFailure, wantSessionID string) {
	t.Helper()
	if failure.SessionRef == nil || failure.SessionRef.Provider != providers.ID("cursor-acp") ||
		failure.SessionRef.Kind != providers.SessionIDKind || failure.SessionRef.ID != wantSessionID {
		t.Fatalf("ExecuteFailure.SessionRef = %#v, want cursor-acp/%s/%s", failure.SessionRef, providers.SessionIDKind, wantSessionID)
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

func TestPeerDisconnectRetainsRedactedStderrAndCancellationPrecedence(t *testing.T) {
	const secret = "super-secret-token"
	request := providers.ExecuteRequest{EnvVars: map[string]string{"ACP_TEST_API_TOKEN": secret}}
	for _, marker := range []string{"peer disconnected before response", "peer disconnected while waiting for pre-response notifications"} {
		t.Run(marker, func(t *testing.T) {
			err := &acpsdk.RequestError{Code: -32603, Message: "Internal error", Data: map[string]any{"error": marker}}
			got := rpcFailure(context.Background(), "initialize", "cursor-acp", err, "agent diagnostic token="+secret, request)
			var failure providers.ExecuteFailure
			if !errors.As(got, &failure) || failure.Kind != providers.ExecuteFailureKindDependency {
				t.Fatalf("failure = %#v, want dependency classification", got)
			}
			want := `ACP provider "cursor-acp" disconnected before responding; retry the request (stderr: agent diagnostic token=<redacted>)`
			if failure.Message != want || strings.Contains(failure.Message, secret) {
				t.Fatalf("failure message = %q, want %q", failure.Message, want)
			}
			if failure.Diagnostics == nil || len(failure.Diagnostics.Progress) != 1 ||
				failure.Diagnostics.Progress[0].Detail != want || failure.Diagnostics.Progress[0].Metadata["error_code"] != "ACP_PEER_CLOSED" {
				t.Fatalf("diagnostics = %#v, want same redacted message and peer-closed code", failure.Diagnostics)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			cancelled := rpcFailure(ctx, "initialize", "cursor-acp", err, "agent diagnostic token="+secret, request)
			if !errors.As(cancelled, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled || strings.Contains(failure.Message, "agent diagnostic") {
				t.Fatalf("cancelled failure = %#v, want cancellation before diagnostic classification", cancelled)
			}
		})
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
		stop, err := handleProtocolFailureLine(mode, writer, scanner.Text(), scanner.Bytes(), &pendingPromptID)
		if err != nil || stop {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read client RPC: %w", err)
	}
	return nil
}

// handleProtocolFailureLine serves one client RPC line and reports whether the
// peer should exit after serving it.
func handleProtocolFailureLine(mode string, writer *bufio.Writer, line string, raw []byte, pendingPromptID *json.RawMessage) (bool, error) {
	if mode == "permission-denied-empty" && strings.Contains(line, `"outcome"`) {
		if !strings.Contains(line, `"optionId":"deny"`) {
			return false, fmt.Errorf("permission response did not select reject option: %s", line)
		}
		return false, writeRPCResult(writer, *pendingPromptID, `{"stopReason":"end_turn"}`)
	}
	var request protocolFailureRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return false, fmt.Errorf("decode client RPC: %w", err)
	}
	switch request.Method {
	case "initialize":
		return handleProtocolFailureInitialize(mode, writer, request.ID)
	case "session/new":
		return handleProtocolFailureSessionNew(mode, writer, request.ID)
	case "session/load":
		return handleProtocolFailureSessionLoad(mode, writer, request.ID)
	case "session/prompt":
		return handleProtocolFailurePrompt(mode, writer, request, pendingPromptID)
	case "$/cancel_request", "session/cancel":
		return true, nil
	default:
		return false, fmt.Errorf("unexpected client RPC method %q", request.Method)
	}
}

type protocolFailureRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

func handleProtocolFailureInitialize(mode string, writer *bufio.Writer, id json.RawMessage) (bool, error) {
	if mode == "init-fail" || mode == "stderr" {
		return true, writeRPCError(writer, id, -32603, "Internal error")
	}
	version := 1
	if mode == "version" {
		version = 999
	}
	if err := writeRPCResult(writer, id, fmt.Sprintf(`{"protocolVersion":%d,"agentCapabilities":{},"authMethods":[]}`, version)); err != nil {
		return false, err
	}
	// version mode exits the peer cleanly after the initialize response so
	// the client observes EOF and rejects the unsupported protocol version.
	return mode == "version", nil
}

func handleProtocolFailureSessionNew(mode string, writer *bufio.Writer, id json.RawMessage) (bool, error) {
	if mode == "resume" || mode == "resume-not-found" {
		return false, fmt.Errorf("unexpected session/new during a continuation - the continued attempt must resume through session/load instead of starting a fresh session")
	}
	return false, writeRPCResult(writer, id, `{"sessionId":"acp-session-service-1","configOptions":[]}`)
}

func handleProtocolFailureSessionLoad(mode string, writer *bufio.Writer, id json.RawMessage) (bool, error) {
	if mode == "resume-not-found" {
		// -32002 is the ACP schema's ErrorCodeResourceNotFound - the
		// real code a conformant agent returns for an unrecognized
		// session/load id.
		return true, writeRPCError(writer, id, -32002, "no rollout found for that session")
	}
	return false, writeRPCResult(writer, id, `{"configOptions":[]}`)
}

func handleProtocolFailurePrompt(mode string, writer *bufio.Writer, request protocolFailureRequest, pendingPromptID *json.RawMessage) (bool, error) {
	switch mode {
	case "peer-closed":
		return true, nil
	case "permission-denied-empty":
		*pendingPromptID = append(json.RawMessage(nil), request.ID...)
		if _, err := fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":"permission-1","method":"session/request_permission","params":{"sessionId":"acp-session-service-1","toolCall":{"toolCallId":"tool-1","title":"Read outside workspace"},"options":[{"optionId":"deny","kind":"reject_once","name":"Deny"}]}}`); err != nil {
			return false, err
		}
		return false, writer.Flush()
	case "fail":
		return true, writeRPCError(writer, request.ID, -32603, "Internal error")
	case "server-failure":
		return true, writeRPCError(writer, request.ID, -32001, "temporarily unavailable")
	case "rate-limit":
		return true, writeRPCError(writer, request.ID, -32603, "Internal error: Rate limit exceeded; secret=private")
	case "cancelled-turn":
		return true, writeRPCResult(writer, request.ID, `{"stopReason":"cancelled"}`)
	default:
		return true, writeRPCResult(writer, request.ID, `{"stopReason":"end_turn"}`)
	}
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

func TestPiModelConnectionMarkerClassification(t *testing.T) {
	const secret = "private-model-endpoint-token"
	marker := map[string]any{"provider": "pi", "outcome": "error", "failureKind": "model_connection", "secret": secret}
	for _, tc := range []struct {
		name string
		id   providers.ID
		code int
		data any
		want providers.ExecuteFailureKind
	}{
		{"pi marker", "pi", -32603, marker, providers.ExecuteFailureKindMisconfigured},
		{"other provider", "other", -32603, marker, providers.ExecuteFailureKindUnknown},
		{"wrong code", "pi", -32602, marker, providers.ExecuteFailureKindUnknown},
		{"missing kind", "pi", -32603, map[string]any{"provider": "pi", "outcome": "error"}, providers.ExecuteFailureKindUnknown},
		{"wrong kind", "pi", -32603, map[string]any{"provider": "pi", "outcome": "error", "failureKind": "other"}, providers.ExecuteFailureKindUnknown},
		{"wrong outcome", "pi", -32603, map[string]any{"provider": "pi", "outcome": "success", "failureKind": "model_connection"}, providers.ExecuteFailureKindUnknown},
		{"wrong provider", "pi", -32603, map[string]any{"provider": "other", "outcome": "error", "failureKind": "model_connection"}, providers.ExecuteFailureKindUnknown},
		{"untyped data", "pi", -32603, secret, providers.ExecuteFailureKindUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &acpsdk.RequestError{Code: tc.code, Message: "Pi assistant turn failed", Data: tc.data}
			failure := rpcFailure(context.Background(), "session/prompt", tc.id, err, "stderr "+secret, providers.ExecuteRequest{})
			got, ok := failure.(providers.ExecuteFailure)
			if !ok || got.Kind != tc.want {
				t.Fatalf("failure = %#v, want kind %q", failure, tc.want)
			}
			if tc.want != providers.ExecuteFailureKindMisconfigured {
				return
			}
			if !strings.Contains(got.Message, "selected model endpoint") || strings.Contains(got.Message, secret) {
				t.Fatalf("unsafe or unactionable message: %q", got.Message)
			}
			if got.Diagnostics == nil || len(got.Diagnostics.Progress) != 1 {
				t.Fatalf("diagnostics = %#v", got.Diagnostics)
			}
			progress := got.Diagnostics.Progress[0]
			if progress.Metadata["error_code"] != "ACP_PI_MODEL_CONNECTION" || progress.Detail != got.Message || strings.Contains(progress.Detail, secret) {
				t.Fatalf("diagnostic = %#v", progress)
			}
		})
	}
}
