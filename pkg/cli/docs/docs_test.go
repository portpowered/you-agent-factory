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
		"workstation",
		"workers",
		"resources",
		"models",
		"batch-work",
		"templates",
	}

	if got := SupportedTopics(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedTopics() = %#v, want %#v", got, want)
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
		"`workstation` - Workstation state",
		"`workers` - Worker types",
		"`resources` - Resource capacity",
		"`models` - Local and hosted model setup",
		"`batch-work` - Batch work input files",
		"`templates` - Prompt template variables",
		"`you docs authoring-factories`",
		"`you docs config`",
		"`you docs workstation`",
		"`you docs batch-work`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("IndexMarkdown() missing %q:\n%s", want, got)
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

func TestMarkdown_RejectsUnsupportedTopics(t *testing.T) {
	t.Parallel()

	_, err := Markdown("unknown")
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: authoring-factories, config, workstation, workers, resources, models, batch-work, templates)` {
		t.Fatalf("unsupported topic error = %q", got)
	}
}
