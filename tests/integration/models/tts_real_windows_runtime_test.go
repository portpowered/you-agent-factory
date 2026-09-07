//go:build windows

package models_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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

func pinnedTTSEnvironment(dirs pinnedTTSDirs) []string {
	environment := story001Environment(dirs.home, dirs.modelCache, "")
	environment = withoutEnvironmentKeys(environment,
		"HF_HOME", "HUGGINGFACE_HUB_CACHE", run.ModelCacheDirEnvironment,
		"TEMP", "TMP", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME",
		"HF_TOKEN", "HUGGINGFACE_TOKEN", "HUGGINGFACE_HUB_TOKEN", "HF_ENDPOINT",
		pinnedTTSRuntimeEvidenceEnv, pinnedTTSEvidenceEnvironment,
	)
	return append(environment,
		"HF_HOME="+dirs.hfHome,
		"HUGGINGFACE_HUB_CACHE="+dirs.hfCache,
		run.ModelCacheDirEnvironment+"="+dirs.modelCache,
		"TEMP="+dirs.temp,
		"TMP="+dirs.temp,
		"XDG_STATE_HOME="+dirs.state,
		"XDG_CACHE_HOME="+filepath.Join(dirs.home, "cache"),
		"XDG_CONFIG_HOME="+filepath.Join(dirs.home, "config"),
		"HF_HUB_DISABLE_TELEMETRY=1",
		pinnedTTSRuntimeEvidenceEnv+"="+dirs.runtimeEvidencePath,
	)
}

func (result pinnedCommandResult) evidence() *pinnedTTSExecution {
	return &pinnedTTSExecution{
		Started:             result.started,
		ProcessExited:       result.processExited,
		ExitCode:            result.exitCode,
		TimedOut:            result.timedOut,
		ChildProcesses:      1,
		ModelCalls:          1,
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
		Observation:            "the supervised CLI process tree exited; the child evidence must also contain its matching exit before cleanup is accepted",
	}
}

