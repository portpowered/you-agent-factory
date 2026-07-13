package mcpcli_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const simpleValidWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

// pkgmaintcheck:ignore-cyclomatic-complexity install smoke keeps discovery, validate, async start, and polling assertions on one documented stdio path.
func TestRunServe_InstallSmoke_DiscoveryValidateAsyncPoll(t *testing.T) {
	service := newContractFixtureService(t)
	projectRoot := writeValidWorkflowFixture(t)

	client, stdinWrite, serveErr := startRunServeSmokeServer(t, service)
	assertInstallSmokeInitialize(t, client)
	assertInstallSmokeDiscovery(t, client)
	assertInstallSmokeValidateSuccess(t, client, projectRoot)
	sessionID := assertInstallSmokeAsyncStart(t, client)
	assertInstallSmokeRunningPoll(t, client, sessionID)
	closeRunServeSmokeServer(t, stdinWrite, serveErr)
}

func startRunServeSmokeServer(
	t *testing.T,
	service *factorysessionexecution.FakeService,
) (*stdioMCPClient, *os.File, <-chan error) {
	t.Helper()
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- mcpcli.RunServe(ctx, mcpcli.ServeConfig{
			Service: service,
			Stdin:   stdinRead,
			Stdout:  stdoutWrite,
		})
	}()
	return newStdioMCPClient(t, stdinWrite, stdoutRead), stdinWrite, serveErr
}

func assertInstallSmokeInitialize(t *testing.T, client *stdioMCPClient) {
	t.Helper()
	initResult := client.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "install-smoke", "version": "test"},
	})
	if initResult.Error != nil {
		t.Fatalf("initialize error = %#v", initResult.Error)
	}
	protocolVersion, _ := initResult.Result["protocolVersion"].(string)
	if protocolVersion != "2024-11-05" {
		t.Fatalf("protocolVersion = %q, want 2024-11-05", protocolVersion)
	}
}

func assertInstallSmokeDiscovery(t *testing.T, client *stdioMCPClient) {
	t.Helper()
	toolsResult := client.call("tools/list", map[string]any{})
	toolNames := toolNamesFromListResult(t, toolsResult.Result)
	for _, want := range []string{
		mcpfactorysession.ToolValidateSource,
		mcpfactorysession.ToolStartAsync,
		mcpfactorysession.ToolGetSession,
		mcpfactorysession.ToolGetResult,
	} {
		if !slices.Contains(toolNames, want) {
			t.Fatalf("tools/list missing %q; got %#v", want, toolNames)
		}
	}
}

func assertInstallSmokeValidateSuccess(t *testing.T, client *stdioMCPClient, projectRoot string) {
	t.Helper()
	validateResponse := decodeToolResponse[factoryapi.FactoryPreviewResult](
		t,
		client.callTool(mcpfactorysession.ToolValidateSource, workflowNamePreviewRequest(projectRoot, "review")),
	)
	if validateResponse.Error != nil || validateResponse.Result == nil || !validateResponse.Result.Valid {
		t.Fatalf("validate_source = %#v, want valid success", validateResponse)
	}
}

func assertInstallSmokeAsyncStart(t *testing.T, client *stdioMCPClient) string {
	t.Helper()
	asyncStart := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, asyncRunningExecutionRequest()),
	)
	if asyncStart.Error != nil || asyncStart.Result == nil {
		t.Fatalf("start_async = %#v, want success", asyncStart)
	}
	if asyncStart.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("start_async status = %q, want RUNNING", asyncStart.Result.Status)
	}
	return asyncStart.Result.SessionId
}

func assertInstallSmokeRunningPoll(t *testing.T, client *stdioMCPClient, sessionID string) {
	t.Helper()
	runningStatus := decodeToolResponse[factoryapi.FactorySessionDurableReadModel](
		t,
		client.callTool(mcpfactorysession.ToolGetSession, map[string]any{"sessionId": sessionID}),
	)
	if runningStatus.Error != nil || runningStatus.Result == nil {
		t.Fatalf("get running = %#v, want success", runningStatus)
	}
	if runningStatus.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("get status = %q, want RUNNING", runningStatus.Result.Status)
	}

	notReady := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      factoryapi.FactorySessionResultModeFinal,
		}),
	)
	if notReady.Result != nil {
		t.Fatalf("get_result running = %#v, want not-ready envelope", notReady.Result)
	}
	if notReady.Error == nil || notReady.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("get_result error = %#v, want factory_session.result.not_ready", notReady.Error)
	}
}

