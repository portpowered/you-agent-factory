package named_invocation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestNamedInvocationSharedSuccess keeps all success-side named-invocation
// behavior on one immutable root process. Each scenario still owns its home,
// working directory, Factory copy, input files, and recording path.
func TestNamedInvocationSharedSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for shared named-invocation success behavior")
	}

	fixture := newNamedInvocationSuccessFixture(t)
	t.Run("factory builder list and help", func(t *testing.T) {
		scenario := fixture.newScenario(t)
		helpOutput, helpStderr := executeCustomerCommand(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			[]string{"you", "run", "--named", packagedFactoryBuilderName, "--help"},
		)
		if helpStderr != "" {
			t.Fatalf("Factory Builder help stderr = %q", helpStderr)
		}
		for _, fragment := range []string{
			"Creates and installs one validated graph or JavaScript Factory from a customer request.",
			"--factory-name",
			"--orchestrator",
			"--builder-provider",
			"--builder-model",
		} {
			if !strings.Contains(helpOutput, fragment) {
				t.Fatalf("Factory Builder help missing %q:\n%s", fragment, helpOutput)
			}
		}

		listOutput, listStderr := executeCustomerCommand(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			[]string{"you", "factory", "list"},
		)
		if listStderr != "" {
			t.Fatalf("factory list stderr = %q", listStderr)
		}
		if !strings.Contains(listOutput, packagedFactoryBuilderName) {
			t.Fatalf("factory list output missing %q:\n%s", packagedFactoryBuilderName, listOutput)
		}
		factoryDir := packagedFactoryPath(scenario.homeDir, packagedFactoryBuilderName)
		if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
			t.Fatalf("Factory Builder was not materialized for named help: %v", err)
		}
		fixture.capturePackagedFactorySources(t, scenario.homeDir)
	})

	t.Run("named goal", func(t *testing.T) {
		scenario := fixture.newScenario(t, support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult))
		fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
		stdout, stderr := executeCustomerCommand(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			[]string{"you", "run", "--named", packagedGoalFactoryName, "--no-record", "--quiet", "hermetic no-server named goal prompt"},
		)
		assertSharedInvocationResult(t, stdout, stderr, wantHermeticInvocationPrimaryResult)
		if got := scenario.provider.CallCount(); got != 1 {
			t.Fatalf("named goal provider calls = %d, want 1", got)
		}
	})

	t.Run("named subagent", func(t *testing.T) {
		requestText := "hermetic no-server named subagent prompt"
		scenario := fixture.newScenario(t, platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout(wantHermeticInvocationPrimaryResult),
		})
		fixture.copyPackagedFactory(t, scenario, packagedSubagentFactoryName)
		stdout, stderr := executeCustomerCommand(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			[]string{"you", "run", "--named", packagedSubagentFactoryName, "--no-record", "--quiet", requestText},
		)
		assertSharedInvocationResult(t, stdout, stderr, wantHermeticInvocationPrimaryResult)
		if stdout == requestText {
			t.Fatalf("stdout echoed submitted request text instead of agent response")
		}
		if got := scenario.provider.CallCount(); got != 1 {
			t.Fatalf("named subagent provider calls = %d, want 1", got)
		}
	})

	t.Run("no-signature compatibility", func(t *testing.T) {
		scenario := fixture.newScenario(t,
			support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
			support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
			support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
			support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
			support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
			support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
		)
		factoryDir := fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
		namedFactoryDir := support.CopyFactoryAsNamed(t, factoryDir, scenario.homeDir, customizedNamedGoalFactoryName)
		namedFactoryPath := filepath.Join(namedFactoryDir, "factory.json")
		support.RemoveInvocationSignatureFixture(t, namedFactoryPath)
		base := []string{"--no-record", "--quiet"}
		cases := []struct {
			name  string
			input []string
			stdin string
		}{
			{name: "positional compatibility", input: []string{"legacy positional input"}},
			{name: "stdin compatibility", input: []string{"-"}, stdin: "legacy stdin input\n"},
			{name: "signature-only syntax remains literal text", input: []string{"--mode", "fast"}},
		}
		for _, test := range cases {
			test := test
			t.Run(test.name, func(t *testing.T) {
				namedArgs := append([]string{"you", "run", "--named", customizedNamedGoalFactoryName}, base...)
				namedArgs = append(namedArgs, test.input...)
				namedStdout, namedStderr := executeCustomerCommandWithStdin(
					t, fixture.process, scenario.environment, scenario.workingDirectory, namedArgs, test.stdin,
				)
				fileArgs := append([]string{"you", "run", "--factory", namedFactoryPath}, base...)
				fileArgs = append(fileArgs, test.input...)
				fileStdout, fileStderr := executeCustomerCommandWithStdin(
					t, fixture.process, scenario.environment, scenario.workingDirectory, fileArgs, test.stdin,
				)
				if namedStderr != "" || fileStderr != "" {
					t.Fatalf("invocation stderr: named=%q file=%q", namedStderr, fileStderr)
				}
				if namedStdout != wantHermeticInvocationPrimaryResult || fileStdout != namedStdout {
					t.Fatalf("selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
				}
			})
		}
		if got := scenario.provider.CallCount(); got != len(cases)*2 {
			t.Fatalf("no-signature provider calls = %d, want %d", got, len(cases)*2)
		}
	})

	t.Run("effective signature parity", func(t *testing.T) {
		scenario := fixture.newScenario(t,
			support.CodexDecisionCommandResult("canonical provider result"),
			support.CodexDecisionCommandResult("canonical provider result"),
		)
		submissionStart := len(fixture.submissions.snapshot())
		factoryDir := fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
		namedFactoryDir := support.CopyFactoryAsNamed(t, factoryDir, scenario.homeDir, customizedNamedGoalFactoryName)
		namedFactoryPath := filepath.Join(namedFactoryDir, "factory.json")
		addEffectiveSignatureFixture(t, namedFactoryPath)
		support.ReplaceGoalWorkstationPrompt(t, namedFactoryPath, "input=${input}|format=${format}|count=${count}|document=${document}|stdin=${body}")
		documentPath := filepath.Join(scenario.workingDirectory, "story.md")
		if err := os.WriteFile(documentPath, []byte("factory invocation document"), 0o600); err != nil {
			t.Fatalf("write FILE_CONTENTS fixture: %v", err)
		}
		common := []string{
			"--no-record", "--quiet",
			"equivalent canonical prompt", "one.md", "two.md",
			"--t", "alpha", "--tag", "beta",
			"--count", "2",
			"--file", documentPath,
			"-",
		}
		namedArgs := append([]string{"you", "run", "--named", customizedNamedGoalFactoryName}, common...)
		namedStdout, namedStderr := executeCustomerCommandWithStdin(
			t, fixture.process, scenario.environment, scenario.workingDirectory, namedArgs, "canonical stdin body",
		)
		fileArgs := append([]string{"you", "run", "--factory", namedFactoryPath}, common...)
		fileStdout, fileStderr := executeCustomerCommandWithStdin(
			t, fixture.process, scenario.environment, scenario.workingDirectory, fileArgs, "canonical stdin body",
		)
		if namedStderr != "" || fileStderr != "" {
			t.Fatalf("invocation stderr: named=%q file=%q", namedStderr, fileStderr)
		}
		if namedStdout == "" || fileStdout != namedStdout {
			t.Fatalf("selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
		}

		records := fixture.submissions.snapshot()[submissionStart:]
		if len(records) != 2 {
			t.Fatalf("canonical submissions = %d, want named and explicit-file records", len(records))
		}
		if !reflect.DeepEqual(records[0].Request.InvocationArguments, records[1].Request.InvocationArguments) {
			t.Fatalf("selection canonical arguments differ: named=%#v file=%#v", records[0].Request.InvocationArguments, records[1].Request.InvocationArguments)
		}
		assertEffectiveSignatureSubmission(t, records[0].Request.InvocationArguments, documentPath)

		calls := scenario.provider.Requests()
		if len(calls) != 2 {
			t.Fatalf("provider calls = %d, want named and explicit-file calls", len(calls))
		}
		wantPrompt := "input=equivalent canonical prompt|format=json|count=2|document=factory invocation document|stdin=canonical stdin body"
		placeholders := []string{"${executorProvider}", "${executorModel}", "${input}", "${format}", "${count}", "${document}", "${body}", "${mode}"}
		for index, call := range calls {
			if call.Command != "codex" {
				t.Fatalf("provider call %d command = %q, want codex", index, call.Command)
			}
			if support.RequestContainsInterpolation(call, placeholders...) {
				t.Fatalf("provider call %d contains unresolved interpolation: %#v", index, call)
			}
			if got := string(call.Stdin); !strings.Contains(got, wantPrompt) {
				t.Fatalf("provider call %d prompt = %q, want resolved prompt fragment %q", index, got, wantPrompt)
			}
		}
	})

	t.Run("default-only input", func(t *testing.T) {
		scenario := fixture.newScenario(t,
			support.CodexDecisionCommandResult("default applied"),
			support.CodexDecisionCommandResult("default applied"),
		)
		submissionStart := len(fixture.submissions.snapshot())
		factoryDir := fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
		namedFactoryDir := support.CopyFactoryAsNamed(t, factoryDir, scenario.homeDir, customizedNamedGoalFactoryName)
		factoryPath := filepath.Join(namedFactoryDir, "factory.json")
		replaceInvocationSignatureFixture(t, factoryPath, map[string]any{
			"parameters": []any{map[string]any{
				"name": "mode", "defaultValue": "safe",
				"bindings": []any{map[string]any{"kind": "NAMED"}},
			}},
		})
		support.ReplaceGoalWorkerInstructions(t, factoryPath, "mode=${mode}")
		support.ReplaceGoalWorkstationPrompt(t, factoryPath, "mode=${mode}")

		namedStdout, namedStderr := executeCustomerCommand(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			emptyInvocationArguments("named", factoryPath, customizedNamedGoalFactoryName),
		)
		fileStdout, fileStderr := executeCustomerCommand(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			emptyInvocationArguments("file", factoryPath, customizedNamedGoalFactoryName),
		)
		if namedStderr != "" || fileStderr != "" {
			t.Fatalf("default-only invocation stderr: named=%q file=%q", namedStderr, fileStderr)
		}
		if namedStdout == "" || fileStdout != namedStdout {
			t.Fatalf("default-only selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
		}

		records := fixture.submissions.snapshot()[submissionStart:]
		if len(records) != 2 || records[0].Request.InvocationArguments == nil || records[1].Request.InvocationArguments == nil {
			t.Fatalf("default-only submissions = %#v, want two canonical requests", records)
		}
		want := workInvocationArgumentDefault("safe")
		for index, record := range records {
			if got := record.Request.InvocationArguments.Arguments["mode"]; !reflect.DeepEqual(got, want) {
				t.Fatalf("default-only argument %d = %#v, want %#v", index, got, want)
			}
		}
		for index, call := range scenario.provider.Requests() {
			if call.Command != "codex" || support.RequestContainsInterpolation(call, "${mode}") {
				t.Fatalf("default-only provider call %d = %#v, want resolved codex request", index, call)
			}
			if !strings.Contains(string(call.Stdin), "mode=safe") {
				t.Fatalf("default-only provider prompt %d = %q, want resolved default", index, call.Stdin)
			}
		}
	})

	t.Run("recorded named invocation", func(t *testing.T) {
		scenario := fixture.newScenario(t,
			support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
		)
		fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
		recordingPath := filepath.Join(scenario.rootDir, "named-invocation.recording.jsonl")
		stdout, stderr := executeCustomerCommand(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			[]string{"you", "run", "--named", packagedGoalFactoryName, "--record", recordingPath, "--quiet", "record a named invocation"},
		)
		assertSharedInvocationResult(t, stdout, stderr, wantHermeticInvocationPrimaryResult)
		payload, err := os.ReadFile(recordingPath)
		if err != nil {
			t.Fatalf("read named invocation recording: %v", err)
		}
		replay := decodeNamedInvocationReplay(t, payload)
		if replay.header.SchemaVersion != "agent-factory.replay.v2" || replay.header.SessionID == "" {
			t.Fatalf("recording header = %#v, want v2 session identity", replay.header)
		}
		if replay.terminal.TerminalState != "FINALIZED" {
			t.Fatalf("recording terminal state = %q, want FINALIZED", replay.terminal.TerminalState)
		}
		if !replay.eventTypes["DISPATCH_REQUEST"] || !replay.eventTypes["DISPATCH_RESPONSE"] {
			t.Fatalf("recording event types = %#v, want canonical dispatch request and response", replay.eventTypes)
		}
		if !replay.eventTypes["SESSION_RESULT_UPDATED"] || replay.resultStatus != "FINAL" {
			t.Fatalf("recording result projection = %q/%v, want FINAL", replay.resultStatus, replay.eventTypes["SESSION_RESULT_UPDATED"])
		}
		if replay.completedWorkID == "" || replay.completedWorkState != "TERMINAL" || replay.completedWorkName != "complete" {
			t.Fatalf("recording completed Work = %q/%q/%q, want complete terminal Work", replay.completedWorkID, replay.completedWorkName, replay.completedWorkState)
		}
		if replay.primaryResult != wantHermeticInvocationPrimaryResult {
			t.Fatalf("recording primary result = %q, want %q", replay.primaryResult, wantHermeticInvocationPrimaryResult)
		}
		if got := scenario.provider.CallCount(); got != 1 {
			t.Fatalf("recorded named provider calls = %d, want one goal worker", got)
		}
	})
}

type namedInvocationReplayLine struct {
	RecordType    string `json:"recordType"`
	SchemaVersion string `json:"schemaVersion"`
	SessionID     string `json:"sessionId"`
	Event         struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	} `json:"event"`
	TerminalState string `json:"terminalState"`
}

type namedInvocationReplay struct {
	header             namedInvocationReplayLine
	terminal           namedInvocationReplayLine
	eventTypes         map[string]bool
	resultStatus       string
	completedWorkID    string
	completedWorkName  string
	completedWorkState string
	primaryResult      string
}

func decodeNamedInvocationReplay(t *testing.T, payload []byte) namedInvocationReplay {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	replay := namedInvocationReplay{eventTypes: make(map[string]bool)}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var line namedInvocationReplayLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("decode recording line %d: %v", lineNumber, err)
		}
		switch line.RecordType {
		case "header":
			if replay.header.RecordType != "" {
				t.Fatalf("recording contains more than one header")
			}
			replay.header = line
		case "event":
			replay.eventTypes[line.Event.Type] = true
			decodeNamedInvocationReplayEvent(t, &replay, line)
		case "terminal":
			if replay.terminal.RecordType != "" {
				t.Fatalf("recording contains more than one terminal record")
			}
			replay.terminal = line
		default:
			t.Fatalf("recording line %d has unknown record type %q", lineNumber, line.RecordType)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read recording lines: %v", err)
	}
	if replay.header.RecordType == "" || replay.terminal.RecordType == "" {
		t.Fatalf("recording missing header or terminal record: %#v", replay)
	}
	return replay
}

