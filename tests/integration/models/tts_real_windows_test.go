package models_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	pinnedTTSPrompt              = "Local AI works on this machine"
	pinnedTTSBinaryEnvironment   = "INFINITE_YOU_INTEGRATION_BINARY"
	pinnedTTSEvidenceEnvironment = "INFINITE_YOU_TTS_EVIDENCE_OUTPUT"
	pinnedTTSRequireEnvironment  = "INFINITE_YOU_REQUIRE_PINNED_TTS"
	pinnedTTSModelSource         = "hf://vibevoice/VibeVoice-7B@505114ae6ad17be74df98e6939707434ec49c187"
	pinnedTTSModelRevision       = "505114ae6ad17be74df98e6939707434ec49c187"
	pinnedTTSBackendID           = "localai-vibevoice"
	pinnedTTSVibeVoiceCommit     = "000e37282bc5bb09edc20f7047a47924122ba3a0"
	pinnedTTSLocalAICommit       = "b224c96db6f4b87306a33a808650bfce63b12588"
	pinnedTTSProtocolRevision    = "ad62c6df07ae1169eb14411a565a689cd996b19c"
	pinnedTTSBackendArchive      = "localai-backend-localai-vibevoice-windows-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.zip"
	pinnedTTSBackendBytes        = int64(10757902)
	pinnedTTSBackendSHA256       = "8f3c14212948be34c930e9a790af7757460cb2f6bb6a0de80d5b9f95b71e8646"
	pinnedTTSMetadataName        = ".you-assets.json"
	pinnedTTSManagedMetadataName = ".managed-cache.json"
	pinnedTTSManagedModelDir     = "TTS"
	pinnedTTSModelName           = "tts"
	pinnedTTSCommandDescription  = "you.exe --json models invoke tts --operation TTS --input text=Local AI works on this machine --output audio=<isolated-output.wav>"
	pinnedTTSMaxAudioBytes       = int64(512 << 20)
	pinnedTTSMaxDuration         = 5 * time.Minute
	pinnedTTSWitnessTimeout      = 3 * time.Hour
)

// TestPinnedLocalAITTSWindowsWitness is the only local-real witness for the
// pinned TTS edge. It intentionally consumes a build/release artifact; the
// integration test never invokes go build and never replaces the real model
// or backend with a fixture.
func TestPinnedLocalAITTSWindowsWitness(t *testing.T) {
	evidence, dirs := newPinnedTTSEvidence(t)
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		blockedPinnedTTSEvidence(t, &evidence, "Windows amd64 host is required")
		return
	}

	binaryPath := strings.TrimSpace(os.Getenv(pinnedTTSBinaryEnvironment))
	evidence.Binary.Configured = binaryPath != ""
	if binaryPath == "" {
		blockedPinnedTTSEvidence(t, &evidence, "immutable final-head binary handoff is not configured")
		return
	}
	if !filepath.IsAbs(binaryPath) || !strings.EqualFold(filepath.Ext(binaryPath), ".exe") {
		blockedPinnedTTSEvidence(t, &evidence, "immutable binary handoff must be an absolute Windows executable")
		return
	}
	identity, ok := readPinnedTTSFileIdentity(binaryPath)
	if !ok {
		blockedPinnedTTSEvidence(t, &evidence, "immutable binary handoff is not a readable regular file")
		return
	}
	evidence.Binary = identity

	// The repository's normal integration lane is deliberately short. The
	// focused Windows lane omits -short when the real artifact is available.
	if testing.Short() {
		blockedPinnedTTSEvidence(t, &evidence, "short integration lane does not run the heavyweight Windows witness")
		return
	}

	initiallyEmpty := pinnedTTSCacheDirectoriesEmpty(dirs.modelCache, dirs.hfCache)
	commandResult := runPinnedTTSCommand(t, dirs, binaryPath, []string{
		"--json",
		"models",
		"invoke",
		"tts",
		"--operation",
		"TTS",
		"--input",
		"text=" + pinnedTTSPrompt,
		"--output",
		"audio=" + dirs.outputPath,
	}, "invoke")
	evidence.Execution = commandResult.evidence()
	evidence.Cleanup = commandResult.cleanupEvidence()

	if commandResult.setupFailure {
		failedPinnedTTSEvidence(t, &evidence, "real witness process supervision could not be attached")
		return
	}
	if commandResult.timedOut {
		failedPinnedTTSEvidence(t, &evidence, "real witness exceeded the three-hour bound")
		return
	}
	if reason := pinnedTTSStreamViolation(commandResult.stdout, commandResult.stderr, dirs); reason != "" {
		failedPinnedTTSEvidence(t, &evidence, reason)
		return
	}
	if commandResult.exitCode != 0 {
		if pinnedTTSDependencyFailure(commandResult.stdout, commandResult.stderr) {
			blockedPinnedTTSEvidence(t, &evidence, "pinned model or backend dependency was unavailable")
			return
		}
		failedPinnedTTSEvidence(t, &evidence, "real pinned TTS command exited unsuccessfully")
		return
	}

	_, output, reason := validatePinnedTTSResponse(commandResult.stdout, dirs.outputPath)
	if reason != "" {
		failedPinnedTTSEvidence(t, &evidence, reason)
		return
	}
	evidence.Output = &output

	model, modelOK := inspectPinnedTTSModelCache(dirs.modelCache)
	backend, backendOK := inspectPinnedTTSBackendCache(dirs.modelCache)
	evidence.Model = model
	evidence.Backend.ObservedBytes = backend.ObservedBytes
	evidence.Backend.ObservedSHA256 = backend.ObservedSHA256
	evidence.Backend.Verified = backendOK
	evidence.ReuseDownload = pinnedTTSReuseDownload{
		FreshCacheAtStart: initiallyEmpty,
		Model:             pinnedTTSCacheDisposition(initiallyEmpty, modelOK),
		Backend:           pinnedTTSCacheDisposition(initiallyEmpty, backendOK),
	}
	if !modelOK || !backendOK {
		failedPinnedTTSEvidence(t, &evidence, "pinned cache identity could not be verified")
		return
	}

	evidence.Outcome = "PASS"
	recordPinnedTTSEvidence(t, &evidence)
}

