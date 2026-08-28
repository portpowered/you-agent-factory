package lifecycle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	placementSuccessFactoryName      = "session-parity-success"
	placementFailureFactoryName      = "session-parity-domain-failure"
	placementCancellationFactoryName = "session-parity-cancel"
	placementTransportFactoryName    = "session-parity-transport-failure"
	lifecycleNamedFactoryStateDir    = ".you-agent-factory"
	lifecycleNamedFactoriesDir       = "factories"
)

// TestCLIRemoteRunStartsDurableSessionOnSelectedServer proves the public
// root.BuildProcess/Process.Execute client path admits work on a real
// root-built server, survives submitting-client disconnect, and leaves the
// returned server-owned session inspectable. The same public server also
// proves request-id replay and conflict behavior without a fabricated HTTP
// responder.
func TestCLIRemoteRunStartsDurableSessionOnSelectedServer(t *testing.T) {
	if lifecycleFixture == nil || lifecycleFixture.client == nil {
		t.Fatal("shared lifecycle fixture is unavailable")
	}
	scenarioID := uuid.NewString()
	runRemoteClientDisconnect(
		t,
		lifecycleFixture.client,
		lifecycleFixture.clientWorkingDir,
		lifecycleFixture.baseURL,
		"same request "+scenarioID,
	)
	assertRemoteRequestIDBehavior(t, lifecycleFixture.baseURL, "functional-remote-retry-"+scenarioID)
}

// TestCLILocalAndRemoteRunSuccessParityThroughRootProcess proves equivalent
// local and selected-server invocations through the public root-built
// Process.Execute boundary. The JavaScript Factory has no provider dependency,
// so both placements exercise the same domain outcome and public JSON envelope.
func TestCLILocalAndRemoteRunSuccessParityThroughRootProcess(t *testing.T) {
	fixture := requireSharedPlacementFixture(t)
	local := executePlacementRun(
		t,
		fixture.client,
		fixture.clientWorkingDir,
		placementSuccessFactoryName,
		"",
		false,
	)
	remote := executePlacementRun(
		t,
		fixture.client,
		fixture.clientWorkingDir,
		placementSuccessFactoryName,
		fixture.baseURL,
		true,
	)
	assertPlacementSuccessParity(t, local, remote, "placement parity complete")
}

// TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess proves the
// same Factory-domain failure stays a failed invocation at both placements,
// with terminal output on stdout and diagnostics on stderr.
func TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess(t *testing.T) {
	fixture := requireSharedPlacementFixture(t)
	local := executePlacementRun(
		t,
		fixture.client,
		fixture.clientWorkingDir,
		placementFailureFactoryName,
		"",
		false,
	)
	remote := executePlacementRun(
		t,
		fixture.client,
		fixture.clientWorkingDir,
		placementFailureFactoryName,
		fixture.baseURL,
		true,
	)
	assertPlacementFailureParity(t, local, remote, "INVOCATION_RUNTIME_FAILURE")
}

// TestCLILocalAndRemoteRunCancellationParityThroughRootProcess proves a
// caller cancellation reaches both local and selected-server placements and
// neither path reports a fabricated successful primary result.
func TestCLILocalAndRemoteRunCancellationParityThroughRootProcess(t *testing.T) {
	fixture := requireSharedPlacementFixture(t)
	fixture.client.initializeCustomerHome(t, fixture.clientWorkingDir)
	local := executePlacementRunWithContext(
		t.Context(),
		t,
		fixture.client,
		fixture.clientWorkingDir,
		placementCancellationFactoryName,
		"",
		false,
	)
	remote := executePlacementRunWithContext(
		t.Context(),
		t,
		fixture.client,
		fixture.clientWorkingDir,
		placementCancellationFactoryName,
		fixture.baseURL,
		true,
	)
	assertPlacementCancellationParity(t, local, remote)
}

