package models_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestStory001CharacterizesAssetOrderAndBodyBytes records the current
// delivered joined-invoke acquisition sequence. It deliberately preserves the
// pre-fix order as an executable baseline for the next story to replace.
func TestStory001CharacterizesAssetOrderAndBodyBytes(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001Environment(homeDir, cacheDir, origin.URL())

	result := runStory001Command(
		t, context.Background(), binaryPath, workDir, environment,
		"models", "invoke", "embed", "--input", "text="+story001ModelInput,
	)
	if result.exitCode != 0 || result.timedOut || result.runError != "" {
		t.Fatalf("controlled joined invoke failed: %s", summarizeProcess(result))
	}

	assets := origin.assetExchanges()
	if len(assets) < 3 {
		t.Fatalf("controlled asset exchange count = %d, want manifest, model, and backend observations: %s", len(assets), compactJSON(assets))
	}
	modelIndex, backendIndex := -1, -1
	modelBytes, backendBytes := int64(0), int64(0)
	for index, exchange := range assets {
		if exchange.StatusCode != http.StatusOK {
			t.Fatalf("controlled asset exchange %d = %#v, want status 200", index, exchange)
		}
		switch {
		case exchange.Path == story001ModelResolvePath():
			if modelIndex < 0 {
				modelIndex = index
			}
			modelBytes += exchange.ResponseBodyBytes
		case strings.HasSuffix(exchange.Path, "/"+story001BackendAsset):
			if backendIndex < 0 {
				backendIndex = index
			}
			backendBytes += exchange.ResponseBodyBytes
		}
	}
	if modelIndex < 0 || backendIndex < 0 {
		t.Fatalf("controlled asset ledger omitted model or backend content: %s", compactJSON(assets))
	}
	if modelIndex > backendIndex {
		t.Fatalf("current baseline order changed unexpectedly: modelIndex=%d backendIndex=%d ledger=%s", modelIndex, backendIndex, compactJSON(assets))
	}
	if modelBytes <= 0 || backendBytes <= 0 {
		t.Fatalf("controlled response body bytes model=%d backend=%d, want both positive: %s", modelBytes, backendBytes, compactJSON(assets))
	}
	explicitCache := inspectStory001Cache(t, cacheDir)
	homeCache := inspectStory001Cache(t, homeDir)
	workSnapshot := inspectStory001Cache(t, workDir)

	t.Logf(
		"STORY-001-EVIDENCE acceptance=asset-order probe=delivered joined invoke command=%q assetLedger=%s modelResponseBytes=%d backendResponseBytes=%d streams=%s explicitCache=%s homeCache=%s workTree=%s",
		"you models invoke embed --input text=<redacted>", compactJSON(assets), modelBytes, backendBytes, summarizeProcess(result), compactJSON(explicitCache), compactJSON(homeCache), compactJSON(workSnapshot),
	)
	if strings.HasSuffix(origin.URL(), ":7438") || strings.Contains(compactJSON(origin.exchangesSnapshot()), ":7438") {
		t.Fatal("story-001 controlled-origin probe observed forbidden port 7438")
	}
}
