package models_test

import (
	"context"
	"testing"
)

// TestStory001CharacterizesOverlappingInvokes records the two-process cache
// collision on the same missing model. The gate releases both model downloads
// only after each process has entered the real staging path.
func TestStory001CharacterizesOverlappingInvokes(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{blockModel: true})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001Environment(homeDir, cacheDir, origin.URL())
	args := []string{"models", "invoke", "embed", "--input", "text=" + story001ModelInput}

	first := startStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
	t.Cleanup(first.stop)
	waitForStory001ModelStarts(t, origin, 1)
	second := startStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
	t.Cleanup(second.stop)
	waitForStory001ModelStarts(t, origin, 1)
	origin.releaseModelContent()
	firstResult := first.wait()
	secondResult := second.wait()
	cacheSnapshot := inspectStory001Cache(t, cacheDir)
	homeSnapshot := inspectStory001Cache(t, homeDir)
	workSnapshot := inspectStory001Cache(t, workDir)

	t.Logf(
		"STORY-001-EVIDENCE acceptance=overlap probe=two concurrent invokes first={%s} second={%s} modelStarts=%d explicitCache=%s homeCache=%s workTree=%s assetLedger=%s",
		summarizeProcess(firstResult), summarizeProcess(secondResult), origin.modelStartCount(), compactJSON(cacheSnapshot), compactJSON(homeSnapshot), compactJSON(workSnapshot), compactJSON(origin.assetExchanges()),
	)
}