func runPinnedTTSDiagnostic(
	t *testing.T,
	evidence *pinnedTTSEvidence,
	dirs pinnedTTSDirs,
	binaryPath string,
) {
	t.Helper()
	beforeModel := inspectPinnedTree(t, dirs.modelCache)
	beforeHF := inspectPinnedTree(t, dirs.hfCache)
	evidence.ReuseDownload.FreshCacheAtStart = pinnedTTSCacheDirectoriesEmpty(dirs.modelCache, dirs.hfCache)

	result := runPinnedTTSCommand(t, dirs, binaryPath, []string{
		"--json", "models", "invoke", "tts", "--operation", "TTS",
		"--input", "text=" + pinnedTTSPrompt, "--output", "audio=" + dirs.outputPath,
	}, "tts-diagnostic")
	evidence.Execution = result.evidence()
	evidence.Cleanup = result.cleanupEvidence()
	updatePinnedTTSCacheEvidence(t, evidence, dirs, beforeModel, beforeHF)

	body, err := os.ReadFile(dirs.runtimeEvidencePath)
	if err != nil {
		failedPinnedTTSEvidence(t, evidence, "private runtime evidence was not produced")
		return
	}
	observation, err := decodePinnedTTSRuntimeEvidence(body)
	if err != nil {
		failedPinnedTTSEvidence(t, evidence, "private runtime evidence was invalid: "+err.Error())
		return
	}
	evidence.StageTrace = observation.StageTrace
	evidence.Failure = observation.Failure
	evidence.ChildEnvironment = observation.ChildEnvironment
	if err := validatePinnedTTSChildCleanup(observation); err != nil {
		failedPinnedTTSEvidence(t, evidence, err.Error())
		return
	}
	if reason := pinnedTTSStreamViolation(result.stdout, result.stderr, dirs); reason != "" {
		failedPinnedTTSEvidence(t, evidence, reason)
		return
	}
	if result.setupFailure || result.timedOut || !result.processExited || !result.processTreeClosed {
		failedPinnedTTSEvidence(t, evidence, "real witness process supervision did not complete within the declared budget")
		return
	}

	switch observation.TerminalOutcome {
	case "FAILED":
		if pinnedTTSDependencyFailure(result.stdout, result.stderr) &&
			!evidence.Model.Verified && !evidence.Backend.Verified {
			blockedPinnedTTSEvidence(t, evidence, "pinned model or backend dependency was unavailable")
			return
		}
		evidence.Outcome = "FAIL"
		evidence.Blocker = "one bounded runtime terminal failure was observed"
		evidence.Cleanup.Checked = evidence.Cleanup.ProcessTreeClosed &&
			!evidence.Cleanup.OwnedProcessRemaining && !evidence.Cleanup.OwnedListenerRemaining
		recordPinnedTTSEvidence(t, evidence)
		t.Logf("bounded Windows TTS failure recorded at %s", evidence.Failure.Stage)
		return
	case "COMPLETED":
		if result.exitCode != 0 {
			failedPinnedTTSEvidence(t, evidence, "CLI exited unsuccessfully after recording a completed runtime")
			return
		}
		if len(observation.ChildEnvironment) == 0 {
			failedPinnedTTSEvidence(t, evidence, "completed TTS runtime did not record the managed child boundary")
			return
		}
	default:
		failedPinnedTTSEvidence(t, evidence, "runtime evidence did not contain a terminal outcome")
		return
	}

	if !evidence.Model.Verified || !evidence.Backend.Verified {
		failedPinnedTTSEvidence(t, evidence, "pinned cache identity could not be verified")
		return
	}
	_, output, reason := validatePinnedTTSResponse(result.stdout, dirs.outputPath)
	if reason != "" {
		failedPinnedTTSEvidence(t, evidence, reason)
		return
	}
	evidence.Output = &output
	evidence.Cleanup.Checked = evidence.Cleanup.ProcessTreeClosed &&
		!evidence.Cleanup.OwnedProcessRemaining && !evidence.Cleanup.OwnedListenerRemaining
	if !evidence.Cleanup.Checked {
		failedPinnedTTSEvidence(t, evidence, "owned process or listener cleanup was not proven")
		return
	}
	evidence.Outcome = "PASS"
	evidence.Blocker = ""
	recordPinnedTTSEvidence(t, evidence)
}

func updatePinnedTTSCacheEvidence(
	t *testing.T,
	evidence *pinnedTTSEvidence,
	dirs pinnedTTSDirs,
	beforeModel, beforeHF pinnedTreeSnapshot,
) {
	t.Helper()
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
	afterModel := inspectPinnedTree(t, dirs.modelCache)
	afterHF := inspectPinnedTree(t, dirs.hfCache)
	evidence.ReuseDownload.Model = pinnedTTSCacheDisposition(evidence.ReuseDownload.FreshCacheAtStart, modelOK)
	evidence.ReuseDownload.Backend = pinnedTTSCacheDisposition(evidence.ReuseDownload.FreshCacheAtStart, backendOK)
	evidence.Caches = pinnedTTSCacheEvidence{
		TTSModel: evidence.ReuseDownload.Model, TTSBackend: evidence.ReuseDownload.Backend,
		ModelCacheEntries: len(afterModel.Entries), HFCacheEntries: len(afterHF.Entries),
		PartialArtifacts:      afterModel.HasPartial() || afterHF.HasPartial(),
		ModelCacheEntriesPrev: len(beforeModel.Entries), HFCacheEntriesPrev: len(beforeHF.Entries),
	}
}

