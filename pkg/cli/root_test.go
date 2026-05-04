package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	docscli "github.com/portpowered/infinite-you/pkg/cli/docs"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestNewRootCommand_HasSubcommands(t *testing.T) {
	root := NewRootCommand()

	want := map[string]bool{
		"config": false,
		"docs":   false,
		"init":   false,
		"run":    false,
		"submit": false,
	}

	for _, sub := range root.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("expected subcommand %q to be registered", name)
		}
	}
}

func TestDocsCommand_HelpDocumentsSupportedTopics(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"docs"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute docs help: %v", err)
	}

	help := out.String()
	for _, want := range append(
		[]string{
			"Print packaged markdown reference topics from the installed binary.",
			"Use one of the supported topic subcommands to print the authored markdown page with no wrapper formatting.",
		},
		docscli.SupportedTopics()...,
	) {
		if !strings.Contains(help, want) {
			t.Fatalf("docs help missing %q:\n%s", want, help)
		}
	}
}

func TestRootCommand_HelpDocumentsSupportedDocsTopics(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	for _, want := range append(
		[]string{
			"Packaged reference topics are also available through infinite-you docs <topic>.",
			"Supported docs topics:",
			"infinite-you docs workstation",
		},
		docscli.SupportedTopics()...,
	) {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestDocsCommand_SupportedTopicsPrintRawPackagedMarkdown(t *testing.T) {
	t.Parallel()

	for _, topic := range docscli.SupportedTopics() {
		topic := topic
		t.Run(topic, func(t *testing.T) {
			t.Parallel()

			want, err := docscli.Markdown(topic)
			if err != nil {
				t.Fatalf("Markdown(%q): %v", topic, err)
			}

			got := string(executeRootCommand(t, "docs", topic))
			if got != want {
				t.Fatalf("docs %s output did not match packaged markdown", topic)
			}
		})
	}
}

func TestDocsCommand_RejectsUnsupportedTopic(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"docs", "unknown"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if !strings.Contains(err.Error(), `unknown command "unknown"`) {
		t.Fatalf("unsupported docs topic error = %q", err.Error())
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-cli-flatten-fixture review=2026-07-18 removal=split-cli-flatten-fixture-before-next-config-cli-change
func TestConfigFlattenCommand_WritesCanonicalLoadableFactoryJSON(t *testing.T) {
	factoryDir := t.TempDir()
	writeFlattenCommandFixture(t, factoryDir)
	assertSplitLayoutLoadUsesCanonicalWorkerFields(t, factoryDir)

	out := executeRootCommand(t, "config", "flatten", factoryDir)
	payload := decodeFlattenPayload(t, out)
	assertCanonicalFlattenPayload(t, payload)
	assertFlattenedOutputParses(t, out)
	standaloneDir := writeStandaloneFlattenOutput(t, out)
	assertStandaloneRuntimeConfigLoads(t, standaloneDir)

	fileOut := executeRootCommand(t, "config", "flatten", filepath.Join(standaloneDir, interfaces.FactoryConfigFile))
	if _, err := factoryconfig.FactoryConfigFromOpenAPIJSON(fileOut); err != nil {
		t.Fatalf("standalone file flatten output should parse through normal factory config path: %v", err)
	}
}

func writeFlattenCommandFixture(t *testing.T, factoryDir string) {
	t.Helper()

	writeRootTestFile(t, filepath.Join(factoryDir, interfaces.FactoryConfigFile), `{
		"name":"flatten-command-fixture",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"resources":[{"name":"agent-slot","capacity":2}]
		}]
	}`)
	writeRootTestFile(t, filepath.Join(factoryDir, "workers", "executor", "AGENTS.md"), `---
type: MODEL_WORKER
model: claude-sonnet-4-6
modelProvider: CLAUDE
executorProvider: SCRIPT_WRAP
stopToken: COMPLETE
---

You are the split-layout executor.`)
	writeRootTestFile(t, filepath.Join(factoryDir, "workstations", "execute-story", "AGENTS.md"), `---
type: MODEL_WORKSTATION
worker: executor
limits:
  maxExecutionTime: 20m
  maxRetries: 2
stopWords: ["DONE"]
---

Process {{ (index .Inputs 0).WorkID }}.`)
}

func executeRootCommand(t *testing.T, args ...string) []byte {
	t.Helper()

	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root command %v: %v", args, err)
	}
	return out.Bytes()
}

func decodeFlattenPayload(t *testing.T, out []byte) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("flattened output is not JSON: %v\n%s", err, string(out))
	}
	return payload
}

func assertCanonicalFlattenPayload(t *testing.T, payload map[string]any) {
	t.Helper()

	if _, ok := payload["workTypes"]; !ok {
		t.Fatalf("expected canonical workTypes key in flattened output")
	}
	if _, ok := payload["work_types"]; ok {
		t.Fatalf("expected flattened output not to include legacy work_types key")
	}

	workstations, ok := payload["workstations"].([]any)
	if !ok || len(workstations) != 1 {
		t.Fatalf("expected one workstation in flattened output, got %#v", payload["workstations"])
	}
	workstation, ok := workstations[0].(map[string]any)
	if !ok {
		t.Fatalf("expected workstation object, got %#v", workstations[0])
	}
	if _, ok := workstation["resources"]; !ok {
		t.Fatalf("expected canonical resources key in flattened workstation")
	}
	workersPayload, ok := payload["workers"].([]any)
	if !ok || len(workersPayload) != 1 {
		t.Fatalf("expected one worker in flattened output, got %#v", payload["workers"])
	}
	workerPayload, ok := workersPayload[0].(map[string]any)
	if !ok {
		t.Fatalf("expected flattened worker to include inline definition, got %#v", workerPayload)
	}
	if workerPayload["model"] != "claude-sonnet-4-6" || workerPayload["modelProvider"] != "CLAUDE" {
		t.Fatalf("expected flattened worker definition to preserve model/provider, got %#v", workerPayload)
	}
	if workerPayload["executorProvider"] != "SCRIPT_WRAP" {
		t.Fatalf("expected flattened worker definition to preserve canonical executorProvider, got %#v", workerPayload)
	}
	for _, retired := range []string{"provider", "sessionId", "concurrency"} {
		if _, ok := workerPayload[retired]; ok {
			t.Fatalf("expected flattened worker definition not to include retired %q field, got %#v", retired, workerPayload)
		}
	}
	if workerPayload["body"] != "You are the split-layout executor." {
		t.Fatalf("expected flattened worker body, got %#v", workerPayload["body"])
	}
	if workstation["type"] != "MODEL_WORKSTATION" {
		t.Fatalf("expected flattened workstation runtime type, got %#v", workstation)
	}
	if workstation["body"] != "Process {{ (index .Inputs 0).WorkID }}." {
		t.Fatalf("expected flattened workstation body to preserve prompt content, got %#v", workstation)
	}
	if _, ok := workstation["promptTemplate"]; ok {
		t.Fatalf("expected flattened workstation to omit promptTemplate, got %#v", workstation)
	}
	if _, ok := workstation["definition"]; ok {
		t.Fatalf("expected flattened workstation runtime config to be flat, got %#v", workstation)
	}
}

func assertFlattenedOutputParses(t *testing.T, out []byte) {
	t.Helper()

	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(out)
	if err != nil {
		t.Fatalf("flattened output should parse through normal factory config path: %v", err)
	}
	if len(cfg.Workers) != 1 || len(cfg.Workstations) != 1 {
		t.Fatalf("expected flattened output to preserve workers/workstations, got %d/%d", len(cfg.Workers), len(cfg.Workstations))
	}
}

func writeStandaloneFlattenOutput(t *testing.T, out []byte) string {
	t.Helper()

	standaloneDir := t.TempDir()
	writeRootTestFile(t, filepath.Join(standaloneDir, interfaces.FactoryConfigFile), string(out))
	return standaloneDir
}

func assertStandaloneRuntimeConfigLoads(t *testing.T, standaloneDir string) {
	t.Helper()

	loaded, err := factoryconfig.LoadRuntimeConfig(standaloneDir, nil)
	if err != nil {
		t.Fatalf("flattened output should load as standalone factory config: %v", err)
	}
	if len(loaded.FactoryConfig().Workers) != 1 || len(loaded.FactoryConfig().Workstations) != 1 {
		t.Fatalf("expected standalone load to preserve workers/workstations, got %d/%d", len(loaded.FactoryConfig().Workers), len(loaded.FactoryConfig().Workstations))
	}
	workerDef, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected standalone flattened worker definition to load")
	}
	if workerDef.Model != "claude-sonnet-4-6" || workerDef.Body != "You are the split-layout executor." {
		t.Fatalf("standalone flattened worker definition = %#v", workerDef)
	}
	if workerDef.SessionID != "" || workerDef.Concurrency != 0 {
		t.Fatalf("standalone flattened worker definition leaked internal-only runtime fields: %#v", workerDef)
	}
	workstationDef, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected standalone flattened workstation definition to load")
	}
	if workstationDef.PromptTemplate != "Process {{ (index .Inputs 0).WorkID }}." || workstationDef.Limits.MaxRetries != 2 {
		t.Fatalf("standalone flattened workstation definition = %#v", workstationDef)
	}
}

