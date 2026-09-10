package root_composition_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	selectedTTSSiblingBody  = "preserve unrelated revision"
	selectedTTSExternalBody = "preserve outside selected cache"
)

// TestModelsSelectedTTSCatalogLifecycleUsesOneCacheRoot proves the complete
// selected-root path through the public root.BuildProcess command boundary:
// invoke, list, inspect, remove, and typed missing-state observations all use
// one canonical managed-runtime revision without touching the default cache.
func TestModelsSelectedTTSCatalogLifecycleUsesOneCacheRoot(t *testing.T) {
	t.Parallel()
	story := setupTTSStory(t)
	selectedRoot := filepath.Join(story.home, "selected-model-cache")
	writeGenericBackendCacheAtRoot(t, selectedRoot, "localai-vibevoice", pinnedTTSBackendSelection(), []byte("pinned-backend-fixture"))
	environment := selectedModelCacheEnvironment(story.environment, selectedRoot)
	defaultRoot := filepath.Join(story.home, ".agent-factory", "models")
	defaultBefore := snapshotSelectedTTSFiles(t, defaultRoot)
	observation := observeSelectedTTSCatalog(t, story, environment, selectedRoot)
	siblingPath, externalSentinel := seedSelectedTTSRemovalSentinels(t, story.home, observation.cachePath)
	removed := executeSelectedTTSCommand(t, story.process, story.dir, environment,
		"you", "--json", "models", "remove", "tTs")
	assertSelectedTTSRemoval(t, removed, observation, siblingPath, externalSentinel)
	assertSelectedTTSMissingState(t, story, environment, selectedRoot, defaultRoot, defaultBefore, observation)

	t.Logf("selected-root TTS lifecycle passed: cachePath=%s revision=%s bytesRemoved=%d selectedRoot=%s defaultRootUnchanged=true networkCalls=%d", observation.cachePath, observation.revision, observation.beforeRemoveBytes, selectedRoot, story.network.Calls())
}

type selectedTTSObservation struct {
	cachePath         string
	revision          string
	beforeRemoveBytes int64
}

func observeSelectedTTSCatalog(
	t *testing.T,
	story ttsStory,
	environment []string,
	selectedRoot string,
) selectedTTSObservation {
	t.Helper()

	outputPath := filepath.Join(story.dir, "selected-tts.wav")
	invoke := executeSelectedTTSCommand(t, story.process, story.dir, environment,
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "selected-root", "--output", outputPath)
	if invoke.err != nil {
		t.Fatalf("Process.Execute(models invoke tts) error = %v\nstdout:\n%s\nstderr:\n%s", invoke.err, invoke.stdout, invoke.stderr)
	}
	audio, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read selected TTS output: %v", err)
	}
	wantAudio := story.protocol.audioFor("selected-root")
	if !bytes.Equal(audio, wantAudio) {
		t.Fatalf("selected TTS audio SHA256 = %s, want %s", digestSelectedTTS(audio), digestSelectedTTS(wantAudio))
	}
	assertSemanticTTSAudio(t, audio, "selected-root output")

	listed := executeSelectedTTSCommand(t, story.process, story.dir, environment,
		"you", "--json", "models", "list")
	if listed.err != nil {
		t.Fatalf("Process.Execute(models list) error = %v\nstdout:\n%s", listed.err, listed.stdout)
	}
	var listResponse factoryapi.ListModelsResponse
	decodeSelectedTTSJSON(t, listed.stdout, &listResponse)
	listModel, ok := findModelSummary(listResponse.Results, models.BuiltInModelNameTTS)
	if !ok {
		t.Fatalf("models list did not contain %q: %#v", models.BuiltInModelNameTTS, listResponse.Results)
	}
	assertSelectedTTSReady(t, "models list", listModel.ManagedRuntime, selectedRoot)

	inspected := executeSelectedTTSCommand(t, story.process, story.dir, environment,
		"you", "--json", "models", "inspect", "TtS")
	if inspected.err != nil {
		t.Fatalf("Process.Execute(models inspect TtS) error = %v\nstdout:\n%s", inspected.err, inspected.stdout)
	}
	var inspectResponse factoryapi.ModelDetail
	decodeSelectedTTSJSON(t, inspected.stdout, &inspectResponse)
	if inspectResponse.Name != models.BuiltInModelNameTTS {
		t.Fatalf("models inspect name = %q, want %q", inspectResponse.Name, models.BuiltInModelNameTTS)
	}
	assertSelectedTTSReady(t, "models inspect", inspectResponse.ManagedRuntime, selectedRoot)
	if listModel.ManagedRuntime.Revision == nil || inspectResponse.ManagedRuntime.Revision == nil ||
		*listModel.ManagedRuntime.Revision != *inspectResponse.ManagedRuntime.Revision {
		t.Fatalf("list/inspect revisions = %v/%v, want one canonical revision", listModel.ManagedRuntime.Revision, inspectResponse.ManagedRuntime.Revision)
	}
	if listModel.ManagedRuntime.CachePath == nil || inspectResponse.ManagedRuntime.CachePath == nil ||
		*listModel.ManagedRuntime.CachePath != *inspectResponse.ManagedRuntime.CachePath {
		t.Fatalf("list/inspect cache paths = %v/%v, want one canonical revision path", listModel.ManagedRuntime.CachePath, inspectResponse.ManagedRuntime.CachePath)
	}

	cachePath := *inspectResponse.ManagedRuntime.CachePath
	bytes := story003RegularFileBytes(t, cachePath)
	if bytes <= 0 {
		t.Fatalf("selected TTS cache bytes before remove = %d, want positive regular-file sum", bytes)
	}
	if !pathWithinRoot(cachePath, selectedRoot) {
		t.Fatalf("selected TTS cache path = %q, escaped selected root %q", cachePath, selectedRoot)
	}
	revision := *inspectResponse.ManagedRuntime.Revision
	return selectedTTSObservation{cachePath: cachePath, revision: revision, beforeRemoveBytes: bytes}
}

