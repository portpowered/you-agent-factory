package goal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalFactoryName                 = "@you/goal"
	packagedGoalMockWorkerAcceptedSummary   = "mock worker accepted"
	packagedGoalRejectThenCompleteSummary   = "finished after rejection"
	packagedGoalContinueThenCompleteSummary = "finished after continue"
)

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
	modelSelectors   []string
}

func (runner *packagedGoalRepeatingProviderRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.modelSelectors = append(runner.modelSelectors, packagedGoalModelSelector(request.Args))
	runner.mu.Unlock()
	return platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(runner.decisionEnvelope),
	}, nil
}

func (runner *packagedGoalRepeatingProviderRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *packagedGoalRepeatingProviderRunner) ModelSelectors() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.modelSelectors...)
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

func packagedGoalWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func runPackagedGoalQuietCLIBatch(
	t *testing.T,
	providerRunner *packagedGoalRepeatingProviderRunner,
	goalText string,
) (stdout string, stderr string) {
	t.Helper()

	process, inputs, model := newPackagedGoalQuietCLI(t, providerRunner, goalText)

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(packaged Goal quiet batch) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	assertPackagedGoalProviderModel(t, providerRunner, model)
	return inputs.Stdout(), inputs.Stderr()
}

func runPackagedGoalQuietCLIBatchWithTimeout(
	t *testing.T,
	providerRunner *packagedGoalRepeatingProviderRunner,
	goalText string,
	timeout time.Duration,
) error {
	t.Helper()

	process, inputs, model := newPackagedGoalQuietCLI(t, providerRunner, goalText)
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	inputs.Input.Context = ctx
	err := process.Execute(inputs.Input)
	if err == nil {
		assertPackagedGoalProviderModel(t, providerRunner, model)
	}
	return err
}

func newPackagedGoalQuietCLI(
	t *testing.T,
	providerRunner platformprocess.CommandRunner,
	goalText string,
) (support.ApplicationProcess, *support.CapturedInputs, string) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	environment := packagedGoalEnvironment(homeDir)
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
	})
	support.CleanupProcess(t, process)
	support.InstallPackagedFactoryWithProcess(
		t, process, environment, workingDirectory, packagedGoalFactoryName,
	)
	model := nextPackagedGoalSelector("isolated-cli")
	args := []string{
		"you", "run",
		"--named", packagedGoalFactoryName,
		"--no-record",
		"--quiet",
		"--provider", "CODEX", "--model", model,
		goalText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = workingDirectory
	return process, inputs, model
}

func assertPackagedGoalProviderModel(
	t *testing.T,
	providerRunner *packagedGoalRepeatingProviderRunner,
	wantModel string,
) {
	t.Helper()
	selectors := providerRunner.ModelSelectors()
	if len(selectors) == 0 {
		t.Fatal("isolated Goal provider received no model selector")
	}
	for index, selector := range selectors {
		if selector != wantModel {
			t.Fatalf("isolated Goal provider selector[%d] = %q, want unique %q", index, selector, wantModel)
		}
	}
}