func assertSplitLayoutLoadUsesCanonicalWorkerFields(t *testing.T, factoryDir string) {
	t.Helper()

	workerDef, err := factoryconfig.LoadWorkerConfig(filepath.Join(factoryDir, "workers", "executor"))
	if err != nil {
		t.Fatalf("LoadWorkerConfig(source split layout): %v", err)
	}
	if workerDef.ModelProvider != "claude" || workerDef.ExecutorProvider != "script_wrap" || workerDef.StopToken != "COMPLETE" {
		t.Fatalf("source split worker definition = %#v, want canonical worker fields", workerDef)
	}
	if workerDef.SessionID != "" || workerDef.Concurrency != 0 {
		t.Fatalf("source split worker definition leaked runtime-only fields: %#v", workerDef)
	}
}

func TestConfigExpandCommand_WritesSplitFactoryLayout(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	writeRootTestFile(t, factoryPath, `{
		"name":"expand-command-fixture",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "expand", factoryPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute config expand: %v", err)
	}
	if !strings.Contains(out.String(), "Expanded factory config into") {
		t.Fatalf("expected expand result output, got %q", out.String())
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("expanded layout should load through normal runtime config path: %v", err)
	}
	if len(loaded.FactoryConfig().Workers) != 1 || len(loaded.FactoryConfig().Workstations) != 1 {
		t.Fatalf("expected expanded layout to preserve workers/workstations, got %d/%d", len(loaded.FactoryConfig().Workers), len(loaded.FactoryConfig().Workstations))
	}
	if _, ok := loaded.Worker("executor"); !ok {
		t.Fatal("expected expanded worker AGENTS.md to load")
	}
	if _, ok := loaded.Workstation("execute-story"); !ok {
		t.Fatal("expected expanded workstation AGENTS.md to load")
	}

	workerAgents := string(readRootTestFile(t, filepath.Join(dir, "workers", "executor", "AGENTS.md")))
	for _, expected := range []string{"type: MODEL_WORKER"} {
		if !strings.Contains(workerAgents, expected) {
			t.Fatalf("expanded worker AGENTS.md missing %q:\n%s", expected, workerAgents)
		}
	}
	for _, retired := range []string{"modelProvider:", "stopToken:", "skipPermissions:"} {
		if strings.Contains(workerAgents, retired) {
			t.Fatalf("expanded worker AGENTS.md should not contain retired %q:\n%s", retired, workerAgents)
		}
	}

	workstationAgents := string(readRootTestFile(t, filepath.Join(dir, "workstations", "execute-story", "AGENTS.md")))
	for _, expected := range []string{"type: MODEL_WORKSTATION", "worker: executor"} {
		if !strings.Contains(workstationAgents, expected) {
			t.Fatalf("expanded workstation AGENTS.md missing %q:\n%s", expected, workstationAgents)
		}
	}
	for _, retired := range []string{"promptFile:", "outputSchema:", "stopWords:"} {
		if strings.Contains(workstationAgents, retired) {
			t.Fatalf("expanded workstation AGENTS.md should not contain retired %q:\n%s", retired, workstationAgents)
		}
	}
}

func TestNewRootCommand_DoesNotExposeRemovedAuditStateSurfaces(t *testing.T) {
	root := NewRootCommand()

	for _, subcommand := range root.Commands() {
		if subcommand.Name() == "audit" {
			t.Fatal("audit command should not be registered")
		}
		if subcommand.Name() == "status" {
			t.Fatal("status command should not be registered")
		}
	}

	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"audit", "state-surfaces"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected removed audit state-surfaces command to fail")
	}
}

func TestReadmeCommandListDoesNotAdvertiseRemovedAuditStateSurfaces(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read package README: %v", err)
	}

	if strings.Contains(string(readme), "infinite-you audit state-surfaces") {
		t.Fatal("README should not advertise removed audit state-surfaces command")
	}
}

func TestReadmeQuickstartDocumentsInitStarterOptions(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read package README: %v", err)
	}

	contents := string(readme)
	for _, want := range []string{
		"agent-factory\n```",
		"agent-factory init\n",
		"agent-factory init --executor claude --dir my-factory",
		"Supported starter scaffold options are `codex` and `claude`.",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("README missing %q:\n%s", want, contents)
		}
	}
}
func TestReadmeDocumentsDocsCommandSurface(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read package README: %v", err)
	}

	contents := string(readme)
	for _, want := range []string{
		"agent-factory docs",
		"agent-factory docs workstation",
		"Supported docs topics are `config`, `workstation`, `workers`, `resources`,",
		"`batch-work`, and `templates`.",
		"agent-factory docs batch-work",
		"agent-factory docs config",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("README missing %q:\n%s", want, contents)
		}
	}
}