// TestCLIRemoteRunTransportFailureKeepsStreamsAndExitObservable proves a
// selected-server transport failure does not leak a success payload onto
// stdout and remains a non-nil Process.Execute failure with a diagnostic.
func TestCLIRemoteRunTransportFailureKeepsStreamsAndExitObservable(t *testing.T) {
	fixture := requireSharedPlacementFixture(t)
	observation := executePlacementRun(
		t,
		fixture.client,
		fixture.clientWorkingDir,
		placementTransportFactoryName,
		"http://127.0.0.1:1",
		true,
	)
	if observation.err == nil {
		t.Fatal("remote Process.Execute error = nil, want transport failure")
	}
	if strings.TrimSpace(observation.stdout) != "" {
		t.Fatalf("remote transport-failure stdout = %q, want empty", observation.stdout)
	}
	if strings.TrimSpace(observation.stderr) == "" {
		t.Fatal("remote transport-failure stderr is empty, want actionable diagnostic")
	}
	if !strings.Contains(observation.stderr, "REMOTE_DURABLE_START_FAILED") {
		t.Fatalf("remote transport-failure stderr = %q, want REMOTE_DURABLE_START_FAILED", observation.stderr)
	}
}

type placementRunObservation struct {
	err    error
	stdout string
	stderr string
}

func executePlacementRun(
	t *testing.T,
	client *lifecycleClientProcess,
	workingDirectory, factoryName, serverURL string,
	remote bool,
) placementRunObservation {
	t.Helper()
	args := []string{}
	if remote {
		args = append(args, "--remote")
	}
	args = append(args, "--json", "run", "--named", factoryName, "--no-record", "placement parity")
	inputs, command := client.startCLI(t, t.Context(), workingDirectory, serverURL, nil, args...)
	<-command.Done()
	if command.Err() != nil {
		command.AcceptError()
	}
	observation := placementRunObservation{
		err:    command.Err(),
		stdout: inputs.Stdout(),
		stderr: inputs.Stderr(),
	}
	registerPlacementResponseCleanup(t, serverURL, remote, observation.stdout)
	return observation
}

func executePlacementRunWithContext(
	parentContext context.Context,
	t *testing.T,
	client *lifecycleClientProcess,
	workingDirectory, factoryName, serverURL string,
	remote bool,
) placementRunObservation {
	t.Helper()
	if remote {
		return executeRemotePlacementRunWithReadiness(
			parentContext, t, client, workingDirectory, factoryName, serverURL,
		)
	}
	return executeLocalPlacementRunWithReadiness(
		parentContext, t, client, workingDirectory, factoryName,
	)
}

func executeLocalPlacementRunWithReadiness(
	parentContext context.Context,
	t *testing.T,
	client *lifecycleClientProcess,
	workingDirectory, factoryName string,
) placementRunObservation {
	t.Helper()

	runContext, cancel := context.WithCancel(parentContext)
	defer cancel()
	readinessOutput := newLocalInvocationReadinessOutput()
	_, command := client.startCLI(
		t,
		runContext,
		workingDirectory,
		"",
		readinessOutput,
		"--json", "run", "--named", factoryName, "--output", "response-stream", "--no-record", "placement parity",
	)
	select {
	case <-readinessOutput.started:
		// Stop uses the support command's existing bounded cancellation
		// lifecycle, so a cancellation-controlled invocation cannot strand the
		// test while it waits for Process.Execute to return.
		command.Stop(t)
	case <-command.Done():
		// Preserve a pre-readiness bootstrap or invocation failure for the
		// parity assertions instead of waiting for an outer test timeout.
		command.AcceptError()
		return placementObservation(command, readinessOutput)
	case <-parentContext.Done():
		command.Stop(t)
		t.Fatalf("local run did not reach invocation readiness: %v\noutput:\n%s", parentContext.Err(), readinessOutput.String())
	}
	return placementObservation(command, readinessOutput)
}

func executeRemotePlacementRunWithReadiness(
	parentContext context.Context,
	t *testing.T,
	client *lifecycleClientProcess,
	workingDirectory, factoryName, serverURL string,
) placementRunObservation {
	t.Helper()

	runContext, cancel := context.WithCancel(parentContext)
	defer cancel()
	admissionOutput := newRemoteAdmissionOutput()
	_, command := client.startCLI(
		t,
		runContext,
		workingDirectory,
		serverURL,
		admissionOutput,
		"--remote", "--verbose", "--json", "run", "--named", factoryName, "--output", "response-stream", "--no-record", "placement parity",
	)
	select {
	case <-admissionOutput.started:
		// Stop uses the support command's existing bounded cancellation
		// lifecycle, so a cancellation-controlled invocation cannot strand the
		// test while it waits for Process.Execute to return.
		command.Stop(t)
	case <-command.Done():
		// Preserve a pre-readiness bootstrap or invocation failure for the
		// parity assertions instead of waiting for an outer test timeout.
		command.AcceptError()
		return placementObservation(command, admissionOutput)
	case <-parentContext.Done():
		command.Stop(t)
		t.Fatalf("remote run did not reach durable-admission readiness: %v\noutput:\n%s", parentContext.Err(), admissionOutput.String())
	}
	sessionID := remoteSessionIDFromAdmissionOutput(t, admissionOutput.String())
	markSessionClean := registerLifecycleSessionCleanup(t, serverURL, sessionID)
	defer func() {
		if err := terminateRemoteFunctionalSession(serverURL, sessionID); err != nil {
			t.Errorf("terminate cancellation parity durable session %s: %v", sessionID, err)
		}
		markSessionClean()
	}()
	return placementObservation(command, admissionOutput)
}

