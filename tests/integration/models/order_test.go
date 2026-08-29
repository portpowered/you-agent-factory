package models_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestStory002ProvesAssetPreflightAndBodyOrder records the delivered
// joined-invoke acquisition sequence after the cost-safe acquisition fix.
func TestStory002ProvesAssetPreflightAndBodyOrder(t *testing.T) {
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
	modelIndex, backendHeadIndex, backendContentIndex := -1, -1, -1
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
			if exchange.Method == http.MethodHead && backendHeadIndex < 0 {
				backendHeadIndex = index
			}
			if exchange.Method == http.MethodGet && backendContentIndex < 0 {
				backendContentIndex = index
				backendBytes += exchange.ResponseBodyBytes
			}
		}
	}
	if modelIndex < 0 || backendHeadIndex < 0 || backendContentIndex < 0 {
		t.Fatalf("controlled asset ledger omitted model or backend content: %s", compactJSON(assets))
	}
	if backendHeadIndex > modelIndex || backendContentIndex > modelIndex {
		t.Fatalf("backend preflight/content followed model content: head=%d backend=%d model=%d ledger=%s", backendHeadIndex, backendContentIndex, modelIndex, compactJSON(assets))
	}
	if modelBytes <= 0 || backendBytes <= 0 {
		t.Fatalf("controlled response body bytes model=%d backend=%d, want both positive: %s", modelBytes, backendBytes, compactJSON(assets))
	}
	if !strings.Contains(string(result.stderr), `models asset estimate modelName="embed" backendBytes=25 modelBytes=23 totalBytes=48`) {
		t.Fatalf("asset estimate stderr = %q, want exact missing-byte estimate", result.stderr)
	}
	explicitCache := inspectStory001Cache(t, cacheDir)
	homeCache := inspectStory001Cache(t, homeDir)
	workSnapshot := inspectStory001Cache(t, workDir)

	t.Logf(
		"STORY-002-EVIDENCE acceptance=asset-preflight-and-order probe=delivered joined invoke command=%q assetLedger=%s modelResponseBytes=%d backendResponseBytes=%d streams=%s explicitCache=%s homeCache=%s workTree=%s",
		"you models invoke embed --input text=<redacted>", compactJSON(assets), modelBytes, backendBytes, summarizeProcess(result), compactJSON(explicitCache), compactJSON(homeCache), compactJSON(workSnapshot),
	)
	if strings.HasSuffix(origin.URL(), ":7438") || strings.Contains(compactJSON(origin.exchangesSnapshot()), ":7438") {
		t.Fatal("story-001 controlled-origin probe observed forbidden port 7438")
	}
}

// TestStory002BuiltUnavailableBackendStopsBeforeModelContent proves the
// delivered invoke checks backend reachability before model metadata/content.
// The controlled backend returns 503 for every reachability attempt, so the
// model response-byte counter is an independent built-process 0/0 witness.
func TestStory002BuiltUnavailableBackendStopsBeforeModelContent(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{failBackend: true})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001Environment(homeDir, cacheDir, origin.URL())
	modelResponseBytesBefore := origin.modelContentResponseBytes()

	result := runStory001Command(
		t, context.Background(), binaryPath, workDir, environment,
		"models", "invoke", "embed", "--input", "text="+story001ModelInput,
	)
	modelResponseBytesAfter := origin.modelContentResponseBytes()
	assets := origin.assetExchanges()
	if result.exitCode != 1 || !result.processExited || result.timedOut {
		t.Fatalf("unavailable-backend invoke did not exit 1: %s", summarizeProcess(result))
	}
	if len(result.stdout) != 0 {
		t.Fatalf("unavailable-backend invoke stdout = %q, want empty", result.stdout)
	}
	if !strings.Contains(string(result.stderr), "MODEL_BACKEND_NOT_READY") {
		t.Fatalf("unavailable-backend invoke stderr = %q, want MODEL_BACKEND_NOT_READY", result.stderr)
	}
	if len(assets) == 0 {
		t.Fatal("unavailable-backend asset ledger is empty")
	}
	for index, exchange := range assets {
		if !strings.HasSuffix(exchange.Path, "/"+story001BackendAsset) {
			t.Fatalf("asset exchange %d = %#v, want backend reachability only", index, exchange)
		}
		if exchange.Method != http.MethodHead || exchange.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("asset exchange %d = %#v, want failed backend HEAD", index, exchange)
		}
	}
	if modelResponseBytesBefore != 0 || modelResponseBytesAfter != 0 {
		t.Fatalf("model response bytes before=%d after=%d, want 0/0: %s", modelResponseBytesBefore, modelResponseBytesAfter, compactJSON(assets))
	}

	t.Logf(
		"STORY-002-EVIDENCE acceptance=unavailable-backend-zero-model-content probe=built invoke command=%q preflightLedger=%s modelResponseBytesBefore=%d modelResponseBytesAfter=%d streams=%s",
		"you models invoke embed --input text=<redacted>", compactJSON(assets), modelResponseBytesBefore, modelResponseBytesAfter, summarizeProcess(result),
	)
	if strings.Contains(origin.URL(), ":7438") || strings.Contains(compactJSON(origin.exchangesSnapshot()), ":7438") {
		t.Fatal("story-002 unavailable-backend probe observed forbidden port 7438")
	}
}
