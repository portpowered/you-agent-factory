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
		"`you docs agents` for orientation",
		"`you submit batch`",
		"`you session list`",
		"`--verbose` or `--debug`",
		"Packaged reference topics:",
		"`agents` - Agent orientation: read order, work submission, command matrix, planner vs executor, and topic router",
		"`authoring-factories` - Practical factory authoring workflow",
		"`config` - factory.json topology, operator model defaults, work types, states, workers, workstations, resources, and portability",
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
	if got := err.Error(); got != `unsupported docs topic "unknown" (supported: agents, authoring-factories, config, mock-workers, record-replay, guards, relationships, work, sessions, mcp-hosts, orchestrators, mcp, workstations, workers, resources, models, packaged-tts, batch-inputs, templates)` {
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
	root := NewRootCommand()
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
		"Running you with no args starts the out-of-the-box continuous factory",
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
	root := NewRootCommand()
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
