package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

func TestModelsDocumentation_ExamplesReachCurrentCLIBoundary(t *testing.T) {
	doc, err := docscli.Markdown("models")
	if err != nil {
		t.Fatalf("Markdown(models) error = %v", err)
	}
	requireDocumentedModelCommands(t, doc)

	originalList := listModels
	originalInspect := inspectModel
	originalPull := pullModel
	originalInvoke := invokeModel
	defer func() {
		listModels = originalList
		inspectModel = originalInspect
		pullModel = originalPull
		invokeModel = originalInvoke
	}()

	var listed bool
	var inspected, pulled string
	var invocations []modelscli.InvokeConfig
	listModels = func(modelscli.ListConfig) error { listed = true; return nil }
	inspectModel = func(cfg modelscli.InspectConfig) error { inspected = cfg.ModelName; return nil }
	pullModel = func(cfg modelscli.PullConfig) error { pulled = cfg.ModelName; return nil }
	invokeModel = func(cfg modelscli.InvokeConfig) error {
		invocations = append(invocations, cfg)
		return nil
	}

	executeDocumentedModelExample(t, []string{"models", "list"})
	executeDocumentedModelExample(t, []string{"models", "inspect", "OMNIVOICE_Q4_K_M"})
	executeDocumentedModelExample(t, []string{"models", "pull", "OMNIVOICE_Q4_K_M"})
	executeDocumentedModelExample(t, []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "Read the release summary.", "--output", "./speech.wav"})
	executeDocumentedModelExample(t, []string{"--json", "models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "Read the release summary."})

	assertDocumentedModelConfigs(t, listed, inspected, pulled, invocations)
	assertModelCommandsRequireName(t)
}

func requireDocumentedModelCommands(t *testing.T, doc string) {
	t.Helper()
	for _, command := range []string{
		"you models list",
		"you models inspect OMNIVOICE_Q4_K_M",
		"you models pull OMNIVOICE_Q4_K_M",
		`you models invoke OMNIVOICE_Q4_K_M --operation TTS --text "Read the release summary." --output ./speech.wav`,
		`you --json models invoke OMNIVOICE_Q4_K_M --operation TTS --text "Read the release summary."`,
	} {
		if !strings.Contains(doc, command) {
			t.Fatalf("packaged models guide missing executable example %q", command)
		}
	}
}

func assertDocumentedModelConfigs(t *testing.T, listed bool, inspected, pulled string, invocations []modelscli.InvokeConfig) {
	t.Helper()
	if !listed || inspected != "OMNIVOICE_Q4_K_M" || pulled != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("documented model task boundary = listed %t, inspected %q, pulled %q", listed, inspected, pulled)
	}
	if len(invocations) != 2 {
		t.Fatalf("documented invocations reaching model boundary = %d, want 2", len(invocations))
	}
	if got := invocations[0]; got.ModelName != "OMNIVOICE_Q4_K_M" || got.Operation != "TTS" || got.Text != "Read the release summary." || got.OutputPath != "./speech.wav" || got.JSON {
		t.Fatalf("documented audio invocation = %#v, want complete model, operation, text, and output", got)
	}
	if got := invocations[1]; got.ModelName != "OMNIVOICE_Q4_K_M" || got.Operation != "TTS" || got.Text != "Read the release summary." || got.OutputPath != "" || !got.JSON {
		t.Fatalf("documented JSON invocation = %#v, want complete model, operation, text, and JSON mode", got)
	}
}

func assertModelCommandsRequireName(t *testing.T) {
	t.Helper()
	for _, subcommand := range []string{"inspect", "pull", "invoke"} {
		root := newLegacyTestRootCommand()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"models", subcommand})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "accepts 1 arg(s), received 0") {
			t.Fatalf("models %s without model error = %v, want required-model failure", subcommand, err)
		}
	}
}

func executeDocumentedModelExample(t *testing.T, args []string) {
	t.Helper()
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %q: %v", strings.Join(args, " "), err)
	}
}

