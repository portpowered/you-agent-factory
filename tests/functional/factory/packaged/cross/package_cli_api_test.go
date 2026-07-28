package cross

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const wantPackagedGoalPrimaryResult = "mock worker accepted"

// TestPackagedFactoryInvokedByCLICanBeInspectedByAPI proves a packaged @you/goal
// invocation started through the public you run CLI with a run-scoped server
// exposes compatible session, status, work, and invocation identity facts through
// the public Factory Session API while the invocation completes.
func TestPackagedFactoryInvokedByCLICanBeInspectedByAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("slow packaged factory CLI/API cross-surface inspectability")
	}

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedGoalFactoryName,
	)
	factoryPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
	mockWorkersPath := writePackagedGoalMockWorkersConfig(t)
	goalText := fmt.Sprintf(
		"functional-packaged-cross-cli-api-inspect-%d",
		time.Now().UnixNano(),
	)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	requestedURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{APIServerStarter: server.Start})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "run",
		"--factory", factoryPath,
		"--with-server",
		"--server", requestedURL,
		"--with-mock-workers",
		"--no-record",
		mockWorkersPath,
		goalText,
	})
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = isolatedHomeEnvironment(homeDir)

	execDone := make(chan error, 1)
	go func() {
		execDone <- process.Execute(inputs.Input)
	}()

	baseURL := server.WaitForURL(t)
	inspection := pollPackagedGoalAPIInspectionDuringCLIInvocation(ctx, t, baseURL)

	if err := <-execDone; err != nil {
		t.Fatalf(
			"CLI packaged-factory invocation: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	var cliResponse factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(inputs.Stdout())), &cliResponse); err != nil {
		t.Fatalf("decode CLI invocation JSON: %v\nstdout:\n%s", err, inputs.Stdout())
	}

	if !inspection.ok {
		t.Fatalf("API inspection never observed a live session/status while CLI invocation ran")
	}

	assertPackagedGoalCLIInvocationInspectableByAPI(
		t,
		cliResponse,
		inspection.session,
		inspection.status,
		inspection.listed,
		wantPackagedGoalPrimaryResult,
	)
}

type packagedGoalAPIInspection struct {
	session factoryapi.FactorySession
	status  factoryapi.StatusResponse
	listed  factoryapi.ListWorkResponse
	ok      bool
}

func pollPackagedGoalAPIInspectionDuringCLIInvocation(
	ctx context.Context,
	t *testing.T,
	baseURL string,
) packagedGoalAPIInspection {
	t.Helper()

	var snapshot packagedGoalAPIInspection
	if err := waitForSessionWorkEndpoint(ctx, baseURL, 30*time.Second); err != nil {
		t.Fatalf("wait for CLI-hosted API server: %v", err)
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return snapshot
		default:
		}

		session, sessionErr := tryGetDefaultSession(baseURL)
		status, statusErr := tryGetStatus(baseURL)
		if sessionErr == nil && statusErr == nil &&
			strings.TrimSpace(session.Id) != "" &&
			strings.TrimSpace(status.RuntimeStatus) != "" {
			snapshot.session = session
			snapshot.status = status
			snapshot.listed, _ = tryListDefaultSessionWork(baseURL)
			snapshot.ok = true
			return snapshot
		}

		select {
		case <-ctx.Done():
			return snapshot
		case <-time.After(25 * time.Millisecond):
		}
	}

	return snapshot
}

func tryGetDefaultSession(baseURL string) (factoryapi.FactorySession, error) {
	response, err := http.Get(
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/~default",
	)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.FactorySession{}, fmt.Errorf("status = %d", response.StatusCode)
	}
	var decoded factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.FactorySession{}, err
	}
	return decoded.AsFactorySession()
}

func tryGetStatus(baseURL string) (factoryapi.StatusResponse, error) {
	response, err := http.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return factoryapi.StatusResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.StatusResponse{}, fmt.Errorf("status = %d", response.StatusCode)
	}
	var decoded factoryapi.StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.StatusResponse{}, err
	}
	return decoded, nil
}

func tryListDefaultSessionWork(baseURL string) (factoryapi.ListWorkResponse, error) {
	response, err := http.Get(support.DefaultSessionWorkURL(baseURL, "/work"))
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.ListWorkResponse{}, fmt.Errorf("status = %d", response.StatusCode)
	}
	var decoded factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	return decoded, nil
}

func assertPackagedGoalCLIInvocationInspectableByAPI(
	t *testing.T,
	cliResponse factoryapi.InvocationResponse,
	session factoryapi.FactorySession,
	status factoryapi.StatusResponse,
	listed factoryapi.ListWorkResponse,
	wantPrimaryResult string,
) {
	t.Helper()

	if cliResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CLI status = %q, want COMPLETED", cliResponse.Status)
	}
	if strings.TrimSpace(cliResponse.RequestId) == "" || strings.TrimSpace(cliResponse.TraceId) == "" {
		t.Fatalf(
			"CLI invocation identity = request %q trace %q, want non-empty public correlation",
			cliResponse.RequestId,
			cliResponse.TraceId,
		)
	}
	cliPrimary := invocationPrimaryResultText(t, cliResponse)
	if cliPrimary != wantPrimaryResult {
		t.Fatalf("CLI primaryResult = %q, want %q", cliPrimary, wantPrimaryResult)
	}

	if strings.TrimSpace(session.Id) == "" {
		t.Fatal("GET /factory-sessions/~default returned empty session id")
	}
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
	for _, work := range listed.Results {
		if work.WorkTypeName == nil || *work.WorkTypeName != "goal" {
			continue
		}
		if support.HasWorkAtCustomerState(listed, stringValue(work.WorkId), "goal:complete") {
			return
		}
	}
	if status.Categories.Terminal > 0 {
		return
	}
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func writePackagedGoalMockWorkersConfig(t *testing.T) string {
	t.Helper()

	checkerCommand, checkerArgs := mockWorkerEchoCommand("plain")
	reviewerCommand, reviewerArgs := mockWorkerEchoCommand("accepted")
	cfg := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: "plan-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: "execute-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: "check-goal",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: checkerCommand,
					Args:    checkerArgs,
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: "review-goal",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: reviewerCommand,
					Args:    reviewerArgs,
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal packaged goal mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-goal.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write packaged goal mock-workers config: %v", err)
	}
	return path
}

func mockWorkerEchoCommand(output string) (string, []string) {
	if runtime.GOOS == "windows" {
		literal := strings.ReplaceAll(output, "'", "''")
		return "powershell.exe", []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"[Console]::Out.Write('" + literal + "')",
		}
	}
	return "/bin/echo", []string{output}
}

func waitForSessionWorkEndpoint(ctx context.Context, baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			support.DefaultSessionWorkURL(baseURL, "/work"),
			nil,
		)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}

	return fmt.Errorf("timed out waiting for GET /work on %s", baseURL)
}

func reserveLocalTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}

func isolatedHomeEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