type placementOutput interface {
	String() string
}

func placementObservation(command *support.ProcessCommand, output placementOutput) placementRunObservation {
	return placementRunObservation{
		err:    command.Err(),
		stdout: output.String(),
		stderr: output.String(),
	}
}

func assertPlacementSuccessParity(
	t *testing.T,
	local, remote placementRunObservation,
	wantPrimaryResult string,
) {
	t.Helper()
	for name, observation := range map[string]placementRunObservation{"local": local, "remote": remote} {
		if observation.err != nil {
			t.Fatalf("%s Process.Execute error = %v\nstdout:\n%s\nstderr:\n%s", name, observation.err, observation.stdout, observation.stderr)
		}
		response := support.DecodeInvocationResponseJSON(t, observation.stdout)
		if response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("%s status = %q, want COMPLETED", name, response.Status)
		}
		if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
			t.Fatalf("%s primaryResult = %#v, want %q", name, response.PrimaryResult, wantPrimaryResult)
		}
		part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
		if err != nil || part.Text != wantPrimaryResult {
			t.Fatalf("%s primaryResult text = %q (decode error %v), want %q", name, part.Text, err, wantPrimaryResult)
		}
		if strings.TrimSpace(observation.stderr) != "" {
			t.Fatalf("%s stderr = %q, want empty on success", name, observation.stderr)
		}
	}
}

func assertPlacementFailureParity(
	t *testing.T,
	local, remote placementRunObservation,
	wantErrorCode string,
) {
	t.Helper()
	var localResponse, remoteResponse factoryapi.InvocationResponse
	for name, observation := range map[string]placementRunObservation{
		"local":  local,
		"remote": remote,
	} {
		if observation.err == nil {
			t.Fatalf("%s Process.Execute error = nil, want terminal domain failure", name)
		}
		if strings.TrimSpace(observation.stderr) == "" {
			t.Fatalf("%s stderr is empty, want terminal failure diagnostic", name)
		}
		response := support.DecodeInvocationResponseJSON(t, observation.stdout)
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("%s status = %q, want FAILED", name, response.Status)
		}
		if response.ErrorCode == nil || string(*response.ErrorCode) != wantErrorCode {
			t.Fatalf("%s errorCode = %#v, want %q", name, response.ErrorCode, wantErrorCode)
		}
		if name == "local" {
			localResponse = response
		} else {
			remoteResponse = response
		}
	}
	if localResponse.Status != remoteResponse.Status ||
		localResponse.ErrorCode == nil || remoteResponse.ErrorCode == nil ||
		*localResponse.ErrorCode != *remoteResponse.ErrorCode {
		t.Fatalf("local/remote failure responses differ: local=%#v remote=%#v", localResponse, remoteResponse)
	}
}

func assertPlacementCancellationParity(
	t *testing.T,
	local, remote placementRunObservation,
) {
	t.Helper()
	for name, observation := range map[string]placementRunObservation{
		"local":  local,
		"remote": remote,
	} {
		if observation.err == nil {
			t.Fatalf("%s Process.Execute error = nil, want context deadline", name)
		}
		if !errors.Is(observation.err, context.DeadlineExceeded) && !errors.Is(observation.err, context.Canceled) {
			t.Fatalf("%s Process.Execute error = %v, want context cancellation", name, observation.err)
		}
		if strings.Contains(observation.stdout, "placement parity complete") {
			t.Fatalf("%s stdout fabricated a successful primary result:\n%s", name, observation.stdout)
		}
		combinedOutput := strings.ToLower(observation.stdout + "\n" + observation.stderr)
		for _, diagnostic := range []string{
			"packaged factory installation",
			"initialize system: system bootstrap initialize partial failure",
		} {
			if strings.Contains(combinedOutput, diagnostic) {
				t.Fatalf("%s reported bootstrap diagnostic %q:\n%s", name, diagnostic, observation.stdout+"\n"+observation.stderr)
			}
		}
	}
}