func TestConfigDocumentation_ExamplesReachCurrentCLIPathBoundary(t *testing.T) {
	doc, err := docscli.Markdown("config")
	if err != nil {
		t.Fatalf("Markdown(config) error = %v", err)
	}
	for _, command := range []string{
		"you config init",
		"you factory config validate ./factory/factory.json",
		"you factory config flatten ./factory > ./dist/factory.json",
		"you factory config expand ./dist/factory.json",
	} {
		if !strings.Contains(doc, command) {
			t.Fatalf("packaged config guide missing executable example %q", command)
		}
	}

	originalValidate := validateFactory
	originalFlatten := flattenFactoryConfig
	originalExpand := expandFactoryConfig
	defer func() {
		validateFactory = originalValidate
		flattenFactoryConfig = originalFlatten
		expandFactoryConfig = originalExpand
	}()

	var validated, flattened, expanded string
	validateFactory = func(cfg factorycli.ValidateConfig) error {
		validated = cfg.Path
		return nil
	}
	flattenFactoryConfig = func(cfg configcli.FactoryConfigFlattenConfig) error {
		flattened = cfg.Path
		return nil
	}
	expandFactoryConfig = func(cfg configcli.FactoryConfigExpandConfig) error {
		expanded = cfg.Path
		return nil
	}

	executeDocumentedConfigExample(t, []string{"factory", "config", "validate", "./factory/factory.json"})
	executeDocumentedConfigExample(t, []string{"factory", "config", "flatten", "./factory"})
	executeDocumentedConfigExample(t, []string{"factory", "config", "expand", "./dist/factory.json"})

	if validated != "./factory/factory.json" || flattened != "./factory" || expanded != "./dist/factory.json" {
		t.Fatalf("documented config paths = validate %q, flatten %q, expand %q", validated, flattened, expanded)
	}

	for _, subcommand := range []string{"validate", "flatten", "expand"} {
		root := newLegacyTestRootCommand()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"factory", "config", subcommand})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "accepts 1 arg(s), received 0") {
			t.Fatalf("factory config %s without path error = %v, want required-path failure", subcommand, err)
		}
	}
}

func executeDocumentedConfigExample(t *testing.T, args []string) {
	t.Helper()
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %q: %v", strings.Join(args, " "), err)
	}
}

func TestRunDocumentation_RepresentativeExamplesReachCurrentCLIBoundary(t *testing.T) {
	doc, err := docscli.Markdown("run")
	if err != nil {
		t.Fatalf("Markdown(run) error = %v", err)
	}
	for _, command := range []string{
		"you run --work ./docs/examples/startup-work.json",
		`you run --factory ./factory.json "Review the release notes"`,
		"you run --dir ./factory --work ./docs/examples/startup-work.json",
	} {
		if !strings.Contains(doc, command) {
			t.Fatalf("packaged run guide missing executable example %q", command)
		}
	}

	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var runs []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		runs = append(runs, cfg)
		return nil
	}

	executeDocumentedRunExample(t, []string{"run", "--work", "./docs/examples/startup-work.json"})
	factoryPath := writePortableFactoryWithDefaultHandling(t, t.TempDir())
	executeDocumentedRunExample(
		t,
		[]string{"run", "--factory", factoryPath, "Review the release notes"},
		programmedTextInvocationInput(work.InputSourcePositionalText, "Review the release notes"),
	)
	executeDocumentedRunExample(t, []string{"run", "--dir", "./factory", "--work", "./docs/examples/startup-work.json"})

	if len(runs) != 3 {
		t.Fatalf("documented examples reaching run boundary = %d, want 3", len(runs))
	}
	if got := runs[0].WorkFile; got != "./docs/examples/startup-work.json" {
		t.Fatalf("default run WorkFile = %q, want explicit documented Work", got)
	}
	if runs[1].InvocationPositionalText == nil || *runs[1].InvocationPositionalText != "Review the release notes" {
		t.Fatalf("factory invocation input = %#v, want documented positional text", runs[1].InvocationPositionalText)
	}
	if got := runs[2].WorkFile; got != "./docs/examples/startup-work.json" {
		t.Fatalf("directory batch WorkFile = %q, want explicit documented Work", got)
	}
}

