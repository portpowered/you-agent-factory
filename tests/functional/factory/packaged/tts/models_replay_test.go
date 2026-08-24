package tts

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestDeliveredPackagedTTSFactoryReachesProtocolFixture proves the shipped
// cmd/factory artifact can execute the installed packaged @you/tts Factory
// through the same external model protocol fixture used by the delivered
// Models tests. The fixture stands in for the VibeVoice backend; it does not
// download or claim to run the real VibeVoice runtime.
func TestDeliveredPackagedTTSFactoryReachesProtocolFixture(t *testing.T) {
	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedTTSFactoryName)
	cacheDir := t.TempDir()
	writePackagedTTSReadyModelCache(t, cacheDir)
	fixture := newDeliveredFactoryTTSProtocolFixture(t)
	binaryPath := buildDeliveredFactoryTTSBinary(t)
	text := "delivered packaged TTS protocol fixture"

	result := runDeliveredFactoryTTSCLI(t, binaryPath, homeDir, cacheDir, fixture.URL(), text)
	t.Logf("runtime proof command: %s", result.command)
	t.Logf("runtime proof exitCode=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	if result.exitCode != 0 {
		t.Fatalf("delivered packaged TTS exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	response := support.DecodeInvocationResponseJSON(t, result.stdout)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("delivered packaged TTS status=%q response=%#v", response.Status, response)
	}
	audio := deliveredFactoryTTSPrimaryAudio(t, response.PrimaryResult)
	if string(audio) != packagedTTSFakeAudioFixture {
		t.Fatalf("delivered packaged TTS audio=%q, want fixture bytes %q", audio, packagedTTSFakeAudioFixture)
	}
	request := fixture.LastRequest(t)
	if request.Operation != "TTS" || request.ModelName != factorydefinitions.DefaultTTSModelName || len(request.Inputs) != 1 || request.Inputs[0].Name != "text" {
		t.Fatalf("protocol fixture request=%#v, want TTS/%s with one text input", request, factorydefinitions.DefaultTTSModelName)
	}
	inputText, err := base64.StdEncoding.DecodeString(request.Inputs[0].ContentBase64)
	if err != nil {
		t.Fatalf("decode protocol fixture input: %v", err)
	}
	if string(inputText) != text {
		t.Fatalf("protocol fixture request=%#v, want TTS/%s with exact text input %q", request, factorydefinitions.DefaultTTSModelName, text)
	}
	if fixture.CallCount() != 1 {
		t.Fatalf("protocol fixture calls=%d, want one delivered Factory TTS call", fixture.CallCount())
	}
}

func deliveredFactoryTTSPrimaryAudio(t *testing.T, content *factoryapi.WorkContent) []byte {
	t.Helper()
	if content == nil || len(*content) != 1 {
		t.Fatalf("delivered packaged TTS primary result = %#v, want one AUDIO part", content)
	}
	audio, err := (*content)[0].AsWorkAudioContentPart()
	if err != nil || !strings.HasPrefix(audio.Url, "data:audio/wav;base64,") {
		t.Fatalf("delivered packaged TTS primary part = %#v, want audio/wav data URL", (*content)[0])
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(audio.Url, "data:audio/wav;base64,"))
	if err != nil {
		t.Fatalf("decode delivered packaged TTS primary audio: %v", err)
	}
	return decoded
}

// TestFactoryTTSModelsRootBuildProcessExecuteRecordsAudio keeps the
// root-process contract visible beside the HTTP and delivered-binary proofs:
// the packaged Factory is invoked with Process.Execute, while only the exact
// Models backend effect is replaced through edges.Edges.
func TestFactoryTTSModelsRootBuildProcessExecuteRecordsAudio(t *testing.T) {
	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedTTSFactoryName)
	cacheDir := t.TempDir()
	writePackagedTTSReadyModelCache(t, cacheDir)
	backend := newPackagedTTSModelsBackend([]byte(packagedTTSFakeAudioFixture))
	edges, closeHost := managedTTSModelEdges(t, backend)
	t.Cleanup(closeHost)
	artifactPath := filepath.Join(t.TempDir(), "root-process-managed-tts.replay.json")
	text := "root process packaged TTS fixture"
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--named", factorydefinitions.PackagedTTSFactoryName,
		"--record", artifactPath, "--output", "primary", "--to", text,
	})
	inputs.Input.Env = append(os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		run.ModelCacheDirEnvironment+"="+cacheDir,
	)
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("root Process.Execute(packaged TTS) error=%v stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("root Process.Execute status=%q response=%#v", response.Status, response)
	}
	if audio := deliveredFactoryTTSPrimaryAudio(t, response.PrimaryResult); string(audio) != packagedTTSFakeAudioFixture {
		t.Fatalf("root Process.Execute audio=%q, want fixture %q", audio, packagedTTSFakeAudioFixture)
	}
	if backend.CallCount() != 1 {
		t.Fatalf("root Process.Execute Models backend calls=%d, want one", backend.CallCount())
	}
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read root Process.Execute recording: %v", err)
	}
	var artifact struct {
		Events []factoryapi.FactoryEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode root Process.Execute recording: %v", err)
	}
	assertManagedFactoryTTSEventOrder(t, artifact.Events, "root Process.Execute")
	observed := collectFactoryTTSDispatchEvents(t, artifact.Events, factorysessions.DefaultSessionID)
	work := managedFactoryTTSWorkRequestWork(t, observed.workRequest, text)
	requestID := factoryTTSRequiredContextID(t, observed.workRequest, "request")
	traceID := factoryTTSRequiredTraceID(t, observed.workRequest)
	dispatchID := factoryTTSRequiredContextID(t, observed.dispatchRequest, "dispatch")
	assertFactoryTTSContextCorrelation(t, observed, *work.WorkId, requestID, traceID, dispatchID)
	assertManagedFactoryTTSAssociation(t, observed.association, dispatchID, "root Process.Execute")
	responsePayload, err := observed.dispatchResponse.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode root Process.Execute DISPATCH_RESPONSE: %v", err)
	}
	if responsePayload.OutputWork == nil || len(*responsePayload.OutputWork) != 1 {
		t.Fatalf("root Process.Execute DISPATCH_RESPONSE outputWork=%#v, want one AUDIO Work", responsePayload.OutputWork)
	}
	if audio := managedFactoryTTSAudioPart(t, (*responsePayload.OutputWork)[0]); audio.ContentType == nil || *audio.ContentType != "audio/wav" {
		t.Fatalf("root Process.Execute event AUDIO=%#v, want audio/wav", audio)
	}
}