func TestDocsIndexDocumentsDocsCommandTopics(t *testing.T) {
	docsReadme, err := os.ReadFile("../../docs/README.md")
	if err != nil {
		t.Fatalf("read package docs README: %v", err)
	}

	contents := string(docsReadme)
	for _, want := range []string{
		"infinite-you docs",
		"`config`",
		"`workstation`",
		"`workers`",
		"`resources`",
		"`batch-work`",
		"`templates`",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("docs README missing %q:\n%s", want, contents)
		}
	}
}

func writeRootTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readRootTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestInitCommand_DefaultDir(t *testing.T) {
	root := NewRootCommand()
	initCmd, _, err := root.Find([]string{"init"})
	if err != nil {
		t.Fatalf("find init: %v", err)
	}

	dirFlag := initCmd.Flags().Lookup("dir")
	if dirFlag == nil {
		t.Fatal("expected --dir flag on init command")
	}
	if dirFlag.DefValue != "factory" {
		t.Errorf("default dir = %q, want %q", dirFlag.DefValue, "factory")
	}

	executorFlag := initCmd.Flags().Lookup("executor")
	if executorFlag == nil {
		t.Fatal("expected --executor flag on init command")
	}
	if executorFlag.DefValue != initcmd.DefaultStarterExecutor {
		t.Errorf("default executor = %q, want %q", executorFlag.DefValue, initcmd.DefaultStarterExecutor)
	}
}

