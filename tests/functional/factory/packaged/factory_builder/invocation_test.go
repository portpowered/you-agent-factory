package factory_builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	factoryBuilderName             = "@you/factory-builder"
	graphFactoryName               = "release-note-review"
	graphFactoryRequest            = "Review submitted release notes and return an approved summary."
	graphFactoryPrimaryReply       = "Release-note review completed."
	javascriptFactoryName          = "release-synthesis"
	javascriptFactoryRequest       = "Run two independent analyses and return one synthesized result."
	javascriptFactoryPrimaryResult = "Synthesized two independent analyses."
	factoryYAMLFile                = "factory.yaml"
)

// TestFactoryBuilderCreatesAndInstallsValidatedGraphFactory proves the public
// Builder invocation can produce a staged YAML graph that the agent validates
// and persists through the normal named-Factory CLI flow before the customer
// resolves and runs the installed Factory.
func TestFactoryBuilderCreatesAndInstallsValidatedGraphFactory(t *testing.T) {
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	runner := &factoryBuilderCommandRunner{
		targetName:      graphFactoryName,
		customerRequest: graphFactoryRequest,
		orchestrator:    "graph",
		environment:     environment,
		operatorRoot:    filepath.Join(homeDir, ".you-agent-factory", "factories"),
		candidateYAML:   representativeGraphFactoryYAML,
		builderResult:   "Factory release-note-review (graph): validation passed and installed through the named Factory create command.",
	}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	runner.process = process

	builder := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--named", factoryBuilderName, "--no-record",
		"--factory-name", graphFactoryName,
		"--orchestrator", "graph",
		"--builder-provider", "CODEX",
		"--builder-model", "gpt-5",
		graphFactoryRequest,
	})
	builder.Input.Env = environment
	builder.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(builder.Input); err != nil {
		t.Fatalf("Process.Execute(factory builder) error = %v\nstdout:\n%s\nstderr:\n%s", err, builder.Stdout(), builder.Stderr())
	}
	assertBuilderCompleted(t, support.DecodeInvocationResponseJSON(t, builder.Stdout()), graphFactoryName, "graph")
	if builder.Stderr() != "" {
		t.Fatalf("factory builder stderr = %q, want empty successful invocation stderr", builder.Stderr())
	}

	installedPath := filepath.Join(homeDir, ".you-agent-factory", "factories", graphFactoryName)
	assertBuilderStageIsWorkspaceScoped(t, workingDirectory, runner.StagePath())
	assertInstalledGraphFactory(t, process, environment, installedPath, runner.operatorRoot)
	assertInstalledGraphFactoryRuns(t, process, environment, workingDirectory, installedPath)
	if got := runner.InstalledFactoryCallCount(); got != 1 {
		t.Fatalf("installed Factory provider command call count = %d, want one customer invocation", got)
	}
}

// TestFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory proves the
// Builder validates and persists a JavaScript Factory through the same public
// named-Factory path as graph definitions before its intended child work runs.
func TestFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory(t *testing.T) {
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	runner := &factoryBuilderCommandRunner{
		targetName:      javascriptFactoryName,
		customerRequest: javascriptFactoryRequest,
		orchestrator:    "javascript",
		environment:     environment,
		operatorRoot:    filepath.Join(homeDir, ".you-agent-factory", "factories"),
		candidateYAML:   representativeJavaScriptFactoryYAML,
		builderResult:   "Factory release-synthesis (javascript): validation passed and installed through the named Factory create command.",
	}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	runner.process = process

	builder := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--named", factoryBuilderName, "--no-record",
		"--factory-name", javascriptFactoryName,
		"--orchestrator", "javascript",
		"--builder-provider", "CODEX",
		"--builder-model", "gpt-5",
		javascriptFactoryRequest,
	})
	builder.Input.Env = environment
	builder.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(builder.Input); err != nil {
		t.Fatalf("Process.Execute(factory builder) error = %v\nstdout:\n%s\nstderr:\n%s", err, builder.Stdout(), builder.Stderr())
	}
	assertBuilderCompleted(t, support.DecodeInvocationResponseJSON(t, builder.Stdout()), javascriptFactoryName, "javascript")
	if builder.Stderr() != "" {
		t.Fatalf("factory builder stderr = %q, want empty successful invocation stderr", builder.Stderr())
	}

	installedPath := filepath.Join(homeDir, ".you-agent-factory", "factories", javascriptFactoryName)
	assertBuilderStageIsWorkspaceScoped(t, workingDirectory, runner.StagePath())
	assertInstalledJavaScriptFactory(t, process, environment, installedPath, runner.operatorRoot)
	assertInstalledJavaScriptFactoryRuns(t, process, environment, workingDirectory, installedPath)
	if got := runner.InstalledFactoryCallCount(); got != 2 {
		t.Fatalf("installed Factory provider command call count = %d, want two intended analysis calls", got)
	}
}

