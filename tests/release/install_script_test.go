package release_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/testutil"
)

func TestInstallScript_InstallsLatestReleaseArchiveAndPrintsPathGuidance(t *testing.T) {
	t.Parallel()

	skipIfInstallScriptUnsupported(t)

	archiveName := "agent-factory_1.2.3_linux_amd64.tar.gz"
	checksumName := "agent-factory_1.2.3_checksums.txt"
	archiveBytes := buildTarGzArchive(t, "agent-factory", []byte("#!/usr/bin/env sh\necho installed-from-test\n"))
	checksumContents := fmt.Sprintf("%s  %s\n", sha256Hex(archiveBytes), archiveName)
	requests := make([]string, 0, 3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch r.URL.Path {
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v1.2.3", http.StatusFound)
		case "/releases/tag/v1.2.3":
			w.WriteHeader(http.StatusOK)
		case "/releases/download/v1.2.3/" + archiveName:
			w.Write(archiveBytes)
		case "/releases/download/v1.2.3/" + checksumName:
			w.Write([]byte(checksumContents))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	output, err := runInstallScript(t, []string{
		"AGENT_FACTORY_INSTALL_BASE_URL=" + server.URL + "/releases",
		"AGENT_FACTORY_INSTALL_DIR=" + installDir,
		"AGENT_FACTORY_INSTALL_OS=linux",
		"AGENT_FACTORY_INSTALL_ARCH=amd64",
		"HOME=" + t.TempDir(),
	})
	if err != nil {
		t.Fatalf("run install.sh: %v\n%s", err, output)
	}

	if !containsRequest(requests, "/releases/latest") {
		t.Fatalf("installer requests = %#v, want latest release resolution", requests)
	}

	installedBinary := filepath.Join(installDir, "agent-factory")
	info, statErr := os.Stat(installedBinary)
	if statErr != nil {
		t.Fatalf("stat installed binary: %v", statErr)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary mode = %v, want executable bit set", info.Mode())
	}
	if !strings.Contains(output, "Installed agent-factory v1.2.3 to "+installedBinary) {
		t.Fatalf("install output = %q, want installed path message", output)
	}
	if !strings.Contains(output, "Add it to your PATH with:") {
		t.Fatalf("install output = %q, want PATH guidance", output)
	}

	run := exec.Command(installedBinary)
	run.Env = append(os.Environ(), "PATH="+os.Getenv("PATH"))
	commandOutput, runErr := run.CombinedOutput()
	if runErr != nil {
		t.Fatalf("run installed binary: %v\n%s", runErr, commandOutput)
	}
	if strings.TrimSpace(string(commandOutput)) != "installed-from-test" {
		t.Fatalf("installed binary output = %q, want installed-from-test", string(commandOutput))
	}
}

func TestInstallScript_FailsOnChecksumMismatch(t *testing.T) {
	t.Parallel()

	skipIfInstallScriptUnsupported(t)

	archiveName := "agent-factory_1.2.3_linux_amd64.tar.gz"
	archiveBytes := buildTarGzArchive(t, "agent-factory", []byte("#!/usr/bin/env sh\necho installed-from-test\n"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/download/v1.2.3/" + archiveName:
			w.Write(archiveBytes)
		case "/releases/download/v1.2.3/agent-factory_1.2.3_checksums.txt":
			w.Write([]byte("deadbeef  " + archiveName + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	output, err := runInstallScript(t, []string{
		"AGENT_FACTORY_INSTALL_BASE_URL=" + server.URL + "/releases",
		"AGENT_FACTORY_INSTALL_DIR=" + installDir,
		"AGENT_FACTORY_INSTALL_OS=linux",
		"AGENT_FACTORY_INSTALL_ARCH=amd64",
		"AGENT_FACTORY_VERSION=1.2.3",
		"HOME=" + t.TempDir(),
	})
	if err == nil {
		t.Fatalf("run install.sh: expected checksum failure\n%s", output)
	}
	if !strings.Contains(output, "checksum mismatch for "+archiveName) {
		t.Fatalf("install output = %q, want checksum mismatch message", output)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "agent-factory")); !os.IsNotExist(statErr) {
		t.Fatalf("installed binary stat err = %v, want not exists after checksum failure", statErr)
	}
}

func TestInstallScript_FailsOnUnsupportedOperatingSystem(t *testing.T) {
	t.Parallel()

	skipIfInstallScriptUnsupported(t)

	output, err := runInstallScript(t, []string{
		"AGENT_FACTORY_INSTALL_OS=solaris",
		"AGENT_FACTORY_VERSION=1.2.3",
		"HOME=" + t.TempDir(),
	})
	if err == nil {
		t.Fatalf("run install.sh: expected unsupported platform failure\n%s", output)
	}
	if !strings.Contains(output, "unsupported operating system 'solaris'") {
		t.Fatalf("install output = %q, want unsupported OS message", output)
	}
}

func TestSmokeInstallScript_InstallsHostedScriptAndSmokesBinary(t *testing.T) {
	t.Parallel()

	skipIfInstallScriptUnsupported(t)

	archiveName := "agent-factory_1.2.3_linux_amd64.tar.gz"
	checksumName := "agent-factory_1.2.3_checksums.txt"
	archiveBytes := buildTarGzArchive(t, "agent-factory", []byte("#!/usr/bin/env sh\nif [ \"${1:-}\" = \"--help\" ]; then\n  exit 0\nfi\necho installed-from-smoke\n"))
	checksumContents := fmt.Sprintf("%s  %s\n", sha256Hex(archiveBytes), archiveName)
	installScript := readInstallScript(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/download/v1.2.3/install.sh":
			w.Write(installScript)
		case "/releases/download/v1.2.3/" + archiveName:
			w.Write(archiveBytes)
		case "/releases/download/v1.2.3/" + checksumName:
			w.Write([]byte(checksumContents))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	output, err := runSmokeInstallScript(t, []string{
		server.URL + "/releases/download/v1.2.3/install.sh",
		"1.2.3",
		installDir,
	}, []string{
		"AGENT_FACTORY_INSTALL_BASE_URL=" + server.URL + "/releases",
		"AGENT_FACTORY_INSTALL_OS=linux",
		"AGENT_FACTORY_INSTALL_ARCH=amd64",
	})
	if err != nil {
		t.Fatalf("run smoke-install.sh: %v\n%s", err, output)
	}

	installedBinary := filepath.Join(installDir, "agent-factory")
	info, statErr := os.Stat(installedBinary)
	if statErr != nil {
		t.Fatalf("stat installed binary: %v", statErr)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary mode = %v, want executable bit set", info.Mode())
	}
	if !strings.Contains(output, "hosted install smoke passed for "+installedBinary) {
		t.Fatalf("smoke output = %q, want success message", output)
	}
}

func runInstallScript(t *testing.T, env []string) (string, error) {
	t.Helper()

	cmd := exec.Command("sh", "install.sh")
	cmd.Dir = testutil.MustRepoRoot(t)
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runSmokeInstallScript(t *testing.T, args []string, env []string) (string, error) {
	t.Helper()

	scriptArgs := append([]string{"scripts/release/smoke-install.sh"}, args...)
	cmd := exec.Command("sh", scriptArgs...)
	cmd.Dir = testutil.MustRepoRoot(t)
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func skipIfInstallScriptUnsupported(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("install.sh runtime smoke is not supported on Windows")
	}
	for _, binary := range []string{"sh", "curl", "tar", "mktemp"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("install.sh runtime smoke requires %s: %v", binary, err)
		}
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		if _, shasumErr := exec.LookPath("shasum"); shasumErr != nil {
			t.Skip("install.sh runtime smoke requires sha256sum or shasum")
		}
	}
}

func readInstallScript(t *testing.T) []byte {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(testutil.MustRepoRoot(t), "install.sh"))
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	return contents
}

func buildTarGzArchive(t *testing.T, name string, contents []byte) []byte {
	t.Helper()

	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)

	header := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(contents)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(contents); err != nil {
		t.Fatalf("write tar contents: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return archive.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func containsRequest(requests []string, want string) bool {
	for _, request := range requests {
		if request == want {
			return true
		}
	}
	return false
}
