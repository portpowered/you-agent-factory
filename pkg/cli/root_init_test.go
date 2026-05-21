package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
)

func TestInitCommand_DefaultDir(t *testing.T) {
	root := NewRootCommand()
	initCmd, _, err := root.Find([]string{"init"})
	if err != nil {
		t.Fatalf("find init: %v", err)
	}

	dirFlag := initCmd.Flags().Lookup("dir")
	if dirFlag == nil {
		t.Fatal("expected --dir flag on init command")
	}
	if dirFlag.DefValue != "factory" {
		t.Errorf("default dir = %q, want %q", dirFlag.DefValue, "factory")
	}

	executorFlag := initCmd.Flags().Lookup("executor")
	if executorFlag == nil {
		t.Fatal("expected --executor flag on init command")
	}
	if executorFlag.DefValue != initcmd.DefaultStarterExecutor {
		t.Errorf("default executor = %q, want %q", executorFlag.DefValue, initcmd.DefaultStarterExecutor)
	}
}

func TestInitCommand_ExecutorFlagMapsToInitConfig(t *testing.T) {
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
	root.SetArgs([]string{"init", "--dir", "custom-factory", "--executor", "claude"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init --executor claude: %v", err)
	}

	if got.Dir != "custom-factory" {
		t.Fatalf("dir = %q, want %q", got.Dir, "custom-factory")
	}
	if got.Executor != "claude" {
		t.Fatalf("executor = %q, want %q", got.Executor, "claude")
	}
}

func TestInitCommand_HelpDocumentsExecutorOptions(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{"--executor", "codex", "claude"} {
		if !strings.Contains(help, want) {
			t.Fatalf("init help missing %q:\n%s", want, help)
		}
	}
	if !strings.Contains(help, "Omitting --executor preserves the default Codex-backed starter scaffold") {
		t.Fatalf("init help should describe default executor behavior:\n%s", help)
	}
	if !strings.Contains(help, "Supported starter scaffold values are codex and claude") {
		t.Fatalf("init help should describe supported executor values:\n%s", help)
	}
}
