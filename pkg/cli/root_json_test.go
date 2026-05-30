package cli

import (
	"bytes"
	"io"
	"testing"

	factorycli "github.com/portpowered/infinite-you/pkg/cli/factory"
	modelscli "github.com/portpowered/infinite-you/pkg/cli/models"
	workcli "github.com/portpowered/infinite-you/pkg/cli/work"
)

func TestRootCommand_HelpDocumentsGlobalJSONFlag(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"--json", "structured JSON on stdout"} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestSupportedCommands_DoNotRegisterLocalJSONFlag(t *testing.T) {
	root := NewRootCommand()
	for _, path := range [][]string{
		{"factory", "query"},
		{"work", "list"},
		{"models", "list"},
		{"models", "inspect"},
		{"models", "invoke"},
		{"models", "pull"},
	} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
		if cmd.LocalFlags().Lookup("json") != nil {
			t.Fatalf("%s registers a local --json flag; use root persistent --json", cmd.CommandPath())
		}
	}
}

func TestFactoryQueryCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalQueryFactory := queryFactory
	defer func() {
		queryFactory = originalQueryFactory
	}()

	var got factorycli.QueryConfig
	queryFactory = func(cfg factorycli.QueryConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "query"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to QueryConfig.JSON")
	}
}

func TestFactorySaveLiveCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalSaveFactoryCurrent := saveFactoryCurrent
	defer func() {
		saveFactoryCurrent = originalSaveFactoryCurrent
	}()

	var got factorycli.SaveCurrentConfig
	saveFactoryCurrent = func(cfg factorycli.SaveCurrentConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "save"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory save with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to SaveCurrentConfig.JSON")
	}
}

func TestModelsInspectCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalInspectModel := inspectModel
	defer func() {
		inspectModel = originalInspectModel
	}()

	var got modelscli.InspectConfig
	inspectModel = func(cfg modelscli.InspectConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "models", "inspect", "OMNIVOICE_Q4_K_M"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models inspect with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to InspectConfig.JSON")
	}
}

func TestWorkListCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "work", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to ListConfig.JSON")
	}
}
