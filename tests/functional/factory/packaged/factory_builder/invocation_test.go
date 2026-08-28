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
	invalidFactoryNewName          = "invalid-release-review"
	invalidFactoryExistingName     = "protected-release-review"
	invalidFactoryRequest          = "Create a release-note review Factory without changing an existing Factory."
	invalidCandidateWorkerName     = "missing-reviewer"
	operatorFactoryRootArgument    = "~/.you-agent-factory/factories"
)

func invokeFactoryBuilder(
	t *testing.T,
	scenario *factoryBuilderScenario,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()
	commandArgs := []string{
		"you", "--json", "run", "--named", factoryBuilderName, "--no-record",
	}
	for _, flag := range []struct {
		name string
		key  string
	}{
		{name: "--factory-name", key: "factoryName"},
		{name: "--orchestrator", key: "orchestrator"},
		{name: "--builder-provider", key: "builderProvider"},
		{name: "--builder-model", key: "builderModel"},
	} {
		value, ok := args[flag.key].(string)
		if ok && strings.TrimSpace(value) != "" {
			commandArgs = append(commandArgs, flag.name, value)
		}
	}
	if request, ok := args["request"].(string); ok && strings.TrimSpace(request) != "" {
		commandArgs = append(commandArgs, request)
	}

	inputs := support.FakeInputs(t.Context(), commandArgs)
	inputs.Input.Env = append([]string(nil), scenario.environment...)
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := scenario.fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(Factory Builder %v) error = %v\nstdout:\n%s\nstderr:\n%s",
			commandArgs, err, inputs.Stdout(), inputs.Stderr(),
		)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("Factory Builder stderr = %q, want empty successful invocation stderr", inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if err := scenario.fixture.recordInvocationRequestID(response.RequestId); err != nil {
		t.Fatalf("Factory Builder invocation request ID: %v", err)
	}
	return response
}

// TestFactoryBuilderCreatesAndInstallsValidatedGraphFactory proves the public
// Builder invocation can produce a staged YAML graph that the agent validates
// and persists through the normal named-Factory CLI flow before the customer
// resolves and runs the installed Factory.
func testFactoryBuilderCreatesAndInstallsValidatedGraphFactory(t *testing.T, fixture *factoryBuilderSharedFixture) {
	runner := &factoryBuilderCommandRunner{
		targetName:      graphFactoryName,
		customerRequest: graphFactoryRequest,
		orchestrator:    "graph",
		candidateYAML:   representativeGraphFactoryYAML,
		builderResult:   "Factory release-note-review (graph): validation passed and installed through the named Factory create command.",
	}
	scenario := fixture.newScenario(t, runner)
	runner.process = fixture.process
	runner.environment = scenario.environment
	runner.operatorRoot = scenario.operatorRoot
	response := invokeFactoryBuilder(t, scenario, map[string]any{
		"request":         graphFactoryRequest,
		"factoryName":     graphFactoryName,
		"orchestrator":    "graph",
		"builderProvider": "CODEX",
		"builderModel":    "gpt-5",
	})
	assertBuilderCompleted(t, response, graphFactoryName, "graph")

	installedPath := filepath.Join(scenario.operatorRoot, graphFactoryName)
	assertBuilderStageIsWorkspaceScoped(t, scenario.workingDirectory, runner.StagePath())
	assertBuilderPersistsToOperatorRoot(t, runner, scenario.workingDirectory)
	assertInstalledGraphFactory(t, fixture.process, scenario.environment, installedPath, runner.operatorRoot)
	assertInstalledGraphFactoryRuns(t, scenario, installedPath)
	if got := runner.InstalledFactoryCallCount(); got != 1 {
		t.Fatalf("installed Factory provider command call count = %d, want one customer invocation", got)
	}
}

// TestFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory proves the
// Builder validates and persists a JavaScript Factory through the same public
// named-Factory path as graph definitions before its intended child work runs.
func testFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory(t *testing.T, fixture *factoryBuilderSharedFixture) {
	runner := &factoryBuilderCommandRunner{
		targetName:      javascriptFactoryName,
		customerRequest: javascriptFactoryRequest,
		orchestrator:    "javascript",
		candidateYAML:   representativeJavaScriptFactoryYAML,
		builderResult:   "Factory release-synthesis (javascript): validation passed and installed through the named Factory create command.",
	}
	scenario := fixture.newScenario(t, runner)
	runner.process = fixture.process
	runner.environment = scenario.environment
	runner.operatorRoot = scenario.operatorRoot
	writeJavaScriptAgentPresetConfig(t, scenario.homeDir)
	response := invokeFactoryBuilder(t, scenario, map[string]any{
		"request":         javascriptFactoryRequest,
		"factoryName":     javascriptFactoryName,
		"orchestrator":    "javascript",
		"builderProvider": "CODEX",
		"builderModel":    "gpt-5",
	})
	assertBuilderCompleted(t, response, javascriptFactoryName, "javascript")

	installedPath := filepath.Join(scenario.operatorRoot, javascriptFactoryName)
	assertBuilderStageIsWorkspaceScoped(t, scenario.workingDirectory, runner.StagePath())
	assertBuilderPersistsToOperatorRoot(t, runner, scenario.workingDirectory)
	assertInstalledJavaScriptFactory(t, fixture.process, scenario.environment, installedPath, runner.operatorRoot)
	assertInstalledJavaScriptFactoryRuns(t, scenario, installedPath)
	if got := runner.InstalledFactoryCallCount(); got != 2 {
		t.Fatalf("installed Factory provider command call count = %d, want two intended analysis calls", got)
	}
}

// TestFactoryBuilderRejectsInvalidGeneratedCandidateWithoutInstallation proves
// the public Builder flow reports actionable validation guidance without
// creating a new destination or replacing and executing a named Factory that
// already exists in the operator-owned catalog.
func testFactoryBuilderRejectsInvalidGeneratedCandidateWithoutInstallation(t *testing.T, fixture *factoryBuilderSharedFixture) {
	for _, testCase := range []struct {
		name               string
		factoryName        string
		installExistingOne bool
	}{
		{name: "new destination", factoryName: invalidFactoryNewName},
		{name: "existing destination", factoryName: invalidFactoryExistingName, installExistingOne: true},
	} {
		testCase := testCase
		t.Run(strings.ReplaceAll(testCase.name, " ", "_"), func(t *testing.T) {
			runner := &factoryBuilderCommandRunner{
				targetName:              testCase.factoryName,
				customerRequest:         invalidFactoryRequest,
				orchestrator:            "graph",
				candidateYAML:           representativeInvalidGraphFactoryYAML,
				expectValidationFailure: true,
				builderResult:           invalidCandidateResult(testCase.factoryName),
			}
			factoryScenario := fixture.newScenario(t, runner)
			runner.process = fixture.process
			runner.environment = factoryScenario.environment
			runner.operatorRoot = factoryScenario.operatorRoot

			installedPath := filepath.Join(factoryScenario.operatorRoot, testCase.factoryName)
			var before []byte
			if testCase.installExistingOne {
				installExistingGraphFactory(t, fixture.process, factoryScenario.environment, factoryScenario.workingDirectory, runner.operatorRoot, testCase.factoryName)
				assertInstalledGraphFactoryRuns(t, factoryScenario, installedPath)
				before = readInstalledFactoryConfig(t, installedPath)
			}
			providerCallsBeforeBuilder := runner.InstalledFactoryCallCount()

			response := invokeFactoryBuilder(t, factoryScenario, map[string]any{
				"request":         invalidFactoryRequest,
				"factoryName":     testCase.factoryName,
				"orchestrator":    "graph",
				"builderProvider": "CODEX",
				"builderModel":    "gpt-5",
			})
			assertInvalidCandidateGuidance(t, response, testCase.factoryName, runner.StagePath())
			assertBuilderRejectedCandidate(t, runner, factoryScenario.workingDirectory, providerCallsBeforeBuilder)

			if testCase.installExistingOne {
				after := readInstalledFactoryConfig(t, installedPath)
				if !bytes.Equal(before, after) {
					t.Fatalf("installed Factory changed after rejected candidate\nbefore:\n%s\nafter:\n%s", before, after)
				}
				assertInstalledGraphFactoryRuns(t, factoryScenario, installedPath)
				return
			}
			if _, err := os.Stat(installedPath); !os.IsNotExist(err) {
				t.Fatalf("invalid candidate installed at %q; stat error = %v", installedPath, err)
			}
		})
	}
}