func TestInitCommand_ExecutorFlagMapsToInitConfig(t *testing.T) {
	originalInitFactory := initFactory
	defer func() {
		initFactory = originalInitFactory
	}()

	var got initcmd.InitConfig
	initFactory = func(cfg initcmd.InitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--dir", "custom-factory", "--executor", "claude"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init --executor claude: %v", err)
	}

	if got.Dir != "custom-factory" {
		t.Fatalf("dir = %q, want %q", got.Dir, "custom-factory")
	}
	if got.Executor != "claude" {
		t.Fatalf("executor = %q, want %q", got.Executor, "claude")
	}
}

func TestInitCommand_HelpDocumentsExecutorOptions(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{"--executor", "codex", "claude"} {
		if !strings.Contains(help, want) {
			t.Fatalf("init help missing %q:\n%s", want, help)
		}
	}
	if !strings.Contains(help, "Omitting --executor preserves the default Codex-backed starter scaffold") {
		t.Fatalf("init help should describe default executor behavior:\n%s", help)
	}
	if !strings.Contains(help, "Supported starter scaffold values are codex and claude") {
		t.Fatalf("init help should describe supported executor values:\n%s", help)
	}
}

func TestRunCommand_VerboseFlag(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	vFlag := runCmd.Flags().Lookup("verbose")
	if vFlag == nil {
		t.Fatal("expected --verbose flag on run command")
	}
	if vFlag.DefValue != "false" {
		t.Errorf("default verbose = %q, want %q", vFlag.DefValue, "false")
	}
	if vFlag.Shorthand != "v" {
		t.Errorf("verbose shorthand = %q, want %q", vFlag.Shorthand, "v")
	}
}