func validatePinnedTTSChildCleanup(observation pinnedTTSRuntimeObservation) error {
	var started, exited *pinnedTTSChildEnvironmentEvidence
	for index := range observation.ChildEnvironment {
		child := &observation.ChildEnvironment[index]
		switch child.Phase {
		case "PROCESS_STARTED":
			if started != nil {
				return errors.New("runtime evidence contained multiple managed child start records")
			}
			started = child
		case "PROCESS_EXITED":
			if exited != nil {
				return errors.New("runtime evidence contained multiple managed child exit records")
			}
			exited = child
		}
	}
	if started == nil {
		return nil
	}
	if exited == nil || exited.ProcessID != started.ProcessID || exited.Backend != started.Backend {
		return errors.New("managed child start and exit evidence did not identify one process")
	}
	if started.Backend != pinnedTTSBackendID || started.ProcessID <= 0 || exited.ProcessID <= 0 {
		return errors.New("managed child evidence did not identify the retained VibeVoice process")
	}
	if exited.ExitClass != "EXITED" && exited.ExitClass != "NONZERO_EXIT" && exited.ExitClass != "WAIT_FAILED" {
		return errors.New("managed child evidence contained an unbounded exit class")
	}
	return nil
}

func decodePinnedTTSRuntimeEvidence(body []byte) (pinnedTTSRuntimeObservation, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return pinnedTTSRuntimeObservation{}, errors.New("evidence file was empty")
	}
	if reason := pinnedTTSForbiddenEvidenceMarker(body); reason != "" {
		return pinnedTTSRuntimeObservation{}, errors.New(reason)
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	observation := pinnedTTSRuntimeObservation{}
	expectedSequence := uint64(1)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var record pinnedTTSRecordedRuntime
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return pinnedTTSRuntimeObservation{}, fmt.Errorf("decode sequence %d: %w", expectedSequence, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return pinnedTTSRuntimeObservation{}, fmt.Errorf("sequence %d contained trailing JSON", expectedSequence)
		}
		if record.Sequence != expectedSequence {
			return pinnedTTSRuntimeObservation{}, fmt.Errorf("evidence sequence = %d, want %d", record.Sequence, expectedSequence)
		}
		expectedSequence++
		switch record.Kind {
		case "STAGE":
			if err := validatePinnedTTSRuntimeRecord(record); err != nil {
				return pinnedTTSRuntimeObservation{}, err
			}
			observation.StageTrace = append(observation.StageTrace, pinnedTTSStageEvidence{
				Stage: record.Stage, Outcome: record.Outcome,
				FailureClass: record.FailureClass, DurationMillis: record.DurationMillis,
			})
		case "TERMINAL":
			if observation.TerminalOutcome != "" {
				return pinnedTTSRuntimeObservation{}, errors.New("runtime evidence contained more than one terminal record")
			}
			if err := validatePinnedTTSRuntimeRecord(record); err != nil {
				return pinnedTTSRuntimeObservation{}, err
			}
			observation.TerminalOutcome = record.Outcome
			if record.Outcome == "FAILED" {
				observation.Failure = &pinnedTTSFailureEvidence{
					Stage: record.Stage, Class: record.FailureClass, CauseSHA256: record.CauseSHA256,
				}
			}
		case "MANAGED_CHILD":
			if record.ProcessID <= 0 || (record.Phase != "PROCESS_STARTED" && record.Phase != "PROCESS_EXITED") {
				return pinnedTTSRuntimeObservation{}, errors.New("managed child evidence had an invalid process observation")
			}
			child := pinnedTTSChildEnvironmentEvidence{
				Sequence: record.Sequence, Kind: record.Kind, Backend: record.Backend,
				ProcessID: record.ProcessID, Phase: record.Phase,
				Environment: append([]pinnedTTSManagedEnvironmentFact(nil), record.Environment...),
				ExitClass:   record.ExitClass,
			}
			if record.Phase == "PROCESS_STARTED" {
				if err := validatePinnedTTSManagedEnvironment(child.Environment); err != nil {
					return pinnedTTSRuntimeObservation{}, err
				}
			}
			observation.ChildEnvironment = append(observation.ChildEnvironment, child)
		default:
			return pinnedTTSRuntimeObservation{}, fmt.Errorf("runtime evidence kind %q was not bounded", record.Kind)
		}
	}
	if err := scanner.Err(); err != nil {
		return pinnedTTSRuntimeObservation{}, fmt.Errorf("read evidence: %w", err)
	}
	if len(observation.StageTrace) == 0 {
		return pinnedTTSRuntimeObservation{}, errors.New("runtime evidence contained no stage trace")
	}
	if observation.TerminalOutcome == "" {
		return pinnedTTSRuntimeObservation{}, errors.New("runtime evidence contained no terminal decision")
	}
	return observation, nil
}

