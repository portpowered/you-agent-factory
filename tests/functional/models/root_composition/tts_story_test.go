package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
	localaiproto "github.com/portpowered/infinite-you/tests/functional/internal/support/localai/protocol"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// TestModelsDirectTTSAliasEndToEndThroughRootBuildProcess proves that the
// generic text input and legacy --text/--output spellings share one request,
// one readiness projection, and one private raw-protocol fixture invocation.
func TestModelsDirectTTSAliasEndToEndThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	story := setupTTSStory(t)
	assertTTSReady(t, story)
	wantAudio := localai.AudioBytes()
	runGenericTTS(t, story, wantAudio)
	assertTTSRoleBundleReads(t, story)
	runAliasTTS(t, story, wantAudio)
	assertEquivalentTTSRequests(t, story.protocol.Calls(), story.temp.directory)
	runExactTTSToASRChain(t, story)
	runTTSFailureMatrix(t, story, wantAudio)
	assertTTSIsolationAndRelease(t, story)
}

func TestModelsDirectTTSKeepsConcurrentScenariosIsolatedThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name string
		text string
	}{
		{name: "scenario-a", text: "isolated-a"},
		{name: "scenario-b", text: "isolated-b"},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			// The sequential journey above reuses one root process. Parallel
			// invocations cannot share it: the public command binds the same
			// customer-visible ~default runtime scope, so each concurrent scenario
			// owns a process and its mutable lifecycle edges.
			story := setupTTSStory(t)
			assertTTSReady(t, story)
			directory := story.dir
			outputPath := filepath.Join(directory, scenario.name+".wav")
			var stdout, stderr bytes.Buffer
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "models", "invoke", "tts", "--operation", "TTS", "--text", scenario.text, "--output", outputPath,
			})
			inputs.Input.Env = story.environment
			inputs.Input.WorkingDirectory = directory
			inputs.Input.Stdout = &stdout
			inputs.Input.Stderr = &stderr
			if err := story.process.Execute(inputs.Input); err != nil {
				t.Fatalf("Process.Execute(%s) error = %v", scenario.name, err)
			}
			got, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read %s output: %v", scenario.name, err)
			}
			want := story.protocol.audioFor(scenario.text)
			if !bytes.Equal(got, want) {
				t.Fatalf("%s audio digest = %s, want %s", scenario.name, ttsDigest(got), ttsDigest(want))
			}
			assertSemanticTTSAudio(t, got, scenario.name+" output")
			if stdout.String() != "Wrote audio: "+outputPath+"\n" || stderr.String() != controlledTTSAssetEstimate {
				t.Fatalf("%s streams = stdout %q stderr %q, want first-use status and controlled asset estimate", scenario.name, stdout.String(), stderr.String())
			}
			if got := story.generic.Calls(); got != 0 {
				t.Fatalf("%s generic TTS backend calls = %d, want zero", scenario.name, got)
			}
			closeRootProcess(t, story.process, "close concurrent TTS root process")
			created, removed, duplicateRemoves := story.temp.Snapshot()
			if created != 1 || removed != created || duplicateRemoves != 0 {
				t.Fatalf("%s TTS staging release = created:%d removed:%d duplicateRemoves:%d, want one isolated one-shot path", scenario.name, created, removed, duplicateRemoves)
			}
			starts, stops, waits, active := story.host.Snapshot()
			if starts == 0 || starts != stops || stops != waits || active != 0 {
				t.Fatalf("%s host release = starts:%d stops:%d waits:%d active:%d, want exactly-once release", scenario.name, starts, stops, waits, active)
			}
			t.Logf("concurrent isolation command: you models invoke tts --operation TTS --text %s --output %s outputSize=%d outputSHA256=%s cache=%s workingDirectory=%s", scenario.text, outputPath, len(got), ttsDigest(got), filepath.Join(story.home, ".agent-factory", "models"), directory)
		})
	}
}

func TestModelsDirectTTSCharacterizesDefaultScopeNonReentrancy(t *testing.T) {
	t.Parallel()
	story := setupTTSStory(t)
	firstContext, cancelFirst := context.WithCancel(t.Context())
	defer cancelFirst()
	firstInputs := support.FakeInputs(firstContext, []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "cancelled", "--output", filepath.Join(story.dir, "blocked.wav"),
	})
	firstInputs.Input.Env = story.environment
	firstInputs.Input.WorkingDirectory = story.dir
	firstDone := make(chan error, 1)
	go func() { firstDone <- story.process.Execute(firstInputs.Input) }()
	select {
	case <-story.protocol.CancellationStarted():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first default-scope invocation")
	}

	secondInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "overlap", "--output", filepath.Join(story.dir, "overlap.wav"),
	})
	secondInputs.Input.Env = story.environment
	secondInputs.Input.WorkingDirectory = story.dir
	secondErr := story.process.Execute(secondInputs.Input)
	if secondErr == nil || !strings.Contains(secondErr.Error(), "runtime is already bound") {
		t.Fatalf("overlapping default-scope invocation error = %v, want bounded already-bound lifecycle failure", secondErr)
	}
	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) && !errors.Is(err, models.ErrInferenceCancelled) {
		t.Fatalf("first default-scope invocation error = %v, want cancellation after overlap characterization", err)
	}
	closeRootProcess(t, story.process, "close non-reentrant TTS root process")
	created, removed, duplicateRemoves := story.temp.Snapshot()
	if created != removed || duplicateRemoves != 0 {
		t.Fatalf("non-reentrant TTS staging release = created:%d removed:%d duplicateRemoves:%d, want exactly-once cleanup", created, removed, duplicateRemoves)
	}
	starts, stops, waits, active := story.host.Snapshot()
	if starts != stops || stops != waits || active != 0 {
		t.Fatalf("non-reentrant host release = starts:%d stops:%d waits:%d active:%d, want no leaked host lifecycle", starts, stops, waits, active)
	}
	t.Logf("lifecycle exception evidence: one root.BuildProcess owns customer scope ~default; overlapping Process.Execute returned bounded runtime-already-bound failure, first invocation canceled, temp/host ledgers released exactly once")
}