func TestRootCommand_NoArgsStartsContinuousRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root no args: %v", err)
	}

	if !got.Continuously {
		t.Fatal("expected no-arg invocation to use continuous mode")
	}
	if !got.Bootstrap {
		t.Fatal("expected no-arg invocation to enable bootstrap mode")
	}
	if !got.OpenDashboard {
		t.Fatal("expected no-arg invocation to enable dashboard auto-open")
	}
	if got.Dir != "factory" {
		t.Errorf("dir = %q, want %q", got.Dir, "factory")
	}
	if got.Port != 7437 {
		t.Errorf("port = %d, want %d", got.Port, 7437)
	}
	if !got.AutoPort {
		t.Fatal("expected no-arg invocation to auto-resolve the dashboard port")
	}
	if got.StartupOutput == nil {
		t.Fatal("expected no-arg invocation to configure startup output")
	}
}

func TestRootCommand_NoArgsAndExplicitRunShareHarnessConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var captured []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		captured = append(captured, cfg)
		mode := "batch"
		if cfg.Continuously {
			mode = "continuous"
		}
		if cfg.StartupOutput != nil {
			fmt.Fprintf(
				cfg.StartupOutput,
				"service startup reached: mode=%s bootstrap=%t open-dashboard=%t\n",
				mode,
				cfg.Bootstrap,
				cfg.OpenDashboard,
			)
		}
		return nil
	}

	var rootOut bytes.Buffer
	rootDefault := NewRootCommand()
	rootDefault.SetOut(&rootOut)
	rootDefault.SetErr(io.Discard)
	rootDefault.SetArgs([]string{})
	if err := rootDefault.Execute(); err != nil {
		t.Fatalf("execute root no args: %v", err)
	}

	var explicitOut bytes.Buffer
	explicitRun := NewRootCommand()
	explicitRun.SetOut(&explicitOut)
	explicitRun.SetErr(io.Discard)
	explicitRun.SetArgs([]string{"run"})
	if err := explicitRun.Execute(); err != nil {
		t.Fatalf("execute explicit run: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("captured run configs = %d, want 2", len(captured))
	}

	noArgs := captured[0]
	explicit := captured[1]
	if !noArgs.Continuously || !noArgs.Bootstrap || !noArgs.OpenDashboard {
		t.Fatalf("no-args config missing documented OOTB defaults: %#v", noArgs)
	}
	if explicit.Continuously || explicit.Bootstrap || explicit.OpenDashboard {
		t.Fatalf("explicit run should not inherit OOTB-only defaults: %#v", explicit)
	}
	if got := rootOut.String(); !strings.Contains(got, "service startup reached: mode=continuous bootstrap=true open-dashboard=true") {
		t.Fatalf("no-args observable startup output = %q, want OOTB service startup", got)
	}
	if got := explicitOut.String(); !strings.Contains(got, "service startup reached: mode=batch bootstrap=false open-dashboard=false") {
		t.Fatalf("explicit run observable startup output = %q, want explicit service startup", got)
	}

	noArgs.Continuously = false
	noArgs.Bootstrap = false
	noArgs.OpenDashboard = false
	noArgs.Logger = nil
	noArgs.StartupOutput = nil
	explicit.Logger = nil
	explicit.StartupOutput = nil
	if !reflect.DeepEqual(noArgs, explicit) {
		t.Fatalf("no-args and explicit run configs diverge outside documented defaults:\nno-args: %#v\nrun:     %#v", noArgs, explicit)
	}
}