type deliveredFactoryTTSCLIResult struct {
	command  string
	exitCode int
	stdout   string
	stderr   string
}

func buildDeliveredFactoryTTSBinary(t *testing.T) string {
	t.Helper()
	name := "you"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), name)
	command := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binaryPath, "./cmd/factory")
	command.Dir = testutil.MustRepoRoot(t)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("build delivered you artifact: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
	t.Logf("runtime proof build: go build -buildvcs=false -o %s ./cmd/factory; exitCode=0 stdout=%q stderr=%q", binaryPath, stdout.String(), stderr.String())
	return binaryPath
}

func runDeliveredFactoryTTSCLI(
	t *testing.T,
	binaryPath, homeDir, cacheDir, endpoint, text string,
) deliveredFactoryTTSCLIResult {
	t.Helper()
	args := []string{
		"--json", "run", "--named", factorydefinitions.PackagedTTSFactoryName,
		"--no-record", "--output", "primary", "--to", text,
	}
	command := exec.CommandContext(t.Context(), binaryPath, args...)
	command.Dir = homeDir
	command.Env = deliveredFactoryTTSEnvironment(homeDir, cacheDir, endpoint)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := deliveredFactoryTTSCLIResult{
		command:  strings.Join(append([]string{binaryPath}, args...), " "),
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: 0,
	}
	if err == nil {
		return result
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("run delivered packaged TTS artifact: %v", err)
	}
	result.exitCode = exitError.ExitCode()
	return result
}

func deliveredFactoryTTSEnvironment(homeDir, cacheDir, endpoint string) []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "HOME=") || strings.HasPrefix(value, "USERPROFILE=") ||
			strings.HasPrefix(value, run.ModelCacheDirEnvironment+"=") || strings.HasPrefix(value, "YOU_MODELS_BACKEND_ENDPOINT=") {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment,
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		run.ModelCacheDirEnvironment+"="+cacheDir,
		"YOU_MODELS_BACKEND_ENDPOINT="+endpoint,
	)
}

type deliveredFactoryTTSProtocolFixture struct {
	server  *httptest.Server
	mu      sync.Mutex
	calls   int
	request deliveredFactoryTTSProtocolRequest
}

type deliveredFactoryTTSProtocolRequest struct {
	ModelName string                             `json:"modelName"`
	Operation string                             `json:"operation"`
	Inputs    []deliveredFactoryTTSProtocolInput `json:"inputs"`
}

type deliveredFactoryTTSProtocolInput struct {
	Name          string `json:"name"`
	Modality      string `json:"modality"`
	ContentType   string `json:"contentType"`
	MediaType     string `json:"mediaType"`
	ContentBase64 string `json:"contentBase64"`
}

type deliveredFactoryTTSProtocolOutput struct {
	Name          string `json:"name"`
	Modality      string `json:"modality"`
	ContentType   string `json:"contentType"`
	MediaType     string `json:"mediaType"`
	ContentBase64 string `json:"contentBase64"`
}