func validatePinnedTTSRuntimeRecord(record pinnedTTSRecordedRuntime) error {
	if !pinnedTTSRuntimeStage(record.Stage) {
		return fmt.Errorf("runtime evidence stage %q was not bounded", record.Stage)
	}
	if record.Outcome != "COMPLETED" && record.Outcome != "FAILED" {
		return fmt.Errorf("runtime evidence outcome %q was not bounded", record.Outcome)
	}
	if record.DurationMillis < 0 {
		return errors.New("runtime evidence duration was negative")
	}
	if record.Outcome == "COMPLETED" {
		if record.FailureClass != "" || record.CauseSHA256 != "" {
			return errors.New("completed runtime evidence carried failure details")
		}
		return nil
	}
	if !pinnedTTSRuntimeFailureClass(record.FailureClass) || !pinnedTTSRuntimeCauseSHA256(record.CauseSHA256) {
		return errors.New("failed runtime evidence did not carry one bounded class and cause digest")
	}
	return nil
}

func pinnedTTSRuntimeStage(stage string) bool {
	switch stage {
	case "ARTIFACT_RESOLVE", "ARTIFACT_DOWNLOAD", "ARTIFACT_DIGEST", "BACKEND_EXTRACT", "BACKEND_START", "PROTOCOL_LOAD", "INVOKE":
		return true
	default:
		return false
	}
}

func pinnedTTSRuntimeFailureClass(class string) bool {
	switch class {
	case "UNAVAILABLE", "INVALID_ARTIFACT", "INTEGRITY_MISMATCH", "EXTRACTION_FAILED", "PROCESS_START_FAILED", "PROCESS_EXITED", "PROTOCOL_INCOMPATIBLE", "INVOCATION_FAILED", "MALFORMED_RESPONSE", "CANCELLED", "TIMED_OUT":
		return true
	default:
		return false
	}
}

func pinnedTTSRuntimeCauseSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validatePinnedTTSManagedEnvironment(facts []pinnedTTSManagedEnvironmentFact) error {
	wanted := map[string]bool{"PATH": false, "TEMP": false, "TMP": false, "VIBEVOICECPP_LIBRARY": false}
	for _, fact := range facts {
		name := strings.ToUpper(strings.TrimSpace(fact.Name))
		if _, ok := wanted[name]; !ok || wanted[name] {
			return fmt.Errorf("managed child environment fact %q was duplicated or not bounded", fact.Name)
		}
		wanted[name] = true
		if !fact.Present || !pinnedTTSRuntimeCauseSHA256(fact.ValueSHA256) {
			return fmt.Errorf("managed child environment fact %q was not present with a bounded digest", fact.Name)
		}
	}
	for name, present := range wanted {
		if !present {
			return fmt.Errorf("managed child environment omitted %s", name)
		}
	}
	return nil
}

func pinnedTTSForbiddenEvidenceMarker(body []byte) string {
	value := strings.ToLower(string(body))
	for _, marker := range []string{
		"hf_token=", "authorization:", "bearer ", "password=", "api_key=", "access_token=",
		"x-amz-signature=", "signed_url=", "grpc://", "tcp://", "127.0.0.1", "localhost:",
		pinnedTTSPrompt, "raw error", "raw_cause", "error_message", "stack_trace",
	} {
		if strings.Contains(value, strings.ToLower(marker)) {
			return "private runtime evidence contained a forbidden raw value"
		}
	}
	return ""
}

