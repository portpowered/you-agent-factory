package models_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
// delivered processes share one content transfer and one generic publication
// while the follower waits for the owner to release the model lock.
func TestStory005SerializesOverlappingBuiltInvokes(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{blockModel: true})
	followerReadiness := installStory005FollowerReadiness(t, origin)
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
	followerReadiness.markOwnerTransferStarted()
	second := startStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
	t.Cleanup(second.stop)
	followerReadiness.wait(t)
	origin.releaseModelContent()
	firstResult := first.wait()
	secondResult := second.wait()
	firstEmbedding := assertStory005SuccessfulEmbed(t, "overlap owner", firstResult)
	secondEmbedding := assertStory005SuccessfulEmbed(t, "overlap follower", secondResult)
	assertStory005SameEmbedding(t, "overlap follower", firstEmbedding, secondEmbedding)

	transferBeforeReuse := story005ModelTransferObservation(origin)
	assertStory005Transfers(t, "overlap", transferBeforeReuse, 1, 1, int64(len(story001ModelBody)))
	committedBeforeReuse := inspectStory005CommittedModel(t, homeDir)
	assertStory005NoPartialState(t,
		inspectStory001Cache(t, cacheDir),
		inspectStory001Cache(t, homeDir),
		inspectStory001Cache(t, workDir),
	)

	followUp := runStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
	followUpEmbedding := assertStory005SuccessfulEmbed(t, "overlap cache-only follow-up", followUp)
	assertStory005SameEmbedding(t, "overlap cache-only follow-up", firstEmbedding, followUpEmbedding)
	transferAfterReuse := story005ModelTransferObservation(origin)
	if transferAfterReuse != transferBeforeReuse {
		t.Fatalf("cache-only follow-up changed model transfer observation: before=%s after=%s", compactJSON(transferBeforeReuse), compactJSON(transferAfterReuse))
	}
	committedAfterReuse := inspectStory005CommittedModel(t, homeDir)
	if committedAfterReuse.SnapshotPath != committedBeforeReuse.SnapshotPath ||
		committedAfterReuse.Metadata.Identity != committedBeforeReuse.Metadata.Identity {
		t.Fatalf("cache-only follow-up changed committed model identity: before=%s after=%s", compactJSON(committedBeforeReuse), compactJSON(committedAfterReuse))
	}
	assertStory005NoPartialState(t,
		inspectStory001Cache(t, cacheDir),
		inspectStory001Cache(t, homeDir),
		inspectStory001Cache(t, workDir),
	)

	t.Logf(
		"STORY-005-EVIDENCE acceptance=semantic-cross-process-publication probe=two delivered concurrent invokes plus cache-only follow-up artifactSHA256=%s artifactBytes=%d transferBeforeReuse=%s transferAfterReuse=%s committed=%s first={%s} second={%s} followUp={%s} assetLedger=%s",
		story001Binary.identity.sha256, story001Binary.identity.size, compactJSON(transferBeforeReuse), compactJSON(transferAfterReuse), compactJSON(committedAfterReuse), summarizeProcess(firstResult), summarizeProcess(secondResult), summarizeProcess(followUp), compactJSON(origin.assetExchanges()),
	)
}

// story005FollowerReadiness observes the second process entering the
// controlled origin after the owner has reached the blocked model transfer.
// This proves the owner and follower are live at the same time before the
// transfer release; the channel is the readiness signal and the timeout is
// only a safety ceiling.
type story005FollowerReadiness struct {
	mu                    sync.Mutex
	ownerTransferObserved bool
	followerReady         chan struct{}
	followerReadyOnce     sync.Once
}

func installStory005FollowerReadiness(t testing.TB, origin *characterizationOrigin) *story005FollowerReadiness {
	t.Helper()
	previousServer := origin.server
	previousServer.Close()
	readiness := &story005FollowerReadiness{followerReady: make(chan struct{})}
	origin.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == story001ManifestPath() {
			readiness.observeManifest()
		}
		origin.ServeHTTP(writer, request)
	}))
	return readiness
}

func (readiness *story005FollowerReadiness) markOwnerTransferStarted() {
	readiness.mu.Lock()
	readiness.ownerTransferObserved = true
	readiness.mu.Unlock()
}

func (readiness *story005FollowerReadiness) observeManifest() {
	readiness.mu.Lock()
	ownerStarted := readiness.ownerTransferObserved
	readiness.mu.Unlock()
	if ownerStarted {
		readiness.followerReadyOnce.Do(func() { close(readiness.followerReady) })
	}
}

