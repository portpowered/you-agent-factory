package restart_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPackagedInstallOwnerGoneAllowsStartup(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("native incarnation queries require Windows or Linux")
	}
	if _, err := (platformprocess.IncarnationProbe{}).LookupProcess(2147483647); !errors.Is(err, platformprocess.ErrProcessGone) {
		t.Fatalf("absence prerequisite: %v", err)
	}
	binary := requireRestartCLIArtifact(t)
	home, factory, metadata := packagedOwnerStartupFixture(t, []byte(`{"pid":2147483647}`))
	daemon := startBoardPersistenceDaemonProcess(t, binary, factory, home, "", "")
	waitForBoardDaemonReady(t, daemon, 30*time.Second)
	command := exec.CommandContext(t.Context(), binary, "run", "--named", "@you/deep-research", "--help")
	command.Dir, command.Env = filepath.Dir(factory), builtcliacceptance.ProcessEnvForIsolatedHome(home)
	output, err := command.CombinedOutput()
	if err != nil || !bytes.Contains(output, []byte("deep-research")) {
		t.Fatalf("installed Factory help: %v %s", err, output)
	}
	if _, err := os.Stat(metadata); !os.IsNotExist(err) {
		t.Fatalf("stale lease remains: %v", err)
	}
	shutdownPlainBoard(t, daemon)
	t.Logf("I-1 gone: SHA256=%s source=%s argv=%q readiness, named Factory help, lease cleanup and public shutdown passed", restartCLIArtifact.SHA256, restartCLIArtifact.SourceHead, daemon.cmd.Args[1:])
}

func TestPackagedInstallLiveForeignOwnerBlocksStartup(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("native incarnation queries require Windows or Linux")
	}
	identity, err := (platformprocess.IncarnationProbe{}).CurrentProcess()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	binary := requireRestartCLIArtifact(t)
	home, factory, metadata := packagedOwnerStartupFixture(t, owner)
	target := filepath.Join(factorydefinitions.NamedFactoriesRoot(home), "@you", "deep-research", "factory.json")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("prior committed content"), 0600); err != nil {
		t.Fatal(err)
	}
	daemon := startBoardPersistenceDaemonProcess(t, binary, factory, home, "", "")
	waitForBoardPersistenceDaemonExit(t, daemon, 30*time.Second)
	if daemon.waitErr == nil || !strings.Contains(daemon.stderr.String(), "packaged factory installation contention") || !strings.Contains(daemon.stderr.String(), "owner_liveness=active") {
		t.Fatalf("live-owner startup: %v stderr=%s", daemon.waitErr, daemon.stderr.String())
	}
	data, err := os.ReadFile(metadata)
	if err != nil || !bytes.Equal(data, owner) {
		t.Fatalf("owner mutated: %s %v", data, err)
	}
	data, err = os.ReadFile(target)
	if err != nil || string(data) != "prior committed content" {
		t.Fatalf("target mutated: %s %v", data, err)
	}
	t.Logf("I-1 live: SHA256=%s source=%s argv=%q exit before readiness, active contention, exact lease and target bytes preserved", restartCLIArtifact.SHA256, restartCLIArtifact.SourceHead, daemon.cmd.Args[1:])
}

func packagedOwnerStartupFixture(t *testing.T, owner []byte) (home, factory, metadata string) {
	t.Helper()
	home, repo := t.TempDir(), t.TempDir()
	factory = filepath.Join(repo, "factory")
	if err := os.Rename(scaffoldBoardPersistenceFactory(t, boardPersistenceFactoryConfig()), factory); err != nil {
		t.Fatal(err)
	}
	writeBoardPersistenceAgentConfig(t, factory, "restart-blocker", "---\ntype: SCRIPT_WORKER\ncommand: unused-idle-worker\n---\n")
	metadata = filepath.Join(factorydefinitions.NamedFactoriesRoot(home), ".you--deep-research.staging-owner", ".owner.json")
	if err := os.MkdirAll(filepath.Dir(metadata), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata, owner, 0600); err != nil {
		t.Fatal(err)
	}
	return home, factory, metadata
}

// This cell consumes the upstream compiled artifact and crosses real OS stderr
// and exit-status boundaries. The exhaustive cause matrix stays in lower layers.
func TestStartupLoadCauseWithoutDebug(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	home, repo := t.TempDir(), t.TempDir()
	path := filepath.Join(repo, "broken.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"run", "--resume", "./broken.json", "--quiet"}
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Dir = repo
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(home)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("broken load exit=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("broken load polluted stdout: %q", stdout.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	var diagnostic factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(lines[0]), &diagnostic); err != nil || diagnostic.Code == "" {
		t.Fatalf("broken load lost coded envelope: %v: %q", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unexpected end of JSON input") || strings.Count(stderr.String(), "cause[0]=") != 1 {
		t.Fatalf("broken load hid or repeated its cause: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "debug:") || strings.Contains(stderr.String(), home) || strings.Contains(stderr.String(), repo) {
		t.Fatalf("broken load leaked private startup details: %q", stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != "{" {
		t.Fatalf("broken load mutated the recording: bytes=%q error=%v", after, err)
	}
	t.Logf("INT-RESTART artifact SHA256=%s source=%s argv=%q exit=%d stdout=%q stderr=%q", restartCLIArtifact.SHA256, restartCLIArtifact.SourceHead, args, exit.ExitCode(), stdout.String(), stderr.String())
}
