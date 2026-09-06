package models_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	pinnedTTSPrompt              = "Local AI works on this machine"
	pinnedTTSBinaryEnvironment   = "INFINITE_YOU_INTEGRATION_BINARY"
	pinnedTTSEvidenceEnvironment = "INFINITE_YOU_TTS_EVIDENCE_OUTPUT"
	pinnedTTSRequireEnvironment  = "INFINITE_YOU_REQUIRE_PINNED_TTS"
	pinnedTTSModelSource         = "hf://mudler/vibevoice.cpp-models/vibevoice-realtime-0.5B-q8_0.gguf@a67807e65e3002e187179a856e96043f75060bc9"
	pinnedTTSModelRevision       = "a67807e65e3002e187179a856e96043f75060bc9"
	pinnedASRModelSource         = "hf://ggerganov/whisper.cpp/ggml-base.en.bin@5359861c739e955e79d9a303bcbc70fb988958b1"
	pinnedASRModelRevision       = "5359861c739e955e79d9a303bcbc70fb988958b1"
	pinnedTTSBackendID           = "localai-vibevoice"
	pinnedASRBackendID           = "localai-whisper"
	pinnedTTSVibeVoiceCommit     = "000e37282bc5bb09edc20f7047a47924122ba3a0"
	pinnedASRWhisperCommit       = "080bbbe85230f624f0b52127f1ae1218247989f9"
	pinnedTTSLocalAICommit       = "b224c96db6f4b87306a33a808650bfce63b12588"
	pinnedTTSProtocolRevision    = "ad62c6df07ae1169eb14411a565a689cd996b19c"
	pinnedTTSBackendArchive      = "localai-backend-localai-vibevoice-windows-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.zip"
	pinnedTTSBackendBytes        = int64(10757902)
	pinnedTTSBackendSHA256       = "8f3c14212948be34c930e9a790af7757460cb2f6bb6a0de80d5b9f95b71e8646"
	pinnedASRBackendArchive      = "localai-backend-localai-whisper-windows-amd64-080bbbe85230f624f0b52127f1ae1218247989f9.zip"
	pinnedASRBackendBytes        = int64(11935463)
	pinnedASRBackendSHA256       = "6956415b4b47b14346e0eacc1c5fa34b15a0c8cf9e7c4e7a3436b8b9e96b63c3"
	pinnedTTSMetadataName        = ".you-assets.json"
	pinnedTTSManagedMetadataName = ".managed-cache.json"
	pinnedTTSManagedModelDir     = "TTS"
	pinnedASRManagedModelDir     = "ASR"
	pinnedTTSModelName           = "tts"
	pinnedTTSCommandDescription  = "you.exe --json models invoke tts --operation TTS --input text=Local AI works on this machine --output audio=<isolated-tts.wav>; then --json models invoke asr --operation ASR --input audio=@<isolated-tts.wav> --output transcript=<isolated-transcript.txt> --output segments=<isolated-segments.json>"
	pinnedTTSMaxAudioBytes       = int64(512 << 20)
	pinnedTTSMaxDuration         = 5 * time.Minute
	pinnedTTSWitnessTimeout      = 3 * time.Hour
)

var (
	pinnedTTSModelArtifacts = []pinnedTTSAssetRequirement{
		{Name: "vibevoice-realtime-0.5B-q8_0.gguf", Bytes: 1699832128, SHA256: "5251e3f0386d1056a90c61b6c7359a4775da44dd19402499bef1989c4b5c653a"},
		{Name: "tokenizer.gguf", Bytes: 5922368, SHA256: "37dc3b722d5677e37e29a57df55aa05c485116eeb5459e57ff8dde616b4986f6"},
		{Name: "voice-en-Carter_man.gguf", Bytes: 8472448, SHA256: "b15cd8b9cae6ee2c3d20b0ee6e7bfe93f13489f8b63b6834e9bbf0dfabf6505a"},
	}
	pinnedASRModelArtifacts = []pinnedTTSAssetRequirement{
		{Name: "ggml-base.en.bin", Bytes: 147964211, SHA256: "a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002"},
	}
)

