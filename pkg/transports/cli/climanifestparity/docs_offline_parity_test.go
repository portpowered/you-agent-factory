package climanifestparity_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	"github.com/spf13/cobra"
)

func TestProductionManifestCompletionParity_DocsFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	docsRecord, err := manifest.CommandByID("you.docs")
	if err != nil {
		t.Fatalf("CommandByID(you.docs) error = %v", err)
	}

	liveRoot := cli.NewRootCommand()
	inventory, err := cliinputs.Walk(liveRoot)
	if err != nil {
		t.Fatalf("cliinputs.Walk() error = %v", err)
	}

	liveArgs, liveFlags := climanifestparity.InputsForCommandPath(inventory, docsRecord.Path)
	mismatches := climanifestparity.CompareCompletionParity(docsRecord, liveArgs, liveFlags)
	if len(mismatches) == 0 {
		return
	}
	t.Fatalf("contract vs live docs completion drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
}

func TestProductionManifestOfflineDocsParity_DocsFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	docsRecord, err := manifest.CommandByID("you.docs")
	if err != nil {
		t.Fatalf("CommandByID(you.docs) error = %v", err)
	}

	t.Run("index-without-topic", func(t *testing.T) {
		var stdout bytes.Buffer
		root := cli.NewRootCommand()
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
			"Packaged reference topics:",
			"`you docs agents`",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("docs index missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("topic-markdown", func(t *testing.T) {
		wantMarkdown, err := docscli.Markdown("config")
		if err != nil {
			t.Fatalf("Markdown(config): %v", err)
		}
		var stdout bytes.Buffer
		root := cli.NewRootCommand()
		root.SetOut(&stdout)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"docs", "config"})
		if err := root.Execute(); err != nil {
			t.Fatalf("execute docs config: %v", err)
		}
		if got := stdout.String(); got != wantMarkdown {
			t.Fatalf("docs topic stdout changed\nwant:\n%s\ngot:\n%s", wantMarkdown, got)
		}
	})

	t.Run("unsupported-topic", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		root := cli.NewRootCommand()
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
	})

	_ = docsRecord
}

func TestGeneratedVsLegacyOfflineDocsParity_DocsFamily(t *testing.T) {
	registry := mustDocsParityRegistry(t)
	invokeFlags := modelsParityInvokeFlagBindings()

	cases := []generatedVsLegacyDocsCase{
		{
			name: "index-without-topic",
			argv: []string{"docs"},
			wantStdout: func(t *testing.T) string {
				return legacyDocsIndexStdout(t)
			},
		},
		{
			name: "topic-markdown",
			argv: []string{"docs", "run"},
			wantStdout: func(t *testing.T) string {
				t.Helper()
				markdown, err := docscli.Markdown("run")
				if err != nil {
					t.Fatalf("Markdown(run): %v", err)
				}
				return markdown
			},
		},
		{
			name:    "unsupported-topic",
			argv:    []string{"docs", "packaged-goal"},
			wantErr: `unsupported docs topic "packaged-goal"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertGeneratedVsLegacyDocsArgvParity(t, registry, invokeFlags, tc)
		})
	}
}

func mustDocsParityRegistry(t *testing.T) *commandregistry.Registry {
	t.Helper()
	registry, err := commandregistry.NewModelsDocsRegistry(commandregistry.ModelsDocsHandlers{
		DocsRunE: commandregistry.DocsRunE(commandregistry.DocsBinding{
			BinaryName:        "you",
			DiagnosticsWriter: func(cmd *cobra.Command) io.Writer { return io.Discard },
			Verbose:           func() bool { return false },
		}),
		ModelsListRunE:    parityNoopRunE,
		ModelsInspectRunE: parityNoopRunE,
		ModelsInvokeRunE:  parityNoopRunE,
		ModelsPullRunE:    parityNoopRunE,
	})
	if err != nil {
		t.Fatalf("NewModelsDocsRegistry() error = %v", err)
	}
	return registry
}