type ttsStory struct {
	process           support.Process
	dir               string
	environment       []string
	home              string
	temp              *ttsTempEffects
	protocol          *ttsPrivateProtocolFixture
	generic           *ttsGenericBackendTrap
	host              *ttsHostLauncher
	network           *rejectingModelAssetHTTP
	assetTrace        *functionalModelAssetTrace
	outputFailurePath string
}

const controlledTTSAssetEstimate = "models asset estimate modelName=\"tts\" backendBytes=0 modelBytes=82 totalBytes=82\n"

func setupTTSStory(t *testing.T) ttsStory {
	t.Helper()
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := functionalTempDir(t)
	ttsTempDirectory := filepath.Join(home, "tts-runtime-temp")
	if err := os.MkdirAll(ttsTempDirectory, 0o755); err != nil {
		t.Fatalf("create TTS runtime temp directory: %v", err)
	}
	temp := &ttsTempEffects{directory: ttsTempDirectory}
	writeControlledBuiltinTTSSource(t, home)
	writeGenericBuiltinTTSCache(t, home)
	writeGenericBuiltinTTSManagedRuntimeCache(t, home)
	writeGenericBuiltinTTSBackendCache(t, home)
	asrDefinition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameASR)
	if !ok {
		t.Fatal("built-in catalog did not publish the ASR model definition")
	}
	writeGenericBuiltinModelCache(t, home, asrDefinition.Source)
	asrSelection, asrBackendBody := fixtureBackendSelection(asrDefinition.Backend)
	writeGenericBackendCache(t, home, asrDefinition.Backend, asrSelection, asrBackendBody)
	selection := pinnedTTSBackendSelection()
	assetTrace := &functionalModelAssetTrace{}
	assetFiles := functionalModelAssetFileSystem{home: home, trace: assetTrace}
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostProtocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	dir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	protocol := newTTSPrivateProtocolFixture(localai.AudioBytes())
	generic := &ttsGenericBackendTrap{}
	host := &ttsHostLauncher{endpoint: modelServer.URL}
	outputFailurePath := filepath.Join(dir, "forced-output-failure.wav")
	process := functionalBuildProcess(t, serviceedges.Edges{
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
		ModelAssetHTTPClient:               rejectingNetwork,
		ModelAssetMakeDirectories:          assetFiles.MkdirAll,
		ModelAssetInspectPath:              assetFiles.Stat,
		ModelAssetResolveHomeDirectory:     assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:       func(string) string { return "" },
		ModelAssetWriteFile:                assetFiles.WriteFile,
		ModelAssetRenamePath:               assetFiles.Rename,
		ModelAssetReadFile:                 assetFiles.ReadFile,
		ModelAssetReadDirectory:            assetFiles.ReadDir,
		ModelAssetCreateFile:               assetFiles.Create,
		ModelAssetOpenFile:                 assetFiles.Open,
		ModelHostProcessLauncher:           host,
		ModelHostProtocolNegotiator:        hostProtocol,
		ModelHostCompatibilityChecker:      compatibility,
		ModelAssetHostPlatform:             models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(_ context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			// Both built-in operations share this isolated cache. The resolver is
			// still explicit so a cache miss cannot accidentally select the TTS
			// backend for ASR.
			if request.Backend == asrDefinition.Backend {
				return asrSelection, nil
			}
			return selection, nil
		},
		ModelInvocationBackend:     generic.Invoke,
		ModelInvocationGRPCDialer:  protocol,
		ModelHostHTTPClient:        modelServer.Client(),
		ModelRuntimeHTTPClient:     modelServer.Client(),
		ModelRuntimeTempDirectory:  func() string { return temp.directory },
		ModelRuntimeCreateTempFile: temp.CreateTemp,
		ModelRuntimeInspectFile:    os.Stat,
		ModelAssetRemovePath:       temp.Remove,
		ModelASRBackend:            protocol.ASRBackend,
		ModelCLIOutputRenamePath: func(oldPath, newPath string) error {
			if newPath == outputFailurePath {
				return errors.New("controlled output publication failure")
			}
			return os.Rename(oldPath, newPath)
		},
	})
	t.Cleanup(func() { closeRootProcess(t, process, "close TTS root process") })
	return ttsStory{
		process: process, dir: dir, environment: functionalHomeEnvironment(home), home: home,
		temp: temp, protocol: protocol, generic: generic, host: host, network: rejectingNetwork, assetTrace: assetTrace,
		outputFailurePath: outputFailurePath,
	}
}

