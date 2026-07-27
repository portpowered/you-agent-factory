package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNamedGoalInvocationParity_PositionalCLIAndAPIShareSuccessOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal invocation parity smoke")
	}

	dir := scaffoldPackagedGoalInvocationFactoryForSmoke(t)
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	goalText := fmt.Sprintf("functional-smoke-goal-parity-positional-%d", time.Now().UnixNano())
	mockWorkersPath := writeDefaultMockWorkersConfig(t)

	apiResponse := invokePackagedGoalViaAPI(t, dir, mockWorkersPath, goalText)
	cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
		t,
		dir,
		factoryPath,
		mockWorkersPath,
		nil,
		goalText,
	)
	if err != nil {
		t.Fatalf("CLI positional invocation: %v\nstderr:\n%s", err, stderr)
	}
	assertNamedGoalInvocationParity(t, apiResponse, cliResponse, packagedGoalMockWorkerAcceptedSummary)
}

func TestNamedGoalInvocationParity_StdinCLIAndAPITextShareSuccessOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal invocation parity smoke")
	}

	dir := scaffoldPackagedGoalInvocationFactoryForSmoke(t)
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	goalText := fmt.Sprintf("functional-smoke-goal-parity-stdin-%d", time.Now().UnixNano())
	mockWorkersPath := writeDefaultMockWorkersConfig(t)

	apiResponse := invokePackagedGoalViaAPI(t, dir, mockWorkersPath, goalText)
	cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
		t,
		dir,
		factoryPath,
		mockWorkersPath,
		strings.NewReader(goalText),
	)
	if err != nil {
		t.Fatalf("CLI stdin invocation: %v\nstderr:\n%s", err, stderr)
	}
	assertNamedGoalInvocationParity(t, apiResponse, cliResponse, packagedGoalMockWorkerAcceptedSummary)
}

func TestNamedGoalInvocationParity_NamedFactoryCLIAndAPIShareSuccessOutcome(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal invocation parity smoke")
	}

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(t, homeDir, publicGoal.PackagedFactoryName)

	goalText := fmt.Sprintf("functional-smoke-named-goal-parity-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)

	apiResponse := invokePackagedGoalViaAPI(t, factoryDir, mockWorkersPath, goalText)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	unrelatedWorkingDir := t.TempDir()
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"run",
		"--named", publicGoal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = unrelatedWorkingDir
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", publicGoal.PackagedFactoryName, err, stdout.String(), stderr.String())
	}

	var cliResponse factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &cliResponse); err != nil {
		t.Fatalf("decode CLI invocation response: %v\nstdout:\n%s", err, stdout.String())
	}
	assertNamedGoalInvocationParity(t, apiResponse, cliResponse, packagedGoalMockWorkerAcceptedSummary)
}

func TestNamedGoalInvocationParity_EmptyInputRejectedWithStableCode(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal invocation parity smoke")
	}

	dir := scaffoldPackagedGoalInvocationFactoryForSmoke(t)
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)

	apiErr := postPackagedGoalInvocationExpectError(t, dir, mockWorkersPath, "   ")
	if string(apiErr.Code) != "INVOCATION_INPUT_EMPTY" {
		t.Fatalf("API error code = %q, want INVOCATION_INPUT_EMPTY", apiErr.Code)
	}

	_, _, stderr, err := runPackagedGoalInvocationCLIJSON(
		t,
		dir,
		factoryPath,
		mockWorkersPath,
		nil,
		"   ",
	)
	if err == nil {
		t.Fatal("expected CLI empty invocation input to fail")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") && !strings.Contains(stderr, "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("CLI failure = %v\nstderr = %q, want INVOCATION_INPUT_EMPTY", err, stderr)
	}
}

func TestNamedGoalInvocationParity_UnresolvedPrimaryResultReportsStableFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal invocation parity smoke")
	}

	dir := scaffoldPackagedGoalInvocationFactoryForSmoke(t)
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	goalText := fmt.Sprintf("functional-smoke-goal-parity-unresolved-%d", time.Now().UnixNano())
	mockWorkersPath := writePackagedGoalUnresolvedMockWorkersConfig(t)

	apiResponse := invokePackagedGoalViaAPI(t, dir, mockWorkersPath, goalText)
	if apiResponse.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("API status = %q, want FAILED", apiResponse.Status)
	}
	if apiResponse.ErrorCode == nil || *apiResponse.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
		t.Fatalf("API errorCode = %#v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED", apiResponse.ErrorCode)
	}
	if apiResponse.PrimaryResult != nil {
		t.Fatalf("API primaryResult = %#v, want nil on unresolved output", apiResponse.PrimaryResult)
	}

	cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
		t,
		dir,
		factoryPath,
		mockWorkersPath,
		nil,
		goalText,
	)
	if err == nil {
		t.Fatal("expected CLI unresolved invocation primary result to fail")
	}
	if !strings.Contains(err.Error(), "INVOCATION_PRIMARY_RESULT_UNRESOLVED") &&
		!strings.Contains(stderr, "INVOCATION_PRIMARY_RESULT_UNRESOLVED") {
		t.Fatalf("CLI failure = %v\nstderr = %q, want INVOCATION_PRIMARY_RESULT_UNRESOLVED", err, stderr)
	}
	if cliResponse.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("CLI status = %q, want FAILED", cliResponse.Status)
	}
	if cliResponse.ErrorCode == nil || *cliResponse.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
		t.Fatalf("CLI errorCode = %#v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED", cliResponse.ErrorCode)
	}
	if cliResponse.PrimaryResult != nil {
		t.Fatalf("CLI primaryResult = %#v, want nil on unresolved output", cliResponse.PrimaryResult)
	}
}