func closeRunServeSmokeServer(t *testing.T, stdinWrite *os.File, serveErr <-chan error) {
	t.Helper()
	if stdinWrite != nil {
		_ = stdinWrite.Close()
	}
	select {
	case err := <-serveErr:
		if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "file already closed") {
			t.Fatalf("RunServe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunServe did not shut down after stdin closed")
	}
}

type stdioMCPClient struct {
	t      *testing.T
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int
}

func newStdioMCPClient(t *testing.T, stdin io.WriteCloser, stdout io.Reader) *stdioMCPClient {
	t.Helper()
	return &stdioMCPClient{t: t, stdin: stdin, stdout: bufio.NewReader(stdout)}
}

type mcpJSONRPCResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Result  map[string]any `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *stdioMCPClient) call(method string, params any) mcpJSONRPCResponse {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		c.t.Fatalf("marshal %s request: %v", method, err)
	}
	if _, err := c.stdin.Write(append(payload, '\n')); err != nil {
		c.t.Fatalf("write %s request: %v", method, err)
	}
	line, err := c.stdout.ReadString('\n')
	if err != nil {
		c.t.Fatalf("read %s response: %v", method, err)
	}
	var response mcpJSONRPCResponse
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		c.t.Fatalf("unmarshal %s response: %v", method, err)
	}
	if response.ID != id {
		c.t.Fatalf("%s response id = %d, want %d", method, response.ID, id)
	}
	return response
}

func (c *stdioMCPClient) callTool(name string, arguments any) mcpJSONRPCResponse {
	c.t.Helper()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		c.t.Fatalf("marshal tool arguments: %v", err)
	}
	return c.call("tools/call", map[string]any{
		"name":      name,
		"arguments": json.RawMessage(encoded),
	})
}

func toolNamesFromListResult(t *testing.T, result map[string]any) []string {
	t.Helper()
	rawTools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result missing tools array: %#v", result)
	}
	names := make([]string, 0, len(rawTools))
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("tool entry = %#v, want object", raw)
		}
		name, _ := tool["name"].(string)
		names = append(names, name)
	}
	return names
}

func decodeToolResponse[T any](t *testing.T, response mcpJSONRPCResponse) mcpfactorysession.ToolResponse[T] {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("tools/call protocol error = %#v", response.Error)
	}
	content, ok := response.Result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("tools/call result missing content: %#v", response.Result)
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("tools/call content[0] = %#v, want object", content[0])
	}
	text, _ := first["text"].(string)
	var toolResponse mcpfactorysession.ToolResponse[T]
	if err := json.Unmarshal([]byte(text), &toolResponse); err != nil {
		t.Fatalf("unmarshal tool response: %v", err)
	}
	return toolResponse
}

func newContractFixtureService(t *testing.T) *factorysessionexecution.FakeService {
	t.Helper()
	path := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(path)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func writeValidWorkflowFixture(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(simpleValidWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func workflowNamePreviewRequest(projectRoot, workflowName string) factoryapi.FactoryPreviewRequest {
	projectRootPtr := projectRoot
	sourceValue := workflowName
	return factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	}
}

func asyncRunningExecutionRequest() factoryapi.FactorySessionExecutionRequest {
	factoryID := "customer-support-triage"
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-run-n-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: &factoryID,
		},
	}
}

func TestRunServe_InstallSmoke_RejectsMissingFixtureCatalogWithoutInjectedService(t *testing.T) {
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	err = mcpcli.RunServe(context.Background(), mcpcli.ServeConfig{
		FixtureCatalogPath: filepath.Join(t.TempDir(), "missing-fixtures.json"),
		Stdout:             stdoutWrite,
		Stdin:              stdinRead,
	})
	if err == nil {
		t.Fatal("RunServe: expected fixture catalog load failure")
	}
	if !strings.Contains(err.Error(), "fixture catalog") && !strings.Contains(err.Error(), "missing-fixtures.json") {
		t.Fatalf("RunServe error = %q, want fixture catalog failure", err)
	}
}