func newDeliveredFactoryTTSProtocolFixture(t *testing.T) *deliveredFactoryTTSProtocolFixture {
	t.Helper()
	fixture := &deliveredFactoryTTSProtocolFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/health":
			writer.WriteHeader(http.StatusOK)
		case "/invoke":
			var payload deliveredFactoryTTSProtocolRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			for index := range payload.Inputs {
				decoded, err := base64.StdEncoding.DecodeString(payload.Inputs[index].ContentBase64)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusBadRequest)
					return
				}
				payload.Inputs[index].ContentBase64 = base64.StdEncoding.EncodeToString(decoded)
			}
			fixture.mu.Lock()
			fixture.calls++
			fixture.request = payload
			fixture.mu.Unlock()
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(struct {
				Outputs []deliveredFactoryTTSProtocolOutput `json:"outputs"`
			}{Outputs: []deliveredFactoryTTSProtocolOutput{{
				Name: "audio", Modality: "AUDIO", ContentType: "audio/wav", MediaType: "audio/wav",
				ContentBase64: base64.StdEncoding.EncodeToString([]byte(packagedTTSFakeAudioFixture)),
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *deliveredFactoryTTSProtocolFixture) URL() string { return fixture.server.URL }

func (fixture *deliveredFactoryTTSProtocolFixture) CallCount() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.calls
}

func (fixture *deliveredFactoryTTSProtocolFixture) LastRequest(t testing.TB) deliveredFactoryTTSProtocolRequest {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.calls == 0 {
		t.Fatal("delivered Factory TTS protocol fixture was not called")
	}
	request := fixture.request
	request.Inputs = append([]deliveredFactoryTTSProtocolInput(nil), fixture.request.Inputs...)
	return request
}

// TestFactoryTTSModelsSuccessAndFailureReplayPreservePublicProjections proves
// the canonical packaged worker reaches Models through the root process. The
// only backend effect replaced by the fixture is ModelInvocationBackend; the
// live Factory history is then replayed without another backend call.
func TestFactoryTTSModelsSuccessAndFailureReplayPreservePublicProjections(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		text := "models-backed factory tts success"
		backend := newPackagedTTSModelsBackend([]byte(packagedTTSFakeAudioFixture))
		artifact, err := (models.InferenceArtifactRef{}).Parse("models-inference:artifact:tts-success")
		if err != nil {
			t.Fatalf("parse TTS artifact: %v", err)
		}
		backend.SetArtifacts([]models.InferenceArtifact{{
			Artifact: artifact, Name: "audio", MediaType: "audio/wav",
			SizeBytes:  int64(len(packagedTTSFakeAudioFixture)),
			Properties: map[string]string{"fixture": "packaged-tts", "label": "speech.wav"},
		}})
		live := runManagedFactoryTTSRecording(t, text, backend, nil)

		if backend.CallCount() != 1 {
			t.Fatalf("Models backend calls after live success = %d, want one", backend.CallCount())
		}
		liveWork := factoryTTSCompletedWork(t, live.work)
		liveAudio := managedFactoryTTSAudioPart(t, liveWork)
		if liveAudio.ContentType == nil || *liveAudio.ContentType != "audio/wav" {
			t.Fatalf("live AUDIO content type = %#v, want audio/wav", liveAudio.ContentType)
		}
		if strings.TrimSpace(liveAudio.Url) == "" && (liveAudio.File == nil || strings.TrimSpace(*liveAudio.File) == "") {
			t.Fatalf("live AUDIO = %#v, want a materialized content reference", liveAudio)
		}
		assertManagedFactoryTTSAudioDigest(t, liveAudio)

		replayed := replayManagedFactoryTTSRecording(t, live.factoryDir, live.homeDir, live.cacheDir, live.artifactPath, backend, true)
		if backend.CallCount() != 1 {
			t.Fatalf("Models backend calls after success replay = %d, want no replay call", backend.CallCount())
		}
		replayedWork := factoryTTSCompletedWork(t, replayed.work)
		assertFactoryTTSWorkEquivalent(t, liveWork, replayedWork, "successful replay Work")
		assertManagedFactoryTTSAudioArtifactLineage(t, live, replayed)
		assertManagedFactoryTTSReplayEvents(t, live, replayed, text, true)
	})

	t.Run("failure", func(t *testing.T) {
		text := "models-backed factory tts failure"
		backend := newPackagedTTSModelsBackend(nil)
		live := runManagedFactoryTTSRecording(t, text, backend, errors.New("fixture TTS backend failure"))

		if backend.CallCount() != 1 {
			t.Fatalf("Models backend calls after live failure = %d, want one", backend.CallCount())
		}
		liveWork := factoryTTSFailedWork(t, live.work)
		assertManagedFactoryTTSFailedWork(t, liveWork, text, "live failed Work")
		assertManagedFactoryTTSFailureModelEvents(
			t,
			collectFactoryTTSDispatchEvents(t, live.events, factorysessions.DefaultSessionID),
			"fixture TTS backend failure",
		)
		assertManagedFactoryTTSFailureDispatchResponse(
			t,
			collectFactoryTTSDispatchEvents(t, live.events, factorysessions.DefaultSessionID).dispatchResponse,
			text,
			"live failure route",
		)
		assertManagedFactoryTTSNoArtifactEvents(t, live.events, "live failure")

		replayed := replayManagedFactoryTTSRecording(t, live.factoryDir, live.homeDir, live.cacheDir, live.artifactPath, backend, false)
		if backend.CallCount() != 1 {
			t.Fatalf("Models backend calls after failure replay = %d, want no replay call", backend.CallCount())
		}
		replayedWork := factoryTTSFailedWork(t, replayed.work)
		assertManagedFactoryTTSFailedWork(t, replayedWork, text, "replayed failed Work")
		replayedEvents := collectFactoryTTSDispatchEvents(t, replayed.events, factorysessions.DefaultSessionID)
		assertManagedFactoryTTSFailureDispatchResponse(t, replayedEvents.dispatchResponse, text, "replayed failure route")
		assertManagedFactoryTTSNoArtifactEvents(t, replayed.events, "replayed failure")
		assertFactoryTTSWorkEquivalent(t, liveWork, replayedWork, "failed replay Work")
		assertManagedFactoryTTSReplayEvents(t, live, replayed, text, false)
	})
}