func invalidCandidateResult(factoryName string) string {
	return "Factory " + factoryName + " (graph): validation failed at factory.worker.danglingReference for " +
		invalidCandidateWorkerName + ". Correction: define that worker or change the workstation to a declared worker, then validate again. The Factory was not installed."
}

func assertInvalidCandidateGuidance(t *testing.T, response factoryapi.InvocationResponse, factoryName, stagePath string) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("Builder invalid-candidate response status = %q, want completed safe guidance; response = %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("Builder invalid-candidate primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("Builder invalid-candidate primary result as text: %v", err)
	}
	for _, want := range []string{factoryName, "validation failed", "factory.worker.danglingReference", invalidCandidateWorkerName, "Correction", "not installed"} {
		if !strings.Contains(part.Text, want) {
			t.Fatalf("Builder invalid-candidate result = %q, want %q", part.Text, want)
		}
	}
	for _, forbidden := range []string{stagePath, "you factory config validate", "api key", "token"} {
		if strings.Contains(strings.ToLower(part.Text), strings.ToLower(forbidden)) {
			t.Fatalf("Builder invalid-candidate result = %q, must redact %q", part.Text, forbidden)
		}
	}
}

func assertBuilderRejectedCandidate(
	t *testing.T,
	runner *factoryBuilderCommandRunner,
	workingDirectory string,
	providerCallsBeforeBuilder int,
) {
	t.Helper()
	assertBuilderStageIsWorkspaceScoped(t, workingDirectory, runner.StagePath())
	if runner.ValidationAttemptCount() != 1 {
		t.Fatalf("candidate validation attempts = %d, want one public CLI validation", runner.ValidationAttemptCount())
	}
	diagnostic := runner.ValidationDiagnostic()
	for _, want := range []string{"Factory validation failed.", "factory.worker.danglingReference", invalidCandidateWorkerName} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("public validation diagnostic = %q, want %q", diagnostic, want)
		}
	}
	if runner.PersistenceAttemptCount() != 0 {
		t.Fatalf("named-Factory persistence attempts = %d, want none after validation failure", runner.PersistenceAttemptCount())
	}
	if runner.InstalledFactoryCallCount() != providerCallsBeforeBuilder {
		t.Fatalf("invalid candidate provider command calls = %d, want %d before the Builder validation failure", runner.InstalledFactoryCallCount(), providerCallsBeforeBuilder)
	}
	assertNoProjectLocalFactoryShadow(t, workingDirectory, runner.targetName)
}

func installExistingGraphFactory(
	t *testing.T,
	process support.Process,
	environment []string,
	workingDirectory, operatorRoot, factoryName string,
) {
	t.Helper()
	stagePath := filepath.Join(workingDirectory, "existing-factory-stage", factoryYAMLFile)
	if err := os.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
		t.Fatalf("create existing Factory staging directory: %v", err)
	}
	if err := os.WriteFile(stagePath, []byte(representativeGraphFactoryYAML), 0o600); err != nil {
		t.Fatalf("write existing Factory staged definition: %v", err)
	}
	create := support.FakeInputs(t.Context(), []string{
		"you", "factory", "create", factoryName, "--from", stagePath, "--dir", operatorRoot,
	})
	create.Input.Env = environment
	create.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(create.Input); err != nil {
		t.Fatalf("Process.Execute(create existing Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, create.Stdout(), create.Stderr())
	}
}