// TestPinnedLocalAITTSASRWindowsWitness is the only local-real witness for
// this lane. It intentionally consumes a build/release artifact; the
// integration test never invokes go build and never replaces the real model,
// backend, or media path with a fixture.
func TestPinnedLocalAITTSASRWindowsWitness(t *testing.T) {
	evidence, dirs := newPinnedTTSEvidence(t)
	binaryPath, ok := preparePinnedTTSBinary(t, &evidence)
	if !ok {
		return
	}
	runPinnedTTSASRChain(t, &evidence, dirs, binaryPath)
}

type pinnedTTSDirs struct {
	root            string
	work            string
	home            string
	state           string
	modelCache      string
	hfHome          string
	hfCache         string
	temp            string
	output          string
	outputPath      string
	reuseOutputPath string
	transcriptPath  string
	segmentsPath    string
	streams         string
}

func newPinnedTTSEvidence(t *testing.T) (pinnedTTSEvidence, pinnedTTSDirs) {
	t.Helper()
	root := t.TempDir()
	dirs := pinnedTTSDirs{
		root:       root,
		work:       filepath.Join(root, "work"),
		home:       filepath.Join(root, "profile"),
		state:      filepath.Join(root, "state"),
		modelCache: filepath.Join(root, "model-cache"),
		hfHome:     filepath.Join(root, "hf-home"),
		hfCache:    filepath.Join(root, "hf-cache"),
		temp:       filepath.Join(root, "temp"),
		output:     filepath.Join(root, "output"),
		streams:    filepath.Join(root, "streams"),
	}
	dirs.outputPath = filepath.Join(dirs.output, "tts-first.wav")
	dirs.reuseOutputPath = filepath.Join(dirs.output, "tts-cache-reuse.wav")
	dirs.transcriptPath = filepath.Join(dirs.output, "asr-transcript.txt")
	dirs.segmentsPath = filepath.Join(dirs.output, "asr-segments.json")
	for _, directory := range []string{
		dirs.work, dirs.home, dirs.state, dirs.modelCache, dirs.hfHome,
		dirs.hfCache, dirs.temp, dirs.output, dirs.streams,
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal("create isolated TTS witness directory")
		}
	}
	writeStory001Factory(t, dirs.work)

	return pinnedTTSEvidence{
		Outcome:  "INCONCLUSIVE",
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
		Command:  pinnedTTSCommandDescription,
		Model: pinnedTTSModelIdentity{
			Name:     pinnedTTSModelName,
			Source:   pinnedTTSModelSource,
			Revision: pinnedTTSModelRevision,
		},
		Backend: pinnedTTSBackendIdentity{
			ID:               pinnedTTSBackendID,
			SourceRepository: "https://github.com/mudler/vibevoice.cpp",
			BackendCommit:    pinnedTTSVibeVoiceCommit,
			VibeVoiceCommit:  pinnedTTSVibeVoiceCommit,
			LocalAICommit:    pinnedTTSLocalAICommit,
			ProtocolRevision: pinnedTTSProtocolRevision,
			Archive:          pinnedTTSBackendArchive,
			ExpectedBytes:    pinnedTTSBackendBytes,
			ExpectedSHA256:   pinnedTTSBackendSHA256,
		},
		ASRModel: pinnedTTSModelIdentity{
			Name:     "asr",
			Source:   pinnedASRModelSource,
			Revision: pinnedASRModelRevision,
		},
		ASRBackend: pinnedTTSBackendIdentity{
			ID:               pinnedASRBackendID,
			SourceRepository: "https://github.com/ggml-org/whisper.cpp",
			BackendCommit:    pinnedASRWhisperCommit,
			LocalAICommit:    pinnedTTSLocalAICommit,
			ProtocolRevision: pinnedTTSProtocolRevision,
			Archive:          pinnedASRBackendArchive,
			ExpectedBytes:    pinnedASRBackendBytes,
			ExpectedSHA256:   pinnedASRBackendSHA256,
		},
		Isolation: pinnedTTSIsolation{
			FreshWorkingDirectory: true,
			FreshUserProfile:      true,
			FreshModelCache:       true,
			FreshHFCache:          true,
			FreshTempDirectory:    true,
			FreshOutputDirectory:  true,
			FreshPortState:        true,
			NetworkPolicy:         "first use permits only the pinned public sources; cache reuse uses a failing proxy probe",
		},
		Budgets: pinnedTTSBudgets{
			Disk:             "80 GiB",
			Download:         "32 GiB",
			ChildProcesses:   4,
			ModelCalls:       3,
			HeavyInvocations: 3,
			SemanticRetries:  0,
			Timeout:          pinnedTTSWitnessTimeout.String(),
		},
		Cleanup: pinnedTTSCleanup{Checked: false},
		Streams: pinnedTTSStreams{
			Redacted:       true,
			RawAudioLogged: false,
		},
		Unproven: []string{
			"non-Windows platforms",
			"configured-server parity",
			"other model operations",
			"Factory journey and Project completion",
		},
	}, dirs
}

