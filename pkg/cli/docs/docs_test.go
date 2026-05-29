package docs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSupportedTopics_ReturnsFixedTopicOrder(t *testing.T) {
	t.Parallel()

	want := []string{
		"authoring-factories",
		"config",
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
}

func TestSupportedTopicCommands_ReturnsCanonicalTopicsAndAliases(t *testing.T) {
	t.Parallel()

	want := []string{
		"authoring-factories",
		"config",
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
		"`config` - Factory configuration",
		"`work` - Work types",
		"`workstations` - Workstation kinds",
		"`workers` - Worker types",
		"`resources` - Resource capacity",
		"`models` - Local and hosted model setup",
		"`batch-inputs` - Batch input files",
		"`templates` - Prompt template variables",
		"`you docs authoring-factories`",
		"`you docs config`",
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

	for _, doc := range topicDocuments {
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
		"you run --dir ./factory --with-mock-workers",
		"you run --dir ./factory --record ./docs/examples/sample-run.replay.json",
		"you run --dir ./factory --replay ./docs/examples/sample-run.replay.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown(authoring-factories) missing %q:\n%s", want, got)
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
		"# Factory JSON And Work Configuration",
		"work types, states, workers, workstations, resources, and routing",
		"## Work Types",
		"## Resources",
		"supportingFiles",
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
		"FACTORY_REQUEST_BATCH",
		"DEPENDS_ON",
		"PARENT_CHILD",
		"factory/inputs/BATCH/default/<request_id>.json",
	} {
		if !strings.Contains(canonical, want) {
			t.Fatalf("Markdown(batch-inputs) missing %q:\n%s", want, canonical)
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

func TestMarkdown_RejectsUnsupportedTopics(t *testing.T) {
	t.Parallel()

	_, err := Markdown("unknown")
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: authoring-factories, config, work, workstations, workers, resources, models, batch-inputs, templates)` {
		t.Fatalf("unsupported topic error = %q", got)
	}
}