func readInstalledFactoryConfig(t *testing.T, installedPath string) []byte {
	t.Helper()
	config, err := os.ReadFile(filepath.Join(installedPath, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read installed Factory configuration: %v", err)
	}
	return config
}

func assertBuilderCompleted(t *testing.T, response factoryapi.InvocationResponse, factoryName, orchestrator string) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("Builder invocation status = %q, want %q; response = %#v", response.Status, factoryapi.InvocationTerminalStatusCompleted, response)
	}
	// The session HTTP contract has no process stderr stream; a successful
	// response must therefore carry no failure diagnostics at this boundary.
	if response.ErrorCode != nil {
		t.Fatalf("successful Builder invocation returned errorCode = %q", *response.ErrorCode)
	}
	if response.Message != nil && strings.TrimSpace(*response.Message) != "" {
		t.Fatalf("successful Builder invocation returned message = %q", *response.Message)
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

func assertBuilderPersistsToOperatorRoot(t *testing.T, runner *factoryBuilderCommandRunner, workingDirectory string) {
	t.Helper()
	want := []string{
		"you", "factory", "create", runner.targetName, "--from", runner.StagePath(),
		"--dir", runner.operatorRoot,
	}
	if got := runner.PersistenceCommand(); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Builder persistence command = %#v, want exact public command %#v", got, want)
	}
	assertNoProjectLocalFactoryShadow(t, workingDirectory, runner.targetName)
}