func assertManagedFactoryTTSAudioDigest(t *testing.T, audio factoryapi.WorkAudioContentPart) {
	t.Helper()
	if !strings.HasPrefix(audio.Url, "data:audio/wav;base64,") {
		t.Fatalf("managed TTS AUDIO URL = %q, want data:audio/wav;base64 reference", audio.Url)
	}
	encoded := strings.TrimPrefix(audio.Url, "data:audio/wav;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode managed TTS AUDIO URL: %v", err)
	}
	wantDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(packagedTTSFakeAudioFixture)))
	gotDigest := fmt.Sprintf("%x", sha256.Sum256(decoded))
	if gotDigest != wantDigest || string(decoded) != packagedTTSFakeAudioFixture {
		t.Fatalf("managed TTS AUDIO bytes digest = %s, want %s; bytes = %q", gotDigest, wantDigest, decoded)
	}
}

func assertManagedFactoryTTSAudioArtifactLineage(
	t *testing.T,
	live, replay managedFactoryTTSRecording,
) {
	t.Helper()
	liveAudio := managedFactoryTTSAudioPart(t, factoryTTSCompletedWork(t, live.work))
	replayAudio := managedFactoryTTSAudioPart(t, factoryTTSCompletedWork(t, replay.work))
	if liveAudio.ArtifactId == nil || strings.TrimSpace(*liveAudio.ArtifactId) == "" {
		t.Fatalf("live AUDIO artifactId = %#v, want non-empty Models artifact reference", liveAudio.ArtifactId)
	}
	if replayAudio.ArtifactId == nil || *replayAudio.ArtifactId != *liveAudio.ArtifactId {
		t.Fatalf("replayed AUDIO artifactId = %#v, want live reference %q", replayAudio.ArtifactId, *liveAudio.ArtifactId)
	}
	if liveAudio.Metadata == nil || (*liveAudio.Metadata)["label"] != "speech.wav" || replayAudio.Metadata == nil || (*replayAudio.Metadata)["label"] != (*liveAudio.Metadata)["label"] {
		t.Fatalf("AUDIO artifact metadata = live:%#v replay:%#v, want label parity", liveAudio.Metadata, replayAudio.Metadata)
	}
	assertManagedFactoryTTSArtifactEventLineage(t, live.events, *liveAudio.ArtifactId, "live")
	assertManagedFactoryTTSArtifactEventLineage(t, replay.events, *liveAudio.ArtifactId, "replay")
}

func assertManagedFactoryTTSArtifactEventLineage(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantArtifactID, label string,
) {
	t.Helper()
	observed := collectFactoryTTSDispatchEvents(t, events, factorysessions.DefaultSessionID)
	payload, err := observed.dispatchResponse.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode %s DISPATCH_RESPONSE: %v", label, err)
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("%s DISPATCH_RESPONSE outputWork = %#v, want one audio Work", label, payload.OutputWork)
	}
	audio := managedFactoryTTSAudioPart(t, (*payload.OutputWork)[0])
	if audio.ArtifactId == nil || *audio.ArtifactId != wantArtifactID {
		t.Fatalf("%s DISPATCH_RESPONSE artifactId = %#v, want %q", label, audio.ArtifactId, wantArtifactID)
	}
	modelResponse, err := observed.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode %s MODEL_RESPONSE: %v", label, err)
	}
	if modelResponse.OutputContent == nil || len(*modelResponse.OutputContent) != 1 {
		t.Fatalf("%s MODEL_RESPONSE outputContent = %#v, want one audio part", label, modelResponse.OutputContent)
	}
	modelAudio, err := (*modelResponse.OutputContent)[0].AsWorkAudioContentPart()
	if err != nil || modelAudio.ArtifactId == nil || *modelAudio.ArtifactId != wantArtifactID {
		t.Fatalf("%s MODEL_RESPONSE artifactId = %#v, want %q", label, modelAudio, wantArtifactID)
	}
}

