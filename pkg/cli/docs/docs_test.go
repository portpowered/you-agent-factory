package docs

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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
		"config",
		"mock-workers",
		"record-replay",
		"guards",
		"relationships",
		"work",
		"sessions",
		"workstations",
		"workers",
		"resources",
		"models",
		"packaged-tts",
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
		"config",
		"mock-workers",
		"record-replay",
		"guards",
		"relationships",
		"work",
		"sessions",
		"workstations",
		"workstation",
		"workers",
		"resources",
		"models",
		"packaged-tts",
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
		"`config` - factory.json topology",
		"`mock-workers` - Mock-worker runs",
		"`record-replay` - Record and replay run modes",
		"`work` - Submitted work",
		"`sessions` - Live factory sessions",
		"`workstations` - Workstation kinds",
		"`workers` - Worker types",
		"`resources` - Resource capacity",
		"`models` - Local and hosted model setup",
		"`packaged-tts` - Packaged @you/tts invocation",
		"`batch-inputs` - Batch input files",
		"`templates` - Prompt template variables",
		"`you docs agents`",
		"`you docs authoring-factories`",
		"`you docs config`",
		"`you docs mock-workers`",
		"`you docs record-replay`",
		"`you docs work`",
		"`you docs sessions`",
		"`you docs workstations`",
		"`you docs packaged-tts`",
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

func TestMarkdown_ConfigDocumentsInvocationContract(t *testing.T) {
	t.Parallel()

	got, err := Markdown("config")
	if err != nil {
		t.Fatalf("Markdown(config) error = %v", err)
	}

	for _, want := range []string{
		"## Invocation Contract",
		"`you run --factory <factory.json> <text>`",
		"`POST /factory-sessions/{session_id}/invocations`",
		"`INVOCATION_INPUT_SOURCE_CONFLICT`",
		"`INVOCATION_INPUT_EMPTY`",
		"`invocationReturn`",
		"`SUBMITTED_WORK_TERMINAL`",
		"`EXPLICIT`",
		"`INVOCATION_PRIMARY_RESULT_UNRESOLVED`",
		"`you docs sessions`",
		"`fileRef` and `audioStream` are reserved future source categories",
		"Top-level `sourceKind: \"text\"` plus canonical `content` (`WorkContent`)",
		`"status":"COMPLETED","primaryResult":[{"type":"text"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(config) missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"RUN_INVOCATION_AMBIGUOUS_INPUT",
		`"output":"Summary text","workId":"work-123"`,
		"source.kind: text",
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

func TestMarkdown_PackagedTTSReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("packaged-tts")
	if err != nil {
		t.Fatalf("Markdown(packaged-tts) error = %v", err)
	}

	for _, want := range []string{
		"# Packaged TTS (`@you/tts`)",
		"you run --named @you/tts",
		"~/.you-agent-factory/factories",
		"@you%2Ftts",
		"artifactPath",
		"mediaType",
		"backend",
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"editable",
		"raw audio",
		"shared invocation contract",
		"INVOCATION_TTS_MODEL_NOT_READY",
		"INVOCATION_TTS_GENERATION_FAILED",
		"`you docs authoring-factories`",
		"`you docs config`",
		"`you docs sessions`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(packaged-tts) missing %q:\n%s", want, got)
		}
	}
	for _, wrapper := range []string{
		"# Docs",
		"Run `you docs packaged-tts`.",
	} {
		if strings.Contains(got, wrapper) {
			t.Fatalf("Markdown(packaged-tts) included wrapper text %q:\n%s", wrapper, got)
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
		"INVOCATION_PRIMARY_RESULT_UNRESOLVED",
		"status: TIMED_OUT",
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

func TestMarkdown_PackagedReferenceTopicsHaveNoPackagedTopicMarkdownLinks(t *testing.T) {
	t.Parallel()

	packagedTopicMD := regexp.MustCompile(`\[[^\]]+\]\((?:\./|\.\./reference/)?([a-z0-9-]+)\.md(?:#[^)]*)?\)`)
	exempt := map[string]bool{"README": true}

	repoRoot := testutil.MustRepoRoot(t)
	referenceDir := filepath.Join(repoRoot, "docs", "reference")
	entries, err := os.ReadDir(referenceDir)
	if err != nil {
		t.Fatalf("ReadDir(docs/reference) error = %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "README.md" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(referenceDir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", entry.Name(), err)
		}
		doc := string(content)
		for _, match := range packagedTopicMD.FindAllString(doc, -1) {
			stem := packagedTopicMD.FindStringSubmatch(match)[1]
			if exempt[stem] {
				continue
			}
			t.Fatalf("%s contains packaged-topic markdown link %q; use `you docs %s` instead", entry.Name(), match, stem)
		}
		if strings.Contains(doc, "you docs authoring-agents-md") {
			t.Fatalf("%s references authoring-agents-md as a docs topic; use docs/reference/authoring-agents-md.md path instead", entry.Name())
		}
	}
}

func TestMarkdown_RejectsUnsupportedTopics(t *testing.T) {
	t.Parallel()

	_, err := Markdown("unknown")
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: agents, authoring-factories, config, mock-workers, record-replay, guards, relationships, work, sessions, workstations, workers, resources, models, packaged-tts, batch-inputs, templates)` {
		t.Fatalf("unsupported topic error = %q", got)
	}
}
