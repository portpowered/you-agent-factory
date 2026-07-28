package callbehavior_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/callbehavior"
)

const javascriptRuntimeCallBehaviorBaselineFixture = "pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/javascript-runtime-call-behavior.json"

func TestProjectInstalledCallBehavior_MatchesCommittedBaseline(t *testing.T) {
	got, err := callbehavior.MarshalInventory(callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, javascriptRuntimeCallBehaviorBaselineFixture)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}

	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"JavaScript runtime call-behavior baseline drift detected; update %s when intentional\n%s",
		javascriptRuntimeCallBehaviorBaselineFixture,
		diagnoseCallBehaviorBaselineDrift(t, want, got),
	)
}

func TestCommittedBaseline_PassesInventoryVerification(t *testing.T) {
	fixturePath := testutil.MustRepoPath(t, javascriptRuntimeCallBehaviorBaselineFixture)
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}

	var inventory callbehavior.Inventory
	if err := json.Unmarshal(raw, &inventory); err != nil {
		t.Fatalf("unmarshal baseline fixture: %v", err)
	}
	if err := callbehavior.VerifyInventory(inventory); err != nil {
		t.Fatalf("VerifyInventory(baseline fixture) error = %v", err)
	}
}

func TestWriteJavaScriptRuntimeCallBehaviorBaseline(t *testing.T) {
	if os.Getenv("UPDATE_JS_RUNTIME_CALL_BEHAVIOR_BASELINES") != "1" {
		t.Skip("set UPDATE_JS_RUNTIME_CALL_BEHAVIOR_BASELINES=1 to rewrite fixtures")
	}

	got, err := callbehavior.MarshalInventory(callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, javascriptRuntimeCallBehaviorBaselineFixture)
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}

func diagnoseCallBehaviorBaselineDrift(t *testing.T, wantRaw, gotRaw []byte) string {
	t.Helper()

	var want, got callbehavior.Inventory
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		return fmt.Sprintf("want %d bytes, got %d bytes; failed to unmarshal fixture: %v", len(wantRaw), len(gotRaw), err)
	}
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		return fmt.Sprintf("want %d bytes, got %d bytes; failed to unmarshal live descriptor: %v", len(wantRaw), len(gotRaw), err)
	}

	if want.FormatVersion != got.FormatVersion {
		return fmt.Sprintf(
			"formatVersion drift: fixture %q, live %q (want %d bytes, got %d bytes)",
			want.FormatVersion,
			got.FormatVersion,
			len(wantRaw),
			len(gotRaw),
		)
	}

	wantByPath := recordsByPath(want)
	gotByPath := recordsByPath(got)

	var lines []string
	lines = append(lines, fmt.Sprintf("want %d bytes, got %d bytes", len(wantRaw), len(gotRaw)))

	for path := range wantByPath {
		if _, ok := gotByPath[path]; !ok {
			lines = append(lines, fmt.Sprintf(`missing record path %q in live descriptor`, path))
		}
	}
	for path := range gotByPath {
		if _, ok := wantByPath[path]; !ok {
			lines = append(lines, fmt.Sprintf(`unexpected record path %q in live descriptor`, path))
		}
	}

	for path, wantRecord := range wantByPath {
		gotRecord, ok := gotByPath[path]
		if !ok {
			continue
		}
		wantJSON, err := json.Marshal(wantRecord)
		if err != nil {
			t.Fatalf("marshal fixture record %q: %v", path, err)
		}
		gotJSON, err := json.Marshal(gotRecord)
		if err != nil {
			t.Fatalf("marshal live record %q: %v", path, err)
		}
		if !bytes.Equal(wantJSON, gotJSON) {
			lines = append(lines, fmt.Sprintf(`record path %q field drift: fixture %s, live %s`, path, wantJSON, gotJSON))
		}
	}

	if len(lines) == 1 {
		lines = append(lines, "byte-identical JSON comparison failed without per-record differences; inspect full payloads")
	}
	return strings.Join(lines, "\n")
}
