package smoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestIntegrationSmoke_GuardedLoopBreakerExampleRejectsRetiredExhaustionRulesAtBoundary(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.AgentFactoryPath(t, "examples/simple-tasks"))
	support.ClearSeedInputs(t, dir)
	writeFactoryTopLevelExhaustionRules(t, dir, []map[string]any{{
		"name":              "review-loop-breaker",
		"watch_workstation": "review-story",
		"max_visits":        3,
		"source": map[string]string{
			"work_type": "story",
			"state":     "init",
		},
		"target": map[string]string{
			"work_type": "story",
			"state":     "failed",
		},
	}})

	_, err := config.LoadRuntimeConfig(dir, nil)
	assertRetiredExhaustionRulesBoundaryError(t, err)
}

func writeFactoryTopLevelExhaustionRules(t *testing.T, dir string, rules []map[string]any) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse factory.json: %v", err)
	}

	cfg["exhaustion_rules"] = rules

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func assertRetiredExhaustionRulesBoundaryError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected retired exhaustion_rules boundary error")
	}
	if !strings.Contains(err.Error(), "exhaustion_rules is retired") {
		t.Fatalf("boundary error = %q, want retired exhaustion_rules guidance", err)
	}
	if !strings.Contains(err.Error(), "guarded LOGICAL_MOVE workstation") {
		t.Fatalf("boundary error = %q, want guarded LOGICAL_MOVE guidance", err)
	}
	if !strings.Contains(err.Error(), "visit_count guard") {
		t.Fatalf("boundary error = %q, want visit_count guidance", err)
	}
}