func TestRunDocumentation_InvocationOutputModeExamplesReachCurrentCLIBoundary(t *testing.T) {
	doc, err := docscli.Markdown("run")
	if err != nil {
		t.Fatalf("Markdown(run) error = %v", err)
	}
	for _, marker := range []string{
		"### Primary-result mode (default)",
		"### Human Factory Event stream mode",
		"### NDJSON automation mode",
		"recordType=factory_event",
		"recordType=invocation_result",
		`you run --named team-review --output response-stream "Review the release notes"`,
		`you --json run --factory ./factory.json --output response-stream "Summarize the changelog"`,
		"`you docs config`",
	} {
		if !strings.Contains(doc, marker) {
			t.Fatalf("packaged run guide missing output-mode marker %q", marker)
		}
	}
	for _, retired := range []string{
		"recordType=progress",
		"recordType=compaction",
		"recordType=primary_result",
		"PROGRESS_FRAGMENT",
		"STREAM_COMPACTION_SIGNAL",
	} {
		if strings.Contains(doc, retired) {
			t.Fatalf("packaged run guide advertises retired output record %q", retired)
		}
	}

	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var runs []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		runs = append(runs, cfg)
		return nil
	}

	factoryPath := writePortableFactoryWithDefaultHandling(t, t.TempDir())
	executeDocumentedRunExample(t, []string{"run", "--factory", factoryPath, "Summarize the changelog"})
	executeDocumentedRunExample(t, []string{"run", "--factory", factoryPath, "--output", "response-stream", "Summarize the changelog"})
	executeDocumentedRunExample(t, []string{"--json", "run", "--factory", factoryPath, "--output", "response-stream", "Summarize the changelog"})

	if len(runs) != 3 {
		t.Fatalf("documented output-mode examples reaching run boundary = %d, want 3", len(runs))
	}
	if runs[0].InvocationOutputMode != runcli.InvocationOutputPrimaryResult {
		t.Fatalf("primary-result mode = %q, want default primary output", runs[0].InvocationOutputMode)
	}
	if runs[1].InvocationOutputMode != runcli.InvocationOutputResponseStream || runs[1].JSONOutput {
		t.Fatalf("human response-stream mode = %#v, want response-stream without JSON", runs[1])
	}
	if runs[2].InvocationOutputMode != runcli.InvocationOutputResponseStream || !runs[2].JSONOutput {
		t.Fatalf("NDJSON response-stream mode = %#v, want response-stream with JSON", runs[2])
	}
}

func executeDocumentedRunExample(t *testing.T, args []string, prepare ...rootInvocationInputScript) {
	t.Helper()
	root := newLegacyTestRootCommand()
	if len(prepare) > 0 {
		root = newLegacyTestRootCommandWithInvocationInput(prepare[0])
	}
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %q: %v", strings.Join(args, " "), err)
	}
}

func TestDocsCommand_NoTopicPrintsDocsIndex(t *testing.T) {
	var stdout bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"docs"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute docs: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"# Docs",
		"`you docs agents` for orientation",
		"`you submit batch`",
		"`you session list`",
		"`--verbose` or `--debug`",
		"Packaged reference topics:",
		"`agents` - Agent orientation: read order, work submission, command matrix, planner vs executor, and topic router",
		"`authoring-factories` - Practical factory authoring workflow",
		"`run` - Supported local, one-shot, batch, continuous, and mock-worker run shapes",
		"`config` - Operator initialization and Factory validation, flattening, expansion, and minimum authoring contract",
		"`mock-workers` - Mock-worker runs",
		"`record-replay` - Record and replay run modes",
		"`guards` - Workstation, input, and factory guards",
		"`relationships` - Batch DEPENDS_ON",
		"`work` - Submitted work: session-scoped work routes, tags, batch cross-links, and submission contracts",
		"`sessions` - Live factory sessions: session list, session show, pause and resume, factory query, status API, dashboard URL, and run modes",
		"`workstations` - Workstation kinds",
		"`workers` - Worker types",
		"`batch-inputs` - Batch input files",
		"`you docs agents`",
		"`you docs authoring-factories`",
		"`you docs run`",
		"`you docs config`",
		"`you docs mock-workers`",
		"`you docs record-replay`",
		"`you docs guards`",
		"`you docs relationships`",
		"`you docs work`",
		"`you docs sessions`",
		"`you docs workstations`",
		"`you docs workers`",
		"`you docs batch-inputs`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("docs index missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"Usage:",
		"Available Commands:",
		"`mcp-hosts` -",
		"`packaged-fusion` -",
		"`packaged-goal` -",
		"`packaged-tts` -",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("docs index should not fall back to Cobra help marker %q:\n%s", unwanted, got)
		}
	}
	for _, topic := range []string{"run", "config", "models", "mcp", "javascript-workflows"} {
		if count := strings.Count(got, "- `"+topic+"` -"); count != 1 {
			t.Fatalf("docs index lists canonical topic %q %d times, want exactly once:\n%s", topic, count, got)
		}
	}
}