func TestRootCommand_HelpDocumentsOOTBQuickstart(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Running infinite-you with no arguments starts the out-of-the-box flow",
		"factory/inputs/task/default",
		"http://localhost:7437/dashboard/ui",
		"printf \"Fix the lint issues\\n\" > factory/inputs/task/default/fix-lint.md",
		"docs",
		"Print packaged markdown reference topics",
		"infinite-you docs workstation",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	for _, disallowed := range []string{"goreleaser", "GoReleaser"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("root help should not include release tooling instruction %q:\n%s", disallowed, help)
		}
	}
}

func TestRunCommand_DebugFlag(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	dFlag := runCmd.Flags().Lookup("debug")
	if dFlag == nil {
		t.Fatal("expected --debug flag on run command")
	}
	if dFlag.DefValue != "false" {
		t.Errorf("default debug = %q, want %q", dFlag.DefValue, "false")
	}
	if dFlag.Shorthand != "d" {
		t.Errorf("debug shorthand = %q, want %q", dFlag.Shorthand, "d")
	}
}

func TestRunCommand_ContinuouslyFlag(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("continuously")
	if flag == nil {
		t.Fatal("expected --continuously flag on run command")
	}
	if flag.DefValue != "false" {
		t.Errorf("default continuously = %q, want %q", flag.DefValue, "false")
	}
	if flag.Usage != "keep the factory alive while idle until cancelled" {
		t.Errorf("continuously usage = %q", flag.Usage)
	}
	if runCmd.Long == "" {
		t.Fatal("expected run command long help text")
	}
	if !strings.Contains(runCmd.Long, "run infinite-you with no arguments") {
		t.Fatal("expected run command long help text to point users to no-arg default flow")
	}
	if !strings.Contains(runCmd.Long, "factory/inputs/task/default") {
		t.Fatal("expected run command long help text to mention default task input path")
	}
	if !strings.Contains(runCmd.Example, "factory/inputs/task/default") {
		t.Fatal("expected run command examples to mention default task input path")
	}
}

func TestRunCommand_RecordAndReplayFlags(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	for _, name := range []string{"record", "replay"} {
		flag := runCmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("expected --%s flag on run command", name)
		}
		if flag.DefValue != "" {
			t.Errorf("--%s default = %q, want empty", name, flag.DefValue)
		}
	}
}

func TestRunCommand_WithMockWorkersFlag(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("with-mock-workers")
	if flag == nil {
		t.Fatal("expected --with-mock-workers flag on run command")
	}
	if flag.DefValue != "" {
		t.Errorf("default with-mock-workers = %q, want empty", flag.DefValue)
	}
	if flag.NoOptDefVal == "" {
		t.Error("with-mock-workers should define an internal optional-value default")
	}
	if !strings.Contains(flag.Usage, "optional mock-workers JSON config path") {
		t.Errorf("with-mock-workers usage = %q", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--with-mock-workers") {
		t.Fatal("expected run command long help text to mention --with-mock-workers")
	}
}

func TestRunCommand_RetiredMockExecutionAliasRejected(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	retiredFlag := "--" + strings.Join([]string{"dry", "run"}, "-")
	root.SetArgs([]string{"run", retiredFlag})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected retired mock-execution alias to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown flag: "+retiredFlag) {
		t.Fatalf("error = %q, want unknown retired flag", err.Error())
	}
	if runCalled {
		t.Fatal("run command should not execute when retired mock-execution alias is unsupported")
	}
}

