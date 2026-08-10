package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewRootCommandFromSubcommandsAttachesInjectedCommands(t *testing.T) {
	root := &cobra.Command{Use: "you"}
	docs := &cobra.Command{Use: "docs"}
	run := &cobra.Command{Use: "run"}

	got := NewRootCommandFromSubcommands(root, RootSubcommands{
		Commands: []*cobra.Command{docs, run},
	})

	if got != root {
		t.Fatal("root constructor replaced the injected root command")
	}
	if found, _, err := got.Find([]string{"docs"}); err != nil || found != docs {
		t.Fatalf("injected docs command = (%v, %v), want supplied command", found, err)
	}
	if found, _, err := got.Find([]string{"run"}); err != nil || found != run {
		t.Fatalf("injected run command = (%v, %v), want supplied command", found, err)
	}
}

func TestMain(m *testing.M) {
	// Cobra's Windows mousetrap check enumerates processes on every Execute.
	// These tests invoke commands in-process and never exercise Explorer launch
	// behavior, so avoid paying that external-system cost for each command tree.
	cobra.MousetrapHelpText = ""

	homeDir, err := os.MkdirTemp("", "you-cli-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create cli test home: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = os.RemoveAll(homeDir)
	}()

	os.Setenv("HOME", homeDir)
	os.Setenv("USERPROFILE", homeDir)
	os.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir))
	os.Setenv("HOMEPATH", string(os.PathSeparator))

	os.Exit(m.Run())
}

func TestProductionRunSubmitFamilyCutoverEnabled(t *testing.T) {
	root := (CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(nil, nil, nil)
	for _, path := range [][]string{{"run"}, {"submit"}, {"submit", "batch"}} {
		cmd, remaining, err := root.Find(path)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", path, err)
		}
		if len(remaining) != 0 {
			t.Fatalf("Find(%v) remaining = %v, want none", path, remaining)
		}
		if cmd.PreRunE == nil || cmd.RunE == nil {
			t.Fatalf("Find(%v) lifecycle = (%t, %t), want retained PreRunE and RunE", path, cmd.PreRunE != nil, cmd.RunE != nil)
		}
	}

	assertDirectCommandCount(t, root, "run", 1)
	assertDirectCommandCount(t, root, "submit", 1)
	submitCmd, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit: %v", err)
	}
	assertDirectCommandCount(t, submitCmd, "batch", 1)
}
