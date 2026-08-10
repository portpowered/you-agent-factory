package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type capturingBootstrapRunner struct {
	lastRequest *factoryapi.InvocationRequest
	lastResult  *apisurface.FactoryInvocationResult
}

func (c *capturingBootstrapRunner) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *capturingBootstrapRunner) GetCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
) (factoryapi.Factory, error) {
	return factoryapi.Factory{Name: "transport-test-factory"}, nil
}

func (c *capturingBootstrapRunner) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	c.lastRequest = cloneInvocationRequestForCapture(request)
	result := apisurface.FactoryInvocationResult{
		Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "mock worker accepted",
		}},
	}
	captured := result
	c.lastResult = &captured
	return result, nil
}

func (c *capturingBootstrapRunner) CloseFactorySession(ctx context.Context, sessionID string) error {
	return nil
}

func cloneInvocationRequestForCapture(request factoryapi.InvocationRequest) *factoryapi.InvocationRequest {
	data, err := json.Marshal(request)
	if err != nil {
		panic(fmt.Sprintf("marshal invocation request for capture: %v", err))
	}
	var cloned factoryapi.InvocationRequest
	if err := json.Unmarshal(data, &cloned); err != nil {
		panic(fmt.Sprintf("unmarshal invocation request for capture: %v", err))
	}
	return &cloned
}

func installCapturingInvocationStub(t *testing.T) *capturingBootstrapRunner {
	t.Helper()

	capture := &capturingBootstrapRunner{}
	openTestInvocationRunner = func(context.Context, *testRuntimeSelections, serviceedges.Edges) (sessionInvocationRunner, error) {
		return capture, nil
	}
	return capture
}

func namedGoalNoServerInvocationRunConfig(t *testing.T, goalText string) RunConfig {
	t.Helper()

	homeDir := t.TempDir()
	setUserHomeForTest(t, homeDir)
	resolution := packagedRunFixtureResolution(goal.PackagedFactoryName, t.TempDir(), homeDir)

	return RunConfig{
		Dir:                        resolution.FactoryDir,
		ExecutionBaseDir:           homeDir,
		NamedFactoryName:           goal.PackagedFactoryName,
		NamedFactoryResolution:     resolution,
		InvocationPositionalText:   &goalText,
		StdinIsTTY:                 func() bool { return true },
		SuppressDashboardRendering: true,
		MockWorkersEnabled:         true,
		MockWorkersConfigPath:      writePackagedGoalNoServerMockWorkersConfig(t),
		DisableDefaultRecording:    true,
		Port:                       noServerInvocationTestPort,
		AutoPort:                   true,
		Logger:                     zap.NewNop(),
	}
}

func runNoServerBootstrapEquivalenceCase(
	t *testing.T,
	goalText string,
	mutate func(*RunConfig),
) (*capturingBootstrapRunner, *bytes.Buffer) {
	t.Helper()

	preserveRunGlobals(t)
	capture := installCapturingInvocationStub(t)
	cfg := namedGoalNoServerInvocationRunConfig(t, goalText)
	if mutate != nil {
		mutate(&cfg)
	}
	var output bytes.Buffer
	cfg.Output = &output

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := Run(ctx, cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return capture, &output
}

func assertCapturedRequestMatchesLogicalAPIText(t *testing.T, capture *capturingBootstrapRunner, goalText string) {
	t.Helper()

	apiRequest, err := invocationRequestFromLogicalAPIText(goalText)
	if err != nil {
		t.Fatalf("invocationRequestFromLogicalAPIText: %v", err)
	}
	if capture.lastRequest == nil {
		t.Fatal("expected InvokeFactorySession request capture on real no-server bootstrap")
	}
	assertEquivalentInvocationRequests(t, capture.lastRequest, apiRequest)
}

func assertCapturedResultMatchesCLIJSONOutput(t *testing.T, capture *capturingBootstrapRunner, output *bytes.Buffer) {
	t.Helper()

	if capture.lastResult == nil {
		t.Fatal("expected InvokeFactorySession result capture on real no-server bootstrap")
	}
	if capture.lastResult.Status != interfaces.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want %q", capture.lastResult.Status, interfaces.InvocationTerminalStatusCompleted)
	}

	var cliResponse factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &cliResponse); err != nil {
		t.Fatalf("decode CLI invocation response: %v\n%s", err, output.String())
	}
	apiResponse := apisurface.InvocationResponseFromResult(*capture.lastResult)
	if !reflect.DeepEqual(cliResponse, apiResponse) {
		t.Fatalf("CLI response = %#v, API projection = %#v", cliResponse, apiResponse)
	}
	assertInvocationResponseMatchesFactoryResult(t, cliResponse, *capture.lastResult)
}

