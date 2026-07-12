package cliinputs_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

const cliCommandInputsBaselineFixture = "contracts/testdata/baseline/cli-command-inputs.json"

func TestWalk_ProductionInventoryMatchesCommittedBaseline(t *testing.T) {
	root := cli.NewRootCommand()

	inventory, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk(production root) error = %v", err)
	}

	got, err := cliinputs.MarshalInventory(inventory)
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, cliCommandInputsBaselineFixture)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}

	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"CLI command inputs baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		cliCommandInputsBaselineFixture,
		len(want),
		len(got),
	)
}

func TestWriteProductionInputsInventoryBaseline(t *testing.T) {
	if os.Getenv("UPDATE_CLI_BASELINES") != "1" {
		t.Skip("set UPDATE_CLI_BASELINES=1 to rewrite fixtures")
	}

	root := cli.NewRootCommand()
	inventory, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk(production root) error = %v", err)
	}
	got, err := cliinputs.MarshalInventory(inventory)
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, cliCommandInputsBaselineFixture)
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}
