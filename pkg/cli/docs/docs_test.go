package docs

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestTopicDocuments_ExplicitMetadataRemainsDeterministic(t *testing.T) {
	t.Parallel()

	if len(topicDocuments) == 0 {
		t.Fatal("topicDocuments must not be empty")
	}

	seenOrders := make(map[int]Topic, len(topicDocuments))
	seenCommands := make(map[string]Topic, len(topicDocuments))
	for _, doc := range topicDocuments {
		if doc.displayOrder <= 0 {
			t.Fatalf("topic %q displayOrder = %d, want positive explicit order", doc.topic, doc.displayOrder)
		}
		if strings.TrimSpace(doc.description) == "" {
			t.Fatalf("topic %q description is empty", doc.topic)
		}
		if strings.TrimSpace(doc.path) == "" {
			t.Fatalf("topic %q path is empty", doc.topic)
		}
		if prior, exists := seenOrders[doc.displayOrder]; exists {
			t.Fatalf("displayOrder %d reused by %q and %q", doc.displayOrder, prior, doc.topic)
		}
		seenOrders[doc.displayOrder] = doc.topic

		commands := []string{string(doc.topic)}
		for _, alias := range doc.aliases {
			commands = append(commands, string(alias))
		}
		for _, command := range commands {
			if prior, exists := seenCommands[command]; exists {
				t.Fatalf("docs topic command %q reused by %q and %q", command, prior, doc.topic)
			}
			seenCommands[command] = doc.topic
		}
	}
}

func TestSupportedTopics_ReturnsFixedTopicOrder(t *testing.T) {
	t.Parallel()

	want := []string{
		"agents",
		"authoring-factories",
		"run",
		"config",
		"mock-workers",
		"record-replay",
		"guards",
		"relationships",
		"work",
		"sessions",
		"orchestrators",
		"javascript-workflows",
		"mcp",
		"workstations",
		"workers",
		"resources",
		"models",
		"batch-inputs",
		"templates",
	}

	if got := SupportedTopics(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedTopics() = %#v, want %#v", got, want)
	}

	got := SupportedTopics()
	got[0] = "mutated"
	if again := SupportedTopics(); !reflect.DeepEqual(again, want) {
		t.Fatalf("SupportedTopics() exposed mutable internal state: %#v", again)
	}
}

func TestSupportedTopicCommands_ReturnsCanonicalTopicsAndAliases(t *testing.T) {
	t.Parallel()

	want := []string{
		"agents",
		"authoring-factories",
		"run",
		"config",
		"mock-workers",
		"record-replay",
		"guards",
		"relationships",
		"work",
		"sessions",
		"orchestrators",
		"javascript-workflows",
		"mcp",
		"workstations",
		"workstation",
		"workers",
		"resources",
		"models",
		"batch-inputs",
		"batch-work",
		"templates",
	}

	if got := SupportedTopicCommands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedTopicCommands() = %#v, want %#v", got, want)
	}
}

func TestTopicRegistry_OrdersTopicsAndClonesAliases(t *testing.T) {
	t.Parallel()

	source := []topicDocument{
		{topic: TopicTemplates, description: "templates", path: "templates.md", displayOrder: 20},
		{topic: TopicConfig, description: "config", path: "config.md", displayOrder: 10, aliases: []Topic{TopicBatchWorkAlias}},
	}

	registry := newTopicRegistry(source)
	if got, want := []Topic{registry.ordered[0].topic, registry.ordered[1].topic}, []Topic{TopicConfig, TopicTemplates}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registry order = %#v, want %#v", got, want)
	}

	source[1].aliases[0] = TopicWorkstationAlias
	if got, want := registry.commandToSource[string(TopicBatchWorkAlias)].topic, TopicConfig; got != want {
		t.Fatalf("alias lookup after source mutation = %q, want %q", got, want)
	}
	if slices.Contains(registry.commandToSource[string(TopicConfig)].aliases, TopicWorkstationAlias) {
		t.Fatal("registry should clone alias slices instead of sharing source storage")
	}
}

func TestTopicSummaries_ReturnsTopicDescriptionsInSupportedOrder(t *testing.T) {
	t.Parallel()

	summaries := TopicSummaries()
	if len(summaries) != len(SupportedTopics()) {
		t.Fatalf("TopicSummaries() length = %d, want %d", len(summaries), len(SupportedTopics()))
	}
	for i, topic := range SupportedTopics() {
		if summaries[i].Name != topic {
			t.Fatalf("TopicSummaries()[%d].Name = %q, want %q", i, summaries[i].Name, topic)
		}
		if strings.TrimSpace(summaries[i].Description) == "" {
			t.Fatalf("TopicSummaries()[%d].Description is empty", i)
		}
	}
}