func seedSelectedTTSRemovalSentinels(t *testing.T, home, cachePath string) (string, string) {
	t.Helper()
	modelRoot := filepath.Dir(cachePath)
	siblingRevision := filepath.Join(modelRoot, "unrelated-revision")
	if err := os.MkdirAll(siblingRevision, 0o755); err != nil {
		t.Fatalf("create unrelated revision: %v", err)
	}
	siblingPath := filepath.Join(siblingRevision, "sentinel.bin")
	if err := os.WriteFile(siblingPath, []byte(selectedTTSSiblingBody), 0o644); err != nil {
		t.Fatalf("write unrelated revision sentinel: %v", err)
	}
	externalSentinel := filepath.Join(home, "outside-selected-cache-sentinel.txt")
	if err := os.WriteFile(externalSentinel, []byte(selectedTTSExternalBody), 0o644); err != nil {
		t.Fatalf("write external sentinel: %v", err)
	}
	return siblingPath, externalSentinel
}

func assertSelectedTTSRemoval(
	t *testing.T,
	removed selectedTTSCommandCapture,
	observation selectedTTSObservation,
	siblingPath, externalSentinel string,
) {
	t.Helper()
	if removed.err != nil {
		t.Fatalf("Process.Execute(models remove tTs) error = %v\nstdout:\n%s", removed.err, removed.stdout)
	}
	var removeResponse factoryapi.ModelRemoveResponse
	decodeSelectedTTSJSON(t, removed.stdout, &removeResponse)
	if removeResponse.ModelName != strings.ToUpper(models.BuiltInModelNameTTS) ||
		removeResponse.Outcome != factoryapi.REMOVED ||
		removeResponse.Revision != observation.revision ||
		removeResponse.CachePath != observation.cachePath ||
		removeResponse.BytesRemoved != observation.beforeRemoveBytes {
		t.Fatalf("models remove response = %#v, want TTS/%s/%s/%d", removeResponse, observation.cachePath, observation.revision, observation.beforeRemoveBytes)
	}
	if _, err := os.Stat(observation.cachePath); !os.IsNotExist(err) {
		t.Fatalf("removed selected TTS cache stat = %v, want absent", err)
	}
	if got, err := os.ReadFile(siblingPath); err != nil || string(got) != selectedTTSSiblingBody {
		t.Fatalf("unrelated revision sentinel = %q/%v, want %q", got, err, selectedTTSSiblingBody)
	}
	if got, err := os.ReadFile(externalSentinel); err != nil || string(got) != selectedTTSExternalBody {
		t.Fatalf("external sentinel = %q/%v, want %q", got, err, selectedTTSExternalBody)
	}
}