func TestDocsCommand_HelpStillUsesCobraHelp(t *testing.T) {
	var stdout bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"docs", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute docs --help: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"Print packaged markdown reference topics",
		"Usage:",
		"docs [topic]",
		"Run without a topic to print the quick-start blurb and packaged docs index.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("docs help missing %q:\n%s", want, got)
		}
	}
}

func TestDocsCommand_BatchAndRelationshipTopicsUseOpenAPICamelCaseFieldNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		topic  string
		want   []string
		absent []string
	}{
		{
			topic: "batch-inputs",
			want: []string{
				"# Batch Inputs",
				"requestId",
				"workTypeName",
				"sourceWorkName",
				"targetWorkName",
			},
			absent: []string{"work_type_name", "source_work_name"},
		},
		{
			topic: "relationships",
			want: []string{
				"# Relationships",
				"requestId",
				"workTypeName",
				"sourceWorkName",
				"targetWorkName",
			},
			absent: []string{"work_type_name", "source_work_name", "target_work_name"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.topic, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"docs", tc.topic})

			if err := root.Execute(); err != nil {
				t.Fatalf("execute docs %s: %v", tc.topic, err)
			}

			got := stdout.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("docs %s missing %q:\n%s", tc.topic, want, got)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Fatalf("docs %s still contains retired field %q:\n%s", tc.topic, absent, got)
				}
			}
		})
	}
}

func TestDocsCommand_CanonicalOperatorTopicsResolve(t *testing.T) {
	t.Parallel()

	cases := []struct {
		topic   string
		heading string
	}{
		{topic: "run", heading: "# Run"},
		{topic: "config", heading: "# Config"},
		{topic: "models", heading: "# Models"},
		{topic: "mcp", heading: "# MCP Host Setup"},
		{topic: "javascript-workflows", heading: "# JavaScript Workflows"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.topic, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"docs", tc.topic})

			if err := root.Execute(); err != nil {
				t.Fatalf("execute docs %s: %v", tc.topic, err)
			}
			if got := stdout.String(); !strings.Contains(got, tc.heading) {
				t.Fatalf("docs %s missing heading %q:\n%s", tc.topic, tc.heading, got)
			}
		})
	}
}

func TestDocsCommand_RetiredTopicsAreUnsupportedWithoutAliases(t *testing.T) {
	t.Parallel()

	for _, topic := range []string{"packaged-fusion", "packaged-goal", "packaged-tts", "mcp-hosts"} {
		topic := topic
		t.Run(topic, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"docs", topic})

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), `unsupported docs topic "`+topic+`"`) {
				t.Fatalf("execute docs %s error = %v, want unsupported-topic error", topic, err)
			}
			if got := stdout.String(); got != "" {
				t.Fatalf("unsupported docs topic %s wrote stdout %q", topic, got)
			}
		})
	}
}

