package cli

import (
	"bytes"
	"io"
	"testing"

	factorycli "github.com/portpowered/infinite-you/pkg/cli/factory"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	modelscli "github.com/portpowered/infinite-you/pkg/cli/models"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
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

func TestRootCommand_HelpDocumentsGlobalServerFlag(t *testing.T) {
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
		"--server",
		"factory API base URI",
		"Global --server",
		"--server http://localhost:9090 --json factory query",
	} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	if bytes.Contains([]byte(help), []byte("--port")) {
		t.Fatalf("root help must not advertise --port:\n%s", help)
	}
}

func TestFactoryQueryCommand_HelpUsesGlobalFlags(t *testing.T) {
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
		"global --json",
		"global --server",
		"you --server http://localhost:9090 --json factory query",
		"you --json factory query",
	} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("factory query help missing %q:\n%s", want, help)
		}
	}
	if bytes.Contains([]byte(help), []byte("--port")) {
		t.Fatalf("factory query help must not advertise --port:\n%s", help)
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

func TestSubmitCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json",
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to SubmitConfig.JSON")
	}
}

func TestFactoryDeleteCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalDeleteFactory := deleteFactory
	defer func() {
		deleteFactory = originalDeleteFactory
	}()

	var got factorycli.DeleteConfig
	deleteFactory = func(cfg factorycli.DeleteConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "delete", "staging"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory delete with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to DeleteConfig.JSON")
	}
}

func TestInitCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalInitFactory := initFactory
	defer func() {
		initFactory = originalInitFactory
	}()

	var got initcmd.InitConfig
	initFactory = func(cfg initcmd.InitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to InitConfig.JSON")
	}
}
