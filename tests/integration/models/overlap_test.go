package models_test

import (
	"context"
	"strings"
	"testing"
)

// TestStory001CharacterizesOverlappingInvokes records the two-process cache
// interaction on the same missing model. The release is deliberately
// controlled so the first process is inside the model transfer before the
// follower is started.
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

// TestStory005SerializesOverlappingBuiltInvokes proves that separate
// delivered processes share one content transfer and one managed publication
// while the follower waits for the owner to release the model lock.
func TestStory005SerializesOverlappingBuiltInvokes(t *testing.T) {
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
	origin.releaseModelContent()
	firstResult := first.wait()
	secondResult := second.wait()
	if firstResult.exitCode != 0 || secondResult.exitCode != 0 ||
		firstResult.timedOut || secondResult.timedOut ||
		firstResult.runError != "" || secondResult.runError != "" {
		t.Fatalf("overlapping invokes failed: first={%s} second={%s}", summarizeProcess(firstResult), summarizeProcess(secondResult))
	}
	if processHasCacheCollision(firstResult) || processHasCacheCollision(secondResult) {
		t.Fatalf("overlapping invokes reported cache collision: first={%s} second={%s}", summarizeProcess(firstResult), summarizeProcess(secondResult))
	}
	if got := origin.modelStartCount(); got != 1 {
		t.Fatalf("model transfer starts = %d, want one owner transfer", got)
	}

	cacheSnapshot := inspectStory001Cache(t, homeDir)
	if len(cacheSnapshot.Partial) != 0 {
		t.Fatalf("cache retained partial artifacts: %#v", cacheSnapshot.Partial)
	}
	if cacheSnapshot.RegularBytes == 0 || !hasCacheEntry(cacheSnapshot, ".you-content-addressed/model/") ||
		!hasCacheEntry(cacheSnapshot, ".managed-cache.json") {
		t.Fatalf("cache did not contain a complete published model: %s", compactJSON(cacheSnapshot))
	}

	t.Logf(
		"STORY-005-EVIDENCE acceptance=cross-process-staging probe=two delivered concurrent invokes modelStarts=%d first={%s} second={%s} cache=%s assetLedger=%s",
		origin.modelStartCount(), summarizeProcess(firstResult), summarizeProcess(secondResult), compactJSON(cacheSnapshot), compactJSON(origin.assetExchanges()),
	)
}

// TestStory005RecoversAfterOwnerExit proves that a later delivered process
// removes an abandoned staging directory after the OS releases its lock.
func TestStory005RecoversAfterOwnerExit(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{blockModel: true})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001Environment(homeDir, cacheDir, origin.URL())
	args := []string{"models", "invoke", "embed", "--input", "text=" + story001ModelInput}

	owner := startStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
	t.Cleanup(owner.stop)
	waitForStory001ModelStarts(t, origin, 1)
	owner.stop()
	origin.releaseModelContent()

	retry := runStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
	if retry.exitCode != 0 || retry.timedOut || retry.runError != "" {
		t.Fatalf("retry after owner exit failed: %s", summarizeProcess(retry))
	}
	if processHasCacheCollision(retry) {
		t.Fatalf("retry after owner exit reported cache collision: %s", summarizeProcess(retry))
	}
	cacheSnapshot := inspectStory001Cache(t, homeDir)
	if len(cacheSnapshot.Partial) != 0 || !hasCacheEntry(cacheSnapshot, ".managed-cache.json") {
		t.Fatalf("retry left incomplete cache state: %s", compactJSON(cacheSnapshot))
	}

	t.Logf(
		"STORY-005-EVIDENCE acceptance=owner-exit-recovery probe=killed delivered owner plus retry modelStarts=%d retry={%s} cache=%s assetLedger=%s",
		origin.modelStartCount(), summarizeProcess(retry), compactJSON(cacheSnapshot), compactJSON(origin.assetExchanges()),
	)
}

func hasCacheEntry(snapshot cacheSnapshot, marker string) bool {
	for _, entry := range snapshot.Entries {
		if strings.Contains(entry, marker) {
			return true
		}
	}
	return false
}
