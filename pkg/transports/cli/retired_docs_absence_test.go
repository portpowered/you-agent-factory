package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
)

var settledRetiredDocsTopics = retiredsurfaceguard.SettledRetiredDocsTopics()

var canonicalDocsTopicSamples = []string{
	"agents",
	"authoring-factories",
	"run",
	"config",
	"mcp",
	"javascript-workflows",
}

var canonicalDocsTopicAliases = []string{
	"batch-work",
	"workstation",
}

func TestRetiredDocsTopics_RejectUnsupportedAtRuntime(t *testing.T) {
	for _, topic := range settledRetiredDocsTopics {
		topic := topic
		t.Run(topic, func(t *testing.T) {
			var stdout strings.Builder
			root := NewRootCommand()
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"docs", topic})

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), `unsupported docs topic "`+topic+`"`) {
				t.Fatalf("execute docs %s error = %v, want unsupported-topic error", topic, err)
			}
			if got := stdout.String(); got != "" {
				t.Fatalf("retired docs topic %s wrote stdout %q", topic, got)
			}
		})
	}
}

func TestRetiredDocsTopics_AbsentFromRegistry(t *testing.T) {
	supported := make(map[string]struct{}, len(docscli.SupportedTopics()))
	for _, topic := range docscli.SupportedTopics() {
		supported[topic] = struct{}{}
	}
	commands := make(map[string]struct{}, len(docscli.SupportedTopicCommands()))
	for _, command := range docscli.SupportedTopicCommands() {
		commands[command] = struct{}{}
	}

	for _, topic := range settledRetiredDocsTopics {
		if _, stillSupported := supported[topic]; stillSupported {
			t.Fatalf("retired topic %q is still listed in SupportedTopics()", topic)
		}
		if _, stillAccepted := commands[topic]; stillAccepted {
			t.Fatalf("retired topic %q is still accepted by SupportedTopicCommands()", topic)
		}
	}
}

func TestRetiredDocsTopics_NoCompatibilityAliases(t *testing.T) {
	retired := make(map[string]struct{}, len(settledRetiredDocsTopics))
	for _, topic := range settledRetiredDocsTopics {
		retired[topic] = struct{}{}
	}

	for _, entry := range docscli.TopicIndexEntries() {
		if _, isRetired := retired[entry.Name]; isRetired {
			t.Fatalf("retired topic %q is registered as a canonical docs topic", entry.Name)
		}
		for _, alias := range entry.Aliases {
			if _, isRetired := retired[alias]; isRetired {
				t.Fatalf("compatibility alias %q on topic %q reintroduces retired docs topic", alias, entry.Name)
			}
		}
	}

	for _, topic := range settledRetiredDocsTopics {
		got, err := docscli.Markdown(topic)
		if err == nil || got != "" {
			t.Fatalf("Markdown(%q) = %q, %v; want unsupported-topic error", topic, got, err)
		}
		if !strings.Contains(err.Error(), `unsupported docs topic "`+topic+`"`) {
			t.Fatalf("Markdown(%q) error = %v, want unsupported-topic error", topic, err)
		}
	}
}

func TestRetiredDocsTopics_AbsentFromDocsCommandValidArgs(t *testing.T) {
	root := NewRootCommand()
	docsCmd, _, err := root.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("find docs command: %v", err)
	}

	validArgs := make(map[string]struct{}, len(docsCmd.ValidArgs))
	for _, arg := range docsCmd.ValidArgs {
		validArgs[arg] = struct{}{}
	}

	for _, topic := range settledRetiredDocsTopics {
		if _, stillAccepted := validArgs[topic]; stillAccepted {
			t.Fatalf("retired topic %q is still listed in docs command ValidArgs", topic)
		}
	}
}

func TestRetiredDocsTopics_AbsentFromDocsIndex(t *testing.T) {
	index := docscli.IndexMarkdown("you")
	for _, topic := range settledRetiredDocsTopics {
		if strings.Contains(index, "`"+topic+"`") {
			t.Fatalf("docs index still lists retired topic %q:\n%s", topic, index)
		}
	}
}

func TestRetiredDocsTopics_CanonicalTopicsRemainResolvable(t *testing.T) {
	for _, topic := range docscli.SupportedTopics() {
		topic := topic
		t.Run("markdown/"+topic, func(t *testing.T) {
			got, err := docscli.Markdown(topic)
			if err != nil {
				t.Fatalf("Markdown(%q): %v", topic, err)
			}
			if strings.TrimSpace(got) == "" {
				t.Fatalf("Markdown(%q) returned empty body", topic)
			}
		})
	}

	for _, topic := range canonicalDocsTopicSamples {
		topic := topic
		t.Run("cli/"+topic, func(t *testing.T) {
			var stdout strings.Builder
			root := NewRootCommand()
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"docs", topic})

			if err := root.Execute(); err != nil {
				t.Fatalf("execute docs %s: %v", topic, err)
			}
			if strings.TrimSpace(stdout.String()) == "" {
				t.Fatalf("execute docs %s returned empty body", topic)
			}
		})
	}

	for _, alias := range canonicalDocsTopicAliases {
		alias := alias
		t.Run("alias/"+alias, func(t *testing.T) {
			got, err := docscli.Markdown(alias)
			if err != nil {
				t.Fatalf("Markdown(%q): %v", alias, err)
			}
			if strings.TrimSpace(got) == "" {
				t.Fatalf("Markdown(%q) returned empty body", alias)
			}
		})
	}
}
