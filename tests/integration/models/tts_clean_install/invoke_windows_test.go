//go:build windows

package tts_clean_install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These are contract checks for the authored PowerShell entry point. They do
// not stand in for the compiled helper artifact used by the OS-boundary lane.
func TestInvocationScriptAcceptsOmittedOptionalVoice(t *testing.T) {
	f := newFixture(t)
	body, err := json.Marshal(f.manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	delete(document["fixtures"].(map[string]any), "voice")
	body, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal omitted-voice manifest: %v", err)
	}
	if err := os.WriteFile(f.invocation.ManifestPath, body, 0o600); err != nil {
		t.Fatalf("write omitted-voice manifest: %v", err)
	}

	exitCode, output := invokePowerShellProbe(t, f)
	if exitCode != ExitJourneyFailure {
		t.Fatalf("omitted-voice probe exit=%d output=%s, want %d", exitCode, output, ExitJourneyFailure)
	}
	var report Report
	reportBody, err := os.ReadFile(f.invocation.ReportPath)
	if err != nil {
		t.Fatalf("read omitted-voice report: %v", err)
	}
	if err := json.Unmarshal(reportBody, &report); err != nil {
		t.Fatalf("decode omitted-voice report: %v", err)
	}
	if report.Verdict != journeyInconclusive {
		t.Fatalf("omitted-voice report verdict=%q, want INCONCLUSIVE", report.Verdict)
	}
	if strings.Contains(string(reportBody), "property 'voice'") {
		t.Fatalf("omitted-voice report retained property lookup failure: %s", reportBody)
	}
}

func TestInvocationScriptRejectsJunctionOutputRootBeforeWrite(t *testing.T) {
	f := newFixture(t)
	target := filepath.Join(f.root, "external-output")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("create external output: %v", err)
	}
	linked := filepath.Join(f.root, "linked-output")
	junctionOutput, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", linked, target).CombinedOutput()
	if err != nil {
		t.Fatalf("create output junction: %v (%s)", err, strings.TrimSpace(string(junctionOutput)))
	}
	t.Cleanup(func() { _ = os.Remove(linked) })
	f.invocation.ReportPath = filepath.Join(linked, "report.json")

	exitCode, probeOutput := invokePowerShellProbe(t, f)
	if exitCode != ExitInputFailure {
		t.Fatalf("junction probe exit=%d output=%s, want %d", exitCode, probeOutput, ExitInputFailure)
	}
	if _, err := os.Stat(filepath.Join(target, "report.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external target report=%v, want no report write", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("read external output: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("external output entries=%d, want zero", len(entries))
	}
}

func invokePowerShellProbe(t *testing.T, f *fixture) (int, string) {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate invocation script")
	}
	scriptPath := filepath.Join(filepath.Dir(sourcePath), "invoke.ps1")
	command := exec.Command("pwsh", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath,
		"-ArtifactPath", f.invocation.ArtifactPath,
		"-ArtifactIdentity", f.invocation.ArtifactIdentity,
		"-ArtifactSha256", f.invocation.ArtifactSHA256,
		"-ManifestPath", f.invocation.ManifestPath,
		"-ReportPath", f.invocation.ReportPath,
	)
	body, err := command.CombinedOutput()
	if err == nil {
		return 0, string(body)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("invoke PowerShell probe: %v", err)
	}
	return exitErr.ExitCode(), fmt.Sprintf("%s (%v)", body, err)
}
