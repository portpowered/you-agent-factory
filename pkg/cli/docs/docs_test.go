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
		"`config` - Factory configuration",
		"`workstation` - Workstation state",
		"`workers` - Worker types",
		"`resources` - Resource capacity",
		"`models` - Local and hosted model setup",
		"`batch-work` - Batch work input files",
		"`templates` - Prompt template variables",
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

func TestMarkdown_RejectsUnsupportedTopics(t *testing.T) {
	t.Parallel()

	_, err := Markdown("unknown")
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: config, workstation, workers, resources, models, batch-work, templates)` {
		t.Fatalf("unsupported topic error = %q", got)
	}
}