func assertManagedFactoryTTSReplayEvents(
	t *testing.T,
	live, replay managedFactoryTTSRecording,
	wantText string,
	wantSuccess bool,
) {
	t.Helper()
	liveEvents := collectFactoryTTSDispatchEvents(t, live.events, factorysessions.DefaultSessionID)
	replayEvents := collectFactoryTTSDispatchEvents(t, replay.events, factorysessions.DefaultSessionID)
	assertManagedFactoryTTSEventOrder(t, live.events, "live")
	assertManagedFactoryTTSEventOrder(t, replay.events, "replay")
	assertFactoryTTSEventProjectionEquivalent(t, live.events, replay.events, "managed TTS replay events")

	liveWorkID := managedFactoryTTSWorkRequestWork(t, liveEvents.workRequest, wantText).WorkId
	if liveWorkID == nil || strings.TrimSpace(*liveWorkID) == "" {
		t.Fatalf("live WORK_REQUEST Work ID = %#v, want non-empty identity", liveWorkID)
	}
	requestID := factoryTTSRequiredContextID(t, liveEvents.workRequest, "request")
	traceID := factoryTTSRequiredTraceID(t, liveEvents.workRequest)
	dispatchID := factoryTTSRequiredContextID(t, liveEvents.dispatchRequest, "dispatch")
	assertFactoryTTSContextCorrelation(t, liveEvents, *liveWorkID, requestID, traceID, dispatchID)
	assertFactoryTTSContextCorrelation(t, replayEvents, *liveWorkID, requestID, traceID, dispatchID)
	assertManagedFactoryTTSAssociation(t, liveEvents.association, dispatchID, "live")
	assertManagedFactoryTTSAssociation(t, replayEvents.association, dispatchID, "replay")

	liveWork := managedFactoryTTSWorkRequestWork(t, liveEvents.workRequest, wantText)
	replayWork := managedFactoryTTSWorkRequestWork(t, replayEvents.workRequest, wantText)
	assertManagedFactoryTTSJSONEqual(t, liveWork, replayWork, "WORK_REQUEST work")

	liveDispatch, err := liveEvents.dispatchRequest.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode live DISPATCH_REQUEST %q: %v", liveEvents.dispatchRequest.Id, err)
	}
	replayDispatch, err := replayEvents.dispatchRequest.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode replay DISPATCH_REQUEST %q: %v", replayEvents.dispatchRequest.Id, err)
	}
	assertManagedFactoryTTSJSONEqual(t, liveDispatch, replayDispatch, "DISPATCH_REQUEST payload")

	assertManagedFactoryTTSModelProjection(t, liveEvents, replayEvents, wantSuccess)
	assertManagedFactoryTTSDispatchProjection(t, liveEvents.dispatchResponse, replayEvents.dispatchResponse, wantSuccess)
}

func assertManagedFactoryTTSEventOrder(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	label string,
) {
	t.Helper()
	want := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
		factoryapi.FactoryEventTypeModelRequest,
		factoryapi.FactoryEventTypeModelResponse,
		factoryapi.FactoryEventTypeDispatchResponse,
	}
	position := 0
	seen := make(map[factoryapi.FactoryEventType]int, len(want))
	for index, event := range events {
		for position < len(want) && event.Type != want[position] {
			position++
		}
		if position == len(want) {
			break
		}
		seen[event.Type]++
		if seen[event.Type] > 1 {
			t.Fatalf("%s event order contains duplicate %s at index %d", label, event.Type, index)
		}
		position++
	}
	if position != len(want) {
		t.Fatalf("%s event order missing %s; events = %#v", label, want[position], eventTypes(events))
	}
}

func eventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func managedFactoryTTSWorkRequestWork(
	t *testing.T,
	event *factoryapi.FactoryEvent,
	wantText string,
) factoryapi.Work {
	t.Helper()
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST %q: %v", event.Id, err)
	}
	if payload.Works == nil || len(*payload.Works) != 1 {
		t.Fatalf("WORK_REQUEST payload = %#v, want one Work", payload)
	}
	item := (*payload.Works)[0]
	if item.WorkId == nil || strings.TrimSpace(*item.WorkId) == "" || item.WorkTypeName == nil || *item.WorkTypeName != "task" {
		t.Fatalf("WORK_REQUEST Work = %#v, want task with identity", item)
	}
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("WORK_REQUEST content = %#v, want one text part", item.Content)
	}
	textPart, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil || textPart.Text != wantText {
		t.Fatalf("WORK_REQUEST text = %#v, want %q", textPart, wantText)
	}
	return item
}

func assertManagedFactoryTTSAssociation(
	t *testing.T,
	event *factoryapi.FactoryEvent,
	wantDispatchID, label string,
) {
	t.Helper()
	if event.Context.DispatchId == nil || *event.Context.DispatchId != wantDispatchID {
		t.Fatalf("%s association dispatch id = %#v, want %q", label, event.Context.DispatchId, wantDispatchID)
	}
	payload, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
	if err != nil {
		t.Fatalf("decode %s association %q: %v", label, event.Id, err)
	}
	if strings.TrimSpace(payload.WorkerSessionId) == "" {
		t.Fatalf("%s association payload = %#v, want Worker Session identity", label, payload)
	}
}

