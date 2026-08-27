package quorum

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedQuorumBranchAWorkstation = "run-quorum-branch-a"
	packagedQuorumBranchBWorkstation = "run-quorum-branch-b"
	packagedQuorumMergeWorkstation   = "merge-quorum"
)

type packagedQuorumCommandRunner struct {
	mu                      sync.Mutex
	callCounts              map[string]int
	requests                map[string]platformprocess.CommandRequest
	capturedMergePromptText string
}

func newPackagedQuorumCommandRunner() *packagedQuorumCommandRunner {
	return &packagedQuorumCommandRunner{
		callCounts: make(map[string]int),
		requests:   make(map[string]platformprocess.CommandRequest),
	}
}

func newPackagedQuorumBranchBFailingCommandRunner() *packagedQuorumBranchBFailingCommandRunner {
	return &packagedQuorumBranchBFailingCommandRunner{
		callCounts: make(map[string]int),
	}
}

type packagedQuorumBranchBFailingCommandRunner struct {
	mu         sync.Mutex
	callCounts map[string]int
}

func (runner *packagedQuorumBranchBFailingCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	lane := packagedQuorumRequestLane(request)
	runner.mu.Lock()
	runner.callCounts[lane]++
	runner.mu.Unlock()

	switch lane {
	case packagedQuorumBranchAWorkstation:
		return packagedQuorumCodexResult("branch A COMPLETE"), nil
	case packagedQuorumBranchBWorkstation:
		return platformprocess.CommandResult{}, errors.New("packaged quorum branch B provider failure")
	default:
		return platformprocess.CommandResult{}, nil
	}
}

func (runner *packagedQuorumBranchBFailingCommandRunner) callCount(workstation string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.callCounts[workstation]
}

func (runner *packagedQuorumCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	lane := packagedQuorumRequestLane(request)
	runner.mu.Lock()
	runner.callCounts[lane]++
	runner.requests[lane] = request
	runner.mu.Unlock()

	switch lane {
	case packagedQuorumBranchAWorkstation:
		return packagedQuorumCodexResult("branch A COMPLETE"), nil
	case packagedQuorumBranchBWorkstation:
		return packagedQuorumCodexResult("branch B COMPLETE"), nil
	case packagedQuorumMergeWorkstation:
		prompt := packagedQuorumCommandPrompt(request)
		runner.mu.Lock()
		runner.capturedMergePromptText = prompt
		runner.mu.Unlock()
		return packagedQuorumCodexResult("merged quorum response:\n" + prompt + "\nCOMPLETE"), nil
	default:
		return platformprocess.CommandResult{}, nil
	}
}

func packagedQuorumCodexResult(result string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(result)}
}

func packagedQuorumCommandPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	if len(request.Args) > 0 {
		return request.Args[len(request.Args)-1]
	}
	return ""
}

func packagedQuorumRequestLane(request platformprocess.CommandRequest) string {
	prompt := packagedQuorumCommandPrompt(request)
	switch {
	case strings.Contains(prompt, "Produce branch A's independent solution"):
		return packagedQuorumBranchAWorkstation
	case strings.Contains(prompt, "Produce branch B's independent solution"):
		return packagedQuorumBranchBWorkstation
	case strings.Contains(prompt, "synthesize one complete customer-facing response"):
		return packagedQuorumMergeWorkstation
	default:
		return "unknown"
	}
}

func (runner *packagedQuorumCommandRunner) callCount(workstation string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.callCounts[workstation]
}

func (runner *packagedQuorumCommandRunner) capturedMergePrompt() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.capturedMergePromptText
}

func (runner *packagedQuorumCommandRunner) assertProviderModel(
	t *testing.T,
	workstation, provider, model string,
) {
	t.Helper()

	runner.mu.Lock()
	request, ok := runner.requests[workstation]
	runner.mu.Unlock()
	if !ok {
		t.Fatalf("no command request for %s", workstation)
	}
	if request.Command != provider || !packagedQuorumContainsArgumentPair(request.Args, "--model", model) {
		t.Fatalf(
			"%s command = %q %#v, want %s provider with model %q",
			workstation,
			request.Command,
			request.Args,
			provider,
			model,
		)
	}
}

func packagedQuorumContainsArgumentPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func runPackagedQuorumInvocation(
	t *testing.T,
	scenario *packagedQuorumScenario,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()
	payload, err := json.Marshal(factoryapi.InvocationRequest{
		RequestId: requestIDForQuorumInvocation(),
		Args:      &args,
	})
	if err != nil {
		t.Fatalf("marshal quorum invocation: %v", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST quorum invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST quorum invocation status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode quorum invocation: %v", err)
	}
	return decoded
}

func requestIDForQuorumInvocation() *string {
	requestID := fmt.Sprintf("packaged-quorum-%d", time.Now().UnixNano())
	return &requestID
}

func assertPackagedQuorumInsufficientSuccessfulMembersFailed(
	t *testing.T,
	response factoryapi.InvocationResponse,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult != nil {
		t.Fatalf(
			"primary result = %#v, want no completed success primary result after insufficient successful members",
			response.PrimaryResult,
		)
	}
	if response.WorkState == nil || !strings.Contains(*response.WorkState, ":failed") {
		t.Fatalf("invocation workState = %#v, want a failed quorum member state", response.WorkState)
	}
	if response.ErrorCode == nil || strings.TrimSpace(string(*response.ErrorCode)) == "" {
		t.Fatalf("errorCode = %#v, want stable public failure code", response.ErrorCode)
	}
	if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
		t.Fatalf("message = %#v, want stable public failure message", response.Message)
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

func assertMergedQuorumPrimaryResult(t *testing.T, result, originalRequest string) {
	t.Helper()
	assertPromptIncludes(
		t,
		result,
		"Original request:\n",
		originalRequest,
		"Branch A output:\n",
		"branch A",
		"Branch B output:\n",
		"branch B",
	)
}

func assertPromptIncludes(t *testing.T, text string, values ...string) {
	t.Helper()
	lastIndex := 0
	for _, value := range values {
		nextIndex := strings.Index(text[lastIndex:], value)
		if nextIndex < 0 {
			t.Fatalf("text = %q, missing %q", text, value)
		}
		lastIndex += nextIndex + len(value)
	}
}

func packagedQuorumRequestText(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("functional packaged quorum required input %d", time.Now().UnixNano())
}

type packagedQuorumGatedCommandRunner struct {
	mu             sync.Mutex
	requests       map[string]platformprocess.CommandRequest
	callCounts     map[string]int
	mergePrompt    string
	startedA       chan struct{}
	startedB       chan struct{}
	startedAOnce   sync.Once
	startedBOnce   sync.Once
	releaseBranchB chan struct{}
}

func newPackagedQuorumGatedCommandRunner() *packagedQuorumGatedCommandRunner {
	return &packagedQuorumGatedCommandRunner{
		requests:       make(map[string]platformprocess.CommandRequest),
		callCounts:     make(map[string]int),
		startedA:       make(chan struct{}),
		startedB:       make(chan struct{}),
		releaseBranchB: make(chan struct{}),
	}
}

func (runner *packagedQuorumGatedCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	lane := packagedQuorumRequestLane(request)
	runner.mu.Lock()
	runner.requests[lane] = request
	runner.callCounts[lane]++
	runner.mu.Unlock()

	switch lane {
	case packagedQuorumBranchAWorkstation:
		runner.startedAOnce.Do(func() { close(runner.startedA) })
		return packagedQuorumCodexResult("branch A COMPLETE"), nil
	case packagedQuorumBranchBWorkstation:
		runner.startedBOnce.Do(func() { close(runner.startedB) })
		select {
		case <-runner.releaseBranchB:
			return packagedQuorumCodexResult("branch B COMPLETE"), nil
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	case packagedQuorumMergeWorkstation:
		prompt := packagedQuorumCommandPrompt(request)
		runner.mu.Lock()
		runner.mergePrompt = prompt
		runner.mu.Unlock()
		return packagedQuorumCodexResult("merged quorum response:\n" + prompt + "\nCOMPLETE"), nil
	default:
		return platformprocess.CommandResult{}, nil
	}
}

func (runner *packagedQuorumGatedCommandRunner) capturedMergePrompt() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.mergePrompt
}

func (runner *packagedQuorumGatedCommandRunner) callCount(workstation string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.callCounts[workstation]
}

func (runner *packagedQuorumGatedCommandRunner) waitForBranchStarts(t *testing.T) {
	t.Helper()
	for _, started := range []<-chan struct{}{runner.startedA, runner.startedB} {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for both quorum branches to start")
		}
	}
}