func assertBuilderCompleted(t *testing.T, response factoryapi.InvocationResponse, factoryName, orchestrator string) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("Builder invocation status = %q, want %q; response = %#v", response.Status, factoryapi.InvocationTerminalStatusCompleted, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("Builder primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("Builder primary result as text part: %v", err)
	}
	for _, want := range []string{factoryName, orchestrator, "validation passed", "installed"} {
		if !strings.Contains(strings.ToLower(part.Text), strings.ToLower(want)) {
			t.Fatalf("Builder primary result = %q, want %q", part.Text, want)
		}
	}
}

func assertBuilderStageIsWorkspaceScoped(t *testing.T, workingDirectory, stagePath string) {
	t.Helper()
	if stagePath == "" {
		t.Fatal("Builder did not stage a candidate")
	}
	relative, err := filepath.Rel(workingDirectory, stagePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("Builder staged candidate at %q, want a workspace-relative path", stagePath)
	}
	if filepath.Base(stagePath) != factoryYAMLFile {
		t.Fatalf("staged candidate = %q, want YAML Factory definition", stagePath)
	}
}

func assertInstalledGraphFactory(
	t *testing.T,
	process support.Process,
	environment []string,
	installedPath, operatorRoot string,
) {
	t.Helper()
	installedConfig := filepath.Join(installedPath, factorydefinitions.FactoryConfigFile)
	validate := support.FakeInputs(t.Context(), []string{"you", "factory", "config", "validate", installedConfig})
	validate.Input.Env = environment
	validate.Input.WorkingDirectory = filepath.Dir(installedPath)
	if err := process.Execute(validate.Input); err != nil {
		t.Fatalf("Process.Execute(validate installed Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, validate.Stdout(), validate.Stderr())
	}

	payload, err := support.FlattenFactoryConfigWithProcessAndEnv(t, process, environment, installedPath)
	if err != nil {
		t.Fatalf("flatten installed Factory: %v", err)
	}
	var definition map[string]any
	if err := json.Unmarshal(payload, &definition); err != nil {
		t.Fatalf("decode flattened installed Factory: %v\npayload:\n%s", err, payload)
	}
	assertRepresentativeGraphDefinition(t, definition)
	assertInstalledFactoryIsListed(t, process, environment, operatorRoot, graphFactoryName)
}

func assertInstalledFactoryIsListed(t *testing.T, process support.Process, environment []string, operatorRoot, factoryName string) {
	t.Helper()
	list := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "list", "--dir", operatorRoot})
	list.Input.Env = environment
	if err := process.Execute(list.Input); err != nil {
		t.Fatalf("Process.Execute(list installed Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, list.Stdout(), list.Stderr())
	}
	if !strings.Contains(list.Stdout(), factoryName) {
		t.Fatalf("factory list output = %q, want installed Factory %q", list.Stdout(), factoryName)
	}
}