func requireSharedPlacementFixture(t *testing.T) *sharedLifecycleFixture {
	t.Helper()
	if lifecycleFixture == nil || lifecycleFixture.client == nil {
		t.Fatal("shared lifecycle placement fixture is unavailable")
	}
	return lifecycleFixture
}

func registerPlacementResponseCleanup(
	t *testing.T,
	serverURL string,
	remote bool,
	stdout string,
) {
	t.Helper()
	if !remote || strings.TrimSpace(serverURL) == "" {
		return
	}
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal(bytesTrimSpace([]byte(stdout)), &response); err != nil || response.SessionId == nil {
		return
	}
	sessionID := strings.TrimSpace(*response.SessionId)
	if sessionID != "" {
		registerLifecycleSessionCleanup(t, serverURL, sessionID)
	}
}

// writeSharedRemoteLifecycleFactory authors the named JavaScript Factory and
// workflow used by the durable witness before the package-scoped server starts.
// The named Factory is placed in the same isolated global catalog used by the
// shared client and server; placement catalogs are immutable for the package
// lifetime and are also authored here before the server starts.
func writeSharedRemoteLifecycleFactory(homeDir, serverFactoryDir string) error {
	placementFactories := []struct {
		name   string
		source string
	}{
		{
			name:   "remote-placement",
			source: "var spin = 0; while (true) { spin += 1; }",
		},
		{
			name:   placementSuccessFactoryName,
			source: `workflow.final("placement parity complete");`,
		},
		{
			name:   placementFailureFactoryName,
			source: `throw new Error("placement parity domain failure");`,
		},
		{
			name:   placementCancellationFactoryName,
			source: "while (true) {}",
		},
		{
			name:   placementTransportFactoryName,
			source: `workflow.final("must not run");`,
		},
	}
	for _, factory := range placementFactories {
		if err := writeSharedNamedPlacementFactory(homeDir, factory.name, factory.source); err != nil {
			return err
		}
	}
	workflowDir := filepath.Join(serverFactoryDir, ".claude", interfaces.WorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return fmt.Errorf("create server workflow directory: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(workflowDir, "remote-idempotency.js"),
		[]byte("return 'idempotency complete';"),
		0o600,
	); err != nil {
		return fmt.Errorf("write server idempotency workflow: %w", err)
	}
	return nil
}

func writeSharedNamedPlacementFactory(homeDir, name, source string) error {
	// This fixture writes the named Factory into the injected home rather than
	// calling the Factory Definitions policy helper from this package. The
	// resulting file is still consumed through the public CLI named-source path.
	namedFactoryDir := filepath.Join(homeDir, lifecycleNamedFactoryStateDir, lifecycleNamedFactoriesDir, name)
	if err := os.MkdirAll(namedFactoryDir, 0o755); err != nil {
		return fmt.Errorf("create named Factory %q directory: %w", name, err)
	}
	rawConfig, err := json.Marshal(map[string]any{
		"name": name,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name":     "prompt",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   source,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal named Factory %q: %w", name, err)
	}
	if err := os.WriteFile(filepath.Join(namedFactoryDir, interfaces.FactoryConfigFile), rawConfig, 0o644); err != nil {
		return fmt.Errorf("write named Factory %q config: %w", name, err)
	}
	return nil
}

func runRemoteClientDisconnect(
	t *testing.T,
	client *lifecycleClientProcess,
	workingDir, serverURL string,
	prompt string,
) {
	t.Helper()
	args := []string{
		"--remote", "--verbose", "run",
		"--named", "remote-placement", "--json", "--output", "response-stream", "--no-record", prompt,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	admissionOutput := newRemoteAdmissionOutput()
	_, command := client.startCLI(t, ctx, workingDir, serverURL, admissionOutput, args...)
	select {
	case <-admissionOutput.started:
	case <-time.After(10 * time.Second):
		command.Stop(t)
		t.Fatalf("remote client did not observe durable admission: client error=%v\nadmission output:\n%s", command.Err(), admissionOutput.String())
	}
	cancel()
	select {
	case <-command.Done():
		command.AcceptError()
		if command.Err() == nil {
			t.Fatal("remote client returned nil after submitting-client disconnect")
		}
	case <-time.After(10 * time.Second):
		command.Stop(t)
		t.Fatal("remote client did not return after submitting connection cancellation")
	}
	sessionID := remoteSessionIDFromAdmissionOutput(t, admissionOutput.String())
	markSessionClean := registerLifecycleSessionCleanup(t, serverURL, sessionID)
	inspection := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID,
	)
	durable, err := inspection.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session %s after client disconnect: %v", sessionID, err)
	}
	if durable.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want returned %q", durable.SessionId, sessionID)
	}
	if durable.Status != factoryapi.FactorySessionDurableLifecycleStatusQueued && durable.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("durable session status after client disconnect = %q, want QUEUED or RUNNING\nadmission output:\n%s", durable.Status, admissionOutput.String())
	}
	if err := terminateRemoteFunctionalSession(serverURL, sessionID); err != nil {
		t.Fatalf("terminate disconnected durable session %s: %v", sessionID, err)
	}
	markSessionClean()
	if strings.Contains(admissionOutput.String(), "http://127.0.0.1:1/") {
		t.Fatalf("remote output leaked a local fallback endpoint: %s", admissionOutput.String())
	}
}