func TestNamedGoalInvocationParity_SourceConflictRejectedBeforeInvocation(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal invocation parity smoke")
	}

	dir := scaffoldPackagedGoalInvocationFactoryForSmoke(t)
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	conflictMessage := "invocation input sources conflict: positional_text, stdin_text"

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		mockWorkersPath,
		"from positional",
	)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("from stdin")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("expected conflicting positional and stdin invocation inputs to fail")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on conflict failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatalf("stderr = %q, want stable conflict code", stderr.String())
	}
	if !strings.Contains(stderr.String(), conflictMessage) {
		t.Fatalf("stderr = %q, want conflict detail %q", stderr.String(), conflictMessage)
	}
}

func invokePackagedGoalViaAPI(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()
	return postPackagedGoalInvocation(t, factoryDir, mockWorkersPath, textInvocationRequestBody(goalText))
}

func postPackagedGoalInvocation(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	body []byte,
) factoryapi.InvocationResponse {
	t.Helper()

	server := startPackagedGoalParityAPIServer(t, factoryDir, mockWorkersPath)
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want 200: %s", response.StatusCode, string(payload))
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func postPackagedGoalInvocationExpectError(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	goalText string,
) factoryapi.ErrorResponse {
	t.Helper()

	body := textInvocationRequestBody(goalText)

	server := startPackagedGoalParityAPIServer(t, factoryDir, mockWorkersPath)
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want 400: %s", response.StatusCode, string(payload))
	}

	var decoded factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation error response: %v", err)
	}
	return decoded
}

func startPackagedGoalParityAPIServer(t *testing.T, factoryDir, mockWorkersPath string) *support.FunctionalAPIServer {
	t.Helper()

	payload, err := os.ReadFile(mockWorkersPath)
	if err != nil {
		t.Fatalf("read customer mock-workers config: %v", err)
	}
	var mockWorkersConfig workers.MockWorkersConfig
	if err := json.Unmarshal(payload, &mockWorkersConfig); err != nil {
		t.Fatalf("decode customer mock-workers config: %v", err)
	}

	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		MockWorkersConfig:         &mockWorkersConfig,
	})
}

func textInvocationRequestBody(goalText string) []byte {
	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: invocationTextSourceKindPtr(),
		Content:    invocationTextContentPtr(goalText),
	})
	if err != nil {
		panic(fmt.Sprintf("marshal invocation request: %v", err))
	}
	return body
}

func invocationTextSourceKindPtr() *factoryapi.InvocationInputSourceKind {
	sourceKind := factoryapi.InvocationInputSourceKindText
	return &sourceKind
}

func invocationTextContentPtr(goalText string) *factoryapi.WorkContent {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: goalText,
	}); err != nil {
		panic(fmt.Sprintf("build invocation text content: %v", err))
	}
	content := factoryapi.WorkContent{part}
	return &content
}

func runPackagedGoalInvocationCLIJSON(
	t *testing.T,
	dir string,
	factoryPath string,
	mockWorkersPath string,
	stdin io.Reader,
	args ...string,
) (factoryapi.InvocationResponse, string, string, error) {
	t.Helper()

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmdArgs := []string{
		"--json",
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		mockWorkersPath,
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, binaryPath, cmdArgs...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = stdin
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	var response factoryapi.InvocationResponse
	if stdout.Len() > 0 {
		if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &response); err != nil {
			t.Fatalf("decode CLI invocation response: %v\nstdout:\n%s", err, stdout.String())
		}
	}
	return response, stdout.String(), stderr.String(), runErr
}

func assertNamedGoalInvocationParity(
	t *testing.T,
	apiResponse factoryapi.InvocationResponse,
	cliResponse factoryapi.InvocationResponse,
	wantPrimaryResult string,
) {
	t.Helper()

	if apiResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("API status = %q, want COMPLETED", apiResponse.Status)
	}
	if cliResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CLI status = %q, want COMPLETED", cliResponse.Status)
	}
	if strings.TrimSpace(apiResponse.RequestId) == "" || strings.TrimSpace(apiResponse.TraceId) == "" {
		t.Fatalf("API submission identity = request %q trace %q, want non-empty invocation scope", apiResponse.RequestId, apiResponse.TraceId)
	}
	if strings.TrimSpace(cliResponse.RequestId) == "" || strings.TrimSpace(cliResponse.TraceId) == "" {
		t.Fatalf("CLI submission identity = request %q trace %q, want non-empty invocation scope", cliResponse.RequestId, cliResponse.TraceId)
	}

	apiText := invocationPrimaryResultText(t, apiResponse)
	cliText := invocationPrimaryResultText(t, cliResponse)
	if apiText != wantPrimaryResult {
		t.Fatalf("API primaryResult = %q, want %q", apiText, wantPrimaryResult)
	}
	if cliText != wantPrimaryResult {
		t.Fatalf("CLI primaryResult = %q, want %q", cliText, wantPrimaryResult)
	}
	if apiText != cliText {
		t.Fatalf("primaryResult mismatch: API = %q, CLI = %q", apiText, cliText)
	}
	if apiResponse.PrimaryResult == nil || cliResponse.PrimaryResult == nil {
		t.Fatal("expected both API and CLI success responses to include primaryResult")
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

func writePackagedGoalUnresolvedMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-executor",
				WorkstationName: publicGoal.PackagedExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"goal output without stop token"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal unresolved packaged goal mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-goal-unresolved.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write unresolved packaged goal mock-workers config: %v", err)
	}
	return path
}