func assertManagedFactoryTTSModelProjection(
	t *testing.T,
	live, replay factoryTTSDispatchEvents,
	wantSuccess bool,
) {
	t.Helper()
	liveRequest, err := live.modelRequest.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode live MODEL_REQUEST %q: %v", live.modelRequest.Id, err)
	}
	replayRequest, err := replay.modelRequest.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode replay MODEL_REQUEST %q: %v", replay.modelRequest.Id, err)
	}
	assertManagedFactoryTTSJSONEqual(t, liveRequest, replayRequest, "MODEL_REQUEST payload")

	liveResponse, err := live.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode live MODEL_RESPONSE %q: %v", live.modelResponse.Id, err)
	}
	replayResponse, err := replay.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode replay MODEL_RESPONSE %q: %v", replay.modelResponse.Id, err)
	}
	if liveResponse.Operation != "TTS" || liveResponse.Model != factorydefinitions.DefaultTTSModelName ||
		liveResponse.Worker != "tts-executor" || liveResponse.ModelRequestId == "" {
		t.Fatalf("live MODEL_RESPONSE identity = %#v, want packaged TTS identity", liveResponse)
	}
	if replayResponse.Operation != liveResponse.Operation || replayResponse.Model != liveResponse.Model ||
		replayResponse.Worker != liveResponse.Worker || replayResponse.ModelRequestId != liveResponse.ModelRequestId ||
		replayResponse.Attempt != liveResponse.Attempt || replayResponse.Outcome != liveResponse.Outcome {
		t.Fatalf("MODEL_RESPONSE stable projection differs\nlive=%#v\nreplay=%#v", liveResponse, replayResponse)
	}
	if wantSuccess {
		if liveResponse.Outcome != factoryapi.InferenceOutcomeSucceeded || liveResponse.OutputContent == nil || replayResponse.OutputContent == nil {
			t.Fatalf("successful MODEL_RESPONSE output = live:%#v replay:%#v", liveResponse, replayResponse)
		}
		assertManagedFactoryTTSJSONEqual(t, *liveResponse.OutputContent, *replayResponse.OutputContent, "MODEL_RESPONSE output content")
		if liveResponse.FailureDetail != nil || replayResponse.FailureDetail != nil || liveResponse.OutputPreview != nil || replayResponse.OutputPreview != nil {
			t.Fatalf("successful MODEL_RESPONSE failure/preview = live:%#v replay:%#v", liveResponse, replayResponse)
		}
		return
	}
	if liveResponse.Outcome != factoryapi.InferenceOutcomeFailed || replayResponse.Outcome != factoryapi.InferenceOutcomeFailed ||
		liveResponse.FailureDetail == nil || replayResponse.FailureDetail == nil {
		t.Fatalf("failed MODEL_RESPONSE = live:%#v replay:%#v", liveResponse, replayResponse)
	}
	if liveResponse.FailureDetail.Reason != replayResponse.FailureDetail.Reason ||
		liveResponse.FailureDetail.Message != replayResponse.FailureDetail.Message ||
		liveResponse.OutputContent != nil || replayResponse.OutputContent != nil ||
		liveResponse.OutputPreview != nil || replayResponse.OutputPreview != nil {
		t.Fatalf("failed MODEL_RESPONSE projection differs\nlive=%#v\nreplay=%#v", liveResponse, replayResponse)
	}
}

func assertManagedFactoryTTSDispatchProjection(
	t *testing.T,
	live, replay *factoryapi.FactoryEvent,
	wantSuccess bool,
) {
	t.Helper()
	livePayload, err := live.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode live DISPATCH_RESPONSE %q: %v", live.Id, err)
	}
	replayPayload, err := replay.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode replay DISPATCH_RESPONSE %q: %v", replay.Id, err)
	}
	if livePayload.TransitionId != "execute-tts" || replayPayload.TransitionId != livePayload.TransitionId ||
		livePayload.Outcome != replayPayload.Outcome {
		t.Fatalf("DISPATCH_RESPONSE stable projection = live:%#v replay:%#v", livePayload, replayPayload)
	}
	if (livePayload.Error == nil) != (replayPayload.Error == nil) ||
		(livePayload.Error != nil && *livePayload.Error != *replayPayload.Error) ||
		(livePayload.FailureDetail == nil) != (replayPayload.FailureDetail == nil) {
		t.Fatalf("DISPATCH_RESPONSE failure projection differs\nlive=%#v\nreplay=%#v", livePayload, replayPayload)
	}
	if livePayload.FailureDetail != nil &&
		(livePayload.FailureDetail.Reason != replayPayload.FailureDetail.Reason || livePayload.FailureDetail.Message != replayPayload.FailureDetail.Message) {
		t.Fatalf("DISPATCH_RESPONSE failure detail differs\nlive=%#v\nreplay=%#v", livePayload.FailureDetail, replayPayload.FailureDetail)
	}
	if wantSuccess {
		if livePayload.Outcome != factoryapi.WorkOutcomeAccepted || livePayload.Output == nil || replayPayload.Output == nil || *livePayload.Output != *replayPayload.Output {
			t.Fatalf("successful DISPATCH_RESPONSE output = live:%#v replay:%#v", livePayload, replayPayload)
		}
	} else if livePayload.Outcome != factoryapi.WorkOutcomeFailed || livePayload.Output != nil || replayPayload.Output != nil {
		t.Fatalf("failed DISPATCH_RESPONSE output = live:%#v replay:%#v", livePayload, replayPayload)
	}
	if (livePayload.OutputWork == nil) != (replayPayload.OutputWork == nil) {
		t.Fatalf("DISPATCH_RESPONSE output Work presence differs: live:%#v replay:%#v", livePayload.OutputWork, replayPayload.OutputWork)
	}
	if livePayload.OutputWork != nil {
		assertManagedFactoryTTSJSONEqual(t, *livePayload.OutputWork, *replayPayload.OutputWork, "DISPATCH_RESPONSE output Work")
	}
}