func TestQuickStartMarkdown_AtMostSixLines(t *testing.T) {
	t.Parallel()

	got := QuickStartMarkdown("you")
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) > 6 {
		t.Fatalf("QuickStartMarkdown() has %d lines, want <= 6:\n%s", len(lines), got)
	}
	for _, want := range []string{
		"`you docs agents`",
		"`you submit batch`",
		"`you session list`",
		"`--verbose` or `--debug`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("QuickStartMarkdown() missing %q:\n%s", want, got)
		}
	}
}

func TestIndexMarkdown_ListsSupportedTopicsWithCommands(t *testing.T) {
	t.Parallel()

	got := IndexMarkdown("you")
	for _, want := range []string{
		"# Docs",
		"`agents` - Agent orientation",
		"`authoring-factories` - Practical factory authoring workflow",
		"`run` - Supported local, one-shot, batch, continuous, and mock-worker run shapes",
		"`config` - Operator initialization and Factory validation",
		"`mock-workers` - Mock-worker runs",
		"`record-replay` - Record and replay run modes",
		"`work` - Submitted work",
		"`sessions` - Live factory sessions",
		"`workstations` - Workstation kinds",
		"`workers` - Worker types",
		"`resources` - Resource capacity",
		"`models` - Local and hosted model setup",
		"`batch-inputs` - Batch input files",
		"`templates` - Prompt template variables",
		"`you docs agents`",
		"`you docs authoring-factories`",
		"`you docs run`",
		"`you docs config`",
		"`you docs mock-workers`",
		"`you docs record-replay`",
		"`you docs work`",
		"`you docs sessions`",
		"`you docs workstations`",
		"`you docs batch-inputs`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("IndexMarkdown() missing %q:\n%s", want, got)
		}
	}
	for _, alias := range []string{"`batch-work`", "`workstation`"} {
		if strings.Contains(got, alias) {
			t.Fatalf("IndexMarkdown() should list canonical topics without %s alias noise:\n%s", alias, got)
		}
	}
}

func TestMarkdown_ReturnsRawPackagedMarkdownForEachSupportedTopic(t *testing.T) {
	t.Parallel()

	for _, doc := range topicRegistry.ordered {
		doc := doc
		t.Run(string(doc.topic), func(t *testing.T) {
			t.Parallel()

			got, err := Markdown(string(doc.topic))
			if err != nil {
				t.Fatalf("Markdown(%q) error = %v", doc.topic, err)
			}

			wantPath := filepath.Join(testutil.MustRepoRoot(t), "docs", "reference", doc.path)
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", wantPath, err)
			}

			if got != string(want) {
				t.Fatalf("Markdown(%q) did not return the raw authored markdown", doc.topic)
			}
			if strings.TrimSpace(got) == "" {
				t.Fatalf("Markdown(%q) returned empty content", doc.topic)
			}
		})
	}
}

func TestMarkdown_AgentsReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("agents")
	if err != nil {
		t.Fatalf("Markdown(agents) error = %v", err)
	}

	for _, want := range []string{
		"# Agents",
		"## Read order",
		"## CLI-only ingress",
		"Autonomous agents must submit work only through the CLI",
		"## Batch submit for agents",
		"### Idempotency and duplicate work",
		"you submit batch",
		"requestId",
		"duplicate batches",
		"you submit batch ./batches/release-story-set.json",
		"## Is the factory running?",
		"you session list",
		"you factory query",
		"you docs sessions",
		"## Operator loop",
		"you work list --name",
		"you submit",
		"--name driver-incident-review",
		"--work-type-name task",
		"--payload request.md",
		"## Command matrix",
		"Operator-only",
		"## Topic router",
		"`you docs config`",
		"[Is the factory running?](#is-the-factory-running?)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(agents) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs agents`.",
		"## Start Here",
		"## Read Order (Any Factory)",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(agents) included wrapper text %q:\n%s", wrapper, got)
		}
	}
	lineCount := strings.Count(got, "\n") + 1
	if lineCount > 220 {
		t.Fatalf("Markdown(agents) line count = %d, want at most 220", lineCount)
	}
	topicRouter := got[strings.Index(got, "## Topic router"):]
	if strings.Contains(topicRouter, ".md)") {
		t.Fatalf("Markdown(agents) topic router must not use packaged-topic .md links:\n%s", topicRouter)
	}
}

