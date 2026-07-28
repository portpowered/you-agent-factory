package symbolidentity_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity"
)

const javascriptRuntimeSymbolsBaselineFixture = "pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/javascript-runtime-symbols.json"

func TestProjectInstalledBindings_MatchesCommittedBaseline(t *testing.T) {
	got, err := symbolidentity.MarshalInventory(symbolidentity.ProjectInstalledBindings())
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, javascriptRuntimeSymbolsBaselineFixture)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}

	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"JavaScript runtime symbol identity baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		javascriptRuntimeSymbolsBaselineFixture,
		len(want),
		len(got),
	)
}

func TestCommittedBaseline_PassesInventoryVerification(t *testing.T) {
	fixturePath := testutil.MustRepoPath(t, javascriptRuntimeSymbolsBaselineFixture)
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}

	var inventory symbolidentity.Inventory
	if err := json.Unmarshal(raw, &inventory); err != nil {
		t.Fatalf("unmarshal baseline fixture: %v", err)
	}
	if err := symbolidentity.VerifyInventory(inventory); err != nil {
		t.Fatalf("VerifyInventory(baseline fixture) error = %v", err)
	}
}

func TestWriteJavaScriptRuntimeSymbolsBaseline(t *testing.T) {
	if os.Getenv("UPDATE_JS_RUNTIME_SYMBOL_BASELINES") != "1" {
		t.Skip("set UPDATE_JS_RUNTIME_SYMBOL_BASELINES=1 to rewrite fixtures")
	}

	got, err := symbolidentity.MarshalInventory(symbolidentity.ProjectInstalledBindings())
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, javascriptRuntimeSymbolsBaselineFixture)
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}