func TestRunCommand_QuietFlag(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("quiet")
	if flag == nil {
		t.Fatal("expected --quiet flag on run command")
	}
	if flag.DefValue != "false" {
		t.Errorf("default quiet = %q, want %q", flag.DefValue, "false")
	}
	if flag.Usage != "suppress dashboard output for quiet or CI-oriented runs" {
		t.Errorf("quiet usage = %q", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--quiet") {
		t.Fatal("expected run command long help text to mention --quiet")
	}
}

func TestRunCommand_RuntimeLogFlags(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	defaults := logging.DefaultRuntimeLogConfig()
	tests := []struct {
		name    string
		def     string
		usageIn string
	}{
		{name: "runtime-log-dir", def: "", usageIn: "directory for structured runtime log files"},
		{name: "runtime-log-max-size-mb", def: "100", usageIn: "rotate each runtime log file"},
		{name: "runtime-log-max-backups", def: "20", usageIn: "maximum rotated runtime log files"},
		{name: "runtime-log-max-age-days", def: "30", usageIn: "maximum days to retain rotated runtime log files"},
		{name: "runtime-log-compress", def: "false", usageIn: "compress rotated runtime log files"},
	}
	tests[1].def = strconv.Itoa(defaults.MaxSize)
	tests[2].def = strconv.Itoa(defaults.MaxBackups)
	tests[3].def = strconv.Itoa(defaults.MaxAge)

	for _, tc := range tests {
		flag := runCmd.Flags().Lookup(tc.name)
		if flag == nil {
			t.Fatalf("expected --%s flag on run command", tc.name)
		}
		if flag.DefValue != tc.def {
			t.Fatalf("--%s default = %q, want %q", tc.name, flag.DefValue, tc.def)
		}
		if !strings.Contains(flag.Usage, tc.usageIn) {
			t.Fatalf("--%s usage = %q, want to contain %q", tc.name, flag.Usage, tc.usageIn)
		}
	}
	if !strings.Contains(runCmd.Long, "Runtime logs are structured JSON rolling files") {
		t.Fatal("expected run command long help text to document runtime log behavior")
	}
	if !strings.Contains(runCmd.Long, "stdout/stderr only on command failures") {
		t.Fatal("expected run command long help text to document command output policy")
	}
}

func TestRunCommand_QuietFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--quiet",
		"--dir", "custom-factory",
		"--workflow", "workflow-1",
		"--work", "work.json",
		"--record", "record.replay.json",
		"--port", "0",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --quiet: %v", err)
	}

	if !got.SuppressDashboardRendering {
		t.Fatal("expected --quiet to suppress dashboard rendering")
	}
	if got.Dir != "custom-factory" {
		t.Errorf("dir = %q, want %q", got.Dir, "custom-factory")
	}
	if got.Workflow != "workflow-1" {
		t.Errorf("workflow = %q, want %q", got.Workflow, "workflow-1")
	}
	if got.WorkFile != "work.json" {
		t.Errorf("work file = %q, want %q", got.WorkFile, "work.json")
	}
	if got.RecordPath != "record.replay.json" {
		t.Errorf("record path = %q, want %q", got.RecordPath, "record.replay.json")
	}
	if got.Port != 0 {
		t.Errorf("port = %d, want %d", got.Port, 0)
	}
	if got.AutoPort {
		t.Fatal("expected explicit --port to disable automatic port resolution")
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set logger")
	}
}

func TestRunCommand_RuntimeLogFlagsMapToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--runtime-log-dir", "logs/runtime",
		"--runtime-log-max-size-mb", "11",
		"--runtime-log-max-backups", "12",
		"--runtime-log-max-age-days", "13",
		"--runtime-log-compress",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with runtime log flags: %v", err)
	}

	if got.RuntimeLogDir != "logs/runtime" {
		t.Fatalf("runtime log dir = %q, want logs/runtime", got.RuntimeLogDir)
	}
	want := logging.RuntimeLogConfig{MaxSize: 11, MaxBackups: 12, MaxAge: 13, Compress: true}
	if got.RuntimeLogConfig != want {
		t.Fatalf("runtime log config = %#v, want %#v", got.RuntimeLogConfig, want)
	}
}

