package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
)

func TestSubmitBatchCommand_HelpDocumentsBatchIngressModes(t *testing.T) {
	root := NewRootCommand()
	batchCmd, _, err := root.Find([]string{"submit", "batch"})
	if err != nil {
		t.Fatalf("find submit batch: %v", err)
	}

	for _, name := range []string{"file", "dry-run", "session"} {
		if f := batchCmd.Flags().Lookup(name); f == nil {
			t.Errorf("expected --%s flag on submit batch command", name)
		}
	}
	for _, name := range []string{"name", "work-type-name", "payload", "work-type-id"} {
		if f := batchCmd.Flags().Lookup(name); f != nil {
			t.Fatalf("submit batch should not expose --%s", name)
		}
	}

	var out bytes.Buffer
	root = NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "batch", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit batch --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"FACTORY_REQUEST_BATCH",
		"you docs batch-inputs",
		"--file",
		"filesystem path",
		"stdin",
		"inline",
		"--dry-run",
		"--session",
		"--server",
		"--json",
		"--verbose",
		"pipe",
		"shell argument length",
		"file or pipe",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("submit batch help missing %q:\n%s", want, help)
		}
	}
	for _, disallowed := range []string{"--name", "--work-type-name", "--payload", "--work-type-id"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("submit batch help should not list %s:\n%s", disallowed, help)
		}
	}
}

func TestSubmitCommand_HelpMentionsBatchSubcommand(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"submit batch",
		"FACTORY_REQUEST_BATCH",
		"you docs batch-inputs",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("submit help missing %q:\n%s", want, help)
		}
	}
}

func TestSubmitBatchCommand_InvokesSubmitBatchHandler(t *testing.T) {
	originalSubmitBatch := submitBatch
	defer func() {
		submitBatch = originalSubmitBatch
	}()

	called := false
	submitBatch = func(cfg submitcli.BatchConfig) error {
		called = true
		if cfg.DryRun {
			t.Fatal("dry-run should default false")
		}
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "batch", "batch.json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit batch: %v", err)
	}
	if !called {
		t.Fatal("expected submit batch handler to run")
	}
}
