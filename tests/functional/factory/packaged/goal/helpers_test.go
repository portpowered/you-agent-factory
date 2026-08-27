package goal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalFactoryName                 = "@you/goal"
	packagedGoalPlanWorkstationName         = "plan-goal"
	packagedGoalExecuteWorkstationName      = "execute-goal"
	packagedGoalCheckWorkstationName        = "check-goal"
	packagedGoalReviewWorkstationName       = "review-goal"
	packagedGoalMockWorkerAcceptedSummary   = "mock worker accepted"
	packagedGoalRejectThenCompleteSummary   = "finished after rejection"
	packagedGoalContinueThenCompleteSummary = "finished after continue"
)

func scaffoldPackagedGoalBuiltInFactory(t *testing.T) string {
	t.Helper()
	dir := support.InstallPackagedFactory(t, t.TempDir(), packagedGoalFactoryName)
	if _, err := support.LoadedFactory(t, dir); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	return dir
}

// newPackagedGoalAcceptedProviderRunner returns a Codex-shaped command runner
// that accepts one executor dispatch with the stable accepted decision.
func newPackagedGoalAcceptedProviderRunner(t *testing.T) *packagedGoalRepeatingProviderRunner {
	t.Helper()
	return &packagedGoalRepeatingProviderRunner{decisionEnvelope: goalDecisionEnvelope(
		"accepted", "", packagedGoalMockWorkerAcceptedSummary,
	)}
}

// newPackagedGoalFailingProviderRunner returns the controlled provider failure
// used by the runtime-failure child while keeping the real Workers boundary.
func newPackagedGoalFailingProviderRunner(t *testing.T) *support.ShapedProviderCommandRunner {
	t.Helper()
	return support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte("mock provider failure"),
	})
}

type packagedGoalRepeatingProviderRunner struct {
	mu               sync.Mutex
	decisionEnvelope string
	calls            int
}

func (runner *packagedGoalRepeatingProviderRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls++
	return platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(runner.decisionEnvelope),
	}, nil
}

func (runner *packagedGoalRepeatingProviderRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func writePackagedGoalBuiltinMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: packagedGoalPlanWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: packagedGoalExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: packagedGoalCheckWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"plain"},
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: packagedGoalReviewWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"accepted"},
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

func invokePackagedGoalWithProviderRunner(
	t *testing.T,
	factoryDir string,
	runner platformprocess.CommandRunner,
	goalText string,
) (*support.FunctionalAPIServer, factoryapi.InvocationResponse) {
	t.Helper()

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	return server, postPackagedGoalInvocationToServer(t, server, goalText)
}

func postPackagedGoalInvocationToServer(
	t *testing.T,
	server *support.FunctionalAPIServer,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()

	body := textInvocationRequestBody(goalText)
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

func goalDecisionEnvelope(decision, feedback, output string) string {
	payload, err := json.Marshal(map[string]string{
		"decision": decision,
		"feedback": feedback,
		"output":   output,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal goal decision envelope: %v", err))
	}
	return string(payload)
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

func assertPackagedGoalInvocationFailedWithRuntimeDetails(t *testing.T, response factoryapi.InvocationResponse) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_RUNTIME_FAILURE", response.ErrorCode)
	}
	if response.Message == nil || !strings.Contains(*response.Message, "invocation failed") || !strings.Contains(*response.Message, `state "goal:failed"`) {
		t.Fatalf("invocation message = %#v, want failed goal explanation", response.Message)
	}
	if response.WorkState == nil || *response.WorkState != "goal:failed" {
		t.Fatalf("invocation workState = %#v, want goal:failed", response.WorkState)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on failed output", response.PrimaryResult)
	}
}

func assertPackagedGoalCompletedWithSummary(
	t *testing.T,
	response factoryapi.InvocationResponse,
	wantSummary string,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if got := primaryResultText(t, response); got != wantSummary {
		t.Fatalf("primaryResult text = %q, want %q", got, wantSummary)
	}
}

func primaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func startPackagedGoalSessionServer(t *testing.T, factoryDir string) *support.FunctionalAPIServer {
	t.Helper()

	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
}

func postPackagedGoalJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()

	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("%s: marshal request: %v", failurePrefix, err)
		}
		body = bytes.NewReader(encoded)
	}
	resp, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, resp.StatusCode, string(payload))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return out
}

func submitPackagedGoalWork(t *testing.T, baseURL, name, text string) factoryapi.SubmitWorkResponse {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"name":         name,
		"workTypeName": "goal",
		"items": []map[string]any{{
			"type": "text",
			"text": text,
		}},
	})
	if err != nil {
		t.Fatalf("marshal packaged goal submit request: %v", err)
	}
	resp, err := http.Post(support.DefaultSessionWorkURL(baseURL, "/work"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work goal submit: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work goal submit status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode goal submit response: %v", err)
	}
	if strings.TrimSpace(support.StringPointerValue(submitted.WorkId)) == "" {
		t.Fatalf("goal submit response = %#v, want work id", submitted)
	}
	return submitted
}

func packagedGoalWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func runPackagedGoalQuietCLIBatch(
	t *testing.T,
	mockWorkersPath string,
	goalText string,
) (stdout string, stderr string) {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, packagedGoalFactoryName)

	args := []string{
		"you", "run",
		"--named", packagedGoalFactoryName,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--quiet",
		goalText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()

	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout(), inputs.Stderr()
}

func runPackagedGoalQuietCLIBatchWithTimeout(
	t *testing.T,
	mockWorkersPath string,
	goalText string,
	timeout time.Duration,
) error {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, packagedGoalFactoryName)

	args := []string{
		"you", "run",
		"--named", packagedGoalFactoryName,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--quiet",
		goalText,
	}
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()

	done := make(chan error, 1)
	go func() {
		done <- support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitForPackagedGoalWorkIDsComplete(
	t *testing.T,
	baseURL string,
	workIDs []string,
	timeout time.Duration,
) []factoryapi.Work {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		found := make(map[string]factoryapi.Work, len(want))
		for _, item := range listed.Results {
			workID := support.StringPointerValue(item.WorkId)
			if want[workID] && packagedGoalWorkStateName(item.State) == "complete" {
				found[workID] = item
			}
		}
		if len(found) == len(want) {
			items := make([]factoryapi.Work, 0, len(workIDs))
			for _, workID := range workIDs {
				items = append(items, found[workID])
			}
			return items
		}
		time.Sleep(100 * time.Millisecond)
	}
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf("timed out waiting for completed goal work IDs %v; last work response: %#v", workIDs, listed)
	return nil
}
