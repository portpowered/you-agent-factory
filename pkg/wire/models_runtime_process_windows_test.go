//go:build windows

package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestModelsProcessLauncherObservesManagedWindowsChildEnvironment(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cmdPath, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatalf("locate Windows command interpreter: %v", err)
	}
	managedExecutable := filepath.Join(root, "vibevoice-cpp.exe")
	commandBody, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("read Windows command interpreter: %v", err)
	}
	if err := os.WriteFile(managedExecutable, commandBody, 0o700); err != nil {
		t.Fatalf("prepare prebuilt managed command: %v", err)
	}
	managedLibrary := filepath.Join(root, "libgovibevoicecpp.dll")
	if err := os.WriteFile(managedLibrary, []byte("controlled DLL marker"), 0o600); err != nil {
		t.Fatalf("prepare managed library marker: %v", err)
	}
	environmentDump := filepath.Join(root, "child-environment.txt")
	staleLibrary := filepath.Join(root, "stale-library.dll")
	environment := appendManagedBackendEnvironment(append([]string(nil), os.Environ()...), []string{
		"TEMP=" + root,
		"TMP=" + root,
		"vIbEvOiCeCpP_LiBrArY=" + staleLibrary,
	})
	evidencePath := filepath.Join(root, "runtime.jsonl")
	recorder := &modelRuntimeEvidenceFileRecorder{path: evidencePath}
	managed, err := (modelsProcessLauncher{recorder: recorder}).Start(
		context.Background(),
		serviceedges.HostProcessStartSpec{
			Backend:      "localai-vibevoice",
			BackendFiles: []string{managedExecutable},
			WorkDir:      root,
			Env:          environment,
			Args: []string{
				"/c",
				"set > child-environment.txt & exit /b 0",
			},
		},
	)
	if err != nil {
		t.Fatalf("start controlled managed child: %v", err)
	}
	defer managed.Stop(context.Background())

	waitErr := waitForManagedProcess(t, managed)
	if waitErr != nil {
		t.Fatalf("controlled managed child exit = %v, want success", waitErr)
	}
	dump, err := os.ReadFile(environmentDump)
	if err != nil {
		t.Fatalf("read child environment dump: %v", err)
	}
	childEnvironment := parseEnvironmentDump(string(dump))
	wantEnvironment := map[string]string{
		"PATH":                 requiredEnvironmentValue(t, environment, "PATH"),
		"TEMP":                 root,
		"TMP":                  root,
		"VIBEVOICECPP_LIBRARY": managedLibrary,
	}
	for name, want := range wantEnvironment {
		if got := childEnvironment[strings.ToUpper(name)]; got != want {
			t.Fatalf("child %s = %q, want managed value %q", name, got, want)
		}
	}

	records := readManagedChildEvidence(t, evidencePath)
	if len(records) != 2 {
		t.Fatalf("managed child evidence records = %d, want start and exit: %#v", len(records), records)
	}
	started, exited := records[0], records[1]
	if started.Kind != managedChildEvidenceKind || started.Phase != managedChildPhaseStarted ||
		started.ProcessID <= 0 || exited.Kind != managedChildEvidenceKind ||
		exited.Phase != managedChildPhaseExited || exited.ProcessID != started.ProcessID ||
		exited.ExitClass != managedChildExitClassExited {
		t.Fatalf("managed child lifecycle evidence = %#v, want one PID with start/exit phases", records)
	}
	wantDigests := map[string]string{
		"PATH":                 environmentValueSHA256(wantEnvironment["PATH"]),
		"TEMP":                 environmentValueSHA256(root),
		"TMP":                  environmentValueSHA256(root),
		"VIBEVOICECPP_LIBRARY": environmentValueSHA256(managedLibrary),
	}
	if len(started.Environment) != len(wantDigests) {
		t.Fatalf("started environment facts = %#v, want four allowlisted facts", started.Environment)
	}
	for _, fact := range started.Environment {
		if !fact.Present || fact.ValueSHA256 != wantDigests[fact.Name] {
			t.Fatalf("started environment fact = %#v, want bounded digest", fact)
		}
	}
	body, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatalf("read managed child evidence: %v", err)
	}
	for _, marker := range []string{root, staleLibrary, managedLibrary, environmentDump, requiredEnvironmentValue(t, environment, "PATH")} {
		if strings.Contains(string(body), marker) {
			t.Fatalf("managed child evidence leaked raw value %q: %s", marker, body)
		}
	}
	if !bytes.Contains(body, []byte(`"sequence":1`)) || !bytes.Contains(body, []byte(`"sequence":2`)) {
		t.Fatalf("managed child evidence sequence = %s, want ordered records", body)
	}
}

func TestModelsProcessLauncherRecordsNonzeroManagedChildExit(t *testing.T) {
	t.Parallel()

	cmdPath, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatalf("locate Windows command interpreter: %v", err)
	}
	evidencePath := filepath.Join(t.TempDir(), "runtime.jsonl")
	recorder := &modelRuntimeEvidenceFileRecorder{path: evidencePath}
	managed, err := (modelsProcessLauncher{recorder: recorder}).Start(
		context.Background(),
		serviceedges.HostProcessStartSpec{
			Command:        cmdPath,
			Args:           []string{"/c", "exit /b 7"},
			Backend:        "localai-vibevoice",
			HealthEndpoint: "grpc://127.0.0.1:1",
		},
	)
	if err != nil {
		t.Fatalf("start nonzero managed child: %v", err)
	}
	defer managed.Stop(context.Background())
	if waitErr := waitForManagedProcess(t, managed); waitErr == nil {
		t.Fatal("nonzero managed child exit = nil, want process exit error")
	}
	records := readManagedChildEvidence(t, evidencePath)
	if len(records) != 2 || records[0].ProcessID <= 0 || records[1].ProcessID != records[0].ProcessID ||
		records[0].Phase != managedChildPhaseStarted || records[1].Phase != managedChildPhaseExited ||
		records[1].ExitClass != managedChildExitClassNonzero || len(records[1].Environment) != 0 {
		t.Fatalf("nonzero managed child evidence = %#v, want distinct bounded exit", records)
	}
}

func waitForManagedProcess(t *testing.T, process interface{ Wait() error }) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		t.Fatal("timed out waiting for controlled managed child")
		return nil
	}
}

func readManagedChildEvidence(t *testing.T, path string) []managedChildEnvironmentEvidence {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read managed child evidence %q: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	var records []managedChildEnvironmentEvidence
	for {
		var record managedChildEnvironmentEvidence
		err := decoder.Decode(&record)
		if err == io.EOF {
			return records
		}
		if err != nil {
			t.Fatalf("decode managed child evidence: %v", err)
		}
		records = append(records, record)
	}
}

func parseEnvironmentDump(dump string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(dump, "\r\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.ToUpper(key)] = value
		}
	}
	return values
}

func requiredEnvironmentValue(t *testing.T, environment []string, name string) string {
	t.Helper()
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, name) {
			return value
		}
	}
	t.Fatalf("required environment value %s is missing", name)
	return ""
}