func assertManagedFactoryTTSJSONEqual(t *testing.T, live, replay any, label string) {
	t.Helper()
	liveJSON, err := json.Marshal(live)
	if err != nil {
		t.Fatalf("marshal live %s: %v", label, err)
	}
	replayJSON, err := json.Marshal(replay)
	if err != nil {
		t.Fatalf("marshal replay %s: %v", label, err)
	}
	if string(liveJSON) != string(replayJSON) {
		t.Fatalf("%s differs\nlive=%s\nreplay=%s", label, liveJSON, replayJSON)
	}
}

func assertManagedFactoryTTSFailureModelEvents(
	t *testing.T,
	events factoryTTSDispatchEvents,
	wantMessage string,
) {
	t.Helper()
	request, err := events.modelRequest.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode managed failed MODEL_REQUEST %q: %v", events.modelRequest.Id, err)
	}
	if request.Operation != "TTS" || request.Worker != "tts-executor" || request.Model != factorydefinitions.DefaultTTSModelName {
		t.Fatalf("managed failed MODEL_REQUEST = %#v, want TTS/tts-executor/%s", request, factorydefinitions.DefaultTTSModelName)
	}
	response, err := events.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode managed failed MODEL_RESPONSE %q: %v", events.modelResponse.Id, err)
	}
	if response.Outcome != factoryapi.InferenceOutcomeFailed || response.ModelRequestId != request.ModelRequestId {
		t.Fatalf("managed failed MODEL_RESPONSE = %#v, want failed response correlated to %q", response, request.ModelRequestId)
	}
	if response.OutputContent != nil || response.OutputPreview != nil {
		t.Fatalf("managed failed MODEL_RESPONSE output = content %#v preview %#v, want no successful output", response.OutputContent, response.OutputPreview)
	}
	if response.FailureDetail == nil || response.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown ||
		!strings.Contains(response.FailureDetail.Message, wantMessage) {
		t.Fatalf("managed failed MODEL_RESPONSE failureDetail = %#v, want unknown message containing %q", response.FailureDetail, wantMessage)
	}
}

func assertManagedFactoryTTSFailedWork(
	t *testing.T,
	item factoryapi.Work,
	wantText, label string,
) {
	t.Helper()
	if item.State == nil || item.State.Name != "failed" || item.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("%s state = %#v, want failed/FAILED", label, item.State)
	}
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("%s content = %#v, want one preserved text part and no AUDIO part", label, item.Content)
	}
	textPart, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil || textPart.Text != wantText {
		t.Fatalf("%s content = %#v, want preserved text %q", label, textPart, wantText)
	}
}

func assertManagedFactoryTTSFailureDispatchResponse(
	t *testing.T,
	event *factoryapi.FactoryEvent,
	wantText, label string,
) {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode %s DISPATCH_RESPONSE %q: %v", label, event.Id, err)
	}
	if payload.Outcome != factoryapi.WorkOutcomeFailed || payload.TransitionId != "execute-tts" || payload.Output != nil {
		t.Fatalf("%s DISPATCH_RESPONSE = %#v, want failed execute-tts without successful output", label, payload)
	}
	if payload.FailureDetail == nil || payload.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown ||
		payload.Error == nil || *payload.Error != payload.FailureDetail.Message {
		t.Fatalf("%s DISPATCH_RESPONSE failure = error:%#v detail:%#v, want provider-neutral matching failure", label, payload.Error, payload.FailureDetail)
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("%s DISPATCH_RESPONSE outputWork = %#v, want one onFailure Work", label, payload.OutputWork)
	}
	assertManagedFactoryTTSFailedWork(t, (*payload.OutputWork)[0], wantText, label+" output Work")
}

func assertManagedFactoryTTSNoArtifactEvents(t *testing.T, events []factoryapi.FactoryEvent, label string) {
	t.Helper()
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeArtifactCreated {
			t.Fatalf("%s emitted ARTIFACT_CREATED event: %#v", label, event)
		}
	}
}