func assertRemoteRequestIDBehavior(t *testing.T, serverURL, requestID string) {
	t.Helper()
	retryWorkflow := "remote-idempotency"
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &retryWorkflow,
		},
	}
	first := postRemoteFunctionalExecution(t, serverURL, request)
	var markSessionClean func()
	if first.SessionId != "" {
		markSessionClean = registerLifecycleSessionCleanup(t, serverURL, first.SessionId)
	}
	replay := postRemoteFunctionalExecution(t, serverURL, request)
	if replay.SessionId != "" && replay.SessionId != first.SessionId {
		registerLifecycleSessionCleanup(t, serverURL, replay.SessionId)
	}
	if first.SessionId == "" || replay.SessionId != first.SessionId {
		t.Fatalf("request-id replay sessions = %q/%q, want one server-owned identity", first.SessionId, replay.SessionId)
	}
	conflictRequest := request
	conflictRequest.Source.WorkflowName = strPtr("remote-idempotency-conflict")
	conflictStatus, conflict := postRemoteFunctionalExecutionConflict(t, serverURL, conflictRequest)
	if conflictStatus != http.StatusConflict || conflict.Code != factoryapi.ErrorResponseCode("EXECUTION_REQUEST_ID_CONFLICT") {
		t.Fatalf("request-id conflict = status %d response %#v, want 409 EXECUTION_REQUEST_ID_CONFLICT", conflictStatus, conflict)
	}
	if err := terminateRemoteFunctionalSession(serverURL, first.SessionId); err != nil {
		t.Fatalf("terminate replayed durable session %s: %v", first.SessionId, err)
	}
	if markSessionClean != nil {
		markSessionClean()
	}
}

// startCLI serializes one asynchronous invocation on the shared root-built
// client. The lock remains held until Process.Execute joins so another test
// cannot reuse invocation-owned streams while this command is disconnecting.
func (client *lifecycleClientProcess) startCLI(
	t *testing.T,
	ctx context.Context,
	workingDir string,
	serverURL string,
	output io.Writer,
	args ...string,
) (*support.CapturedInputs, *support.ProcessCommand) {
	t.Helper()
	client.mu.Lock()
	if client.process == nil {
		client.mu.Unlock()
		t.Fatal("shared lifecycle client process is unavailable")
	}
	cmdArgs := []string{"you"}
	if strings.TrimSpace(serverURL) != "" {
		cmdArgs = append(cmdArgs, "--server", serverURL)
	}
	cmdArgs = append(cmdArgs, args...)
	inputs := support.FakeInputs(ctx, cmdArgs)
	inputs.Input.Env = append([]string(nil), client.env...)
	inputs.Input.WorkingDirectory = workingDir
	if output != nil {
		inputs.Input.Stdout = output
		inputs.Input.Stderr = output
	}
	var invocationID string
	if lifecycleFixture != nil && lifecycleFixture.ledger != nil {
		invocationID = lifecycleFixture.ledger.beginInvocation(t.Name() + " Process.Execute")
		t.Cleanup(func() {
			if err := lifecycleFixture.ledger.closeInvocation(invocationID); err != nil {
				t.Errorf("record lifecycle invocation cleanup census: %v", err)
			}
		})
	}
	command := support.StartProcessCommand(t, client.process, inputs.Input)
	go func() {
		<-command.Done()
		client.mu.Unlock()
	}()
	return inputs, command
}

