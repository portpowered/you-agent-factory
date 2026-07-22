package commandidentity_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
)

const cliCommandsBaselineFixture = "contracts/testdata/baseline/cli-commands.json"

func TestWalk_ProductionInventoryMatchesCommittedBaseline(t *testing.T) {
	inventory := productionCLIObservation(t).Snapshot.Commands

	got, err := commandidentity.MarshalInventory(inventory)
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, cliCommandsBaselineFixture)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}

	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"CLI command identity baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		cliCommandsBaselineFixture,
		len(want),
		len(got),
	)
}

func TestWriteProductionInventoryBaseline(t *testing.T) {
	if os.Getenv("UPDATE_CLI_BASELINES") != "1" {
		t.Skip("set UPDATE_CLI_BASELINES=1 to rewrite fixtures")
	}

	inventory := productionCLIObservation(t).Snapshot.Commands
	got, err := commandidentity.MarshalInventory(inventory)
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, cliCommandsBaselineFixture)
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}
