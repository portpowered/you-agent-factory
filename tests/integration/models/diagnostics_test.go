package models_test

import (
	"context"
	"testing"
)

// TestStory001CharacterizesFiveFailingInvokes retains every built-process
// exit and stream for the same controlled failure. It does not assert the
// eventual deterministic diagnostic contract; that is owned by story 004.
func TestStory001CharacterizesFiveFailingInvokes(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{failManifest: true})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001Environment(homeDir, cacheDir, origin.URL())
	args := []string{"--debug", "models", "invoke", "embed", "--input", "text=" + story001ModelInput}
	results := make([]builtProcessResult, 0, 5)
	for index := 0; index < 5; index++ {
		result := runStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
		if result.timedOut || !result.processExited || result.exitCode == 0 {
			t.Fatalf("failing invoke %d did not produce a terminal failure: %s", index+1, summarizeProcess(result))
		}
		results = append(results, result)
	}

	for index, result := range results {
		t.Logf("STORY-001-EVIDENCE acceptance=five-failures probe=invoke-%d %s", index+1, summarizeProcess(result))
	}
	t.Logf("STORY-001-EVIDENCE acceptance=five-failures originRequests=%d assetLedger=%s", len(origin.exchangesSnapshot()), compactJSON(origin.assetExchanges()))
}