func TestRun_NoServerBootstrap_PositionalInputMatchesAPIContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap CLI/API invocation equivalence")
	}

	goalText := "no-server bootstrap positional parity prompt"
	capture, _ := runNoServerBootstrapEquivalenceCase(t, goalText, nil)
	assertCapturedRequestMatchesLogicalAPIText(t, capture, goalText)
}

func TestRun_NoServerBootstrap_StdinInputMatchesAPIContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap CLI/API invocation equivalence")
	}

	goalText := "no-server bootstrap stdin parity prompt"
	capture, _ := runNoServerBootstrapEquivalenceCase(t, goalText, func(cfg *RunConfig) {
		cfg.InvocationPositionalText = nil
		cfg.PreparedInvocationInput = preparedTextInvocationInputPtr(work.InputSourceStdinText, goalText)
	})
	assertCapturedRequestMatchesLogicalAPIText(t, capture, goalText)
}

func TestRun_NoServerBootstrap_SuccessJSONMatchesAPIProjection(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap CLI/API invocation equivalence")
	}

	goalText := "no-server bootstrap json parity prompt"
	capture, output := runNoServerBootstrapEquivalenceCase(t, goalText, func(cfg *RunConfig) {
		cfg.JSONOutput = true
	})
	assertCapturedResultMatchesCLIJSONOutput(t, capture, output)
}

func TestRun_NoServerBootstrap_TextPrimaryResultFollowsInvocationReturn(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap CLI/API invocation equivalence")
	}

	goalText := "no-server bootstrap primary-result prompt"
	_, output := runNoServerBootstrapEquivalenceCase(t, goalText, nil)
	if got := output.String(); got != "mock worker accepted" {
		t.Fatalf("stdout = %q, want packaged goal invocationReturn primary result", got)
	}
	if got := output.String(); got == goalText {
		t.Fatalf("stdout echoed submitted goal text instead of invocationReturn primary result")
	}
}

func writePackagedGoalNoServerMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-executor",
				WorkstationName: goal.PackagedExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
		},
	}
	return writeMockWorkersConfig(t, cfg, "mock-workers-packaged-goal-no-server.json")
}

func writeMockWorkersConfig(t *testing.T, cfg workers.MockWorkersConfig, name string) string {
	t.Helper()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}

const noServerInvocationTestPort = 38317

// TestNoServerNamedInvocationIntegrationAndEquivalenceProof is the consolidated
// package integration and invocation-equivalence proof for hermetic named
// one-shot invocation on the shared no-server bootstrap path. It fails if named
// runs regress to requiring a listening HTTP server or drift from shared CLI/API
// input-resolution and primary-result contracts.
func TestNoServerNamedInvocationIntegrationAndEquivalenceProof(t *testing.T) {
	if testing.Short() {
		t.Skip("consolidated package integration and invocation-equivalence proof for no-server named invocation")
	}

	t.Run("hermetic named success without listener", func(t *testing.T) {
		preserveRunGlobals(t)

		goalText := "consolidated no-server named integration proof"
		cfg := namedGoalNoServerInvocationRunConfig(t, goalText)
		cfg.AutoPort = true
		var output bytes.Buffer
		cfg.Output = &output

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		if err := Run(ctx, cfg); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := output.String(); got != "mock worker accepted" {
			t.Fatalf("stdout = %q, want invocationReturn primary result mock worker accepted", got)
		}
	})

	t.Run("shared input resolution and primary-result equivalence", func(t *testing.T) {
		goalText := "consolidated no-server equivalence proof"
		capture, output := runNoServerBootstrapEquivalenceCase(t, goalText, func(cfg *RunConfig) {
			cfg.JSONOutput = true
		})
		assertCapturedRequestMatchesLogicalAPIText(t, capture, goalText)
		assertCapturedResultMatchesCLIJSONOutput(t, capture, output)
	})
}

func namedSubagentNoServerInvocationRunConfig(t *testing.T, requestText string) RunConfig {
	t.Helper()

	homeDir := t.TempDir()
	setUserHomeForTest(t, homeDir)
	resolution := packagedRunFixtureResolution(interfaces.PackagedSubagentFactoryName, t.TempDir(), homeDir)

	return RunConfig{
		Dir:                        resolution.FactoryDir,
		ExecutionBaseDir:           homeDir,
		NamedFactoryName:           interfaces.PackagedSubagentFactoryName,
		NamedFactoryResolution:     resolution,
		InvocationPositionalText:   &requestText,
		StdinIsTTY:                 func() bool { return true },
		SuppressDashboardRendering: true,
		MockWorkersEnabled:         true,
		MockWorkersConfigPath:      writePackagedSubagentNoServerMockWorkersConfig(t),
		DisableDefaultRecording:    true,
		Port:                       noServerInvocationTestPort,
		AutoPort:                   true,
		Logger:                     zap.NewNop(),
	}
}