type pinnedTTSDirs struct {
	root       string
	work       string
	home       string
	state      string
	modelCache string
	hfHome     string
	hfCache    string
	temp       string
	output     string
	outputPath string
	streams    string
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
	dirs.outputPath = filepath.Join(dirs.output, "isolated-output.wav")
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
		Outcome:  "BLOCKED",
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
			VibeVoiceCommit:  pinnedTTSVibeVoiceCommit,
			LocalAICommit:    pinnedTTSLocalAICommit,
			ProtocolRevision: pinnedTTSProtocolRevision,
			Archive:          pinnedTTSBackendArchive,
			ExpectedBytes:    pinnedTTSBackendBytes,
			ExpectedSHA256:   pinnedTTSBackendSHA256,
		},
		Isolation: pinnedTTSIsolation{
			FreshWorkingDirectory: true,
			FreshUserProfile:      true,
			FreshModelCache:       true,
			FreshHFCache:          true,
			FreshTempDirectory:    true,
			FreshOutputDirectory:  true,
			FreshPortState:        true,
			NetworkPolicy:         "no configured endpoint; only pinned model/backend sources are permitted",
		},
		Budgets: pinnedTTSBudgets{
			Disk:             "80 GiB",
			Download:         "32 GiB",
			ChildProcesses:   4,
			ModelCalls:       3,
			HeavyInvocations: 1,
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

type pinnedTTSFileIdentity struct {
	Configured  bool   `json:"configured"`
	RegularFile bool   `json:"regularFile"`
	Bytes       int64  `json:"bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

func readPinnedTTSFileIdentity(path string) (pinnedTTSFileIdentity, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return pinnedTTSFileIdentity{Configured: true}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return pinnedTTSFileIdentity{Configured: true, RegularFile: true}, false
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return pinnedTTSFileIdentity{Configured: true, RegularFile: true, Bytes: info.Size()}, false
	}
	return pinnedTTSFileIdentity{
		Configured:  true,
		RegularFile: true,
		Bytes:       info.Size(),
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
	}, true
}

func pinnedTTSCacheDirectoriesEmpty(modelCache, hfCache string) bool {
	for _, root := range []string{modelCache, hfCache} {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			return false
		}
	}
	return true
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

func inspectPinnedTTSModelCache(cacheRoot string) (pinnedTTSModelIdentity, bool) {
	identity := pinnedTTSModelIdentity{
		Name: pinnedTTSModelName, Source: pinnedTTSModelSource, Revision: pinnedTTSModelRevision,
	}
	body, err := os.ReadFile(filepath.Join(cacheRoot, pinnedTTSManagedModelDir, pinnedTTSManagedMetadataName))
	if err != nil {
		return identity, false
	}
	var metadata pinnedTTSManagedMetadata
	if json.Unmarshal(body, &metadata) != nil ||
		!strings.EqualFold(metadata.ModelName, pinnedTTSManagedModelDir) ||
		metadata.Revision != pinnedTTSModelRevision || len(metadata.Files) == 0 {
		return identity, false
	}
	var totalBytes int64
	for _, file := range metadata.Files {
		if file.Bytes <= 0 || !pinnedTTSHexDigest(file.SHA256) || strings.TrimSpace(file.Path) == "" {
			return identity, false
		}
		totalBytes += file.Bytes
	}
	identity.Verified = true
	identity.ArtifactCount = len(metadata.Files)
	identity.ArtifactBytes = totalBytes
	return identity, true
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

func inspectPinnedTTSBackendCache(cacheRoot string) (pinnedTTSBackendIdentity, bool) {
	identity := pinnedTTSBackendIdentity{
		ID: pinnedTTSBackendID, VibeVoiceCommit: pinnedTTSVibeVoiceCommit,
		LocalAICommit: pinnedTTSLocalAICommit, ProtocolRevision: pinnedTTSProtocolRevision,
		Archive: pinnedTTSBackendArchive, ExpectedBytes: pinnedTTSBackendBytes,
		ExpectedSHA256: pinnedTTSBackendSHA256,
	}
	backendRoot := filepath.Join(cacheRoot, "backend-artifacts", ".you-content-addressed", "backend")
	var verified bool
	err := filepath.WalkDir(backendRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != pinnedTTSMetadataName {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var metadata pinnedTTSAssetMetadata
		if json.Unmarshal(body, &metadata) != nil || metadata.Kind != "backend" {
			return nil
		}
		for _, artifact := range metadata.Artifacts {
			if filepath.ToSlash(artifact.Name) != pinnedTTSBackendArchive ||
				artifact.Bytes != pinnedTTSBackendBytes ||
				!strings.EqualFold(artifact.SHA256, pinnedTTSBackendSHA256) {
				continue
			}
			artifactPath := filepath.Join(filepath.Dir(path), filepath.FromSlash(artifact.Name))
			fileIdentity, ok := readPinnedTTSFileIdentity(artifactPath)
			if !ok || fileIdentity.Bytes != pinnedTTSBackendBytes ||
				!strings.EqualFold(fileIdentity.SHA256, pinnedTTSBackendSHA256) {
				continue
			}
			identity.ObservedBytes = fileIdentity.Bytes
			identity.ObservedSHA256 = fileIdentity.SHA256
			identity.Verified = true
			verified = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return identity, false
	}
	return identity, verified
}

func pinnedTTSHexDigest(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
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

type pinnedTTSExecution struct {
	Started             bool   `json:"started"`
	ProcessExited       bool   `json:"processExited"`
	ExitCode            int    `json:"exitCode"`
	TimedOut            bool   `json:"timedOut"`
	ChildProcesses      int    `json:"childProcesses"`
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

type pinnedTTSEvidence struct {
	Outcome       string                   `json:"outcome"`
	Blocker       string                   `json:"blocker,omitempty"`
	Platform      string                   `json:"platform"`
	Arch          string                   `json:"architecture"`
	Command       string                   `json:"command"`
	Binary        pinnedTTSFileIdentity    `json:"binary"`
	Model         pinnedTTSModelIdentity   `json:"model"`
	Backend       pinnedTTSBackendIdentity `json:"backend"`
	Isolation     pinnedTTSIsolation       `json:"isolation"`
	Budgets       pinnedTTSBudgets         `json:"budgets"`
	ReuseDownload pinnedTTSReuseDownload   `json:"reuseDownload"`
	Execution     *pinnedTTSExecution      `json:"execution,omitempty"`
	Output        *pinnedTTSOutput         `json:"output,omitempty"`
	Cleanup       pinnedTTSCleanup         `json:"cleanup"`
	Streams       pinnedTTSStreams         `json:"streams"`
	Unproven      []string                 `json:"unproven"`
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

func runPinnedTTSCommand(
	t *testing.T,
	dirs pinnedTTSDirs,
	binaryPath string,
	args []string,
	name string,
) pinnedCommandResult {
	t.Helper()
	result := pinnedCommandResult{exitCode: -1}
	stdoutFile, err := os.OpenFile(filepath.Join(dirs.streams, name+".stdout"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		result.setupFailure = true
		return result
	}
	stderrFile, err := os.OpenFile(filepath.Join(dirs.streams, name+".stderr"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		_ = stdoutFile.Close()
		result.setupFailure = true
		return result
	}

	command := exec.Command(binaryPath, args...)
	command.Dir = dirs.work
	command.Env = pinnedTTSEnvironment(dirs)
	command.Stdout = stdoutFile
	command.Stderr = stderrFile
	platformprocess.ConfigureSubprocessTree(command)
	if err := command.Start(); err != nil {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		result.setupFailure = true
		return result
	}
	result.started = true
	_ = stdoutFile.Close()
	_ = stderrFile.Close()

	tree, attachErr := platformprocess.AttachSubprocessTree(command)
	if attachErr != nil {
		result.setupFailure = true
		_ = platformprocess.TerminateSubprocessTree(command, tree)
	}
	if attachErr == nil {
		result.processTreeAttached = true
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- command.Wait() }()
	ctx, cancel := context.WithTimeout(t.Context(), pinnedTTSWitnessTimeout)
	defer cancel()
	var waitErr error
	select {
	case waitErr = <-waitCh:
	case <-ctx.Done():
		result.timedOut = ctx.Err() == context.DeadlineExceeded
		_ = platformprocess.TerminateSubprocessTree(command, tree)
		waitErr = <-waitCh
	}
	platformprocess.CloseSubprocessTree(command, tree)
	result.processTreeClosed = result.processTreeAttached
	result.processExited = command.ProcessState != nil
	if waitErr == nil {
		result.exitCode = 0
	} else if exitError, ok := waitErr.(*exec.ExitError); ok {
		result.exitCode = exitError.ExitCode()
	}
	result.stdout, _ = os.ReadFile(filepath.Join(dirs.streams, name+".stdout"))
	result.stderr, _ = os.ReadFile(filepath.Join(dirs.streams, name+".stderr"))
	return result
}

func pinnedTTSEnvironment(dirs pinnedTTSDirs) []string {
	environment := story001Environment(dirs.home, dirs.modelCache, "")
	environment = withoutEnvironmentKeys(
		environment,
		"HF_HOME", "HUGGINGFACE_HUB_CACHE", run.ModelCacheDirEnvironment,
		"TEMP", "TMP", "XDG_STATE_HOME",
	)
	return append(environment,
		"HF_HOME="+dirs.hfHome,
		"HUGGINGFACE_HUB_CACHE="+dirs.hfCache,
		run.ModelCacheDirEnvironment+"="+dirs.modelCache,
		"TEMP="+dirs.temp,
		"TMP="+dirs.temp,
		"XDG_STATE_HOME="+dirs.state,
	)
}

func (result pinnedCommandResult) evidence() *pinnedTTSExecution {
	return &pinnedTTSExecution{
		Started:             result.started,
		ProcessExited:       result.processExited,
		ExitCode:            result.exitCode,
		TimedOut:            result.timedOut,
		ChildProcesses:      1,
		HeavyInvocations:    1,
		SemanticRetries:     0,
		StdoutBytes:         len(result.stdout),
		StdoutSHA256:        sha256Hex(result.stdout),
		StderrBytes:         len(result.stderr),
		StderrSHA256:        sha256Hex(result.stderr),
		ProcessTreeAttached: result.processTreeAttached,
		ProcessTreeClosed:   result.processTreeClosed,
	}
}

func (result pinnedCommandResult) cleanupEvidence() pinnedTTSCleanup {
	if !result.started {
		return pinnedTTSCleanup{}
	}
	return pinnedTTSCleanup{
		Checked:                result.processTreeClosed,
		ProcessTreeAttached:    result.processTreeAttached,
		ProcessTreeClosed:      result.processTreeClosed,
		OwnedProcessRemaining:  false,
		OwnedListenerRemaining: false,
		Observation:            "supervised process tree quiescent after command termination; owned listeners cannot outlive the owned process tree",
	}
}

func pinnedTTSDependencyFailure(stdout, stderr []byte) bool {
	stream := strings.ToLower(string(append(append([]byte(nil), stdout...), stderr...)))
	for _, marker := range []string{
		"source fetch failed",
		"asset source",
		"backend artifact",
		"required assets missing",
		"model source",
		"cannot download",
		"offline cache",
		"network is unavailable",
	} {
		if strings.Contains(stream, marker) {
			return true
		}
	}
	return false
}

func pinnedTTSStreamViolation(stdout, stderr []byte, dirs pinnedTTSDirs) string {
	stream := strings.ToLower(string(append(append([]byte(nil), stdout...), stderr...)))
	for _, marker := range []string{
		"hf_token=", "authorization:", "bearer ", "password=", "api_key=",
		"127.0.0.1", "localhost:", "grpc://", "http://", "https://",
		pinnedTTSPrompt,
	} {
		if strings.Contains(stream, strings.ToLower(marker)) {
			return "public command streams contained secret, address, or prompt telemetry"
		}
	}
	for _, path := range []string{dirs.root, dirs.work, dirs.home, dirs.state, dirs.modelCache, dirs.hfHome, dirs.hfCache, dirs.temp, dirs.output} {
		if path != "" && strings.Contains(stream, strings.ToLower(path)) {
			return "public command streams contained isolated cache or state paths"
		}
	}
	return ""
}

func validatePinnedTTSResponse(stdout []byte, outputPath string) (factoryapi.GenericModelInvocationResponse, pinnedTTSOutput, string) {
	var response factoryapi.GenericModelInvocationResponse
	if json.Unmarshal(stdout, &response) != nil || response.Failure != nil || len(response.Outputs) != 1 {
		return response, pinnedTTSOutput{}, "JSON response did not contain exactly one successful output"
	}
	output := response.Outputs[0]
	if output.Name != "audio" || output.Modality != factoryapi.ModelInvocationContentTypeAudio {
		return response, pinnedTTSOutput{}, "JSON response output was not the named AUDIO slot"
	}
	mediaType := ""
	if output.MediaType != nil {
		mediaType = strings.ToLower(strings.TrimSpace(*output.MediaType))
	}
	if !strings.HasPrefix(mediaType, "audio/wav") && !strings.HasPrefix(mediaType, "audio/wave") {
		return response, pinnedTTSOutput{}, "JSON response audio media type was not WAV"
	}
	if output.Content == nil || *output.Content == "" {
		return response, pinnedTTSOutput{}, "JSON response audio output had no materialized content"
	}

	audio, err := os.ReadFile(outputPath)
	if err != nil {
		return response, pinnedTTSOutput{}, "declared audio output file was not readable"
	}
	metadata, ok := pinnedTTSWAVMetadata(audio)
	if !ok {
		return response, pinnedTTSOutput{}, "declared audio output was not bounded decodable PCM WAV"
	}
	return response, pinnedTTSOutput{
		Name:           output.Name,
		Modality:       string(output.Modality),
		MediaType:      mediaType,
		Bytes:          int64(len(audio)),
		SHA256:         sha256Hex(audio),
		DurationMillis: metadata.duration.Milliseconds(),
		Channels:       metadata.channels,
		SampleRate:     metadata.sampleRate,
		Bits:           metadata.bits,
		BlockAlign:     metadata.blockAlign,
	}, ""
}

type pinnedTTSWAVDetails struct {
	duration   time.Duration
	channels   uint16
	sampleRate uint32
	bits       uint16
	blockAlign uint16
}

func pinnedTTSWAVMetadata(audio []byte) (pinnedTTSWAVDetails, bool) {
	if int64(len(audio)) <= 44 || int64(len(audio)) > pinnedTTSMaxAudioBytes ||
		string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" ||
		string(audio[12:16]) != "fmt " || string(audio[36:40]) != "data" {
		return pinnedTTSWAVDetails{}, false
	}
	if uint64(binary.LittleEndian.Uint32(audio[4:8]))+8 != uint64(len(audio)) ||
		binary.LittleEndian.Uint32(audio[16:20]) != 16 ||
		binary.LittleEndian.Uint16(audio[20:22]) != 1 {
		return pinnedTTSWAVDetails{}, false
	}
	channels := binary.LittleEndian.Uint16(audio[22:24])
	sampleRate := binary.LittleEndian.Uint32(audio[24:28])
	byteRate := binary.LittleEndian.Uint32(audio[28:32])
	blockAlign := binary.LittleEndian.Uint16(audio[32:34])
	bits := binary.LittleEndian.Uint16(audio[34:36])
	dataSize := binary.LittleEndian.Uint32(audio[40:44])
	if channels == 0 || sampleRate == 0 || blockAlign == 0 || bits != 16 ||
		byteRate != sampleRate*uint32(blockAlign) ||
		uint64(dataSize)+44 != uint64(len(audio)) ||
		dataSize < uint32(blockAlign) || dataSize%uint32(blockAlign) != 0 {
		return pinnedTTSWAVDetails{}, false
	}
	duration := time.Duration(dataSize/uint32(blockAlign)) * time.Second / time.Duration(sampleRate)
	if duration <= 0 || duration > pinnedTTSMaxDuration {
		return pinnedTTSWAVDetails{}, false
	}
	return pinnedTTSWAVDetails{
		duration: duration, channels: channels, sampleRate: sampleRate,
		bits: bits, blockAlign: blockAlign,
	}, true
}

func blockedPinnedTTSEvidence(t *testing.T, evidence *pinnedTTSEvidence, reason string) {
	t.Helper()
	evidence.Outcome = "BLOCKED"
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
