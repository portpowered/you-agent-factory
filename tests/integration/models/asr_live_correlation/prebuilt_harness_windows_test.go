//go:build windows && managed_process_integration

package asrlivecorrelation_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	harnessPathEnvironment      = "INFINITE_YOU_ASR_LIVE_CORRELATION_HARNESS"
	harnessSHAEnvironment       = "INFINITE_YOU_ASR_LIVE_CORRELATION_HARNESS_SHA256"
	responseManifestEnv         = "INFINITE_YOU_ASR_LIVE_CORRELATION_RESPONSE_FIRST_MANIFEST"
	exitFirstManifestEnv        = "INFINITE_YOU_ASR_LIVE_CORRELATION_EXIT_FIRST_MANIFEST"
	manifestEnvironment         = "INFINITE_YOU_ASR_LIVE_CORRELATION_MANIFEST"
	harnessTestBinaryEntrypoint = "^TestASRLiveCorrelationCompiledHarness$"
)

type harnessManifestHeader struct {
	Scenario       string `json:"scenario"`
	EvidenceOutput string `json:"evidence_output"`
}

// TestASRLiveCorrelationConsumesOnePrebuiltHarness owns the integration
// scenario. The helper binary is compiled once and passed in by Make; this
// driver runs the two purpose-distinct manifests serially and starts the
// exit-first call only after the response-first call produced evidence.
func TestASRLiveCorrelationConsumesOnePrebuiltHarness(t *testing.T) {
	binaryPath, binarySHA := requirePrebuiltHarness(t)
	responseManifest := strings.TrimSpace(os.Getenv(responseManifestEnv))
	exitFirstManifest := strings.TrimSpace(os.Getenv(exitFirstManifestEnv))
	if responseManifest == "" && exitFirstManifest == "" {
		runHarness(t, binaryPath, "")
		return
	}
	if responseManifest == "" || exitFirstManifest == "" {
		t.Fatal("both purpose-distinct ASR manifests are required")
	}
	response := validateManifestHeader(t, responseManifest, "response_first")
	exitFirst := validateManifestHeader(t, exitFirstManifest, "exit_first_after_rpc_terminal")
	runHarness(t, binaryPath, responseManifest)
	assertEvidenceWritten(t, response.EvidenceOutput)
	runHarness(t, binaryPath, exitFirstManifest)
	assertEvidenceWritten(t, exitFirst.EvidenceOutput)
	t.Logf("ASR live-correlation integration used prebuilt harness sha256=%s", binarySHA)
}

func requirePrebuiltHarness(t *testing.T) (string, string) {
	t.Helper()
	binaryPath := strings.TrimSpace(os.Getenv(harnessPathEnvironment))
	wantSHA := strings.TrimSpace(os.Getenv(harnessSHAEnvironment))
	if binaryPath == "" || wantSHA == "" || !filepath.IsAbs(binaryPath) || len(wantSHA) != sha256.Size*2 {
		t.Skip("ASR live-correlation prebuilt harness is not configured")
	}
	for _, character := range wantSHA {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			t.Fatal("ASR live-correlation harness digest is invalid")
		}
	}
	contents, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read immutable ASR live-correlation harness: %v", err)
	}
	digest := sha256.Sum256(contents)
	actualSHA := hex.EncodeToString(digest[:])
	if actualSHA != wantSHA {
		t.Fatal("ASR live-correlation harness identity does not match its supplied digest")
	}
	return binaryPath, actualSHA
}

func validateManifestHeader(t *testing.T, path, wantScenario string) harnessManifestHeader {
	t.Helper()
	if !filepath.IsAbs(path) {
		t.Fatal("ASR live-correlation manifest path must be absolute")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ASR live-correlation manifest: %v", err)
	}
	var header harnessManifestHeader
	if err := json.Unmarshal(contents, &header); err != nil || header.Scenario != wantScenario ||
		!filepath.IsAbs(header.EvidenceOutput) {
		t.Fatalf("ASR live-correlation manifest does not describe the expected scenario")
	}
	return header
}

func runHarness(t *testing.T, binaryPath, manifestPath string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binaryPath,
		"-test.run="+harnessTestBinaryEntrypoint, "-test.count=1", "-test.timeout=3m30s",
	)
	command.Env = withoutManifestEnvironment(os.Environ())
	if manifestPath != "" {
		command.Env = append(command.Env, manifestEnvironment+"="+manifestPath)
	}
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatal("prebuilt ASR live-correlation harness exceeded its timeout")
		}
		t.Fatalf("prebuilt ASR live-correlation harness failed: %v", err)
	}
}

func withoutManifestEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, manifestEnvironment) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func assertEvidenceWritten(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		t.Fatal("prebuilt ASR live-correlation harness did not produce its evidence file")
	}
}