func runNoServerSubagentBootstrapEquivalenceCase(
	t *testing.T,
	requestText string,
	mutate func(*RunConfig),
) (*capturingBootstrapRunner, *bytes.Buffer) {
	t.Helper()

	preserveRunGlobals(t)
	capture := installCapturingInvocationStub(t)
	cfg := namedSubagentNoServerInvocationRunConfig(t, requestText)
	if mutate != nil {
		mutate(&cfg)
	}
	var output bytes.Buffer
	cfg.Output = &output

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := Run(ctx, cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return capture, &output
}

func TestRun_NamedSubagentNoServerBootstrap_TextPrimaryResultIsAgentResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap subagent primary-result selection")
	}

	requestText := "no-server bootstrap subagent primary-result prompt"
	_, output := runNoServerSubagentBootstrapEquivalenceCase(t, requestText, nil)
	if got := output.String(); got != "mock worker accepted" {
		t.Fatalf("stdout = %q, want agent response mock worker accepted", got)
	}
	if got := output.String(); got == requestText {
		t.Fatalf("stdout echoed submitted request text instead of agent response")
	}
}

func TestRun_NamedSubagentNoServerBootstrap_SuccessJSONMatchesAPIProjection(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for no-server bootstrap subagent CLI/API projection")
	}

	requestText := "no-server bootstrap subagent json parity prompt"
	capture, output := runNoServerSubagentBootstrapEquivalenceCase(t, requestText, func(cfg *RunConfig) {
		cfg.JSONOutput = true
	})
	assertCapturedRequestMatchesLogicalAPIText(t, capture, requestText)
	assertCapturedResultMatchesCLIJSONOutput(t, capture, output)
	if capture.lastResult == nil || len(capture.lastResult.PrimaryResult) == 0 {
		t.Fatalf("capture result = %#v, want primary result", capture.lastResult)
	}
	if got := capture.lastResult.PrimaryResult[0].Text; got != "mock worker accepted" {
		t.Fatalf("primaryResult text = %q, want agent response mock worker accepted", got)
	}
	if got := capture.lastResult.PrimaryResult[0].Text; got == requestText {
		t.Fatalf("primaryResult echoed submitted request text instead of agent response")
	}
	if !strings.Contains(output.String(), `"primaryResult"`) {
		t.Fatalf("json output = %s, want primaryResult field", output.String())
	}
	if strings.Count(output.String(), `"primaryResult"`) != 1 {
		t.Fatalf("json output = %s, want exactly one primaryResult field", output.String())
	}
}

func writePackagedSubagentNoServerMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      interfaces.PackagedSubagentWorkerName,
				WorkstationName: interfaces.PackagedSubagentRunWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-subagent-no-server.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}

// TestNoServerNamedSubagentInvocationIntegrationAndEquivalenceProof is the
// consolidated package integration and invocation-equivalence proof for hermetic
// named one-shot @you/subagent invocation on the shared no-server bootstrap path.
func TestNoServerNamedSubagentInvocationIntegrationAndEquivalenceProof(t *testing.T) {
	if testing.Short() {
		t.Skip("consolidated package integration and invocation-equivalence proof for no-server named subagent invocation")
	}

	t.Run("hermetic named success without listener", func(t *testing.T) {
		preserveRunGlobals(t)

		requestText := "consolidated no-server named subagent integration proof"
		cfg := namedSubagentNoServerInvocationRunConfig(t, requestText)
		cfg.AutoPort = true
		var output bytes.Buffer
		cfg.Output = &output

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		if err := Run(ctx, cfg); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := output.String(); got != "mock worker accepted" {
			t.Fatalf("stdout = %q, want agent response mock worker accepted", got)
		}
		if got := output.String(); got == requestText {
			t.Fatalf("stdout echoed submitted request text instead of agent response")
		}
	})

	t.Run("shared input resolution and primary-result equivalence", func(t *testing.T) {
		requestText := "consolidated no-server subagent equivalence proof"
		capture, output := runNoServerSubagentBootstrapEquivalenceCase(t, requestText, func(cfg *RunConfig) {
			cfg.JSONOutput = true
		})
		assertCapturedRequestMatchesLogicalAPIText(t, capture, requestText)
		assertCapturedResultMatchesCLIJSONOutput(t, capture, output)
	})
}
