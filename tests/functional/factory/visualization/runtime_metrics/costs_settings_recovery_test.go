package runtime_metrics_test

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// M08-S keeps metrics outside the Settings directory and submits no Work, so
// only the real Settings read is faulted. The shared filesystem edge delegates
// this path unchanged; the controlled M07 fault remains a separate witness.
func (group *costsProcessGroup) checkPhysicalSettingsRecovery(t *testing.T) {
	home := t.TempDir()
	fixture := group.start(t, home, endToEndPricedModel, t.TempDir())
	foreign := group.start(t, t.TempDir(), endToEndPricedModel, t.TempDir())
	empty := fixture.parity(t)
	peer := foreign.parity(t)
	if string(empty.Status) != "NO_USAGE" || empty.KnownCost != nil || empty.Coverage.EncounteredRows != 0 || len(empty.LineItems) != 0 {
		t.Fatalf("original empty report=%#v", empty)
	}
	settingsDir := filepath.Join(home, ".you-agent-factory")
	// Resolve the real directory before mutation and reject any path escaping
	// the scenario-owned profile, including a symlink introduced during setup.
	resolved, err := filepath.EvalSymlinks(settingsDir)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(home, resolved)
	if err != nil || relative != ".you-agent-factory" {
		t.Fatalf("Settings directory is outside owned profile: path=%q relative=%q error=%v", resolved, relative, err)
	}
	settingsDir = resolved
	backup := settingsDir + ".saved"
	if err := os.Rename(settingsDir, backup); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, os.ErrPermission) {
			t.Skipf("M08-S unavailable: Windows denies renaming the active no-work Settings directory; native Linux proof is required: %v", err)
		}
		t.Fatal(err)
	}
	restored := false
	t.Cleanup(func() {
		if !restored {
			if err := os.Remove(settingsDir); err != nil && !os.IsNotExist(err) {
				t.Errorf("remove owned Settings fault file: %v", err)
			}
			if err := os.Rename(backup, settingsDir); err != nil {
				t.Errorf("restore owned Settings directory: %v", err)
			}
		}
	})
	if err := os.WriteFile(settingsDir, []byte("unavailable Settings directory secret-token private-path"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture.assertFailure(t, fixture.id, http.StatusInternalServerError, "COSTS_QUERY_FAILED")
	if current := foreign.parity(t); !reflect.DeepEqual(peer, current) {
		t.Fatalf("owned Settings fault changed foreign report: %#v", current)
	}
	if err := os.Remove(settingsDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, settingsDir); err != nil {
		t.Fatal(err)
	}
	restored = true
	if recovered := fixture.parity(t); !reflect.DeepEqual(empty, recovered) {
		t.Fatalf("Settings recovery changed original NO_USAGE report: %#v", recovered)
	}
}
