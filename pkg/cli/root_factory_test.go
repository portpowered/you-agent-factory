package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestFactoryCommand_RegistersSubcommands(t *testing.T) {
	root := NewRootCommand()
	for _, path := range [][]string{
		{"factory", "query"},
		{"factory", "list"},
		{"factory", "save"},
		{"factory", "update"},
		{"factory", "delete"},
	} {
		if _, _, err := root.Find(path); err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
	}
}

func TestFactoryCommand_HelpDocumentsSubcommandsAndExamples(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"query",
		"list",
		"save",
		"update",
		"delete",
		"global --server",
		"you factory query",
		"you factory list",
		"you factory save staging --from ./factory.json",
		"you factory update staging --from ./factory.json",
		"you factory delete staging",
		"you factory save",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory help missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "--port") {
		t.Fatalf("factory help must not advertise --port:\n%s", help)
	}
}