func decodeNamedInvocationReplayEvent(t *testing.T, replay *namedInvocationReplay, line namedInvocationReplayLine) {
	t.Helper()
	switch line.Event.Type {
	case "SESSION_RESULT_UPDATED":
		var result struct {
			ResultStatus string `json:"resultStatus"`
		}
		if err := json.Unmarshal(line.Event.Payload, &result); err != nil {
			t.Fatalf("decode SESSION_RESULT_UPDATED payload: %v", err)
		}
		replay.resultStatus = result.ResultStatus
	case "DISPATCH_RESPONSE":
		var response struct {
			Output     string `json:"output"`
			OutputWork []struct {
				WorkID string `json:"workId"`
				State  struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"state"`
			} `json:"outputWork"`
		}
		if err := json.Unmarshal(line.Event.Payload, &response); err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE payload: %v", err)
		}
		replay.primaryResult = response.Output
		for _, outputWork := range response.OutputWork {
			if outputWork.WorkID == "" || outputWork.State.Name != "complete" {
				continue
			}
			replay.completedWorkID = outputWork.WorkID
			replay.completedWorkName = outputWork.State.Name
			replay.completedWorkState = outputWork.State.Type
			return
		}
	}
}

func assertSharedInvocationResult(t *testing.T, stdout, stderr, want string) {
	t.Helper()
	if stderr != "" {
		t.Fatalf("named invocation stderr = %q, want empty", stderr)
	}
	if stdout != want {
		t.Fatalf("stdout = %q, want primary result %q", stdout, want)
	}
}

func workInvocationArgumentDefault(value string) work.InvocationArgument {
	return work.InvocationArgument{
		Values:    []string{value},
		ValueMode: work.InvocationParameterValueModeExact,
		Sources:   []work.InvocationArgumentSource{{Kind: string(work.ArgumentSourceKindDefault), Name: "default"}},
	}
}

type namedInvocationSuccessFixture struct {
	process                   support.ApplicationProcess
	provider                  *namedInvocationProviderRouter
	listener                  *listenerStartObservation
	submissions               *canonicalSubmissionObservation
	packagedFactorySources    map[string]string
	packagedFactorySourceHome string
	processBuilds             atomic.Int32
}

type namedInvocationScenario struct {
	rootDir          string
	homeDir          string
	workingDirectory string
	environment      []string
	provider         *testutil.ProviderCommandRunner
}

func newNamedInvocationSuccessFixture(t *testing.T) *namedInvocationSuccessFixture {
	t.Helper()
	fixture := &namedInvocationSuccessFixture{
		provider:                  newNamedInvocationProviderRouter(),
		listener:                  &listenerStartObservation{},
		submissions:               &canonicalSubmissionObservation{},
		packagedFactorySources:    make(map[string]string),
		packagedFactorySourceHome: t.TempDir(),
	}
	process, err := fixture.buildProcess(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.listener.Start,
		ProviderCommandRunner: fixture.provider,
		SubmissionRecorder:    fixture.submissions.observe,
	})
	if err != nil {
		t.Fatalf("build shared named-invocation root process: %v", err)
	}
	fixture.process = process
	// Register this assertion before CleanupProcess so the reusable process is
	// closed before the fixture verifies its final route and listener counts.
	t.Cleanup(func() {
		if got := fixture.processBuilds.Load(); got != 1 {
			t.Errorf("shared named-invocation process constructions = %d, want 1", got)
		}
		if got := fixture.listener.calls.Load(); got != 0 {
			t.Errorf("shared named-invocation HTTP listener starts = %d, want 0", got)
		}
		if got := fixture.provider.routeCount(); got != 0 {
			t.Errorf("shared named-invocation provider routes after cleanup = %d, want 0", got)
		}
	})
	support.CleanupProcess(t, process)
	return fixture
}

func (fixture *namedInvocationSuccessFixture) buildProcess(
	ctx context.Context,
	edges serviceedges.Edges,
) (support.ApplicationProcess, error) {
	fixture.processBuilds.Add(1)
	return support.BuildProcessWithContext(ctx, edges)
}

func (fixture *namedInvocationSuccessFixture) capturePackagedFactorySources(
	t *testing.T,
	homeDir string,
) {
	t.Helper()
	for _, name := range []string{
		packagedGoalFactoryName,
		packagedSubagentFactoryName,
		packagedFactoryBuilderName,
	} {
		sourceDir := packagedFactoryPath(homeDir, name)
		if _, err := os.Stat(filepath.Join(sourceDir, "factory.json")); err != nil {
			t.Fatalf("materialized packaged Factory %q missing from source home: %v", name, err)
		}
		fixture.packagedFactorySources[name] = support.CopyFactoryAsNamed(
			t, sourceDir, fixture.packagedFactorySourceHome, name,
		)
	}
}

func (fixture *namedInvocationSuccessFixture) copyPackagedFactory(
	t *testing.T,
	scenario *namedInvocationScenario,
	name string,
) string {
	t.Helper()
	sourceDir := fixture.packagedFactorySources[name]
	if sourceDir == "" {
		t.Fatalf("packaged Factory %q has not been captured", name)
	}
	return support.CopyFactoryAsNamed(t, sourceDir, scenario.homeDir, name)
}

func packagedFactoryPath(homeDir, name string) string {
	return filepath.Join(
		append([]string{homeDir, ".you-agent-factory", "factories"}, strings.Split(name, "/")...)...,
	)
}

func (fixture *namedInvocationSuccessFixture) newScenario(
	t *testing.T,
	results ...platformprocess.CommandResult,
) *namedInvocationScenario {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	for label, path := range map[string]string{"home": homeDir, "working directory": workingDirectory} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create named-invocation %s: %v", label, err)
		}
	}
	scenario := &namedInvocationScenario{
		rootDir:          rootDir,
		homeDir:          homeDir,
		workingDirectory: workingDirectory,
		environment:      namedInvocationEnvironment(homeDir),
	}
	if len(results) == 0 {
		return scenario
	}
	scenario.provider = testutil.NewProviderCommandRunner(results...)
	if err := fixture.provider.register(workingDirectory, scenario.provider); err != nil {
		t.Fatalf("register named-invocation provider route: %v", err)
	}
	t.Cleanup(func() { fixture.provider.unregister(workingDirectory) })
	return scenario
}

func namedInvocationEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

type namedInvocationProviderRouter struct {
	mu     sync.RWMutex
	routes map[string]platformprocess.CommandRunner
}

func newNamedInvocationProviderRouter() *namedInvocationProviderRouter {
	return &namedInvocationProviderRouter{routes: make(map[string]platformprocess.CommandRunner)}
}

func (router *namedInvocationProviderRouter) register(path string, runner platformprocess.CommandRunner) error {
	path = normalizeNamedInvocationWorkDir(path)
	if path == "" || runner == nil {
		return errors.New("named-invocation provider route requires a directory and runner")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[path]; exists {
		return errors.New("named-invocation provider route is already registered")
	}
	router.routes[path] = runner
	return nil
}

func (router *namedInvocationProviderRouter) unregister(path string) {
	router.mu.Lock()
	delete(router.routes, normalizeNamedInvocationWorkDir(path))
	router.mu.Unlock()
}

func (router *namedInvocationProviderRouter) routeCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.routes)
}

func (router *namedInvocationProviderRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	router.mu.RLock()
	runner := router.routes[normalizeNamedInvocationWorkDir(request.WorkDir)]
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, errors.New("named-invocation provider route unavailable")
	}
	return runner.Run(ctx, request)
}

func normalizeNamedInvocationWorkDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err == nil {
		return cleaned
	}
	return filepath.Clean(path)
}

func TestNamedInvocationProviderRouterRejectsAmbiguousAndUnknownRoutes(t *testing.T) {
	router := newNamedInvocationProviderRouter()
	runner := testutil.NewProviderCommandRunner(support.CodexDecisionCommandResult("unused"))
	workingDirectory := t.TempDir()
	if err := router.register(workingDirectory, runner); err != nil {
		t.Fatalf("register provider route: %v", err)
	}
	if err := router.register(workingDirectory, testutil.NewProviderCommandRunner()); err == nil {
		t.Fatal("duplicate provider route error = nil, want deterministic ambiguity failure")
	}
	if _, err := router.Run(context.Background(), platformprocess.CommandRequest{WorkDir: t.TempDir()}); err == nil {
		t.Fatal("unknown provider route error = nil, want sanitized zero-match failure")
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("unknown provider route consumed %d queued results, want 0", got)
	}
}
