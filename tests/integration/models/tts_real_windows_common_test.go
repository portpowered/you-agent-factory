package models_test

import "time"

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
	pinnedTTSCommandDescription  = "you.exe --json models invoke tts --operation TTS --input text=Local AI works on this machine --output audio=<isolated-tts.wav>"
	pinnedTTSMaxAudioBytes       = int64(512 << 20)
	pinnedTTSMaxDuration         = 5 * time.Minute
	pinnedTTSWitnessTimeout      = 90 * time.Minute
	pinnedTTSRuntimeEvidenceEnv  = "INFINITE_YOU_INTEGRATION_MODEL_RUNTIME_EVIDENCE"
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

type pinnedTTSDirs struct {
	root                string
	work                string
	home                string
	state               string
	modelCache          string
	hfHome              string
	hfCache             string
	temp                string
	output              string
	outputPath          string
	streams             string
	runtimeEvidencePath string
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

type pinnedTTSEvidence struct {
	Outcome          string                              `json:"outcome"`
	Blocker          string                              `json:"blocker,omitempty"`
	Platform         string                              `json:"platform"`
	Arch             string                              `json:"architecture"`
	Command          string                              `json:"command"`
	Binary           pinnedTTSFileIdentity               `json:"binary"`
	Model            pinnedTTSModelIdentity              `json:"model"`
	Backend          pinnedTTSBackendIdentity            `json:"backend"`
	Isolation        pinnedTTSIsolation                  `json:"isolation"`
	Budgets          pinnedTTSBudgets                    `json:"budgets"`
	ReuseDownload    pinnedTTSReuseDownload              `json:"reuseDownload"`
	Execution        *pinnedTTSExecution                 `json:"execution,omitempty"`
	Output           *pinnedTTSOutput                    `json:"output,omitempty"`
	Cleanup          pinnedTTSCleanup                    `json:"cleanup"`
	Streams          pinnedTTSStreams                    `json:"streams"`
	Caches           pinnedTTSCacheEvidence              `json:"caches"`
	StageTrace       []pinnedTTSStageEvidence            `json:"stageTrace"`
	Failure          *pinnedTTSFailureEvidence           `json:"failure,omitempty"`
	ChildEnvironment []pinnedTTSChildEnvironmentEvidence `json:"childEnvironment"`
	Unproven         []string                            `json:"unproven"`
}

type pinnedTTSExecution struct {
	Started             bool   `json:"started"`
	ProcessExited       bool   `json:"processExited"`
	ExitCode            int    `json:"exitCode"`
	TimedOut            bool   `json:"timedOut"`
	ChildProcesses      int    `json:"childProcesses"`
	ModelCalls          int    `json:"modelCalls"`
	HeavyInvocations    int    `json:"heavyInvocations"`
	SemanticRetries     int    `json:"semanticRetries"`
	StdoutBytes         int    `json:"stdoutBytes"`
	StdoutSHA256        string `json:"stdoutSha256"`
	StderrBytes         int    `json:"stderrBytes"`
	StderrSHA256        string `json:"stderrSha256"`
	ProcessTreeAttached bool   `json:"processTreeAttached"`
	ProcessTreeClosed   bool   `json:"processTreeClosed"`
}

type pinnedTTSOutput struct {
	Name           string `json:"name"`
	Modality       string `json:"modality"`
	MediaType      string `json:"mediaType"`
	Bytes          int64  `json:"bytes"`
	SHA256         string `json:"sha256"`
	DurationMillis int64  `json:"durationMillis"`
	Channels       uint16 `json:"channels"`
	SampleRate     uint32 `json:"sampleRate"`
	Bits           uint16 `json:"bits"`
	BlockAlign     uint16 `json:"blockAlign"`
}

type pinnedTTSStageEvidence struct {
	Stage          string `json:"stage"`
	Outcome        string `json:"outcome"`
	FailureClass   string `json:"failureClass,omitempty"`
	DurationMillis int64  `json:"durationMillis"`
}

type pinnedTTSFailureEvidence struct {
	Stage       string `json:"stage"`
	Class       string `json:"class"`
	CauseSHA256 string `json:"causeSha256"`
}

type pinnedTTSCleanup struct {
	Checked                bool   `json:"checked"`
	ProcessTreeAttached    bool   `json:"processTreeAttached"`
	ProcessTreeClosed      bool   `json:"processTreeClosed"`
	OwnedProcessRemaining  bool   `json:"ownedProcessRemaining"`
	OwnedListenerRemaining bool   `json:"ownedListenerRemaining"`
	Observation            string `json:"observation,omitempty"`
}

type pinnedTTSStreams struct {
	Redacted       bool `json:"redacted"`
	RawAudioLogged bool `json:"rawAudioLogged"`
}

type pinnedTTSChildEnvironmentEvidence struct {
	Sequence    uint64                            `json:"sequence"`
	Kind        string                            `json:"kind"`
	Backend     string                            `json:"backend"`
	ProcessID   int                               `json:"process_id"`
	Phase       string                            `json:"phase"`
	Environment []pinnedTTSManagedEnvironmentFact `json:"environment,omitempty"`
	ExitClass   string                            `json:"exit_class,omitempty"`
}

type pinnedTTSManagedEnvironmentFact struct {
	Name        string `json:"name"`
	Present     bool   `json:"present"`
	ValueSHA256 string `json:"value_sha256,omitempty"`
}

type pinnedTTSCacheEvidence struct {
	TTSModel              string `json:"ttsModel"`
	TTSBackend            string `json:"ttsBackend"`
	ModelCacheEntries     int    `json:"modelCacheEntries"`
	HFCacheEntries        int    `json:"hfCacheEntries"`
	PartialArtifacts      bool   `json:"partialArtifacts"`
	ModelCacheEntriesPrev int    `json:"modelCacheEntriesBefore"`
	HFCacheEntriesPrev    int    `json:"hfCacheEntriesBefore"`
}

type pinnedTTSRecordedRuntime struct {
	Sequence       uint64                            `json:"sequence"`
	Kind           string                            `json:"kind"`
	Stage          string                            `json:"stage,omitempty"`
	Outcome        string                            `json:"outcome"`
	FailureClass   string                            `json:"failure_class,omitempty"`
	DurationMillis int64                             `json:"duration_millis"`
	CauseSHA256    string                            `json:"cause_sha256,omitempty"`
	Backend        string                            `json:"backend,omitempty"`
	ProcessID      int                               `json:"process_id,omitempty"`
	Phase          string                            `json:"phase,omitempty"`
	Environment    []pinnedTTSManagedEnvironmentFact `json:"environment,omitempty"`
	ExitClass      string                            `json:"exit_class,omitempty"`
}

type pinnedTTSRuntimeObservation struct {
	StageTrace       []pinnedTTSStageEvidence
	Failure          *pinnedTTSFailureEvidence
	ChildEnvironment []pinnedTTSChildEnvironmentEvidence
	TerminalOutcome  string
}

type pinnedCommandResult struct {
	stdout              []byte
	stderr              []byte
	exitCode            int
	started             bool
	processExited       bool
	timedOut            bool
	setupFailure        bool
	processTreeAttached bool
	processTreeClosed   bool
}

type pinnedTTSFileIdentity struct {
	Configured  bool   `json:"configured"`
	RegularFile bool   `json:"regularFile"`
	Path        string `json:"path,omitempty"`
	Commit      string `json:"commit,omitempty"`
	Tree        string `json:"tree,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type pinnedTreeSnapshot struct {
	Entries map[string]string
}

type pinnedTTSModelIdentity struct {
	Name          string `json:"name"`
	Source        string `json:"source"`
	Revision      string `json:"revision"`
	Verified      bool   `json:"verified"`
	ArtifactCount int    `json:"artifactCount,omitempty"`
	ArtifactBytes int64  `json:"artifactBytes,omitempty"`
}

type pinnedTTSBackendIdentity struct {
	ID               string `json:"id"`
	SourceRepository string `json:"sourceRepository,omitempty"`
	BackendCommit    string `json:"backendCommit,omitempty"`
	VibeVoiceCommit  string `json:"vibeVoiceCommit"`
	LocalAICommit    string `json:"localAICommit"`
	ProtocolRevision string `json:"protocolRevision"`
	Archive          string `json:"archive"`
	ExpectedBytes    int64  `json:"expectedBytes"`
	ExpectedSHA256   string `json:"expectedSha256"`
	ObservedBytes    int64  `json:"observedBytes,omitempty"`
	ObservedSHA256   string `json:"observedSha256,omitempty"`
	Verified         bool   `json:"verified"`
}

type pinnedTTSManagedMetadata struct {
	ModelName string                  `json:"modelName"`
	Revision  string                  `json:"revision"`
	Files     []pinnedTTSMetadataFile `json:"files"`
}

type pinnedTTSMetadataFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type pinnedTTSAssetMetadata struct {
	Kind      string                      `json:"kind"`
	Artifacts []pinnedTTSAssetRequirement `json:"artifacts"`
}

type pinnedTTSAssetRequirement struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