func TestDocsCommand_UnsupportedTopicReturnsCanonicalTopicError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"docs", "unknown"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: agents, authoring-factories, run, config, mock-workers, record-replay, guards, relationships, work, sessions, orchestrators, javascript-workflows, mcp, workstations, workers, resources, models, batch-inputs, templates)` {
		t.Fatalf("unexpected docs error %q", got)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("unsupported topic should not write stdout, got %q", got)
	}
	if got := stderr.String(); strings.Contains(got, "unknown command") || strings.Contains(got, "Available Commands:") {
		t.Fatalf("unsupported topic should not fall back to cobra unknown-command output:\n%s", got)
	}
}

func TestRootCommand_HelpDocumentsConciseOrientation(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	long := root.Long
	if len(long)+len(root.Example) > 1200 {
		t.Fatalf("root Long+Example length = %d, want <= 1200", len(long)+len(root.Example))
	}

	for _, want := range []string{
		"What:",
		"How to use:",
		"Agents:",
		"you run --work ./docs/examples/startup-work.json",
		"http://localhost:7437/dashboard/ui",
		"you docs agents",
		"you submit batch",
		"you session list",
		"Use --verbose or --debug for stderr diagnostics; full policy in you docs",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	for _, disallowed := range []string{
		"goreleaser",
		"GoReleaser",
		"Default command output is customer-facing",
		"Supported docs topics:",
		"must not include full prompts",
		"full work payloads",
		"printf \"Fix the lint issues",
		"you docs workstations",
	} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("root help should not include %q:\n%s", disallowed, help)
		}
	}

	for _, section := range []struct {
		name  string
		limit int
	}{
		{name: "What", limit: 4},
		{name: "How to use", limit: 4},
		{name: "Agents", limit: 4},
	} {
		lines := sectionProseLines(long, section.name)
		if len(lines) == 0 {
			t.Fatalf("root Long missing prose for section %q", section.name)
		}
		if len(lines) > section.limit {
			t.Fatalf("root Long section %q has %d prose lines, want <= %d:\n%s", section.name, len(lines), section.limit, strings.Join(lines, "\n"))
		}
	}
}

func TestRunCommand_HelpUsesCanonicalDocsAndCompleteInputs(t *testing.T) {
	var stdout bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --help: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"you run --work ./docs/examples/startup-work.json",
		"you run --dir factory --work ./docs/examples/startup-work.json",
		`you run --named @you/tts --output primary "Read the release summary."`,
		"you docs authoring-factories",
		"you docs run",
		"you docs sessions",
		"you docs models",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run help missing %q:\n%s", want, got)
		}
	}
	for _, retired := range []string{
		"run you with no arguments",
		"\n  you\n",
		"you docs packaged-fusion",
		"you docs packaged-goal",
		"you docs packaged-tts",
		"you run --named @you/tts\n",
		"~/.you-agent-factory/you-agent-factories",
	} {
		if strings.Contains(got, retired) {
			t.Fatalf("run help contains retired or incomplete guidance %q:\n%s", retired, got)
		}
	}
}

func sectionProseLines(long, sectionName string) []string {
	marker := sectionName + ":\n"
	start := strings.Index(long, marker)
	if start < 0 {
		return nil
	}
	start += len(marker)
	rest := long[start:]
	nextSection := strings.Index(rest, ":\n")
	if nextSection >= 0 {
		// Only trim at the next labeled section header (word(s) ending with colon on its own line).
		if idx := strings.Index(rest, "\n\n"); idx >= 0 && idx < nextSection {
			rest = rest[:idx]
		}
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}
	return strings.Split(rest, "\n")
}

func TestRootCommand_HelpDocumentsDiagnosticsOneLiner(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	long := root.Long
	agentSection := sectionProseLines(long, "Agents")
	if len(agentSection) == 0 {
		t.Fatal("expected Agents section prose in root Long")
	}
	lastLine := agentSection[len(agentSection)-1]
	for _, want := range []string{"--verbose", "--debug", "stderr diagnostics", "full policy in you docs"} {
		if !strings.Contains(lastLine, want) {
			t.Fatalf("Agents diagnostics one-liner missing %q in %q", want, lastLine)
		}
	}
	for _, absent := range []string{
		"JSON stdout remains parseable",
		"access tokens",
		"full model input text",
		"sensitive generated content",
	} {
		if strings.Contains(long, absent) {
			t.Fatalf("root Long should not include full diagnostics policy marker %q", absent)
		}
	}
}

func TestDocsCommand_VerboseLogsTopicResolutionWithoutChangingMarkdown(t *testing.T) {
	wantMarkdown, err := docscli.Markdown("config")
	if err != nil {
		t.Fatalf("Markdown(config): %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"docs", "config", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute docs config --verbose: %v", err)
	}

	if got := stdout.String(); got != wantMarkdown {
		t.Fatalf("stdout markdown changed\nwant:\n%s\ngot:\n%s", wantMarkdown, got)
	}
	got := stderr.String()
	for _, want := range []string{
		"docs request topic=config",
		"docs resolved topic=config",
		"contentBytes=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr missing %q:\n%s", want, got)
		}
	}
}

func TestInitCommand_DefaultDir(t *testing.T) {
	root := newLegacyTestRootCommand()
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
	if executorFlag.DefValue != factorydefinitions.DefaultStarterExecutor {
		t.Errorf("default executor = %q, want %q", executorFlag.DefValue, factorydefinitions.DefaultStarterExecutor)
	}
}

func TestInitCommand_ExecutorFlagMapsToInitConfig(t *testing.T) {
	originalInitFactory := initFactory
	defer func() {
		initFactory = originalInitFactory
	}()

	var got factorydefinitions.ScaffoldConfig
	initFactory = func(cfg factorydefinitions.ScaffoldConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
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

func TestInitCommand_HelpDocumentsProviderModelSetup(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Configure provider and model defaults",
		"--provider",
		"--model",
		"provider must be registered",
		"any non-empty model identifier",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("init help missing %q:\n%s", want, help)
		}
	}
}