func assertTTSReady(t *testing.T, story ttsStory) {
	t.Helper()
	listInputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "list"})
	listInputs.Input.Env = story.environment
	listInputs.Input.WorkingDirectory = story.dir
	if err := story.process.Execute(listInputs.Input); err != nil {
		t.Fatalf("Process.Execute(models list) error = %v", err)
	}
	var listed factoryapi.ListModelsResponse
	if err := jsonUnmarshalFunctional(listInputs.Stdout(), &listed); err != nil {
		t.Fatalf("decode models list: %v\n%s", err, listInputs.Stdout())
	}
	tts, ok := findModelSummary(listed.Results, models.BuiltInModelNameTTS)
	if !ok {
		t.Fatalf("models list did not include %q: %#v", models.BuiltInModelNameTTS, listed.Results)
	}
	if tts.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		tts.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("models list TTS runtime = %#v diagnostics=%v, want READY/INSTALLED", tts.ManagedRuntime, tts.ManagedRuntime.Diagnostics)
	}
	t.Logf("runtime proof command: you --json models list exitCode=0 model=tts readiness=%s lifecycle=%s", tts.ManagedRuntime.ReadinessState, tts.ManagedRuntime.LifecycleState)
}

func runGenericTTS(t *testing.T, story ttsStory, wantAudio []byte) {
	t.Helper()
	var genericStdout, genericStderr bytes.Buffer
	genericInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--input", "text=hello",
	})
	genericInputs.Input.Env = story.environment
	genericInputs.Input.WorkingDirectory = story.dir
	genericInputs.Input.Stdout = &genericStdout
	genericInputs.Input.Stderr = &genericStderr
	if err := story.process.Execute(genericInputs.Input); err != nil {
		t.Fatalf("Process.Execute(generic TTS stdout) error = %v", err)
	}
	if !bytes.Equal(genericStdout.Bytes(), wantAudio) {
		t.Fatalf("generic TTS stdout = %d bytes, want exact fixture audio %d bytes", genericStdout.Len(), len(wantAudio))
	}
	assertSemanticTTSAudio(t, genericStdout.Bytes(), "generic stdout")
	if genericStderr.String() != controlledTTSAssetEstimate {
		t.Fatalf("generic TTS stderr = %q, want controlled bundle estimate %q", genericStderr.String(), controlledTTSAssetEstimate)
	}
	t.Logf("runtime proof command: you models invoke tts --operation TTS --input text=hello")
	t.Logf("runtime proof exitCode=0 stdout=<raw audio bytes> mediaType=audio/wav size=%d sha256=%s stderr=%q", len(genericStdout.Bytes()), ttsDigest(genericStdout.Bytes()), genericStderr.String())
}

func runAliasTTS(t *testing.T, story ttsStory, wantAudio []byte) {
	t.Helper()
	aliasPath := filepath.Join(functionalTempDir(t), "alias.wav")
	var aliasStdout, aliasStderr bytes.Buffer
	aliasInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "hello", "--output", aliasPath,
	})
	aliasInputs.Input.Env = story.environment
	aliasInputs.Input.WorkingDirectory = story.dir
	aliasInputs.Input.Stdout = &aliasStdout
	aliasInputs.Input.Stderr = &aliasStderr
	if err := story.process.Execute(aliasInputs.Input); err != nil {
		t.Fatalf("Process.Execute(direct TTS alias) error = %v", err)
	}
	aliasAudio, err := os.ReadFile(aliasPath)
	if err != nil {
		t.Fatalf("read direct TTS alias output: %v", err)
	}
	if !bytes.Equal(aliasAudio, wantAudio) {
		t.Fatalf("direct TTS alias audio = %d bytes, want exact fixture audio %d bytes", len(aliasAudio), len(wantAudio))
	}
	assertSemanticTTSAudio(t, aliasAudio, "legacy output file")
	if aliasStdout.String() != "Wrote audio: "+aliasPath+"\n" || aliasStderr.Len() != 0 {
		t.Fatalf("direct TTS alias streams = stdout %q stderr %q, want status-only stdout and empty stderr", aliasStdout.String(), aliasStderr.String())
	}
	t.Logf("runtime proof command: you models invoke tts --operation TTS --text hello --output %s", aliasPath)
	t.Logf("runtime proof exitCode=0 stdout=%q stderr=%q output mediaType=audio/wav size=%d sha256=%s", aliasStdout.String(), aliasStderr.String(), len(aliasAudio), ttsDigest(aliasAudio))
}