func TestRunCommand_WithMockWorkersFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--with-mock-workers", "mock-workers.json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --with-mock-workers: %v", err)
	}

	if !got.MockWorkersEnabled {
		t.Fatal("expected --with-mock-workers to enable mock workers")
	}
	if got.MockWorkersConfigPath != "mock-workers.json" {
		t.Fatalf("mock workers config path = %q, want %q", got.MockWorkersConfigPath, "mock-workers.json")
	}
}

func TestRunCommand_WithMockWorkersFlagWithoutPathMapsToDefaultConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--with-mock-workers"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --with-mock-workers without path: %v", err)
	}

	if !got.MockWorkersEnabled {
		t.Fatal("expected --with-mock-workers to enable mock workers")
	}
	if got.MockWorkersConfigPath != "" {
		t.Fatalf("mock workers config path = %q, want empty default path", got.MockWorkersConfigPath)
	}
}

func TestRunCommand_VerboseFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --verbose: %v", err)
	}

	if !got.Verbose {
		t.Fatal("expected --verbose to enable service verbose logging")
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set logger")
	}
}

func TestSubmitCommand_HelpAdvertisesWorkTypeNameOnly(t *testing.T) {
	root := NewRootCommand()
	submitCmd, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit: %v", err)
	}

	for _, name := range []string{"work-type-name", "payload"} {
		f := submitCmd.Flags().Lookup(name)
		if f == nil {
			t.Errorf("expected --%s flag on submit command", name)
			continue
		}
	}
	for _, name := range []string{"factory", "factory-id", "work-type-id"} {
		if f := submitCmd.Flags().Lookup(name); f != nil {
			t.Fatalf("submit command should not expose --%s", name)
		}
	}

	var out bytes.Buffer
	root = NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --help: %v", err)
	}

	help := out.String()
	if !strings.Contains(help, "--work-type-name") {
		t.Fatalf("submit help should list --work-type-name:\n%s", help)
	}
	if !strings.Contains(help, "work type name to submit to") {
		t.Fatalf("submit help should describe work type names:\n%s", help)
	}
	for _, disallowed := range []string{"--work-type-id", "--factory-id", "--factory"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("submit help should not list %s:\n%s", disallowed, help)
		}
	}
}

func TestSubmitCommand_WorkTypeIDFlagIsRejected(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--work-type-id", "legacy-task",
		"--payload", "request.md",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected removed --work-type-id flag to fail")
	}
	if !strings.Contains(err.Error(), "unknown flag: --work-type-id") {
		t.Fatalf("removed flag error = %q, want unknown flag", err.Error())
	}
	if called {
		t.Fatal("submit command should not run when --work-type-id is supplied")
	}
}

func TestSubmitCommand_MissingWorkTypeNameReturnsLocalValidationError(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return submitcli.Submit(cfg)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--payload", "work.json"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing work type name to fail")
	}
	if !called {
		t.Fatal("expected submit validation to run")
	}
	if got := err.Error(); got != "--work-type-name is required" {
		t.Fatalf("missing work type error = %q, want --work-type-name is required", got)
	}
}

func TestSubmitCommand_MissingPayloadReturnsLocalValidationError(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return submitcli.Submit(cfg)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--work-type-name", "tasks"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing payload to fail")
	}
	if !called {
		t.Fatal("expected submit validation to run")
	}
	if got := err.Error(); got != "--payload is required" {
		t.Fatalf("missing payload error = %q, want --payload is required", got)
	}
}

func TestSubmitCommand_WorkTypeNameFlagMapsToSubmitConfig(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--work-type-name", "tasks",
		"--payload", "request.md",
		"--port", "7437",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --work-type-name: %v", err)
	}

	if got.WorkTypeName != "tasks" {
		t.Fatalf("work type name = %q, want tasks", got.WorkTypeName)
	}
	if got.Payload != "request.md" {
		t.Fatalf("payload = %q, want request.md", got.Payload)
	}
	if got.Port != 7437 {
		t.Fatalf("port = %d, want 7437", got.Port)
	}
}