func managedFactoryTTSAudioPart(t *testing.T, item factoryapi.Work) factoryapi.WorkAudioContentPart {
	t.Helper()
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("managed TTS Work content = %#v, want one AUDIO part", item.Content)
	}
	audio, err := (*item.Content)[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("managed TTS Work content as AUDIO = %v; content = %#v", err, item.Content)
	}
	if audio.Type != factoryapi.WorkContentPartTypeAudio ||
		(audio.Url == "" && (audio.File == nil || strings.TrimSpace(*audio.File) == "")) {
		t.Fatalf("managed TTS AUDIO = %#v, want a non-empty content reference", audio)
	}
	return audio
}

type managedFactoryTTSRecording struct {
	factoryDir   string
	homeDir      string
	cacheDir     string
	artifactPath string
	work         factoryapi.ListWorkResponse
	events       []factoryapi.FactoryEvent
}

func runManagedFactoryTTSRecording(
	t *testing.T,
	text string,
	backend *packagedTTSModelsBackend,
	failure error,
) managedFactoryTTSRecording {
	t.Helper()

	homeDir := t.TempDir()
	dir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)

	if failure != nil {
		backend.SetFailure(failure)
	}
	cacheDir := t.TempDir()
	writePackagedTTSReadyModelCache(t, cacheDir)
	edges, closeHost := managedTTSModelEdges(t, backend)
	t.Cleanup(closeHost)
	artifactPath := filepath.Join(t.TempDir(), "managed-tts-recording.replay.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--named", factorydefinitions.PackagedTTSFactoryName,
		"--record", artifactPath, "--output", "primary", "--to", text,
	})
	inputs.Input.Env = append(os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		run.ModelCacheDirEnvironment+"="+cacheDir,
	)
	inputs.Input.WorkingDirectory = dir
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if failure == nil && err != nil {
		t.Fatalf("root Process.Execute(managed TTS) error=%v stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if failure != nil && err == nil {
		t.Fatalf("root Process.Execute(managed TTS) error=nil for expected failure; stdout=%q", inputs.Stdout())
	}
	wantStatus := factoryapi.InvocationTerminalStatusCompleted
	if failure != nil {
		wantStatus = factoryapi.InvocationTerminalStatusFailed
	}
	if wantStatus == factoryapi.InvocationTerminalStatusCompleted && err != nil {
		t.Fatalf("root Process.Execute(managed TTS) status=%q error=%v stdout=%q stderr=%q", wantStatus, err, inputs.Stdout(), inputs.Stderr())
	}
	events := readManagedFactoryTTSRecording(t, artifactPath)
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("recorded TTS artifact %q: %v", artifactPath, err)
	}
	return managedFactoryTTSRecording{
		factoryDir: dir, homeDir: homeDir, cacheDir: cacheDir, artifactPath: artifactPath,
		work: managedFactoryTTSOutputWork(t, events), events: events,
	}
}

func readManagedFactoryTTSRecording(t *testing.T, artifactPath string) []factoryapi.FactoryEvent {
	t.Helper()
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read recorded TTS artifact %q: %v", artifactPath, err)
	}
	var artifact struct {
		SchemaVersion string                    `json:"schemaVersion"`
		Events        []factoryapi.FactoryEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode recorded TTS artifact %q: %v", artifactPath, err)
	}
	if artifact.SchemaVersion != "agent-factory.replay.v1" {
		t.Fatalf("managed TTS recording schema = %q, want agent-factory.replay.v1", artifact.SchemaVersion)
	}
	return artifact.Events
}

func managedFactoryTTSOutputWork(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.ListWorkResponse {
	t.Helper()
	observed := collectFactoryTTSDispatchEvents(t, events, factorysessions.DefaultSessionID)
	payload, err := observed.dispatchResponse.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode managed TTS DISPATCH_RESPONSE: %v", err)
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("managed TTS DISPATCH_RESPONSE outputWork = %#v, want one terminal Work", payload.OutputWork)
	}
	return factoryapi.ListWorkResponse{Results: []factoryapi.Work{(*payload.OutputWork)[0]}}
}

func replayManagedFactoryTTSRecording(
	t *testing.T,
	factoryDir, homeDir, cacheDir string,
	artifactPath string,
	backend *packagedTTSModelsBackend,
	wantSuccess bool,
) managedFactoryTTSRecording {
	t.Helper()
	edges, closeHost := managedTTSModelEdges(t, backend)
	t.Cleanup(closeHost)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--dir", factoryDir,
		"--replay", artifactPath, "--no-record", "--output", "primary",
	})
	inputs.Input.Env = append(os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		run.ModelCacheDirEnvironment+"="+cacheDir,
	)
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if wantSuccess && err != nil {
		t.Fatalf("root Process.Execute(replay managed TTS) error=%v stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	events := readManagedFactoryTTSRecording(t, artifactPath)
	return managedFactoryTTSRecording{
		factoryDir: factoryDir, artifactPath: artifactPath,
		work: managedFactoryTTSOutputWork(t, events), events: events,
	}
}