func assertNoProjectLocalFactoryShadow(t *testing.T, workingDirectory, factoryName string) {
	t.Helper()
	shadow := filepath.Join(workingDirectory, factorydefinitions.FactoryDir, factoryName)
	if _, err := os.Stat(shadow); !os.IsNotExist(err) {
		t.Fatalf("project-local Factory shadow at %q; stat error = %v", shadow, err)
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
	agents, ok := javascript["agents"].(map[string]any)
	if !ok || len(agents) != 1 {
		t.Fatalf("JavaScript agents = %#v, want one named agent role", javascript["agents"])
	}
	for agentID, value := range agents {
		analyst, ok := value.(map[string]any)
		if !ok || analyst["preset"] != "careful-review" {
			t.Fatalf("%s agent = %#v, want careful-review preset", agentID, value)
		}
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
	if !ok || !hasStringValue(defaultPolicy["allowedPermissions"], "DEFAULT") || defaultPolicy["maxAgents"] != float64(2) || defaultPolicy["concurrency"] != float64(2) {
		t.Fatalf("JavaScript defaultPolicy = %#v, want bounded DEFAULT two-agent policy", javascript["defaultPolicy"])
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
	scenario *factoryBuilderScenario,
	installedPath string,
) {
	t.Helper()
	run := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", filepath.Join(installedPath, factorydefinitions.FactoryConfigFile), "--no-record",
		"--output", "response-stream",
		"--provider", "CODEX", "--model", "gpt-5",
		"--briefs", "Analyze the release plan and identify important risks.",
	})
	run.Input.Env = append([]string(nil), scenario.environment...)
	run.Input.WorkingDirectory = scenario.workingDirectory
	if err := scenario.fixture.process.Execute(run.Input); err != nil {
		t.Fatalf("Process.Execute(run installed JavaScript Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, run.Stdout(), run.Stderr())
	}
	if run.Stderr() != "" {
		t.Fatalf("installed JavaScript Factory stderr = %q, want empty successful invocation stderr", run.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, run.Stdout())
	if err := scenario.fixture.recordInvocationRequestID(response.RequestId); err != nil {
		t.Fatalf("installed JavaScript Factory invocation request ID: %v", err)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("installed JavaScript Factory invocation status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("installed JavaScript Factory primary result = %#v, want one JSON part", response.PrimaryResult)
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
	scenario *factoryBuilderScenario,
	installedPath string,
) {
	t.Helper()
	run := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", filepath.Join(installedPath, factorydefinitions.FactoryConfigFile), "--no-record",
		"--provider", "CODEX", "--model", "gpt-5",
		"Review these release notes for publication.",
	})
	run.Input.Env = append([]string(nil), scenario.environment...)
	run.Input.WorkingDirectory = scenario.workingDirectory
	if err := scenario.fixture.process.Execute(run.Input); err != nil {
		t.Fatalf("Process.Execute(run installed Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, run.Stdout(), run.Stderr())
	}
	if run.Stderr() != "" {
		t.Fatalf("installed Factory stderr = %q, want empty successful invocation stderr", run.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, run.Stdout())
	if err := scenario.fixture.recordInvocationRequestID(response.RequestId); err != nil {
		t.Fatalf("installed Factory invocation request ID: %v", err)
	}
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
	routingCalls            int
	process                 support.Process
	targetName              string
	customerRequest         string
	orchestrator            string
	environment             []string
	operatorRoot            string
	candidateYAML           string
	builderResult           string
	expectValidationFailure bool

	mu                           sync.Mutex
	stagePath                    string
	builderPrepared              bool
	installedFactoryCommandCalls int
	validationAttemptCount       int
	persistenceAttemptCount      int
	validationDiagnostic         string
	persistenceCommand           []string
}

func (runner *factoryBuilderCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	// Every Builder invocation is now routed first: the classifier decides
	// whether the message is a build request or a question. These cells all
	// supply genuine build requests, so the router answers "build" and the
	// build workstation runs exactly as before.
	if isBuilderRoutingPrompt(request) {
		runner.mu.Lock()
		runner.routingCalls++
		runner.mu.Unlock()
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("build")}, nil
	}
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
				if runner.expectValidationFailure && runner.ValidationDiagnostic() != "" {
					return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(runner.builderResult)}, nil
				}
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
	runner.mu.Lock()
	runner.stagePath = stagePath
	runner.validationAttemptCount++
	runner.mu.Unlock()
	if err := runner.executeCustomerCommand(ctx, request, []string{"you", "factory", "config", "validate", stagePath}); err != nil {
		runner.mu.Lock()
		runner.validationDiagnostic = err.Error()
		runner.mu.Unlock()
		return fmt.Errorf("validate staged Factory candidate: %w", err)
	}
	persistenceCommand := []string{
		"you", "factory", "create", runner.targetName, "--from", stagePath,
		"--dir", runner.operatorRoot,
	}
	runner.mu.Lock()
	runner.persistenceAttemptCount++
	runner.persistenceCommand = append([]string(nil), persistenceCommand...)
	runner.mu.Unlock()
	if err := runner.executeCustomerCommand(ctx, request, persistenceCommand); err != nil {
		return fmt.Errorf("install validated Factory candidate: %w", err)
	}
	return nil
}

func (runner *factoryBuilderCommandRunner) assertProviderInstructions(request platformprocess.CommandRequest) error {
	prompt := string(request.Stdin)
	for _, want := range []string{
		"You are Factory Builder.",
		runner.customerRequest,
		"New Factory name: `" + runner.targetName + "`",
		"Requested orchestrator: `" + runner.orchestrator + "`",
		"validation code, field, or source location",
		"you factory create " + runner.targetName + " --from <staged-candidate> --dir " + operatorFactoryRootArgument,
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

func (runner *factoryBuilderCommandRunner) ValidationAttemptCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.validationAttemptCount
}

func (runner *factoryBuilderCommandRunner) PersistenceAttemptCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.persistenceAttemptCount
}

func (runner *factoryBuilderCommandRunner) ValidationDiagnostic() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.validationDiagnostic
}

func (runner *factoryBuilderCommandRunner) PersistenceCommand() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.persistenceCommand...)
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

const representativeInvalidGraphFactoryYAML = `name: invalid-release-note-review
description:
  type: LOCALIZABLE_ASSET
  value: Deliberately invalid review Factory used to prove Builder rejection.
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
    worker: missing-reviewer
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
    agents:
      release-analyst:
        preset: careful-review
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
      allowedPermissions:
        - DEFAULT
      maxAgents: 2
      concurrency: 2
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

func writeJavaScriptAgentPresetConfig(t *testing.T, homeDir string) {
	t.Helper()

	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create operator config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5"
  },
  "workerPresets": [{
    "id": "careful-review",
    "modelProvider": "codex",
    "model": "gpt-5",
    "reasoningEffort": "medium"
  }]
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write operator agent preset config: %v", err)
	}
}

// isBuilderRoutingPrompt reports whether a provider call is the Factory
// Builder's routing classification rather than a build or an installed-Factory
// command.
func isBuilderRoutingPrompt(request platformprocess.CommandRequest) bool {
	return strings.Contains(string(request.Stdin), "return exactly one lowercase label: `build` or `help`")
}