func assertInstalledJavaScriptFactory(
	t *testing.T,
	process support.Process,
	environment []string,
	installedPath, operatorRoot string,
) {
	t.Helper()
	installedConfig := filepath.Join(installedPath, factorydefinitions.FactoryConfigFile)
	validate := support.FakeInputs(t.Context(), []string{"you", "factory", "config", "validate", installedConfig})
	validate.Input.Env = environment
	validate.Input.WorkingDirectory = filepath.Dir(installedPath)
	if err := process.Execute(validate.Input); err != nil {
		t.Fatalf("Process.Execute(validate installed JavaScript Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, validate.Stdout(), validate.Stderr())
	}

	payload, err := support.FlattenFactoryConfigWithProcessAndEnv(t, process, environment, installedPath)
	if err != nil {
		t.Fatalf("flatten installed JavaScript Factory: %v", err)
	}
	var definition map[string]any
	if err := json.Unmarshal(payload, &definition); err != nil {
		t.Fatalf("decode flattened JavaScript Factory: %v\npayload:\n%s", err, payload)
	}
	assertRepresentativeJavaScriptDefinition(t, definition)
	assertInstalledFactoryIsListed(t, process, environment, operatorRoot, javascriptFactoryName)
}

func assertRepresentativeJavaScriptDefinition(t *testing.T, definition map[string]any) {
	t.Helper()
	if definition["name"] != javascriptFactoryName {
		t.Fatalf("installed JavaScript Factory name = %#v, want %q", definition["name"], javascriptFactoryName)
	}
	orchestrator, ok := definition["orchestrator"].(map[string]any)
	if !ok || orchestrator["kind"] != "JAVASCRIPT" {
		t.Fatalf("orchestrator = %#v, want JAVASCRIPT", definition["orchestrator"])
	}
	javascript, ok := orchestrator["javascript"].(map[string]any)
	if !ok {
		t.Fatalf("javascript orchestrator config = %#v, want metadata, args schema, policy, and inline source", orchestrator["javascript"])
	}
	inlineSource, ok := javascript["inlineSource"].(map[string]any)
	if !ok || inlineSource["encoding"] != "utf-8" || strings.TrimSpace(fmt.Sprint(inlineSource["inline"])) == "" {
		t.Fatalf("JavaScript inline source = %#v, want non-empty utf-8 source", javascript["inlineSource"])
	}
	argsSchema, ok := javascript["argsSchema"].(map[string]any)
	if !ok || argsSchema["type"] != "object" || !hasStringValue(argsSchema["required"], "briefs") {
		t.Fatalf("JavaScript argsSchema = %#v, want required briefs object input", javascript["argsSchema"])
	}
	defaultPolicy, ok := javascript["defaultPolicy"].(map[string]any)
	if !ok || defaultPolicy["mode"] != "READ_ONLY" || defaultPolicy["maxAgents"] != float64(2) || defaultPolicy["concurrency"] != float64(2) || defaultPolicy["allowNetwork"] != false {
		t.Fatalf("JavaScript defaultPolicy = %#v, want bounded read-only two-agent policy", javascript["defaultPolicy"])
	}
	signature, ok := definition["invocationSignature"].(map[string]any)
	if !ok || !hasInvocationParameter(signature["parameters"], "briefs") {
		t.Fatalf("invocationSignature = %#v, want required briefs input", definition["invocationSignature"])
	}
}

func hasStringValue(value any, want string) bool {
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertInstalledJavaScriptFactoryRuns(
	t *testing.T,
	process support.Process,
	environment []string,
	workingDirectory, installedPath string,
) {
	t.Helper()
	run := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", filepath.Join(installedPath, factorydefinitions.FactoryConfigFile), "--no-record",
		"--provider", "CODEX", "--model", "gpt-5",
		"--briefs", "Analyze the release plan and identify important risks.",
	})
	run.Input.Env = environment
	run.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(run.Input); err != nil {
		t.Fatalf("Process.Execute(run installed JavaScript Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, run.Stdout(), run.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, run.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("installed JavaScript Factory invocation status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("installed JavaScript Factory primaryResult = %#v, want one JSON part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("installed JavaScript Factory primary result as JSON: %v", err)
	}
	result, ok := part.Json.(map[string]any)
	if !ok || result["summary"] != javascriptFactoryPrimaryResult || result["analysisCount"] != float64(2) {
		t.Fatalf("installed JavaScript Factory primary result = %#v, want synthesized two-analysis result", part.Json)
	}
}

func assertInstalledGraphFactoryRuns(
	t *testing.T,
	process support.Process,
	environment []string,
	workingDirectory, installedPath string,
) {
	t.Helper()
	run := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", filepath.Join(installedPath, factorydefinitions.FactoryConfigFile), "--no-record",
		"--provider", "CODEX", "--model", "gpt-5",
		"Review these release notes for publication.",
	})
	run.Input.Env = environment
	run.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(run.Input); err != nil {
		t.Fatalf("Process.Execute(run installed Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, run.Stdout(), run.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, run.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("installed Factory invocation status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("installed Factory primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || part.Text != graphFactoryPrimaryReply {
		t.Fatalf("installed Factory primary result = %#v, want %q", response.PrimaryResult, graphFactoryPrimaryReply)
	}
}

func assertRepresentativeGraphDefinition(t *testing.T, definition map[string]any) {
	t.Helper()
	if definition["name"] != graphFactoryName {
		t.Fatalf("installed Factory name = %#v, want %q", definition["name"], graphFactoryName)
	}
	assertNamedDefinitionItem(t, definition["workTypes"], "release-note-review", "workTypes")
	assertNamedDefinitionItem(t, definition["workers"], "release-notes-reviewer", "workers")
	workstation := assertNamedDefinitionItem(t, definition["workstations"], "review-release-notes", "workstations")
	if workstation["worker"] != "release-notes-reviewer" || workstation["type"] != "AGENT_RUN" {
		t.Fatalf("review workstation = %#v, want agent-run release-notes-reviewer", workstation)
	}
	if !hasWorkRoute(workstation["inputs"], "release-note-review", "init") ||
		!hasWorkRoute(workstation["outputs"], "release-note-review", "complete") ||
		!hasWorkRoute(workstation["onFailure"], "release-note-review", "failed") {
		t.Fatalf("review workstation routes = %#v, want init/complete/failed release-note-review routes", workstation)
	}
	invocationReturn, ok := definition["invocationReturn"].(map[string]any)
	if !ok || invocationReturn["workTypeName"] != "release-note-review" || invocationReturn["terminalState"] != "complete" {
		t.Fatalf("invocationReturn = %#v, want release-note-review complete", definition["invocationReturn"])
	}
	signature, ok := definition["invocationSignature"].(map[string]any)
	if !ok || !hasInvocationParameter(signature["parameters"], "releaseNotes") {
		t.Fatalf("invocationSignature = %#v, want required releaseNotes input", definition["invocationSignature"])
	}
}

func assertNamedDefinitionItem(t *testing.T, value any, name, kind string) map[string]any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want a list", kind, value)
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if ok && item["name"] == name {
			return item
		}
	}
	t.Fatalf("%s = %#v, want item %q", kind, value, name)
	return nil
}

