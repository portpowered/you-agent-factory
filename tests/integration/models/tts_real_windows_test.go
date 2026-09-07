package models_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	runPinnedTTSDiagnostic(t, &evidence, dirs, binaryPath)
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
	dirs.runtimeEvidencePath = filepath.Join(dirs.root, "runtime-evidence.jsonl")
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
		Isolation: pinnedTTSIsolation{
			FreshWorkingDirectory: true,
			FreshUserProfile:      true,
			FreshModelCache:       true,
			FreshHFCache:          true,
			FreshTempDirectory:    true,
			FreshOutputDirectory:  true,
			FreshPortState:        true,
			NetworkPolicy:         "one first-use call permits only the pinned public sources; no retry or cache-reuse probe",
		},
		Budgets: pinnedTTSBudgets{
			Disk:             "24 GiB",
			Download:         "4 GiB incremental",
			ChildProcesses:   4,
			ModelCalls:       1,
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
			"ASR semantic execution",
			"Factory journey and Project completion",
		},
	}, dirs
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