func (readiness *story005FollowerReadiness) wait(t testing.TB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), story001ServerTimeout)
	defer cancel()
	select {
	case <-readiness.followerReady:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for follower manifest request while owner transfer was blocked")
	}
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
	retryEmbedding := assertStory005SuccessfulEmbed(t, "owner-exit recovery retry", retry)
	transferBeforeReuse := story005ModelTransferObservation(origin)
	assertStory005Transfers(t, "owner-exit recovery", transferBeforeReuse, 2, 1, int64(len(story001ModelBody)))
	committedBeforeReuse := inspectStory005CommittedModel(t, homeDir)
	assertStory005NoPartialState(t,
		inspectStory001Cache(t, cacheDir),
		inspectStory001Cache(t, homeDir),
		inspectStory001Cache(t, workDir),
	)

	followUp := runStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
	followUpEmbedding := assertStory005SuccessfulEmbed(t, "owner-exit cache-only follow-up", followUp)
	assertStory005SameEmbedding(t, "owner-exit cache-only follow-up", retryEmbedding, followUpEmbedding)
	transferAfterReuse := story005ModelTransferObservation(origin)
	if transferAfterReuse != transferBeforeReuse {
		t.Fatalf("owner-exit cache-only follow-up changed model transfer observation: before=%s after=%s", compactJSON(transferBeforeReuse), compactJSON(transferAfterReuse))
	}
	committedAfterReuse := inspectStory005CommittedModel(t, homeDir)
	if committedAfterReuse.SnapshotPath != committedBeforeReuse.SnapshotPath ||
		committedAfterReuse.Metadata.Identity != committedBeforeReuse.Metadata.Identity {
		t.Fatalf("owner-exit cache-only follow-up changed committed model identity: before=%s after=%s", compactJSON(committedBeforeReuse), compactJSON(committedAfterReuse))
	}
	assertStory005NoPartialState(t,
		inspectStory001Cache(t, cacheDir),
		inspectStory001Cache(t, homeDir),
		inspectStory001Cache(t, workDir),
	)

	t.Logf(
		"STORY-005-EVIDENCE acceptance=semantic-owner-exit-recovery probe=killed delivered owner plus retry and cache-only follow-up artifactSHA256=%s artifactBytes=%d transferBeforeReuse=%s transferAfterReuse=%s committed=%s retry={%s} followUp={%s} assetLedger=%s",
		story001Binary.identity.sha256, story001Binary.identity.size, compactJSON(transferBeforeReuse), compactJSON(transferAfterReuse), compactJSON(committedAfterReuse), summarizeProcess(retry), summarizeProcess(followUp), compactJSON(origin.assetExchanges()),
	)
}

type story005TransferObservation struct {
	ModelStarts        int   `json:"modelStarts"`
	CompletedTransfers int   `json:"completedTransfers"`
	ResponseBytes      int64 `json:"responseBytes"`
}

func story005ModelTransferObservation(origin *characterizationOrigin) story005TransferObservation {
	observation := story005TransferObservation{ModelStarts: origin.modelStartCount()}
	for _, exchange := range origin.assetExchanges() {
		if exchange.Method != http.MethodGet || exchange.Path != story001ModelResolvePath() || exchange.StatusCode != http.StatusOK {
			continue
		}
		observation.CompletedTransfers++
		observation.ResponseBytes += exchange.ResponseBodyBytes
	}
	return observation
}

func assertStory005Transfers(
	t testing.TB,
	label string,
	got story005TransferObservation,
	wantStarts, wantCompleted int,
	wantBytes int64,
) {
	t.Helper()
	if got.ModelStarts != wantStarts || got.CompletedTransfers != wantCompleted || got.ResponseBytes != wantBytes {
		t.Fatalf("%s model transfer observation = %s, want starts=%d completed=%d responseBytes=%d", label, compactJSON(got), wantStarts, wantCompleted, wantBytes)
	}
}

func assertStory005SuccessfulEmbed(t testing.TB, label string, result builtProcessResult) []float64 {
	t.Helper()
	if result.exitCode != 0 || !result.processExited || result.timedOut || result.runError != "" {
		t.Fatalf("%s failed: %s", label, summarizeProcess(result))
	}
	if processHasCacheCollision(result) {
		t.Fatalf("%s reported cache collision: %s", label, summarizeProcess(result))
	}
	var embedding []float64
	if err := json.Unmarshal(result.stdout, &embedding); err != nil {
		t.Fatalf("%s stdout = %q, want JSON embedding array: %v", label, result.stdout, err)
	}
	want := []float64{0.1, 0.2, 0.3, 0.4}
	if len(embedding) != len(want) {
		t.Fatalf("%s embedding = %#v, want four values", label, embedding)
	}
	for index := range want {
		if math.Abs(embedding[index]-want[index]) > 1e-12 {
			t.Fatalf("%s embedding = %#v, want %#v", label, embedding, want)
		}
	}
	return embedding
}