func hasWorkRoute(value any, workType, state string) bool {
	routes, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range routes {
		route, ok := raw.(map[string]any)
		if ok && route["workType"] == workType && route["state"] == state {
			return true
		}
	}
	return false
}

func hasInvocationParameter(value any, name string) bool {
	parameters, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range parameters {
		parameter, ok := raw.(map[string]any)
		if ok && parameter["name"] == name && parameter["required"] == true {
			return true
		}
	}
	return false
}

type factoryBuilderCommandRunner struct {
	process         support.Process
	targetName      string
	customerRequest string
	orchestrator    string
	environment     []string
	operatorRoot    string
	candidateYAML   string
	builderResult   string

	mu                           sync.Mutex
	stagePath                    string
	builderPrepared              bool
	installedFactoryCommandCalls int
}

func (runner *factoryBuilderCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	builderRequest := strings.Contains(string(request.Stdin), "You are Factory Builder.")
	prepareBuilder := builderRequest && !runner.builderPrepared
	if prepareBuilder {
		runner.builderPrepared = true
	}
	if !builderRequest {
		runner.installedFactoryCommandCalls++
	}
	runner.mu.Unlock()
	if builderRequest {
		if prepareBuilder {
			if err := runner.stageValidateAndInstall(ctx, request); err != nil {
				return platformprocess.CommandResult{}, err
			}
		}
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(runner.builderResult)}, nil
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(graphFactoryPrimaryReply)}, nil
}

func (runner *factoryBuilderCommandRunner) stageValidateAndInstall(ctx context.Context, request platformprocess.CommandRequest) error {
	if runner.process == nil {
		return fmt.Errorf("Factory Builder provider runner has no customer process")
	}
	if err := runner.assertProviderInstructions(request); err != nil {
		return err
	}
	stagePath := filepath.Join(request.WorkDir, "factory-builder-stage", runner.targetName, factoryYAMLFile)
	if err := os.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
		return fmt.Errorf("stage Factory candidate: %w", err)
	}
	if err := os.WriteFile(stagePath, []byte(runner.candidateYAML), 0o600); err != nil {
		return fmt.Errorf("write staged Factory candidate: %w", err)
	}
	if err := runner.executeCustomerCommand(ctx, request, []string{"you", "factory", "config", "validate", stagePath}); err != nil {
		return fmt.Errorf("validate staged Factory candidate: %w", err)
	}
	if err := runner.executeCustomerCommand(ctx, request, []string{
		"you", "factory", "create", runner.targetName, "--from", stagePath,
		"--dir", runner.operatorRoot,
	}); err != nil {
		return fmt.Errorf("install validated Factory candidate: %w", err)
	}
	runner.mu.Lock()
	runner.stagePath = stagePath
	runner.mu.Unlock()
	return nil
}