func TestMarkdown_AgentsDocumentsMinimalOperatorSpotCheckFlow(t *testing.T) {
	t.Parallel()

	got, err := Markdown("agents")
	if err != nil {
		t.Fatalf("Markdown(agents) error = %v", err)
	}

	for _, cmd := range []string{
		"you session list",
		"you submit batch",
		"you work list --name",
	} {
		if !strings.Contains(got, cmd) {
			t.Fatalf("Markdown(agents) missing operator spot-check command %q", cmd)
		}
	}
	if !strings.Contains(got, "you submit batch ./") {
		t.Fatalf("Markdown(agents) missing file-path batch example (you submit batch ./…)")
	}
	operatorLoop := got[strings.Index(got, "## Operator loop"):]
	if operatorLoop == got {
		t.Fatal("Markdown(agents) missing ## Operator loop section")
	}
	for _, step := range []string{"Check running", "Submit", "Verify"} {
		if !strings.Contains(operatorLoop, step) {
			t.Fatalf("Markdown(agents) operator loop missing step %q", step)
		}
	}
}

func TestMarkdown_AuthoringFactoriesReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("authoring-factories")
	if err != nil {
		t.Fatalf("Markdown(authoring-factories) error = %v", err)
	}

	for _, want := range []string{
		"# Authoring Factories",
		"you run --factory ./factory.json \"Fix the lint issues\"",
		"handlingBehavior: [\"DEFAULT\"]",
		"you run --dir ./factory --with-mock-workers",
		"you docs mock-workers",
		"you docs record-replay",
		"you docs packaged-goal",
		"you docs packaged-tts",
		"you run --named @you/goal",
		"docs/examples/mock-workers.json",
		"requestId",
		"workTypeName",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(authoring-factories) missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{
		"work_type_name",
		"source_work_name",
		`"request_id"`,
	} {
		if strings.Contains(got, absent) {
			t.Fatalf("Markdown(authoring-factories) still contains retired field %q:\n%s", absent, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs authoring-factories`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(authoring-factories) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_RunDocumentsSupportedRunShapes(t *testing.T) {
	t.Parallel()

	got, err := Markdown("run")
	if err != nil {
		t.Fatalf("Markdown(run) error = %v", err)
	}

	for _, want := range []string{
		"# Run",
		"## Run the current Factory",
		"you run --work ./docs/examples/startup-work.json",
		"you run --dir ./factory --work ./docs/examples/startup-work.json",
		`you run --factory ./factory.json "Review the release notes"`,
		`you run --named team-review "Review the release notes"`,
		"you run --dir ./factory --continuously --work ./batches/release.json",
		"you run --dir ./factory --with-mock-workers --work ./docs/examples/startup-work.json",
		`you run --named team-review --output response-stream "Review the release notes"`,
		"configured primary result",
		"you docs batch-inputs",
		"you docs javascript-workflows",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(run) missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"you run --workflow",
		"you factory save",
		"you docs packaged-fusion",
		"you docs packaged-goal",
		"you docs packaged-tts",
		"\n```bash\nyou\n```",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Markdown(run) contains retired or incomplete invocation %q:\n%s", forbidden, got)
		}
	}
}

func TestMarkdown_ConfigOwnsCurrentOperatorAndFactoryConfigTasks(t *testing.T) {
	t.Parallel()

	got, err := Markdown("config")
	if err != nil {
		t.Fatalf("Markdown(config) error = %v", err)
	}

	for _, want := range []string{
		"## Initialize Operator And System Configuration",
		"you config init",
		"~/.you-agent-factory/config.json",
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER",
		"YOU_DEFAULT_WORKER_MODEL",
		"--default-worker-model-provider",
		"--default-worker-model",
		"file < env < flag",
		"## Validate Or Transform A Factory",
		"you factory config validate ./factory/factory.json",
		"you factory config validate ./factory",
		"you factory config flatten ./factory > ./dist/factory.json",
		"you factory config expand ./dist/factory.json",
		"Every command requires a concrete Factory",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(config) missing %q:\n%s", want, got)
		}
	}
}

func TestMarkdown_ConfigKeepsMinimumFactoryAuthoringContract(t *testing.T) {
	t.Parallel()

	got, err := Markdown("config")
	if err != nil {
		t.Fatalf("Markdown(config) error = %v", err)
	}

	for _, want := range []string{
		"## Minimum Factory Authoring Contract",
		"workTypes",
		"handlingBehavior",
		"workers",
		"workstations",
		"resources",
		"invocationSignature",
		"`invocationReturn`",
		"supportingFiles",
		"you docs workers",
		"you docs workstations",
		"you docs resources",
		"you docs run",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(config) missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"you config validate",
		"you config flatten",
		"you config expand",
		"you factory save",
		"## Run Controls",
		"## Clean invocation contract",
		"\n```bash\nyou\n```",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Markdown(config) still contains stale invocation contract %q:\n%s", forbidden, got)
		}
	}
}

func TestMarkdown_WorkReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("work")
	if err != nil {
		t.Fatalf("Markdown(work) error = %v", err)
	}

	for _, want := range []string{
		"# Submitted Work",
		"## Single-Work API Submission",
		"## Submission contract shapes",
		"SubmitWorkRequest",
		"WorkRequest",
		"`items` cannot be combined with `content` or `payload` on",
		"POST /factory-sessions/{session_id}/work",
		"workTypeName",
		"## CLI `you submit` success and verify loop",
		"Submitted: <name> (<workTypeName>)",
		"Verify: you work show <work-id>",
		"you work list --name <name>",
		"--work-type-name <type>",
		"`workId` is JSON `null`",
		"/factory-sessions/~default/work",
		"factory not reachable at <url>",
		"submission failed (<status>)",
		"Verbose request and response diagnostics",
		"stderr",
		"## Tags And Prompt Templates",
		"Token.Tags",
		"`you docs config`",
		"`you docs batch-inputs`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(work) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs work`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(work) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_BatchInputsAndCompatibilityAliasReturnRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	canonical, err := Markdown("batch-inputs")
	if err != nil {
		t.Fatalf("Markdown(batch-inputs) error = %v", err)
	}
	alias, err := Markdown("batch-work")
	if err != nil {
		t.Fatalf("Markdown(batch-work) error = %v", err)
	}

	if alias != canonical {
		t.Fatal("Markdown(batch-work) should return the canonical batch-inputs markdown")
	}
	for _, want := range []string{
		"# Batch Inputs",
		"## Batch ingress comparison",
		"`WorkRequest`",
		"works[].content",
		"`you submit batch`",
		"`you submit`",
		"`you run --work <path>`",
		"factory/inputs/BATCH/default/<request_id>.json",
		"## CLI batch submit (`you submit batch`)",
		"you submit batch --dry-run",
		"you submit batch --file",
		"cat batch.json | you submit batch",
		"## Quick reference",
		"## Before you submit",
		"factory.json",
		"factory/docs/overview.md",
		"factory/docs/README.md",
		"`you docs batch-work` is a compatibility alias",
		"FACTORY_REQUEST_BATCH",
		"DEPENDS_ON",
		"PARENT_CHILD",
		"factory/inputs/BATCH/default/<request_id>.json",
		"requestId",
		"workTypeName",
		"sourceWorkName",
		"targetWorkName",
		"requiredState",
		"## Visualize batch dependencies (`you work visualize`)",
		"you work visualize batch.json > my-graph.mermaid",
		"you work visualize --format markdown-mermaid batch.json > graph.md",
		"Graph nodes represent work items",
		"It does not submit",
		"render diagram images",
	} {
		if !strings.Contains(canonical, want) {
			t.Fatalf("Markdown(batch-inputs) missing %q:\n%s", want, canonical)
		}
	}
	for _, absent := range []string{
		"work_type_name",
		"source_work_name",
	} {
		if strings.Contains(canonical, absent) {
			t.Fatalf("Markdown(batch-inputs) still contains retired field %q:\n%s", absent, canonical)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs batch-inputs`.",
	} {
		if strings.Contains(canonical, wrapper) {
			t.Fatalf("Markdown(batch-inputs) included wrapper text %q:\n%s", wrapper, canonical)
		}
	}
}

func TestMarkdown_WorkstationsAndCompatibilityAliasReturnRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	canonical, err := Markdown("workstations")
	if err != nil {
		t.Fatalf("Markdown(workstations) error = %v", err)
	}
	alias, err := Markdown("workstation")
	if err != nil {
		t.Fatalf("Markdown(workstation) error = %v", err)
	}

	if alias != canonical {
		t.Fatal("Markdown(workstation) should return the canonical workstations markdown")
	}
	for _, want := range []string{
		"# Workstations Reference",
		"workstation authoring contract",
		"MODEL_WORKSTATION",
		"CLASSIFIER_WORKSTATION",
		"LOGICAL_MOVE",
	} {
		if !strings.Contains(canonical, want) {
			t.Fatalf("Markdown(workstations) missing %q:\n%s", want, canonical)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs workstations`.",
	} {
		if strings.Contains(canonical, wrapper) {
			t.Fatalf("Markdown(workstations) included wrapper text %q:\n%s", wrapper, canonical)
		}
	}
}

func TestMarkdown_MockWorkersReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("mock-workers")
	if err != nil {
		t.Fatalf("Markdown(mock-workers) error = %v", err)
	}

	for _, want := range []string{
		"# Mock Workers",
		"mockWorkers",
		"unmatchedDispatchPolicy",
		"passthrough",
		"you run --dir <factory> --with-mock-workers",
		"you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json",
		"docs/examples/mock-workers-script.json",
		"docs/examples/mock-workers-mixed.json",
		"runType",
		"accept",
		"reject",
		"script",
		"scriptConfig",
		"rejectConfig",
		"workingDirectory",
		"timeout",
		"default accepted",
		"docs/examples/mock-workers.json",
		"docs/examples/startup-work.json",
		"## Reviewer Verification",
		"you docs mock-workers",
		"Do not rely on a live real-agent passthrough run for signoff",
		"automated service and runner tests",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(mock-workers) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs mock-workers`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(mock-workers) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_GuardsReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("guards")
	if err != nil {
		t.Fatalf("Markdown(guards) error = %v", err)
	}

	for _, want := range []string{
		"# Guards",
		"## Quick Choice",
		"VISIT_COUNT",
		"MATCHES_FIELDS",
		"SAME_NAME",
		"ALL_CHILDREN_COMPLETE",
		"ANY_CHILD_FAILED",
		"INFERENCE_THROTTLE_GUARD",
		"LOGICAL_MOVE",
		"maxVisits",
		"matchInput",
		"limits.maxExecutionTime",
		"limits.maxRetries",
		"`you docs workstations`",
		"`you docs relationships`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(guards) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs guards`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(guards) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_RelationshipsReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("relationships")
	if err != nil {
		t.Fatalf("Markdown(relationships) error = %v", err)
	}

	for _, want := range []string{
		"# Relationships",
		"## Quick Choice",
		"DEPENDS_ON",
		"PARENT_CHILD",
		"SPAWNED_BY",
		"requiredState",
		"sourceWorkName",
		"targetWorkName",
		"workTypeName",
		"requestId",
		"FACTORY_REQUEST_BATCH",
		"Whole-Batch Validation",
		"Parent-Aware Guard Linkage",
		"ALL_CHILDREN_COMPLETE",
		"`you docs guards`",
		"`you docs batch-inputs`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(relationships) missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{
		"work_type_name",
		"source_work_name",
		"target_work_name",
	} {
		if strings.Contains(got, absent) {
			t.Fatalf("Markdown(relationships) still contains retired field %q:\n%s", absent, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs relationships`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(relationships) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_RecordReplayReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("record-replay")
	if err != nil {
		t.Fatalf("Markdown(record-replay) error = %v", err)
	}

	for _, want := range []string{
		"# Record and Replay",
		"~/.you-agent-factory/recordings/",
		"factory-session-~default-HHMMSS-<unique-id>.json",
		"Recording saved:",
		"you run --dir ./factory --record ./docs/examples/sample-run.replay.json",
		"you run --dir ./factory --replay ./docs/examples/sample-run.replay.json",
		"you run --dir ./factory --no-record",
		"you run --factory ./workflow.js",
		"you run --record ./recordings/workflow-run.json --factory ./workflow.js",
		"you run --replay ./recordings/workflow-run.json --factory ./workflow.js",
		"you workflow status <session-id>",
		"you workflow events <session-id>",
		"you workflow artifacts <session-id>",
		"you workflow result <session-id> --mode final",
		"replayCompatibilityVersion",
		"raw JavaScript runtime state",
		"provider transcripts",
		"child-dispatch lists",
		"does not invoke a provider, dispatch a child, or execute the JavaScript source",
		"`--record` with `--replay`",
		"`--no-record` with `--record`",
		"does not delete old artifacts automatically",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(record-replay) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs record-replay`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(record-replay) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_MCPReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("mcp")
	if err != nil {
		t.Fatalf("Markdown(mcp) error = %v", err)
	}

	for _, want := range []string{
		"# MCP Host Setup",
		"you mcp serve",
		`"args": ["mcp", "serve"]`,
		"you.factory_session.validate_source",
		"you.factory_session.start_async",
		"## Choose A Backing Mode",
		"## Run The First-Host Smoke",
		"## Troubleshoot Setup And Calls",
		"fixture catalog not found",
		"factory_session.result.not_ready",
		"you mcp serve --runtime",
		"Fixture-backed (default)",
		"`you docs orchestrators`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(mcp) missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{
		"you docs mcp-hosts",
		"[Orchestrators](orchestrators.md)",
		"Follow-Up Cell For Async Install Smoke",
		"follow-up-cell-mcp-session-serve.md",
	} {
		if strings.Contains(got, absent) {
			t.Fatalf("Markdown(mcp) still contains packaged-topic markdown link %q:\n%s", absent, got)
		}
	}
}

func TestMarkdown_ModelsDocumentsOperatorTasksAndFactoryBoundary(t *testing.T) {
	t.Parallel()

	got, err := Markdown("models")
	if err != nil {
		t.Fatalf("Markdown(models) error = %v", err)
	}

	for _, want := range []string{
		"# Models",
		"you models list",
		"you models inspect OMNIVOICE_Q4_K_M",
		"you models pull OMNIVOICE_Q4_K_M",
		"--operation TTS",
		`--text "Read the release summary."`,
		"--output ./speech.wav",
		"you --json models invoke",
		"INFERENCE_RUN",
		"INFERENCE_WORKER",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(models) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs models`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(models) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_SessionsReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("sessions")
	if err != nil {
		t.Fatalf("Markdown(sessions) error = %v", err)
	}

	for _, want := range []string{
		"# Sessions and Runtime",
		"## Session list",
		"you session list",
		"you session list --json",
		"you session list --port 9090",
		"## Session pause and resume",
		"you session pause",
		"you session resume",
		"POST /factory-sessions/{session_id}/pause",
		"POST /factory-sessions/{session_id}/resume",
		"Paused factory session",
		"Resumed factory session",
		"SESSION_LIFECYCLE_CONTROL",
		"Buffered work while paused",
		"make docs-reference-smoke",
		"## Factory query",
		"you factory query",
		"you --json factory query",
		"you --server http://localhost:9090 factory query",
		"GET /factory-sessions/{session_id}/status",
		"factoryState",
		"runtimeStatus",
		"categories",
		"totalTokens",
		"## Session invocation API",
		"POST /factory-sessions/{session_id}/invocations",
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"INVOCATION_INPUT_EMPTY",
		"INVOCATION_BLOCKED",
		"INVOCATION_NEEDS_HUMAN",
		"INVOCATION_PAUSED",
		"INVOCATION_INTERRUPTED",
		"INVOCATION_RUNTIME_FAILURE",
		"INVOCATION_TIMED_OUT",
		"INVOCATION_CANCELED",
		"INVOCATION_PRIMARY_RESULT_UNRESOLVED",
		"status: TIMED_OUT",
		"status: CANCELED",
		"`primaryResult`",
		`"sourceKind": "text"`,
		`"primaryResult": [`,
		"http://localhost:7437/dashboard/ui",
		"## `--server` and `--session` routing",
		"you submit --session session-beta",
		"you run --continuously",
		"`you docs agents`",
		"`you docs work`",
		"`you docs config`",
		"`fileRef`",
		"`audioStream`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(sessions) missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"source.kind",
		`"parts": [`,
		`primaryResult": {`,
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Markdown(sessions) still contains stale invocation contract %q:\n%s", forbidden, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs sessions`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(sessions) included wrapper text %q:\n%s", wrapper, got)
		}
	}
}

func TestMarkdown_RejectsUnsupportedTopics(t *testing.T) {
	t.Parallel()

	_, err := Markdown("unknown")
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: agents, authoring-factories, run, config, mock-workers, record-replay, guards, relationships, work, sessions, orchestrators, javascript-workflows, mcp, workstations, workers, resources, models, batch-inputs, templates)` {
		t.Fatalf("unsupported topic error = %q", got)
	}
}