func runExactTTSToASRChain(t *testing.T, story ttsStory) {
	t.Helper()
	const phrase = "Local AI works on this machine"
	audio, ttsPath := runExactChainTTS(t, story, phrase)

	transcriptPath := filepath.Join(story.dir, "exact-chain-transcript.txt")
	segmentsPath := filepath.Join(story.dir, "exact-chain-segments.json")
	var asrStdout, asrStderr bytes.Buffer
	asrInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", models.BuiltInModelNameASR, "--operation", "ASR", "--input", "audio=@" + ttsPath,
		"--output", "transcript=" + transcriptPath, "--output", "segments=" + segmentsPath,
	})
	asrInputs.Input.Env = story.environment
	asrInputs.Input.WorkingDirectory = story.dir
	asrInputs.Input.Stdout = &asrStdout
	asrInputs.Input.Stderr = &asrStderr
	if err := story.process.Execute(asrInputs.Input); err != nil {
		t.Fatalf("Process.Execute(exact-chain ASR) error = %v", err)
	}
	transcript, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("read exact-chain transcript: %v", err)
	}
	if string(transcript) != phrase {
		t.Fatalf("exact-chain transcript = %q, want semantic phrase %q", transcript, phrase)
	}
	segments, err := os.ReadFile(segmentsPath)
	if err != nil {
		t.Fatalf("read exact-chain segments: %v", err)
	}
	var decoded []struct {
		ID    int32  `json:"id"`
		Start int64  `json:"start"`
		End   int64  `json:"end"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(segments, &decoded); err != nil {
		t.Fatalf("decode exact-chain segments: %v", err)
	}
	if len(decoded) == 0 || decoded[0].Text != phrase {
		t.Fatalf("exact-chain segments = %#v, want one semantic segment", decoded)
	}
	duration := wavDurationMilliseconds(audio)
	var previousStart, previousEnd int64
	for index, segment := range decoded {
		if segment.ID < 0 || segment.Start < 0 || segment.End <= segment.Start || segment.End > int64(duration) ||
			(index > 0 && (segment.Start < previousStart || segment.End < previousEnd)) {
			t.Fatalf("exact-chain segment[%d] = %#v, want finite nonnegative monotonic timestamps within %.3fms", index, segment, duration)
		}
		previousStart, previousEnd = segment.Start, segment.End
	}
	asrCalls := story.protocol.ASRCalls()
	if len(asrCalls) == 0 || !bytes.Equal(asrCalls[len(asrCalls)-1].Audio, audio) || asrCalls[len(asrCalls)-1].MediaType != "audio/wav" {
		t.Fatalf("exact-chain ASR calls = %#v, want exact TTS bytes with audio/wav", asrCalls)
	}
	if story.network.Calls() != 0 {
		t.Fatalf("exact-chain cache reuse network calls = %d, want zero", story.network.Calls())
	}
	if entries, readErr := os.ReadDir(story.temp.directory); readErr != nil || len(entries) != 0 {
		t.Fatalf("exact-chain staging entries = %v, read error = %v; want no owned temporary files", entries, readErr)
	}
	t.Logf("runtime proof TTS->ASR command chain: tts text=%q output=%s; asr input=%s transcript=%s segments=%s", phrase, ttsPath, ttsPath, transcriptPath, segmentsPath)
	t.Logf("runtime proof exact-byte lineage ttsSHA256=%s asrSHA256=%s mediaType=audio/wav transcript=%q segmentBytes=%d durationMs=%.3f stdout=%q stderr=%q", ttsDigest(audio), ttsDigest(asrCalls[len(asrCalls)-1].Audio), phrase, len(segments), duration, asrStdout.String(), asrStderr.String())

	var jsonOutput, jsonStderr bytes.Buffer
	jsonInputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", models.BuiltInModelNameASR, "--operation", "ASR", "--input", "audio=@" + ttsPath,
	})
	jsonInputs.Input.Env = story.environment
	jsonInputs.Input.WorkingDirectory = story.dir
	jsonInputs.Input.Stdout = &jsonOutput
	jsonInputs.Input.Stderr = &jsonStderr
	if err := story.process.Execute(jsonInputs.Input); err != nil {
		t.Fatalf("Process.Execute(exact-chain ASR JSON) error = %v", err)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(jsonOutput.Bytes(), &response); err != nil {
		t.Fatalf("decode exact-chain ASR JSON: %v\n%s", err, jsonOutput.String())
	}
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "transcript" || response.Outputs[1].Name != "segments" ||
		response.Outputs[0].MediaType == nil || *response.Outputs[0].MediaType != "text/plain" ||
		response.Outputs[1].MediaType == nil || *response.Outputs[1].MediaType != "application/json" {
		t.Fatalf("exact-chain ASR JSON outputs = %#v, want transcript/text/plain then segments/application/json", response.Outputs)
	}
	t.Logf("runtime proof ASR JSON output identities transcript=text/plain segments=application/json stdout=%s stderr=%q", jsonOutput.String(), jsonStderr.String())
}

func runTTSFailureMatrix(t *testing.T, story ttsStory, wantAudio []byte) {
	t.Helper()

	story.protocol.FailNextDial()
	runTTSFailure(t, story, "startup unavailable", models.InvocationFailureClassBackendReadiness)
	runTTSFailure(t, story, "backend unavailable", models.InvocationFailureClassBackendReadiness)
	runTTSFailure(t, story, "protocol mismatch", models.InvocationFailureClassBackendProtocol)
	runTTSFailure(t, story, "malformed response", models.InvocationFailureClassMalformedResponse)
	runTTSFailure(t, story, "oversized response", models.InvocationFailureClassMalformedResponse)
	runTTSOutputWriteFailure(t, story)
	runTTSCancellation(t, story)
	runTTSRecovery(t, story, wantAudio)
}

func runTTSFailure(
	t *testing.T,
	story ttsStory,
	text string,
	wantClass models.InvocationFailureClass,
) {
	t.Helper()
	failurePath := filepath.Join(functionalTempDir(t), "failure.wav")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", text, "--output", failurePath,
	})
	inputs.Input.Env = story.environment
	inputs.Input.WorkingDirectory = story.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := story.process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf("Process.Execute(TTS %q) error = nil, want typed failure", text)
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != wantClass {
		t.Fatalf("Process.Execute(TTS %q) error = %v failure = %#v, want class %s", text, err, failure, wantClass)
	}
	if _, statErr := os.Stat(failurePath); !os.IsNotExist(statErr) {
		t.Fatalf("failed TTS %q output stat error = %v, want no partial file", text, statErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed TTS %q stdout = %q, want empty", text, stdout.String())
	}
	assertRedactedTTSFailure(t, err, stdout.String(), stderr.String(), failurePath, text)
	t.Logf("runtime proof command: you models invoke tts --operation TTS --text %q --output %s", text, failurePath)
	t.Logf("runtime proof exitCode=1 failureClass=%s stdout=%q stderr=%q outputExists=false", failure.Class, stdout.String(), stderr.String())
}

func runTTSOutputWriteFailure(t *testing.T, story ttsStory) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "output failure", "--output", story.outputFailurePath,
	})
	inputs.Input.Env = story.environment
	inputs.Input.WorkingDirectory = story.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := story.process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(TTS output failure) error = nil, want publication failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("TTS output failure stdout = %q, want empty", stdout.String())
	}
	if _, statErr := os.Stat(story.outputFailurePath); !os.IsNotExist(statErr) {
		t.Fatalf("TTS output failure target stat error = %v, want no published file", statErr)
	}
	assertRedactedTTSFailure(t, err, stdout.String(), stderr.String(), story.outputFailurePath, "output failure")
	t.Logf("runtime proof command: you models invoke tts --operation TTS --text %q --output %s", "output failure", story.outputFailurePath)
	t.Logf("runtime proof exitCode=1 stdout=%q stderr=%q outputExists=false error=%q", stdout.String(), stderr.String(), err.Error())
}

func runTTSCancellation(t *testing.T, story ttsStory) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	failurePath := filepath.Join(functionalTempDir(t), "cancelled.wav")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(ctx, []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "cancelled", "--output", failurePath,
	})
	inputs.Input.Env = story.environment
	inputs.Input.WorkingDirectory = story.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- story.process.Execute(inputs.Input) }()
	select {
	case <-story.protocol.CancellationStarted():
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the raw TTS edge to observe cancellation readiness")
	}
	err := <-done
	if !errors.Is(err, context.Canceled) && !errors.Is(err, models.ErrInferenceCancelled) {
		t.Fatalf("cancelled TTS error = %v, want context cancellation", err)
	}
	select {
	case <-story.protocol.CancellationObserved():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for raw TTS edge cancellation observation")
	}
	if _, statErr := os.Stat(failurePath); !os.IsNotExist(statErr) || stdout.Len() != 0 {
		t.Fatalf("cancelled TTS left output or stdout: output=%v stdout=%q", statErr, stdout.String())
	}
	assertRedactedTTSFailure(t, err, stdout.String(), stderr.String(), failurePath, "")
	t.Logf("runtime proof command: you models invoke tts --operation TTS --text cancelled --output %s", failurePath)
	t.Logf("runtime proof exitCode=1 cancellationObserved=true stdout=%q stderr=%q outputExists=false", stdout.String(), stderr.String())
}

func runTTSRecovery(t *testing.T, story ttsStory, wantAudio []byte) {
	t.Helper()
	recoveryPath := filepath.Join(functionalTempDir(t), "recovery.wav")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "tts", "--operation", "TTS", "--text", "after failure", "--output", recoveryPath,
	})
	inputs.Input.Env = story.environment
	inputs.Input.WorkingDirectory = story.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := story.process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(recovered direct TTS alias) error = %v", err)
	}
	recoveryAudio, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatalf("read recovered direct TTS output: %v", err)
	}
	if !bytes.Equal(recoveryAudio, wantAudio) {
		t.Fatalf("recovered direct TTS audio = %d bytes, want exact fixture audio %d bytes", len(recoveryAudio), len(wantAudio))
	}
	assertSemanticTTSAudio(t, recoveryAudio, "recovered output file")
	if stdout.String() != "Wrote audio: "+recoveryPath+"\n" || stderr.Len() != 0 {
		t.Fatalf("recovered direct TTS streams = stdout %q stderr %q, want status-only stdout and empty stderr", stdout.String(), stderr.String())
	}
	t.Logf("runtime proof command: you models invoke tts --operation TTS --text after-failure --output %s", recoveryPath)
	t.Logf("runtime proof exitCode=0 stdout=%q stderr=%q output mediaType=audio/wav size=%d sha256=%s", stdout.String(), stderr.String(), len(recoveryAudio), ttsDigest(recoveryAudio))
}

func assertTTSIsolationAndRelease(t *testing.T, story ttsStory) {
	t.Helper()
	if got := story.generic.Calls(); got != 0 {
		t.Fatalf("generic TTS backend calls = %d, want zero private-route fallback calls", got)
	}
	closeRootProcess(t, story.process, "close TTS root process for release proof")
	created, removed, duplicateRemoves := story.temp.Snapshot()
	if created == 0 || created != removed || duplicateRemoves != 0 {
		t.Fatalf("TTS staging release = created:%d removed:%d duplicateRemoves:%d, want one removal per staged path", created, removed, duplicateRemoves)
	}
	starts, stops, waits, active := story.host.Snapshot()
	if starts == 0 || starts != stops || stops != waits || active != 0 {
		t.Fatalf("TTS host release = starts:%d stops:%d waits:%d active:%d, want exactly-once stop/wait and no active host", starts, stops, waits, active)
	}
	dials, invokes, closes, doubleCloses := story.protocol.Snapshot()
	if invokes == 0 || invokes != closes || doubleCloses != 0 {
		t.Fatalf("TTS protocol release = dials:%d invokes:%d closes:%d doubleCloses:%d, want exactly one close per invoked connection", dials, invokes, closes, doubleCloses)
	}
	t.Logf("functional evidence platform=%s root=BuildProcess stateHome=%s cacheState=%s tempState=%s network=asset-rejected/raw-local-edge budgets={modelBytes:0,realCalls:0,retries:0} fixture=private-raw-protobuf method=/backend.Backend/TTS dials=%d invokes=%d closes=%d hostStarts=%d hostStops=%d hostWaits=%d tempCreated=%d tempRemoved=%d genericFallbackCalls=%d", runtime.GOOS, story.home, filepath.Join(story.home, ".agent-factory", "models"), story.temp.directory, dials, invokes, closes, starts, stops, waits, created, removed, story.generic.Calls())
}

func ttsDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func assertEquivalentTTSRequests(t *testing.T, requests []ttsProtocolCall, tempDirectory string) {
	t.Helper()
	if len(requests) != 2 {
		t.Fatalf("TTS requests = %d, want generic and alias", len(requests))
	}
	for index, request := range requests {
		if request.Method != "/backend.Backend/TTS" || request.Model != models.BuiltInModelNameTTS || request.Text != "hello" {
			t.Fatalf("raw TTS request[%d] = %#v, want pinned method/model/text", index, request)
		}
		if request.Destination == "" || filepath.Dir(request.Destination) != tempDirectory {
			t.Fatalf("raw TTS request[%d] destination = %q, want isolated temp directory %q", index, request.Destination, tempDirectory)
		}
	}
	if requests[0].Model != requests[1].Model || requests[0].Text != requests[1].Text || requests[0].Destination == requests[1].Destination {
		t.Fatalf("generic and alias TTS requests differ:\ngeneric=%#v\nalias=%#v", requests[0], requests[1])
	}
	t.Logf("request baseline generic_alias=equivalent method=%q model=%q text=%q destinations=isolated-and-unique", requests[0].Method, requests[0].Model, requests[0].Text)
}

type ttsProtocolCall struct {
	Method      string
	Endpoint    string
	Model       string
	Text        string
	Destination string
}

type asrInvocationCall struct {
	Audio     []byte
	MediaType string
}

type ttsPrivateProtocolFixture struct {
	mu                sync.Mutex
	audio             []byte
	calls             []ttsProtocolCall
	asrCalls          []asrInvocationCall
	dials             int
	invokes           int
	closes            int
	doubleCloses      int
	failNextDial      bool
	cancelStarted     chan struct{}
	cancelObserved    chan struct{}
	cancelStartOnce   sync.Once
	cancelObserveOnce sync.Once
}

func newTTSPrivateProtocolFixture(audio []byte) *ttsPrivateProtocolFixture {
	return &ttsPrivateProtocolFixture{
		audio:          append([]byte(nil), audio...),
		cancelStarted:  make(chan struct{}),
		cancelObserved: make(chan struct{}),
	}
}

func (fixture *ttsPrivateProtocolFixture) FailNextDial() {
	fixture.mu.Lock()
	fixture.failNextDial = true
	fixture.mu.Unlock()
}

func (fixture *ttsPrivateProtocolFixture) Dial(_ context.Context, endpoint string) (platformgrpc.Connection, error) {
	fixture.mu.Lock()
	fixture.dials++
	fail := fixture.failNextDial
	fixture.failNextDial = false
	fixture.mu.Unlock()
	if fail {
		return nil, status.Error(codes.Unavailable, "fixture-secret endpoint unavailable")
	}
	return &ttsPrivateProtocolConnection{fixture: fixture, endpoint: endpoint}, nil
}

func (fixture *ttsPrivateProtocolFixture) recordCall(call ttsProtocolCall) {
	fixture.mu.Lock()
	fixture.calls = append(fixture.calls, call)
	fixture.invokes++
	fixture.mu.Unlock()
}

func (fixture *ttsPrivateProtocolFixture) Calls() []ttsProtocolCall {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return append([]ttsProtocolCall(nil), fixture.calls...)
}

func (fixture *ttsPrivateProtocolFixture) ASRBackend(ctx context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.ASRBackendResponse{}, err
	}
	fixture.mu.Lock()
	fixture.asrCalls = append(fixture.asrCalls, asrInvocationCall{
		Audio: append([]byte(nil), request.Audio...), MediaType: request.MediaType,
	})
	fixture.mu.Unlock()
	if !bytes.Equal(request.Audio, fixture.audioFor("Local AI works on this machine")) {
		return models.ASRBackendResponse{}, errors.New("fixture-secret ASR received unexpected audio")
	}
	return models.ASRBackendResponse{
		Text:     "Local AI works on this machine",
		Segments: []models.ASRBackendSegment{{ID: 0, Start: 0, End: 10, Text: "Local AI works on this machine"}},
	}, nil
}

func (fixture *ttsPrivateProtocolFixture) ASRCalls() []asrInvocationCall {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	result := make([]asrInvocationCall, len(fixture.asrCalls))
	for index, call := range fixture.asrCalls {
		result[index] = asrInvocationCall{Audio: append([]byte(nil), call.Audio...), MediaType: call.MediaType}
	}
	return result
}

func (fixture *ttsPrivateProtocolFixture) CancellationStarted() <-chan struct{} {
	return fixture.cancelStarted
}

func (fixture *ttsPrivateProtocolFixture) CancellationObserved() <-chan struct{} {
	return fixture.cancelObserved
}

func (fixture *ttsPrivateProtocolFixture) Snapshot() (dials, invokes, closes, doubleCloses int) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.dials, fixture.invokes, fixture.closes, fixture.doubleCloses
}

type ttsPrivateProtocolConnection struct {
	fixture  *ttsPrivateProtocolFixture
	endpoint string
	closed   bool
}

func (connection *ttsPrivateProtocolConnection) Invoke(
	ctx context.Context,
	method string,
	payload []byte,
) ([]byte, error) {
	if method != "/backend.Backend/TTS" {
		return nil, status.Error(codes.Unimplemented, "fixture-secret unexpected method")
	}
	request := &localaiproto.TTSRequest{}
	if err := proto.Unmarshal(payload, request); err != nil {
		return nil, status.Error(codes.InvalidArgument, "fixture-secret malformed request")
	}
	connection.fixture.recordCall(ttsProtocolCall{
		Method: method, Endpoint: connection.endpoint, Model: request.GetModel(),
		Text: request.GetText(), Destination: request.GetDst(),
	})
	switch strings.TrimSpace(request.GetText()) {
	case "backend unavailable":
		return nil, status.Error(codes.Unavailable, "fixture-secret prompt=backend unavailable")
	case "protocol mismatch":
		return nil, status.Error(codes.FailedPrecondition, "fixture-secret protocol mismatch")
	case "malformed response":
		return []byte{0xff, 0x00, 0x7f}, nil
	case "oversized response":
		audio := make([]byte, (16<<20)+1)
		if err := os.WriteFile(request.GetDst(), audio, 0o600); err != nil {
			return nil, fmt.Errorf("fixture-secret output write failed: %w", err)
		}
		return proto.Marshal(&localaiproto.Result{Success: true})
	case "cancelled":
		connection.fixture.cancelStartOnce.Do(func() { close(connection.fixture.cancelStarted) })
		select {
		case <-ctx.Done():
			connection.fixture.cancelObserveOnce.Do(func() { close(connection.fixture.cancelObserved) })
			return nil, ctx.Err()
		}
	}
	audio := connection.fixture.audioFor(request.GetText())
	if err := os.WriteFile(request.GetDst(), audio, 0o600); err != nil {
		return nil, fmt.Errorf("fixture-secret output write failed: %w", err)
	}
	return proto.Marshal(&localaiproto.Result{Message: "fixture audio written", Success: true})
}

func (connection *ttsPrivateProtocolConnection) Close() error {
	connection.fixture.mu.Lock()
	connection.fixture.closes++
	if connection.closed {
		connection.fixture.doubleCloses++
	}
	connection.closed = true
	connection.fixture.mu.Unlock()
	return nil
}

func (fixture *ttsPrivateProtocolFixture) audioFor(text string) []byte {
	audio := append([]byte(nil), fixture.audio...)
	if len(audio) >= 46 {
		switch text {
		case "isolated-a":
			audio[44], audio[45] = 0x11, 0x22
		case "isolated-b":
			audio[44], audio[45] = 0x33, 0x44
		}
	}
	return audio
}

type ttsGenericBackendTrap struct {
	mu    sync.Mutex
	calls int
}

func (trap *ttsGenericBackendTrap) Invoke(
	context.Context,
	models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	trap.mu.Lock()
	trap.calls++
	trap.mu.Unlock()
	return nil, nil, errors.New("generic TTS fallback trap invoked")
}

func (trap *ttsGenericBackendTrap) Calls() int {
	trap.mu.Lock()
	defer trap.mu.Unlock()
	return trap.calls
}

type ttsTempEffects struct {
	directory  string
	mu         sync.Mutex
	created    map[string]struct{}
	removed    []string
	duplicates int
}

func (effects *ttsTempEffects) CreateTemp(
	directory string,
	pattern string,
) (interface {
	Close() error
	Name() string
}, error) {
	temporary, err := os.CreateTemp(directory, pattern)
	if err == nil && strings.Contains(pattern, ".you-model-tts-") {
		effects.mu.Lock()
		if effects.created == nil {
			effects.created = make(map[string]struct{})
		}
		effects.created[temporary.Name()] = struct{}{}
		effects.mu.Unlock()
	}
	return temporary, err
}

func (effects *ttsTempEffects) Remove(path string) error {
	effects.mu.Lock()
	if _, ok := effects.created[path]; ok {
		for _, removed := range effects.removed {
			if removed == path {
				effects.duplicates++
				break
			}
		}
		effects.removed = append(effects.removed, path)
	}
	effects.mu.Unlock()
	return os.Remove(path)
}

func (effects *ttsTempEffects) Snapshot() (created, removed, duplicates int) {
	effects.mu.Lock()
	defer effects.mu.Unlock()
	return len(effects.created), len(effects.removed), effects.duplicates
}

type ttsHostLauncher struct {
	mu       sync.Mutex
	endpoint string
	starts   int
	stops    int
	waits    int
	active   int
}

func (launcher *ttsHostLauncher) Start(
	context.Context,
	serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	launcher.starts++
	launcher.active++
	endpoint := launcher.endpoint
	launcher.mu.Unlock()
	return &ttsHostProcess{launcher: launcher, endpoint: endpoint, stopped: make(chan struct{})}, nil
}

func (launcher *ttsHostLauncher) Snapshot() (starts, stops, waits, active int) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts, launcher.stops, launcher.waits, launcher.active
}

type ttsHostProcess struct {
	launcher *ttsHostLauncher
	endpoint string
	stopped  chan struct{}
	once     sync.Once
}

func (process *ttsHostProcess) HealthEndpoint() string { return process.endpoint }

func (process *ttsHostProcess) Wait() error {
	process.launcher.mu.Lock()
	process.launcher.waits++
	process.launcher.mu.Unlock()
	<-process.stopped
	return nil
}

func (process *ttsHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		process.launcher.mu.Lock()
		process.launcher.stops++
		process.launcher.active--
		process.launcher.mu.Unlock()
		close(process.stopped)
	})
	return nil
}

func assertRedactedTTSFailure(
	t *testing.T,
	err error,
	stdout string,
	stderr string,
	destination string,
	prompt string,
) {
	t.Helper()
	haystack := strings.Join([]string{err.Error(), stdout, stderr}, "\n")
	for _, secret := range []string{"fixture-secret", "prompt=", "raw-audio"} {
		if strings.Contains(haystack, secret) {
			t.Fatalf("TTS failure leaked %q: %q", secret, haystack)
		}
	}
	if destination != "" && strings.Contains(haystack, destination) {
		t.Fatalf("TTS failure leaked staged/output path %q: %q", destination, haystack)
	}
	if prompt != "" && strings.Contains(err.Error(), prompt) {
		t.Fatalf("TTS failure leaked prompt %q: %q", prompt, err.Error())
	}
}

func assertSemanticTTSAudio(t *testing.T, audio []byte, label string) {
	t.Helper()
	const (
		wavHeaderSize  = 44
		pcmFormat      = 1
		monoChannels   = 1
		bitsPerSample  = 16
		minimumSamples = 1
	)
	if len(audio) < wavHeaderSize {
		t.Fatalf("%s audio length = %d, want at least %d-byte WAV header", label, len(audio), wavHeaderSize)
	}
	if string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[12:16]) != "fmt " || string(audio[36:40]) != "data" {
		t.Fatalf("%s audio has invalid RIFF/WAVE PCM chunk markers", label)
	}
	if got := uint64(binary.LittleEndian.Uint32(audio[4:8])) + 8; got != uint64(len(audio)) {
		t.Fatalf("%s RIFF size = %d, want payload length %d", label, got, len(audio))
	}
	if got := binary.LittleEndian.Uint32(audio[16:20]); got != 16 {
		t.Fatalf("%s fmt chunk size = %d, want PCM fmt size 16", label, got)
	}
	if got := binary.LittleEndian.Uint16(audio[20:22]); got != pcmFormat {
		t.Fatalf("%s audio format = %d, want PCM format %d", label, got, pcmFormat)
	}
	channels := binary.LittleEndian.Uint16(audio[22:24])
	if channels != monoChannels {
		t.Fatalf("%s channels = %d, want %d", label, channels, monoChannels)
	}
	sampleRate := binary.LittleEndian.Uint32(audio[24:28])
	byteRate := binary.LittleEndian.Uint32(audio[28:32])
	blockAlign := binary.LittleEndian.Uint16(audio[32:34])
	bits := binary.LittleEndian.Uint16(audio[34:36])
	if sampleRate == 0 || blockAlign == 0 || bits != bitsPerSample {
		t.Fatalf("%s PCM format = sampleRate:%d blockAlign:%d bits:%d, want nonzero/16-bit PCM", label, sampleRate, blockAlign, bits)
	}
	if want := sampleRate * uint32(blockAlign); byteRate != want {
		t.Fatalf("%s byte rate = %d, want %d from sample rate and block alignment", label, byteRate, want)
	}
	dataSize := binary.LittleEndian.Uint32(audio[40:44])
	if uint64(dataSize)+wavHeaderSize != uint64(len(audio)) || dataSize < minimumSamples*uint32(blockAlign) || dataSize%uint32(blockAlign) != 0 {
		t.Fatalf("%s data chunk size = %d, want aligned nonempty payload for %d-byte samples", label, dataSize, blockAlign)
	}
	duration := time.Second * time.Duration(dataSize/uint32(blockAlign)) / time.Duration(sampleRate)
	if duration <= 0 {
		t.Fatalf("%s duration = %s, want nonzero duration", label, duration)
	}
	t.Logf("semantic audio baseline label=%s mediaType=audio/wav codec=PCM channels=%d sampleRate=%d bits=%d bytes=%d sha256=%s duration=%s", label, channels, sampleRate, bits, len(audio), ttsDigest(audio), duration)
}

func jsonUnmarshalFunctional(data string, target any) error {
	return json.Unmarshal([]byte(data), target)
}