func (runner *factoryBuilderCommandRunner) assertProviderInstructions(request platformprocess.CommandRequest) error {
	prompt := string(request.Stdin)
	for _, want := range []string{
		"You are Factory Builder.",
		runner.customerRequest,
		"New Factory name: `" + runner.targetName + "`",
		"Requested orchestrator: `" + runner.orchestrator + "`",
	} {
		if !strings.Contains(prompt, want) {
			return fmt.Errorf("Factory Builder provider prompt must include %q", want)
		}
	}
	return nil
}

func (runner *factoryBuilderCommandRunner) executeCustomerCommand(ctx context.Context, request platformprocess.CommandRequest, args []string) error {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runner.process.Execute(root.Input{
		Args: args, Env: runner.environment, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Context: ctx, WorkingDirectory: request.WorkDir,
	}); err != nil {
		return fmt.Errorf("%w; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	return nil
}

func (runner *factoryBuilderCommandRunner) StagePath() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.stagePath
}

func (runner *factoryBuilderCommandRunner) InstalledFactoryCallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.installedFactoryCommandCalls
}

var _ platformprocess.CommandRunner = (*factoryBuilderCommandRunner)(nil)

const representativeGraphFactoryYAML = `name: release-note-review
description:
  type: LOCALIZABLE_ASSET
  value: Reviews submitted release notes and returns an approved summary.
invocationReturn:
  policy: EXPLICIT
  workTypeName: release-note-review
  terminalState: complete
invocationSignature:
  parameters:
    - name: releaseNotes
      description: Release notes to review before publication.
      externalName: release-notes
      required: true
      bindings:
        - kind: POSITIONAL
          position: 1
workTypes:
  - name: release-note-review
    handlingBehavior:
      - DEFAULT
    states:
      - name: init
        type: INITIAL
      - name: complete
        type: TERMINAL
      - name: failed
        type: FAILED
workers:
  - name: release-notes-reviewer
    type: AGENT_WORKER
    skipPermissions: true
    agentTools:
      policy: DISABLED
workstations:
  - name: review-release-notes
    type: AGENT_RUN
    worker: release-notes-reviewer
    body: |
      Review the submitted release notes and return an approved summary.
    inputs:
      - workType: release-note-review
        state: init
    outputs:
      - workType: release-note-review
        state: complete
    onFailure:
      - workType: release-note-review
        state: failed
`

const representativeJavaScriptFactoryYAML = `name: release-synthesis
description:
  type: LOCALIZABLE_ASSET
  value: Runs two independent release analyses and returns their synthesis.
invocationSignature:
  parameters:
    - name: briefs
      description: Release briefs to analyze and synthesize.
      externalName: briefs
      required: true
      bindings:
        - kind: POSITIONAL
          position: 1
        - kind: NAMED
orchestrator:
  kind: JAVASCRIPT
  javascript:
    metadata:
      purpose: bounded-release-synthesis
    argsSchema:
      type: object
      required:
        - briefs
      properties:
        briefs:
          type: string
      additionalProperties: false
    defaultPolicy:
      mode: READ_ONLY
      maxAgents: 2
      concurrency: 2
      allowNetwork: false
    inlineSource:
      encoding: utf-8
      inline: |
        return (async function () {
          phase("analyze");
          const analyses = await parallel([
            {
              label: "analysis-alpha",
              prompt: "Analyze these release briefs for strengths and risks: " + args.briefs,
            },
            {
              label: "analysis-beta",
              prompt: "Independently analyze these release briefs for omissions and risks: " + args.briefs,
            },
          ]);
          workflow.final({
            summary: "Synthesized two independent analyses.",
            analysisCount: analyses.length,
          });
        })();
`