func assertSelectedTTSMissingState(
	t *testing.T,
	story ttsStory,
	environment []string,
	selectedRoot, defaultRoot string,
	defaultBefore map[string]string,
	observation selectedTTSObservation,
) {
	t.Helper()
	selectedAfterRemove := snapshotSelectedTTSFiles(t, selectedRoot)
	_, repeatedRemoveErr := executeSelectedTTSCommandResult(t, story.process, story.dir, environment,
		"you", "--json", "models", "remove", "tts")
	if repeatedRemoveErr == nil || !errors.Is(repeatedRemoveErr, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("repeated selected TTS remove error = %v, want ErrModelCacheNotFound", repeatedRemoveErr)
	}

	missingList := executeSelectedTTSCommand(t, story.process, story.dir, environment,
		"you", "--json", "models", "list")
	if missingList.err != nil {
		t.Fatalf("Process.Execute(missing models list) error = %v\nstdout:\n%s", missingList.err, missingList.stdout)
	}
	var missingListResponse factoryapi.ListModelsResponse
	decodeSelectedTTSJSON(t, missingList.stdout, &missingListResponse)
	missingModel, ok := findModelSummary(missingListResponse.Results, models.BuiltInModelNameTTS)
	if !ok {
		t.Fatalf("missing models list did not contain %q: %#v", models.BuiltInModelNameTTS, missingListResponse.Results)
	}
	assertSelectedTTSMissing(t, "missing models list", missingModel.ManagedRuntime)

	missingInspect, missingInspectErr := executeSelectedTTSCommandResult(t, story.process, story.dir, environment,
		"you", "--json", "models", "inspect", "tts")
	if missingInspectErr == nil {
		var missingInspectResponse factoryapi.ModelDetail
		decodeSelectedTTSJSON(t, missingInspect.stdout, &missingInspectResponse)
		assertSelectedTTSMissing(t, "missing models inspect", missingInspectResponse.ManagedRuntime)
	} else if !errors.Is(missingInspectErr, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("missing selected TTS inspect error = %v, want ErrModelCacheNotFound or missing projection", missingInspectErr)
	}
	if got := snapshotSelectedTTSFiles(t, selectedRoot); !reflect.DeepEqual(got, selectedAfterRemove) {
		t.Fatalf("missing-state catalog commands mutated selected cache: before=%v after=%v", selectedAfterRemove, got)
	}
	if got := snapshotSelectedTTSFiles(t, defaultRoot); !reflect.DeepEqual(got, defaultBefore) {
		t.Fatalf("selected-root lifecycle mutated default cache: before=%v after=%v", defaultBefore, got)
	}
	if got := story.network.Calls(); got != 0 {
		t.Fatalf("selected TTS lifecycle network calls = %d, want zero", got)
	}
	assertSelectedTTSAssetTrace(t, story.assetTrace.snapshot(), selectedRoot, defaultRoot)
	if observation.beforeRemoveBytes <= 0 || strings.TrimSpace(observation.revision) == "" {
		t.Fatal("selected TTS observation lost canonical removal facts")
	}
}