type pinnedTTSIsolation struct {
	FreshWorkingDirectory bool   `json:"freshWorkingDirectory"`
	FreshUserProfile      bool   `json:"freshUserProfile"`
	FreshModelCache       bool   `json:"freshModelCache"`
	FreshHFCache          bool   `json:"freshHFCache"`
	FreshTempDirectory    bool   `json:"freshTempDirectory"`
	FreshOutputDirectory  bool   `json:"freshOutputDirectory"`
	FreshPortState        bool   `json:"freshPortState"`
	NetworkPolicy         string `json:"networkPolicy"`
}

type pinnedTTSBudgets struct {
	Disk             string `json:"disk"`
	Download         string `json:"download"`
	ChildProcesses   int    `json:"childProcesses"`
	ModelCalls       int    `json:"modelCalls"`
	HeavyInvocations int    `json:"heavyInvocations"`
	SemanticRetries  int    `json:"semanticRetries"`
	Timeout          string `json:"timeout"`
}

type pinnedTTSReuseDownload struct {
	FreshCacheAtStart bool   `json:"freshCacheAtStart"`
	Model             string `json:"model"`
	Backend           string `json:"backend"`
}

func pinnedTTSCacheDisposition(fresh, verified bool) string {
	if !verified {
		return "unverified"
	}
	if fresh {
		return "downloaded"
	}
	return "reused"
}

func blockedPinnedTTSEvidence(t *testing.T, evidence *pinnedTTSEvidence, reason string) {
	t.Helper()
	evidence.Outcome = "INCONCLUSIVE"
	evidence.Blocker = reason
	recordPinnedTTSEvidence(t, evidence)
	if strings.EqualFold(strings.TrimSpace(os.Getenv(pinnedTTSRequireEnvironment)), "1") {
		t.Fatal("required pinned Windows TTS witness was blocked by an unavailable dependency")
	}
}

func failedPinnedTTSEvidence(t *testing.T, evidence *pinnedTTSEvidence, reason string) {
	t.Helper()
	evidence.Outcome = "FAIL"
	evidence.Blocker = reason
	recordPinnedTTSEvidence(t, evidence)
	t.Fatal("pinned Windows TTS witness failed; inspect the bounded evidence artifact")
}

func recordPinnedTTSEvidence(t *testing.T, evidence *pinnedTTSEvidence) {
	t.Helper()
	if path := strings.TrimSpace(os.Getenv(pinnedTTSEvidenceEnvironment)); path != "" {
		body, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil || os.MkdirAll(filepath.Dir(path), 0o755) != nil || os.WriteFile(path, append(body, '\n'), 0o600) != nil {
			t.Fatal("write pinned TTS evidence artifact")
		}
	}
	t.Logf(
		"PINNED-TTS-EVIDENCE outcome=%s blocker=%s binarySHA256=%s modelRevision=%s backendSHA256=%s outputBytes=%d outputSHA256=%s",
		evidence.Outcome, evidence.Blocker, evidence.Binary.SHA256, evidence.Model.Revision,
		evidence.Backend.ObservedSHA256, evidence.OutputBytes(), evidence.OutputSHA256(),
	)
}

func (evidence pinnedTTSEvidence) OutputBytes() int64 {
	if evidence.Output == nil {
		return 0
	}
	return evidence.Output.Bytes
}

func (evidence pinnedTTSEvidence) OutputSHA256() string {
	if evidence.Output == nil {
		return ""
	}
	return evidence.Output.SHA256
}