func (client *lifecycleClientProcess) initializeCustomerHome(
	t testing.TB,
	workingDir string,
) {
	t.Helper()
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.process == nil {
		t.Fatal("shared lifecycle client process is unavailable")
	}
	support.InitializeCustomerHomeWithProcess(t, client.process, client.env, workingDir)
}

type remoteAdmissionOutput struct {
	mu      sync.Mutex
	output  strings.Builder
	started chan struct{}
	once    sync.Once
}

func newRemoteAdmissionOutput() *remoteAdmissionOutput {
	return &remoteAdmissionOutput{started: make(chan struct{})}
}

func (writer *remoteAdmissionOutput) Write(p []byte) (int, error) {
	writer.mu.Lock()
	_, _ = writer.output.Write(p)
	started := strings.Contains(writer.output.String(), "remote durable start response") || strings.Contains(writer.output.String(), "session-started/")
	writer.mu.Unlock()
	if started {
		writer.once.Do(func() { close(writer.started) })
	}
	return len(p), nil
}

func (writer *remoteAdmissionOutput) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.output.String()
}

type localInvocationReadinessOutput struct {
	mu      sync.Mutex
	output  strings.Builder
	started chan struct{}
	once    sync.Once
}

func newLocalInvocationReadinessOutput() *localInvocationReadinessOutput {
	return &localInvocationReadinessOutput{started: make(chan struct{})}
}

func (writer *localInvocationReadinessOutput) Write(p []byte) (int, error) {
	writer.mu.Lock()
	_, _ = writer.output.Write(p)
	started := strings.Contains(writer.output.String(), `"recordType":"factory_event"`)
	writer.mu.Unlock()
	if started {
		writer.once.Do(func() { close(writer.started) })
	}
	return len(p), nil
}

func (writer *localInvocationReadinessOutput) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.output.String()
}

func remoteSessionIDFromAdmissionOutput(t *testing.T, output string) string {
	t.Helper()
	for _, marker := range []string{"sessionId=", `"sessionId":"`} {
		start := strings.Index(output, marker)
		if start < 0 {
			continue
		}
		value := output[start+len(marker):]
		if end := strings.IndexAny(value, " \r\n\",}"); end >= 0 {
			value = value[:end]
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	t.Fatalf("durable admission output did not contain a session id: %s", output)
	return ""
}

func postRemoteFunctionalExecution(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	status, response, body := postRemoteFunctionalExecutionRaw(t, serverURL, request)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		t.Fatalf("remote durable retry status = %d body = %s", status, body)
	}
	return response
}

func postRemoteFunctionalExecutionConflict(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) (int, factoryapi.ErrorResponse) {
	t.Helper()
	status, _, body := postRemoteFunctionalExecutionRaw(t, serverURL, request)
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode remote request-id conflict: %v body=%s", err, body)
	}
	return status, response
}

func postRemoteFunctionalExecutionRaw(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) (int, factoryapi.FactorySessionExecutionResponse, []byte) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal remote durable request: %v", err)
	}
	httpRequest, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, strings.TrimSuffix(serverURL, "/")+"/factory-sessions/async", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build remote durable request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("POST remote durable request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read remote durable response: %v", err)
	}
	var decoded factoryapi.FactorySessionExecutionResponse
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if err := json.Unmarshal(responseBody, &decoded); err != nil {
			t.Fatalf("decode remote durable response: %v body=%s", err, responseBody)
		}
	}
	return response.StatusCode, decoded, responseBody
}

func terminateRemoteFunctionalSession(serverURL, sessionID string) error {
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/terminate",
		bytes.NewReader([]byte(`{}`)),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode == http.StatusConflict && strings.Contains(string(body), `"outcome":"TERMINAL_SESSION"`) {
			return nil
		}
		return fmt.Errorf("status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func strPtr(value string) *string { return &value }
