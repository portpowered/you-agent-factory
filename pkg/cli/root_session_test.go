package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestSessionCommand_RegistersSubcommands(t *testing.T) {
	root := NewRootCommand()
	for _, path := range [][]string{
		{"session", "list"},
		{"session", "create"},
		{"session", "delete"},
	} {
		if _, _, err := root.Find(path); err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
	}
}

func TestSessionCommand_HelpDocumentsSubcommandsAndExamples(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"list",
		"create",
		"delete",
		"you session list",
		"you session list --json",
		"you session create --dir /workspace/fleet --port 9090",
		"you session delete session-beta --port 9090 --json",
		"same default --port as work list",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("session help missing %q:\n%s", want, help)
		}
	}
}

func TestRootCommand_HelpDocumentsSessionCommand(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"session",
		"List, open, and close live factory sessions on a running host",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestWorkListCommand_HelpDocumentsSessionListDiscoverability(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "list", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"you session list",
		"discover live session ids",
		"--name",
		"--work-type-name",
		"--trace-id",
		"before pagination",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("work list help missing %q:\n%s", want, help)
		}
	}
}

func TestWorkShowCommand_HelpDocumentsVerifyFlow(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "show", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work show --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"you session list",
		"work show <work-id>",
		"work list",
		"--session",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("work show help missing %q:\n%s", want, help)
		}
	}
}

func TestFactoryQueryCommand_HelpDocumentsSessionListDiscoverability(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"you session list",
		"discover live session ids",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory query help missing %q:\n%s", want, help)
		}
	}
}
