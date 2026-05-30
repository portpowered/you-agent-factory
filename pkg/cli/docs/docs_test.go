package docs

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
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
		"authoring-factories",
		"config",
		"mock-workers",
		"record-replay",
		"guards",
		"relationships",
		"work",
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
		"authoring-factories",
		"config",
		"mock-workers",
		"record-replay",
		"guards",
		"relationships",
		"work",
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
		{topic: TopicTemplates, description: "templates", path: "reference/templates.md", displayOrder: 20},
		{topic: TopicConfig, description: "config", path: "reference/config.md", displayOrder: 10, aliases: []Topic{TopicBatchWorkAlias}},
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

func TestIndexMarkdown_ListsSupportedTopicsWithCommands(t *testing.T) {
	t.Parallel()

	got := IndexMarkdown("you")
	for _, want := range []string{
		"# Docs",
		"`authoring-factories` - Practical factory authoring workflow",
		"`config` - factory.json topology, work types, states, workers, workstations, resources, and portability",
		"`mock-workers` - Mock-worker runs",
		"`record-replay` - Record and replay run modes",
		"`work` - Submitted work: POST /work, tags, batch cross-links, and submission contracts",
		"`workstations` - Workstation kinds",
		"`workers` - Worker types",
		"`resources` - Resource capacity",
		"`models` - Local and hosted model setup",
		"`batch-inputs` - Batch input files",
		"`templates` - Prompt template variables",
		"`you docs authoring-factories`",
		"`you docs config`",
		"`you docs mock-workers`",
		"`you docs record-replay`",
		"`you docs work`",
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

			want, err := os.ReadFile(filepath.FromSlash(doc.path))
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", doc.path, err)
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

func TestMarkdown_WorkReturnsRawAuthoredMarkdown(t *testing.T) {
	t.Parallel()

	got, err := Markdown("work")
	if err != nil {
		t.Fatalf("Markdown(work) error = %v", err)
	}

	for _, want := range []string{
		"# Submitted Work",
		"## Single-Work API Submission",
		"POST /work",
		"workTypeName",
		"## Tags And Prompt Templates",
		"Token.Tags",
		"[Config](config.md)",
		"[Batch Inputs](batch-inputs.md)",
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
		"you run --dir <factory> --with-mock-workers",
		"you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json",
		"runType",
		"accept",
		"reject",
		"script",
		"scriptConfig",
		"rejectConfig",
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
		"[Workstations](workstations.md)",
		"[Relationships](relationships.md)",
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
		"[Guards](guards.md)",
		"[Batch Inputs](batch-inputs.md)",
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

func TestMarkdown_RejectsUnsupportedTopics(t *testing.T) {
	t.Parallel()

	_, err := Markdown("unknown")
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: authoring-factories, config, mock-workers, record-replay, guards, relationships, work, workstations, workers, resources, models, batch-inputs, templates)` {
		t.Fatalf("unsupported topic error = %q", got)
	}
}
