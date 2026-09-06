package models_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
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

type pinnedASROutput struct {
	InputBytes        int64  `json:"inputBytes"`
	InputSHA256       string `json:"inputSha256"`
	Transcript        string `json:"transcript"`
	TranscriptBytes   int64  `json:"transcriptBytes"`
	TranscriptSHA256  string `json:"transcriptSha256"`
	SegmentsBytes     int64  `json:"segmentsBytes"`
	SegmentsSHA256    string `json:"segmentsSha256"`
	SegmentCount      int    `json:"segmentCount"`
	MediaType         string `json:"mediaType"`
	SegmentsMediaType string `json:"segmentsMediaType"`
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

type pinnedTTSCacheEvidence struct {
	TTSModel     string `json:"ttsModel"`
	TTSBackend   string `json:"ttsBackend"`
	ASRModel     string `json:"asrModel"`
	ASRBackend   string `json:"asrBackend"`
	ReuseNetwork string `json:"reuseNetwork"`
}

type pinnedTTSEvidence struct {
	Outcome        string                   `json:"outcome"`
	Blocker        string                   `json:"blocker,omitempty"`
	Platform       string                   `json:"platform"`
	Arch           string                   `json:"architecture"`
	Command        string                   `json:"command"`
	Binary         pinnedTTSFileIdentity    `json:"binary"`
	Model          pinnedTTSModelIdentity   `json:"model"`
	Backend        pinnedTTSBackendIdentity `json:"backend"`
	Isolation      pinnedTTSIsolation       `json:"isolation"`
	Budgets        pinnedTTSBudgets         `json:"budgets"`
	ReuseDownload  pinnedTTSReuseDownload   `json:"reuseDownload"`
	Execution      *pinnedTTSExecution      `json:"execution,omitempty"`
	ReuseExecution *pinnedTTSExecution      `json:"reuseExecution,omitempty"`
	ASRExecution   *pinnedTTSExecution      `json:"asrExecution,omitempty"`
	Output         *pinnedTTSOutput         `json:"output,omitempty"`
	ReuseOutput    *pinnedTTSOutput         `json:"reuseOutput,omitempty"`
	ASR            *pinnedASROutput         `json:"asr,omitempty"`
	ASRModel       pinnedTTSModelIdentity   `json:"asrModel"`
	ASRBackend     pinnedTTSBackendIdentity `json:"asrBackend"`
	Cleanup        pinnedTTSCleanup         `json:"cleanup"`
	Streams        pinnedTTSStreams         `json:"streams"`
	Caches         pinnedTTSCacheEvidence   `json:"caches"`
	Unproven       []string                 `json:"unproven"`
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
	return runPinnedTTSCommandWithNetwork(t, dirs, binaryPath, args, name, "")
}

func runPinnedTTSCommandWithNetwork(
	t *testing.T,
	dirs pinnedTTSDirs,
	binaryPath string,
	args []string,
	name string,
	networkProxy string,
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
	command.Env = pinnedTTSEnvironment(dirs, networkProxy)
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
	_ = stdoutFile.Close()
	_ = stderrFile.Close()
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

func pinnedTTSEnvironment(dirs pinnedTTSDirs, networkProxy string) []string {
	environment := story001Environment(dirs.home, dirs.modelCache, "")
	environment = withoutEnvironmentKeys(
		environment,
		"HF_HOME", "HUGGINGFACE_HUB_CACHE", run.ModelCacheDirEnvironment,
		"TEMP", "TMP", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME",
		"HF_TOKEN", "HUGGINGFACE_TOKEN", "HUGGINGFACE_HUB_TOKEN", "HF_ENDPOINT",
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy",
	)
	environment = append(environment,
		"HF_HOME="+dirs.hfHome,
		"HUGGINGFACE_HUB_CACHE="+dirs.hfCache,
		run.ModelCacheDirEnvironment+"="+dirs.modelCache,
		"TEMP="+dirs.temp,
		"TMP="+dirs.temp,
		"XDG_STATE_HOME="+dirs.state,
		"XDG_CACHE_HOME="+filepath.Join(dirs.home, "cache"),
		"XDG_CONFIG_HOME="+filepath.Join(dirs.home, "config"),
		"HF_HUB_DISABLE_TELEMETRY=1",
	)
	if networkProxy != "" {
		environment = append(environment,
			"HTTP_PROXY="+networkProxy,
			"HTTPS_PROXY="+networkProxy,
			"ALL_PROXY="+networkProxy,
			"http_proxy="+networkProxy,
			"https_proxy="+networkProxy,
			"all_proxy="+networkProxy,
			"NO_PROXY=",
			"no_proxy=",
		)
	}
	return environment
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
		"access_token=", "x-amz-signature=", "signed_url=", "127.0.0.1", "localhost:", "grpc://", "tcp://",
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

type pinnedASRSegment struct {
	ID    int32  `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
}

func validatePinnedASRResponse(
	stdout []byte, inputPath, transcriptPath, segmentsPath string, ttsOutput pinnedTTSOutput,
) (pinnedASROutput, string) {
	var response factoryapi.GenericModelInvocationResponse
	if json.Unmarshal(stdout, &response) != nil || response.Failure != nil || len(response.Outputs) != 2 {
		return pinnedASROutput{}, "ASR JSON response did not contain exactly two successful outputs"
	}
	transcriptOutput, segmentsOutput := response.Outputs[0], response.Outputs[1]
	if transcriptOutput.Name != "transcript" || transcriptOutput.Modality != factoryapi.ModelInvocationContentTypeText ||
		segmentsOutput.Name != "segments" || segmentsOutput.Modality != factoryapi.ModelInvocationContentTypeJSON {
		return pinnedASROutput{}, "ASR JSON response outputs were not ordered transcript and segments slots"
	}
	transcriptMedia := pinnedResponseMediaType(transcriptOutput.MediaType)
	segmentsMedia := pinnedResponseMediaType(segmentsOutput.MediaType)
	if transcriptMedia != "text/plain" || segmentsMedia != "application/json" {
		return pinnedASROutput{}, "ASR JSON response media types were not text/plain and application/json"
	}
	transcript, err := os.ReadFile(transcriptPath)
	if err != nil {
		return pinnedASROutput{}, "ASR transcript output was not readable"
	}
	if normalizePinnedTranscript(string(transcript)) != normalizePinnedTranscript(pinnedTTSPrompt) {
		return pinnedASROutput{}, "ASR transcript was not semantically equivalent to the TTS prompt"
	}
	segmentsBody, err := os.ReadFile(segmentsPath)
	if err != nil {
		return pinnedASROutput{}, "ASR segments output was not readable"
	}
	var segments []pinnedASRSegment
	if err := decodePinnedJSON(segmentsBody, &segments); err != nil || len(segments) == 0 {
		return pinnedASROutput{}, "ASR segments output was not one nonempty JSON array"
	}
	var previousStart, previousEnd int64
	var segmentText strings.Builder
	for index, segment := range segments {
		if segment.ID < 0 || segment.Start < 0 || segment.End <= segment.Start ||
			segment.End > ttsOutput.DurationMillis || strings.TrimSpace(segment.Text) == "" ||
			(index > 0 && (segment.Start < previousStart || segment.End < previousEnd)) {
			return pinnedASROutput{}, "ASR segments were not finite, nonnegative, monotonic, and duration-bounded"
		}
		if segmentText.Len() > 0 {
			segmentText.WriteByte(' ')
		}
		segmentText.WriteString(segment.Text)
		previousStart, previousEnd = segment.Start, segment.End
	}
	if !strings.Contains(normalizePinnedTranscript(segmentText.String()), normalizePinnedTranscript(pinnedTTSPrompt)) {
		return pinnedASROutput{}, "ASR segment text was not semantically equivalent to the TTS prompt"
	}
	audio, err := os.ReadFile(inputPath)
	if err != nil {
		return pinnedASROutput{}, "TTS WAV was not available for exact-byte ASR identity"
	}
	return pinnedASROutput{
		InputBytes:        int64(len(audio)),
		InputSHA256:       sha256Hex(audio),
		Transcript:        strings.TrimSpace(string(transcript)),
		TranscriptBytes:   int64(len(transcript)),
		TranscriptSHA256:  sha256Hex(transcript),
		SegmentsBytes:     int64(len(segmentsBody)),
		SegmentsSHA256:    sha256Hex(segmentsBody),
		SegmentCount:      len(segments),
		MediaType:         transcriptMedia,
		SegmentsMediaType: segmentsMedia,
	}, ""
}

func pinnedResponseMediaType(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(strings.SplitN(*value, ";", 2)[0]))
}

func normalizePinnedTranscript(value string) string {
	var normalized strings.Builder
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			normalized.WriteRune(character)
			continue
		}
		normalized.WriteByte(' ')
	}
	return strings.Join(strings.Fields(normalized.String()), " ")
}

func decodePinnedJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
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

func preparePinnedTTSBinary(t *testing.T, evidence *pinnedTTSEvidence) (string, bool) {
	t.Helper()
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		blockedPinnedTTSEvidence(t, evidence, "Windows amd64 host is required")
		return "", false
	}
	binaryPath := strings.TrimSpace(os.Getenv(pinnedTTSBinaryEnvironment))
	evidence.Binary.Configured = binaryPath != ""
	if binaryPath == "" {
		blockedPinnedTTSEvidence(t, evidence, "immutable final-head binary handoff is not configured")
		return "", false
	}
	if !filepath.IsAbs(binaryPath) || !strings.EqualFold(filepath.Ext(binaryPath), ".exe") {
		blockedPinnedTTSEvidence(t, evidence, "immutable binary handoff must be an absolute Windows executable")
		return "", false
	}
	identity, ok := readPinnedTTSFileIdentity(binaryPath)
	if !ok {
		blockedPinnedTTSEvidence(t, evidence, "immutable binary handoff is not a readable regular file")
		return "", false
	}
	evidence.Binary = identity
	if testing.Short() {
		blockedPinnedTTSEvidence(t, evidence, "short integration lane does not run the heavyweight Windows witness")
		return "", false
	}
	return binaryPath, true
}

func runPinnedTTSASRChain(t *testing.T, evidence *pinnedTTSEvidence, dirs pinnedTTSDirs, binaryPath string) {
	t.Helper()
	initiallyEmpty := pinnedTTSCacheDirectoriesEmpty(dirs.modelCache, dirs.hfCache)
	output, beforeModel, beforeHF, ok := runPinnedTTSFirstUse(t, evidence, dirs, binaryPath, initiallyEmpty)
	if !ok {
		return
	}
	proxy, err := pinnedTTSBlockedProxy(t)
	if err != nil {
		failedPinnedTTSEvidence(t, evidence, "could not configure the cache-reuse network-failure probe")
		return
	}
	if _, ok := runPinnedTTSCacheReuse(t, evidence, dirs, binaryPath, proxy, beforeModel, beforeHF); !ok {
		return
	}
	asrOutput, ok := runPinnedTTSExactASR(t, evidence, dirs, binaryPath, output)
	if !ok {
		return
	}
	if asrOutput.InputSHA256 != output.SHA256 {
		failedPinnedTTSEvidence(t, evidence, "ASR input digest did not match the first-use TTS WAV")
		return
	}
	for _, root := range []string{dirs.modelCache, dirs.hfCache, dirs.output, dirs.state} {
		if snapshot := inspectPinnedTree(t, root); snapshot.HasPartial() {
			failedPinnedTTSEvidence(t, evidence, "exact-byte chain left a partial artifact or output")
			return
		}
	}
	evidence.Caches = pinnedTTSCacheEvidence{
		TTSModel:     evidence.ReuseDownload.Model,
		TTSBackend:   evidence.ReuseDownload.Backend,
		ASRModel:     "downloaded",
		ASRBackend:   "downloaded",
		ReuseNetwork: "blocked-proxy probe; zero cache changes and no artifact download",
	}
	evidence.Cleanup.Checked = evidence.Cleanup.ProcessTreeClosed &&
		!evidence.Cleanup.OwnedProcessRemaining && !evidence.Cleanup.OwnedListenerRemaining
	evidence.Cleanup.Observation = "all three supervised command trees exited; no partial cache/output entries remain"
	evidence.Outcome = "PASS"
	recordPinnedTTSEvidence(t, evidence)
}

func runPinnedTTSFirstUse(
	t *testing.T,
	evidence *pinnedTTSEvidence,
	dirs pinnedTTSDirs,
	binaryPath string,
	initiallyEmpty bool,
) (pinnedTTSOutput, pinnedTreeSnapshot, pinnedTreeSnapshot, bool) {
	t.Helper()
	first := runPinnedTTSCommand(t, dirs, binaryPath, []string{
		"--json", "models", "invoke", "tts", "--operation", "TTS",
		"--input", "text=" + pinnedTTSPrompt, "--output", "audio=" + dirs.outputPath,
	}, "tts-first")
	evidence.Execution = first.evidence()
	evidence.Cleanup = first.cleanupEvidence()
	if first.setupFailure {
		failedPinnedTTSEvidence(t, evidence, "real witness process supervision could not be attached")
		return pinnedTTSOutput{}, pinnedTreeSnapshot{}, pinnedTreeSnapshot{}, false
	}
	if first.timedOut {
		failedPinnedTTSEvidence(t, evidence, "real witness exceeded the three-hour bound")
		return pinnedTTSOutput{}, pinnedTreeSnapshot{}, pinnedTreeSnapshot{}, false
	}
	if reason := pinnedTTSStreamViolation(first.stdout, first.stderr, dirs); reason != "" {
		failedPinnedTTSEvidence(t, evidence, reason)
		return pinnedTTSOutput{}, pinnedTreeSnapshot{}, pinnedTreeSnapshot{}, false
	}
	if first.exitCode != 0 {
		if pinnedTTSDependencyFailure(first.stdout, first.stderr) {
			blockedPinnedTTSEvidence(t, evidence, "pinned model or backend dependency was unavailable")
			return pinnedTTSOutput{}, pinnedTreeSnapshot{}, pinnedTreeSnapshot{}, false
		}
		failedPinnedTTSEvidence(t, evidence, "real pinned TTS command exited unsuccessfully")
		return pinnedTTSOutput{}, pinnedTreeSnapshot{}, pinnedTreeSnapshot{}, false
	}
	_, output, reason := validatePinnedTTSResponse(first.stdout, dirs.outputPath)
	if reason != "" {
		failedPinnedTTSEvidence(t, evidence, reason)
		return pinnedTTSOutput{}, pinnedTreeSnapshot{}, pinnedTreeSnapshot{}, false
	}
	evidence.Output = &output
	model, modelOK := inspectPinnedExactModelCache(
		dirs.modelCache, pinnedTTSManagedModelDir, pinnedTTSModelRevision, pinnedTTSModelArtifacts,
	)
	backend, backendOK := inspectPinnedBackendCache(
		dirs.modelCache, pinnedTTSBackendArchive, pinnedTTSBackendBytes, pinnedTTSBackendSHA256,
	)
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
		failedPinnedTTSEvidence(t, evidence, "pinned cache identity could not be verified")
		return pinnedTTSOutput{}, pinnedTreeSnapshot{}, pinnedTreeSnapshot{}, false
	}
	return output, inspectPinnedTree(t, dirs.modelCache), inspectPinnedTree(t, dirs.hfCache), true
}

func runPinnedTTSCacheReuse(
	t *testing.T,
	evidence *pinnedTTSEvidence,
	dirs pinnedTTSDirs,
	binaryPath, proxy string,
	beforeModel, beforeHF pinnedTreeSnapshot,
) (pinnedTTSOutput, bool) {
	t.Helper()
	second := runPinnedTTSCommandWithNetwork(t, dirs, binaryPath, []string{
		"--json", "models", "invoke", "tts", "--operation", "TTS",
		"--input", "text=" + pinnedTTSPrompt, "--output", dirs.reuseOutputPath,
	}, "tts-cache-reuse", proxy)
	evidence.ReuseExecution = second.evidence()
	evidence.Cleanup = mergePinnedTTSCleanup(evidence.Cleanup, second.cleanupEvidence())
	if second.setupFailure || second.timedOut {
		failedPinnedTTSEvidence(t, evidence, "cache-reuse process supervision did not complete")
		return pinnedTTSOutput{}, false
	}
	if reason := pinnedTTSStreamViolation(second.stdout, second.stderr, dirs); reason != "" {
		failedPinnedTTSEvidence(t, evidence, reason)
		return pinnedTTSOutput{}, false
	}
	if second.exitCode != 0 {
		failedPinnedTTSEvidence(t, evidence, "cache-reuse TTS unexpectedly contacted or failed to use the immutable cache")
		return pinnedTTSOutput{}, false
	}
	_, output, reason := validatePinnedTTSResponse(second.stdout, dirs.reuseOutputPath)
	if reason != "" {
		failedPinnedTTSEvidence(t, evidence, "cache-reuse "+reason)
		return pinnedTTSOutput{}, false
	}
	evidence.ReuseOutput = &output
	if after := inspectPinnedTree(t, dirs.modelCache); !beforeModel.Equal(after) {
		failedPinnedTTSEvidence(t, evidence, "cache-reuse changed the managed model/backend cache")
		return pinnedTTSOutput{}, false
	}
	if after := inspectPinnedTree(t, dirs.hfCache); !beforeHF.Equal(after) {
		failedPinnedTTSEvidence(t, evidence, "cache-reuse changed the Hugging Face cache")
		return pinnedTTSOutput{}, false
	}
	return output, true
}

func runPinnedTTSExactASR(
	t *testing.T,
	evidence *pinnedTTSEvidence,
	dirs pinnedTTSDirs,
	binaryPath string,
	ttsOutput pinnedTTSOutput,
) (pinnedASROutput, bool) {
	t.Helper()
	asr := runPinnedTTSCommand(t, dirs, binaryPath, []string{
		"--json", "models", "invoke", "asr", "--operation", "ASR",
		"--input", "audio=@" + dirs.outputPath,
		"--output", "transcript=" + dirs.transcriptPath,
		"--output", "segments=" + dirs.segmentsPath,
	}, "asr-exact-bytes")
	evidence.ASRExecution = asr.evidence()
	evidence.Cleanup = mergePinnedTTSCleanup(evidence.Cleanup, asr.cleanupEvidence())
	if asr.setupFailure || asr.timedOut {
		failedPinnedTTSEvidence(t, evidence, "exact-byte ASR process supervision did not complete")
		return pinnedASROutput{}, false
	}
	if reason := pinnedTTSStreamViolation(asr.stdout, asr.stderr, dirs); reason != "" {
		failedPinnedTTSEvidence(t, evidence, reason)
		return pinnedASROutput{}, false
	}
	if asr.exitCode != 0 {
		if pinnedTTSDependencyFailure(asr.stdout, asr.stderr) {
			blockedPinnedTTSEvidence(t, evidence, "pinned ASR model or backend dependency was unavailable")
			return pinnedASROutput{}, false
		}
		failedPinnedTTSEvidence(t, evidence, "exact-byte ASR command exited unsuccessfully")
		return pinnedASROutput{}, false
	}
	output, reason := validatePinnedASRResponse(
		asr.stdout, dirs.outputPath, dirs.transcriptPath, dirs.segmentsPath, ttsOutput,
	)
	if reason != "" {
		failedPinnedTTSEvidence(t, evidence, reason)
		return pinnedASROutput{}, false
	}
	evidence.ASR = &output
	asrModel, asrModelOK := inspectPinnedExactModelCache(
		dirs.modelCache, pinnedASRManagedModelDir, pinnedASRModelRevision, pinnedASRModelArtifacts,
	)
	asrBackend, asrBackendOK := inspectPinnedBackendCache(
		dirs.modelCache, pinnedASRBackendArchive, pinnedASRBackendBytes, pinnedASRBackendSHA256,
	)
	evidence.ASRModel = asrModel
	evidence.ASRBackend.ObservedBytes = asrBackend.ObservedBytes
	evidence.ASRBackend.ObservedSHA256 = asrBackend.ObservedSHA256
	evidence.ASRBackend.Verified = asrBackendOK
	if !asrModelOK || !asrBackendOK {
		failedPinnedTTSEvidence(t, evidence, "pinned ASR cache identity could not be verified")
		return pinnedASROutput{}, false
	}
	return output, true
}
