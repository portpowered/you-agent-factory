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
	factoryBuilderName       = "@you/factory-builder"
	graphFactoryName         = "release-note-review"
	graphFactoryRequest      = "Review submitted release notes and return an approved summary."
	graphFactoryPrimaryReply = "Release-note review completed."
	factoryYAMLFile          = "factory.yaml"
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
		environment:     environment,
		operatorRoot:    filepath.Join(homeDir, ".you-agent-factory", "factories"),
		candidateYAML:   representativeGraphFactoryYAML,
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
	assertBuilderCompleted(t, support.DecodeInvocationResponseJSON(t, builder.Stdout()))
	if builder.Stderr() != "" {
		t.Fatalf("factory builder stderr = %q, want empty successful invocation stderr", builder.Stderr())
	}

	installedPath := filepath.Join(homeDir, ".you-agent-factory", "factories", graphFactoryName)
	assertBuilderStageIsWorkspaceScoped(t, workingDirectory, runner.StagePath())
	assertInstalledGraphFactory(t, process, environment, installedPath)
	assertInstalledGraphFactoryRuns(t, process, environment, workingDirectory)
	if got := runner.InstalledFactoryCallCount(); got != 1 {
		t.Fatalf("installed Factory provider command call count = %d, want one customer invocation", got)
	}
}

func assertBuilderCompleted(t *testing.T, response factoryapi.InvocationResponse) {
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
	for _, want := range []string{graphFactoryName, "graph", "validation passed", "installed"} {
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

func assertInstalledGraphFactory(t *testing.T, process support.Process, environment []string, installedPath string) {
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
}

func assertInstalledGraphFactoryRuns(t *testing.T, process support.Process, environment []string, workingDirectory string) {
	t.Helper()
	run := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--named", graphFactoryName, "--no-record",
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
	environment     []string
	operatorRoot    string
	candidateYAML   string

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
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(
			"Factory release-note-review (graph): validation passed and installed through the named Factory create command.",
		)}, nil
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
		"Requested orchestrator: `graph`",
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
