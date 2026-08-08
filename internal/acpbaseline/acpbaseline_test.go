package acpbaseline_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/acpbaseline"
)

// TestEmbeddedScenariosAreValid keeps a malformed script from being discovered
// only when a capture is already halfway through a paid provider run.
func TestEmbeddedScenariosAreValid(t *testing.T) {
	t.Parallel()

	scenarios, err := acpbaseline.LoadScenarios()
	if err != nil {
		t.Fatalf("LoadScenarios() error = %v", err)
	}
	if len(scenarios) < 5 {
		t.Fatalf("scenarios = %d, want the five documented experiments", len(scenarios))
	}
	seen := map[string]bool{}
	for _, scenario := range scenarios {
		if seen[scenario.Name] {
			t.Fatalf("duplicate scenario name %q", scenario.Name)
		}
		seen[scenario.Name] = true
		if err := scenario.Validate(); err != nil {
			t.Fatalf("scenario %q: %v", scenario.Name, err)
		}
		if strings.TrimSpace(scenario.Description) == "" {
			t.Fatalf("scenario %q has no description", scenario.Name)
		}
	}
}

// TestDigestPreservesStructureAndDropsContent is the property that makes a
// digest safe to commit: every key and shape survives, and no string content
// does.
func TestDigestPreservesStructureAndDropsContent(t *testing.T) {
	t.Parallel()

	raw := `{"v":1,"seq":7,"t":"2026-01-01T00:00:00Z","conn":"c","peer":"agent",` +
		`"dir":"in","stream":"stdout","bytes":10,` +
		`"frame":{"jsonrpc":"2.0","method":"session/update","params":{` +
		`"sessionId":"secret-session","update":{"sessionUpdate":"agent_message_chunk",` +
		`"content":{"type":"text","text":"the customer's private prompt text"}}}}}`

	digested, err := acpbaseline.DigestRecordLine([]byte(raw), 1)
	if err != nil {
		t.Fatalf("DigestRecordLine() error = %v", err)
	}
	text := string(digested)

	for _, leaked := range []string{"private prompt", "secret-session"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("digest leaked %q: %s", leaked, text)
		}
	}
	// Structure is the whole point: keys and discriminators must survive, or a
	// comparison has nothing to read.
	for _, kept := range []string{"sessionUpdate", "agent_message_chunk", "content", "params"} {
		if !strings.Contains(text, kept) {
			t.Fatalf("digest dropped structural key %q: %s", kept, text)
		}
	}
	if strings.Contains(text, `"t":`) {
		t.Fatalf("digest retained a wall-clock timestamp, so re-captures cannot diff cleanly: %s", text)
	}
	if _, found := acpbaseline.ContainsRawContent(digested); found {
		t.Fatalf("digest still reports raw content: %s", text)
	}
}

// TestContainsRawContentCatchesAnUndigestedTranscript proves the commit guard
// actually rejects the mistake it exists to prevent.
func TestContainsRawContentCatchesAnUndigestedTranscript(t *testing.T) {
	t.Parallel()

	raw := `{"v":1,"seq":1,"conn":"c","peer":"agent","dir":"in","stream":"stdout",` +
		`"frame":{"method":"session/update","params":{"text":"a real prompt"}}}`
	where, found := acpbaseline.ContainsRawContent([]byte(raw))
	if !found {
		t.Fatal("an undigested transcript was not detected")
	}
	if where == "" {
		t.Fatal("detection reported no location")
	}
}

func TestScrubRedactsSecretsAndIsIdempotent(t *testing.T) {
	t.Parallel()

	line := `{"authorization":"Bearer abcdefghijkl","key":"sk-abcdefghijklmnopqrst"}`
	once := acpbaseline.Scrub(line, nil)
	if strings.Contains(once, "abcdefghijklmnopqrst") || strings.Contains(once, "Bearer abcdefghijkl") {
		t.Fatalf("scrub left a secret: %s", once)
	}
	if twice := acpbaseline.Scrub(once, nil); twice != once {
		t.Fatalf("scrub is not idempotent:\n once %s\n twice %s", once, twice)
	}
}

// TestCompareComputesVerdicts pins the verdict rules, since every GAP row
// becomes a work item and an incorrect verdict manufactures or hides one.
func TestCompareComputesVerdicts(t *testing.T) {
	t.Parallel()

	ours := &acpbaseline.CapabilityMatrix{
		Agent:                 acpbaseline.OurAgentName,
		SessionUpdateVariants: map[string]int{"tool_call": 1, "usage_update": 2},
	}
	theirs := &acpbaseline.CapabilityMatrix{
		Agent:                 "cursor-agent",
		SessionUpdateVariants: map[string]int{"tool_call": 3, "plan": 1},
	}

	verdicts := map[string]acpbaseline.Verdict{}
	for _, row := range acpbaseline.Compare([]*acpbaseline.CapabilityMatrix{ours, theirs}) {
		verdicts[row.Capability] = row.Verdict
	}

	cases := map[string]acpbaseline.Verdict{
		"session/update -> tool_call":    acpbaseline.VerdictParity,
		"session/update -> plan":         acpbaseline.VerdictGap,
		"session/update -> usage_update": acpbaseline.VerdictExtra,
	}
	for capability, want := range cases {
		if got := verdicts[capability]; got != want {
			t.Fatalf("%s verdict = %q, want %q", capability, got, want)
		}
	}
}

// TestVerifyCommittedRejectsARawTranscript proves the guard fails a tree that
// should never have been committed, which is what makes the policy real.
func TestVerifyCommittedRejectsARawTranscript(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	raw := `{"v":1,"seq":1,"conn":"c","peer":"agent","dir":"in","stream":"stdout",` +
		`"frame":{"method":"session/update","params":{"text":"a real prompt"}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "x.digest.jsonl"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	findings, err := acpbaseline.VerifyCommitted(dir)
	if err != nil {
		t.Fatalf("VerifyCommitted() error = %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("VerifyCommitted accepted an undigested transcript")
	}
}

// TestCommittedBaselinesStayPublishable is the standing guard over the tree
// this repository actually commits.
func TestCommittedBaselinesStayPublishable(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "docs", "internal", "projects", "acp-program", "baselines")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skip("no committed baselines yet")
	}
	findings, err := acpbaseline.VerifyCommitted(root)
	if err != nil {
		t.Fatalf("VerifyCommitted() error = %v", err)
	}
	if len(findings) > 0 {
		t.Fatalf("committed baselines violate the publish policy:\n  %s",
			strings.Join(findings, "\n  "))
	}
}

// TestOurCommittedMatrixReflectsShippedBehavior ties the captured baseline back
// to the behavior this program changed, so a stale capture cannot silently
// misreport what we do.
func TestOurCommittedMatrixReflectsShippedBehavior(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "docs", "internal", "projects", "acp-program",
		"baselines", "you", "2026-08-06", "capability-matrix.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("no committed self-capture yet")
	}
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	var matrix acpbaseline.CapabilityMatrix
	if err := json.Unmarshal(data, &matrix); err != nil {
		t.Fatalf("decode matrix: %v", err)
	}

	if matrix.ConfigOptionCategory != "model" {
		t.Fatalf("config option category = %q, want model", matrix.ConfigOptionCategory)
	}
	if matrix.ConfigOptionCount < 15 {
		t.Fatalf("config option count = %d, want every installed packaged Factory",
			matrix.ConfigOptionCount)
	}
	if matrix.SessionUpdateVariants["available_commands_update"] == 0 {
		t.Fatal("self-capture recorded no available_commands_update")
	}
}