func assertStory005SameEmbedding(t testing.TB, label string, want, got []float64) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s embedding length = %d, want %d", label, len(got), len(want))
	}
	for index := range want {
		if math.Abs(want[index]-got[index]) > 1e-12 {
			t.Fatalf("%s embedding = %#v, want prior semantic output %#v", label, got, want)
		}
	}
}

func assertStory005NoPartialState(t testing.TB, snapshots ...cacheSnapshot) {
	t.Helper()
	for _, snapshot := range snapshots {
		incomplete := make([]string, 0)
		for _, entry := range snapshot.Entries {
			if strings.Contains(entry, ".partial") || strings.Contains(entry, ".previous") {
				incomplete = append(incomplete, entry)
			}
		}
		if len(incomplete) != 0 {
			t.Fatalf("cache retained incomplete staging entries: %#v", incomplete)
		}
	}
}

type story005ObservedArtifact struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type story005ObservedMetadata struct {
	Kind      string                     `json:"kind"`
	Identity  string                     `json:"identity"`
	Source    string                     `json:"source"`
	SourceKey string                     `json:"sourceKey"`
	Artifacts []story005ObservedArtifact `json:"artifacts"`
}

type story005CommittedModel struct {
	MetadataPath  string                   `json:"metadataPath"`
	SnapshotPath  string                   `json:"snapshotPath"`
	Metadata      story005ObservedMetadata `json:"metadata"`
	PayloadBytes  int64                    `json:"payloadBytes"`
	PayloadSHA256 string                   `json:"payloadSHA256"`
}

func inspectStory005CommittedModel(t testing.TB, root string) story005CommittedModel {
	t.Helper()
	metadataPaths := make([]string, 0, 1)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != ".you-assets.json" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !strings.Contains("/"+filepath.ToSlash(relative), "/.you-content-addressed/model/") {
			return nil
		}
		metadataPaths = append(metadataPaths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("inspect committed generic model cache: %v", err)
	}
	if len(metadataPaths) != 1 {
		t.Fatalf("committed generic model metadata files = %#v, want exactly one", metadataPaths)
	}
	metadataPath := metadataPaths[0]
	metadataBody, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read committed generic model metadata: %v", err)
	}
	var metadata story005ObservedMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		t.Fatalf("decode committed generic model metadata %q: %v", metadataPath, err)
	}
	if metadata.Kind != "model" || metadata.Identity == "" || metadata.Source == "" || metadata.SourceKey == "" || len(metadata.Artifacts) != 1 {
		t.Fatalf("committed generic model metadata = %#v, want one identified model artifact", metadata)
	}
	artifact := metadata.Artifacts[0]
	if artifact.Name != story001ModelAsset || artifact.Bytes != int64(len(story001ModelBody)) || !strings.EqualFold(artifact.SHA256, sha256Hex(story001ModelBody)) {
		t.Fatalf("committed generic model observed artifact = %#v, want controlled size/digest for %q", artifact, story001ModelAsset)
	}
	snapshotPath := filepath.Dir(metadataPath)
	if strings.HasSuffix(snapshotPath, ".partial") || strings.HasSuffix(snapshotPath, ".previous") || len(filepath.Base(snapshotPath)) != 64 {
		t.Fatalf("committed generic model snapshot path = %q, want content-addressed committed identity", snapshotPath)
	}
	payloadPath := filepath.Join(snapshotPath, filepath.FromSlash(artifact.Name))
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatalf("read committed generic model payload %q: %v", payloadPath, err)
	}
	payloadDigest := sha256Hex(payload)
	if int64(len(payload)) != artifact.Bytes || !strings.EqualFold(payloadDigest, artifact.SHA256) {
		t.Fatalf("committed generic model payload size=%d sha256=%s, metadata=%#v", len(payload), payloadDigest, artifact)
	}
	return story005CommittedModel{
		MetadataPath:  metadataPath,
		SnapshotPath:  snapshotPath,
		Metadata:      metadata,
		PayloadBytes:  int64(len(payload)),
		PayloadSHA256: payloadDigest,
	}
}