// TestModelsSelectedTTSPullThenCatalogUsesOneCacheRoot proves that the
// selected catalog scope also carries the same cache root through a controlled
// first pull before list and inspect observe the installed revision.
func TestModelsSelectedTTSPullThenCatalogUsesOneCacheRoot(t *testing.T) {
	t.Parallel()
	story := setupTTSStory(t)
	selectedRoot := filepath.Join(story.home, "selected-pull-cache")
	writeGenericBackendCacheAtRoot(t, selectedRoot, "localai-vibevoice", pinnedTTSBackendSelection(), []byte("pinned-backend-fixture"))
	environment := selectedModelCacheEnvironment(story.environment, selectedRoot)
	defaultRoot := filepath.Join(story.home, ".agent-factory", "models")
	defaultBefore := snapshotSelectedTTSFiles(t, defaultRoot)

	pulled := executeSelectedTTSCommand(t, story.process, story.dir, environment,
		"you", "--json", "models", "pull", "TtS")
	var pullResponse factoryapi.ModelPullResponse
	decodeSelectedTTSJSON(t, pulled.stdout, &pullResponse)
	if !strings.EqualFold(pullResponse.ModelName, models.BuiltInModelNameTTS) ||
		pullResponse.Outcome != factoryapi.ModelPullOutcomePULLED ||
		pullResponse.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY ||
		pullResponse.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("selected TTS pull response = %#v, want TTS/PULLED/INSTALLED_SUCCESSFULLY/READY", pullResponse)
	}
	if pullResponse.CachePath == "" || pullResponse.ManagedRuntimePull.CachePath == nil ||
		*pullResponse.ManagedRuntimePull.CachePath != pullResponse.CachePath ||
		!pathWithinRoot(pullResponse.CachePath, selectedRoot) {
		t.Fatalf("selected TTS pull cache paths = %q/%v, want one path within %q", pullResponse.CachePath, pullResponse.ManagedRuntimePull.CachePath, selectedRoot)
	}
	pullBytes := story003RegularFileBytes(t, pullResponse.CachePath)
	if pullBytes <= 0 {
		t.Fatalf("selected TTS pull regular-file bytes = %d, want positive", pullBytes)
	}

	listed := executeSelectedTTSCommand(t, story.process, story.dir, environment,
		"you", "--json", "models", "list")
	var listResponse factoryapi.ListModelsResponse
	decodeSelectedTTSJSON(t, listed.stdout, &listResponse)
	listModel, ok := findModelSummary(listResponse.Results, models.BuiltInModelNameTTS)
	if !ok {
		t.Fatalf("selected pull models list did not contain %q: %#v", models.BuiltInModelNameTTS, listResponse.Results)
	}
	assertSelectedTTSReady(t, "selected pull models list", listModel.ManagedRuntime, selectedRoot)

	inspected := executeSelectedTTSCommand(t, story.process, story.dir, environment,
		"you", "--json", "models", "inspect", "tts")
	var inspectResponse factoryapi.ModelDetail
	decodeSelectedTTSJSON(t, inspected.stdout, &inspectResponse)
	assertSelectedTTSReady(t, "selected pull models inspect", inspectResponse.ManagedRuntime, selectedRoot)
	if inspectResponse.ManagedRuntime.CachePath == nil ||
		*inspectResponse.ManagedRuntime.CachePath != pullResponse.CachePath ||
		listModel.ManagedRuntime.Revision == nil || inspectResponse.ManagedRuntime.Revision == nil ||
		*listModel.ManagedRuntime.Revision != *inspectResponse.ManagedRuntime.Revision {
		t.Fatalf("selected pull catalog identity diverged: pull=%#v list=%#v inspect=%#v", pullResponse.ManagedRuntimePull, listModel.ManagedRuntime, inspectResponse.ManagedRuntime)
	}
	if listModel.ManagedRuntime.CacheBytes == nil || *listModel.ManagedRuntime.CacheBytes != pullBytes ||
		inspectResponse.ManagedRuntime.CacheBytes == nil || *inspectResponse.ManagedRuntime.CacheBytes != pullBytes {
		t.Fatalf("selected pull cache bytes = list:%v inspect:%v independent:%d", listModel.ManagedRuntime.CacheBytes, inspectResponse.ManagedRuntime.CacheBytes, pullBytes)
	}
	if got := snapshotSelectedTTSFiles(t, defaultRoot); !reflect.DeepEqual(got, defaultBefore) {
		t.Fatalf("selected TTS pull mutated default cache: before=%v after=%v", defaultBefore, got)
	}
	if got := story.network.Calls(); got != 0 {
		t.Fatalf("selected TTS pull network calls = %d, want zero", got)
	}
	assertSelectedTTSAssetTrace(t, story.assetTrace.snapshot(), selectedRoot, defaultRoot)

	revision := ""
	if inspectResponse.ManagedRuntime.Revision != nil {
		revision = *inspectResponse.ManagedRuntime.Revision
	}
	t.Logf("selected-root TTS pull passed: cachePath=%s revision=%s independentBytes=%d downloadedFiles=%d selectedRoot=%s defaultRootUnchanged=true networkCalls=%d", pullResponse.CachePath, revision, pullBytes, len(pullResponse.DownloadedFiles), selectedRoot, story.network.Calls())
}