func pinnedTTSDependencyFailure(stdout, stderr []byte) bool {
	stream := strings.ToLower(string(append(append([]byte(nil), stdout...), stderr...)))
	for _, marker := range []string{
		"source fetch failed", "asset source", "backend artifact", "required assets missing",
		"model source", "cannot download", "offline cache", "network is unavailable",
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
		"hf_token=", "authorization:", "bearer ", "password=", "api_key=", "access_token=",
		"x-amz-signature=", "signed_url=", "127.0.0.1", "localhost:", "grpc://", "tcp://",
		pinnedTTSPrompt,
	} {
		if strings.Contains(stream, strings.ToLower(marker)) {
			return "public command streams contained secret, address, or prompt telemetry"
		}
	}
	if bytes.Contains(stdout, []byte("RIFF")) || bytes.Contains(stderr, []byte("RIFF")) {
		return "public command streams contained raw audio bytes"
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
		Name: output.Name, Modality: string(output.Modality), MediaType: mediaType,
		Bytes: int64(len(audio)), SHA256: sha256Hex(audio), DurationMillis: metadata.duration.Milliseconds(),
		Channels: metadata.channels, SampleRate: metadata.sampleRate, Bits: metadata.bits, BlockAlign: metadata.blockAlign,
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
		binary.LittleEndian.Uint32(audio[16:20]) != 16 || binary.LittleEndian.Uint16(audio[20:22]) != 1 {
		return pinnedTTSWAVDetails{}, false
	}
	channels := binary.LittleEndian.Uint16(audio[22:24])
	sampleRate := binary.LittleEndian.Uint32(audio[24:28])
	byteRate := binary.LittleEndian.Uint32(audio[28:32])
	blockAlign := binary.LittleEndian.Uint16(audio[32:34])
	bits := binary.LittleEndian.Uint16(audio[34:36])
	dataSize := binary.LittleEndian.Uint32(audio[40:44])
	if channels == 0 || sampleRate == 0 || blockAlign == 0 || bits != 16 ||
		byteRate != sampleRate*uint32(blockAlign) || uint64(dataSize)+44 != uint64(len(audio)) ||
		dataSize < uint32(blockAlign) || dataSize%uint32(blockAlign) != 0 {
		return pinnedTTSWAVDetails{}, false
	}
	duration := time.Duration(dataSize/uint32(blockAlign)) * time.Second / time.Duration(sampleRate)
	if duration <= 0 || duration > pinnedTTSMaxDuration {
		return pinnedTTSWAVDetails{}, false
	}
	return pinnedTTSWAVDetails{duration: duration, channels: channels, sampleRate: sampleRate, bits: bits, blockAlign: blockAlign}, true
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
	commit, tree, ok := readPinnedTTSBuildIdentity(t)
	if !ok {
		blockedPinnedTTSEvidence(t, evidence, "source commit/tree identity could not be recorded")
		return "", false
	}
	identity.Path = filepath.Clean(binaryPath)
	identity.Commit = commit
	identity.Tree = tree
	evidence.Binary = identity
	if testing.Short() {
		blockedPinnedTTSEvidence(t, evidence, "short integration lane does not run the heavyweight Windows witness")
		return "", false
	}
	return binaryPath, true
}

func readPinnedTTSBuildIdentity(t *testing.T) (string, string, bool) {
	t.Helper()
	root := testutil.MustRepoRoot(t)
	read := func(args ...string) (string, bool) {
		command := exec.CommandContext(t.Context(), "git", args...)
		command.Dir = root
		output, err := command.Output()
		return strings.TrimSpace(string(output)), err == nil
	}
	commit, commitOK := read("rev-parse", "--verify", "HEAD")
	tree, treeOK := read("rev-parse", "--verify", "HEAD^{tree}")
	return commit, tree, commitOK && treeOK && commit != "" && tree != ""
}
