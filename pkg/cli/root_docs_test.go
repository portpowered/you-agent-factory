package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	docscli "github.com/portpowered/infinite-you/pkg/cli/docs"
)

func TestDocsCommand_NoTopicPrintsDocsIndex(t *testing.T) {
	var stdout bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"docs"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute docs: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"# Docs",
		"`authoring-factories` - Practical factory authoring workflow",
		"`config` - Factory.json topology: work types, states, routing, resources, and portability.",
		"`mock-workers` - Mock-worker runs",
		"`record-replay` - Record and replay run modes",
		"`guards` - Workstation, input, and factory guards",
		"`relationships` - Batch DEPENDS_ON",
		"`work` - Submitted work contracts: POST /work, tags, tokens, and batch cross-links.",
		"`workstations` - Workstation kinds",
		"`workers` - Worker types",
		"`batch-inputs` - Batch input files",
		"`you docs authoring-factories`",
		"`you docs config`",
		"`you docs mock-workers`",
		"`you docs record-replay`",
		"`you docs guards`",
		"`you docs relationships`",
		"`you docs work`",
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
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("docs index should not fall back to Cobra help marker %q:\n%s", unwanted, got)
		}
	}
}

func TestDocsCommand_HelpStillUsesCobraHelp(t *testing.T) {
	var stdout bytes.Buffer
	root := NewRootCommand()
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
		"Run without a topic to print the packaged docs index.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("docs help missing %q:\n%s", want, got)
		}
	}
}

func TestDocsCommand_BatchAndRelationshipTopicsUseOpenAPICamelCaseFieldNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		topic   string
		want    []string
		absent  []string
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
			root := NewRootCommand()
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

func TestDocsCommand_UnsupportedTopicReturnsCanonicalTopicError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"docs", "unknown"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: authoring-factories, config, mock-workers, record-replay, guards, relationships, work, workstations, workers, resources, models, batch-inputs, templates)` {
		t.Fatalf("unexpected docs error %q", got)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("unsupported topic should not write stdout, got %q", got)
	}
	if got := stderr.String(); strings.Contains(got, "unknown command") || strings.Contains(got, "Available Commands:") {
		t.Fatalf("unsupported topic should not fall back to cobra unknown-command output:\n%s", got)
	}
}

func TestDocsCommand_VerboseLogsTopicResolutionWithoutChangingMarkdown(t *testing.T) {
	wantMarkdown, err := docscli.Markdown("config")
	if err != nil {
		t.Fatalf("Markdown(config): %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand()
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