type selectedTTSCommandCapture struct {
	stdout string
	stderr string
	err    error
}

func executeSelectedTTSCommand(
	t *testing.T,
	process support.Process,
	workingDirectory string,
	environment []string,
	arguments ...string,
) selectedTTSCommandCapture {
	t.Helper()
	capture, err := executeSelectedTTSCommandResult(t, process, workingDirectory, environment, arguments...)
	if err != nil {
		t.Fatalf("Process.Execute(%s) error = %v\nstdout:\n%s\nstderr:\n%s", strings.Join(arguments, " "), err, capture.stdout, capture.stderr)
	}
	return capture
}

func executeSelectedTTSCommandResult(
	t *testing.T,
	process support.Process,
	workingDirectory string,
	environment []string,
	arguments ...string,
) (selectedTTSCommandCapture, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), arguments)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = workingDirectory
	err := process.Execute(inputs.Input)
	return selectedTTSCommandCapture{stdout: inputs.Stdout(), stderr: inputs.Stderr(), err: err}, err
}

func selectedModelCacheEnvironment(base []string, selectedRoot string) []string {
	environment := make([]string, 0, len(base)+1)
	for _, value := range base {
		if strings.HasPrefix(value, runcli.ModelCacheDirEnvironment+"=") {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, runcli.ModelCacheDirEnvironment+"="+selectedRoot)
}

func assertSelectedTTSReady(t *testing.T, phase string, runtime factoryapi.ManagedRuntime, selectedRoot string) {
	t.Helper()
	if runtime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		runtime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("%s runtime = %#v diagnostics=%v, want READY/INSTALLED", phase, runtime, runtime.Diagnostics)
	}
	if runtime.Revision == nil || strings.TrimSpace(*runtime.Revision) == "" {
		t.Fatalf("%s revision = %#v, want canonical revision", phase, runtime.Revision)
	}
	if runtime.CachePath == nil || !pathWithinRoot(*runtime.CachePath, selectedRoot) {
		t.Fatalf("%s cache path = %#v, want path within selected root %q", phase, runtime.CachePath, selectedRoot)
	}
	if runtime.CacheBytes == nil || *runtime.CacheBytes <= 0 {
		t.Fatalf("%s cache bytes = %#v, want positive regular-file bytes", phase, runtime.CacheBytes)
	}
}

func assertSelectedTTSMissing(t *testing.T, phase string, runtime factoryapi.ManagedRuntime) {
	t.Helper()
	if runtime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING ||
		runtime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("%s runtime = %#v, want MISSING/NOT_INSTALLED", phase, runtime)
	}
}

func decodeSelectedTTSJSON(t *testing.T, raw string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		t.Fatalf("decode selected TTS JSON output: %v\noutput:\n%s", err, raw)
	}
}

func snapshotSelectedTTSFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			snapshot[relative] = "symlink"
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			snapshot[relative] = info.Mode().String()
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(body)
		snapshot[relative] = fmt.Sprintf("%d:%s", info.Size(), hex.EncodeToString(digest[:]))
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return snapshot
	}
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snapshot
}

func assertSelectedTTSAssetTrace(t *testing.T, trace []string, selectedRoot, defaultRoot string) {
	t.Helper()
	selectedSeen := false
	for _, entry := range trace {
		separator := strings.IndexByte(entry, ':')
		if separator < 0 {
			continue
		}
		path := entry[separator+1:]
		if pathWithinRoot(path, defaultRoot) {
			t.Fatalf("asset trace touched default cache root: %q", entry)
		}
		if pathWithinRoot(path, selectedRoot) {
			selectedSeen = true
		}
	}
	if !selectedSeen {
		t.Fatalf("asset trace did not observe selected cache root %q: %#v", selectedRoot, trace)
	}
}

func digestSelectedTTS(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
