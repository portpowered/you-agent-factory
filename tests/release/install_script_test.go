package release_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const repoInstallScriptPath = "scripts/install.sh"

func TestInstallScript_InstallsLatestReleaseArchiveAndPrintsPathGuidance(t *testing.T) {
	t.Parallel()

	skipIfInstallScriptUnsupported(t)

	archiveName := "you_1.2.3_linux_amd64.tar.gz"
	checksumName := "you_1.2.3_checksums.txt"
	archiveBytes := buildTarGzArchive(t, "you", readInstallTestBinary(t))
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

	homeDir := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "bin")
	output, err := runInstallScript(t, []string{
		"INFINITE_YOU_INSTALL_BASE_URL=" + server.URL + "/releases",
		"INFINITE_YOU_INSTALL_DIR=" + installDir,
		"INFINITE_YOU_INSTALL_OS=linux",
		"INFINITE_YOU_INSTALL_ARCH=amd64",
		"HOME=" + homeDir,
	})
	if err != nil {
		t.Fatalf("run install.sh: %v\n%s", err, output)
	}

	if !containsRequest(requests, "/releases/latest") {
		t.Fatalf("installer requests = %#v, want latest release resolution", requests)
	}

	installedBinary := filepath.Join(installDir, "you")
	info, statErr := os.Stat(installedBinary)
	if statErr != nil {
		t.Fatalf("stat installed binary: %v", statErr)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary mode = %v, want executable bit set", info.Mode())
	}
	if !strings.Contains(output, "Installed you v1.2.3 to "+installedBinary) {
		t.Fatalf("install output = %q, want installed path message", output)
	}
	if !strings.Contains(output, "Initializing operator/system config and default factories.") {
		t.Fatalf("install output = %q, want post-install config init message", output)
	}
	if !strings.Contains(output, "Created system config at") {
		t.Fatalf("install output = %q, want config init success output", output)
	}
	if !strings.Contains(output, "Add it to your PATH with:") {
		t.Fatalf("install output = %q, want PATH guidance", output)
	}

	configPath := configPathFromBuiltCLI(t, installedBinary, homeDir)
	if _, statErr := os.Stat(configPath); statErr != nil {
		t.Fatalf("stat post-install config: %v", statErr)
	}
}

func TestInstallScript_FailsOnChecksumMismatch(t *testing.T) {
	t.Parallel()

	skipIfInstallScriptUnsupported(t)

	archiveName := "you_1.2.3_linux_amd64.tar.gz"
	archiveBytes := buildTarGzArchive(t, "you", []byte("#!/usr/bin/env sh\necho installed-from-test\n"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/download/v1.2.3/" + archiveName:
			w.Write(archiveBytes)
		case "/releases/download/v1.2.3/you_1.2.3_checksums.txt":
			w.Write([]byte("deadbeef  " + archiveName + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	output, err := runInstallScript(t, []string{
		"INFINITE_YOU_INSTALL_BASE_URL=" + server.URL + "/releases",
		"INFINITE_YOU_INSTALL_DIR=" + installDir,
		"INFINITE_YOU_INSTALL_OS=linux",
		"INFINITE_YOU_INSTALL_ARCH=amd64",
		"INFINITE_YOU_VERSION=1.2.3",
		"HOME=" + t.TempDir(),
	})
	if err == nil {
		t.Fatalf("run install.sh: expected checksum failure\n%s", output)
	}
	if !strings.Contains(output, "checksum mismatch for "+archiveName) {
		t.Fatalf("install output = %q, want checksum mismatch message", output)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "you")); !os.IsNotExist(statErr) {
		t.Fatalf("installed binary stat err = %v, want not exists after checksum failure", statErr)
	}
}

func TestInstallScript_FailsWhenPostInstallConfigInitFails(t *testing.T) {
	t.Parallel()

	skipIfInstallScriptUnsupported(t)

	archiveName := "you_1.2.3_linux_amd64.tar.gz"
	checksumName := "you_1.2.3_checksums.txt"
	archiveBytes := buildTarGzArchive(t, "you", readInstallTestBinary(t))
	checksumContents := fmt.Sprintf("%s  %s\n", sha256Hex(archiveBytes), archiveName)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/download/v1.2.3/" + archiveName:
			w.Write(archiveBytes)
		case "/releases/download/v1.2.3/" + checksumName:
			w.Write([]byte(checksumContents))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	homeDir := t.TempDir()
	configPath := configPathFromBuiltCLI(t, buildReleaseSmokeBinary(t), homeDir)
	configParent := filepath.Dir(configPath)
	if err := requirePathWithin(configParent, homeDir); err != nil {
		t.Fatalf("config init returned unsafe parent path: %v", err)
	}
	if err := os.RemoveAll(configParent); err != nil {
		t.Fatalf("remove initialized config parent: %v", err)
	}
	if err := os.WriteFile(configParent, []byte("blocked"), 0o644); err != nil {
		t.Fatalf("write blocking config parent: %v", err)
	}

	installDir := filepath.Join(t.TempDir(), "bin")
	output, err := runInstallScript(t, []string{
		"INFINITE_YOU_INSTALL_BASE_URL=" + server.URL + "/releases",
		"INFINITE_YOU_INSTALL_DIR=" + installDir,
		"INFINITE_YOU_INSTALL_OS=linux",
		"INFINITE_YOU_INSTALL_ARCH=amd64",
		"INFINITE_YOU_VERSION=1.2.3",
		"HOME=" + homeDir,
	})
	if err == nil {
		t.Fatalf("run install.sh: expected post-install config init failure\n%s", output)
	}
	if !strings.Contains(output, "failed to initialize operator/system config and default factories") {
		t.Fatalf("install output = %q, want actionable config init failure message", output)
	}
}

func configPathFromBuiltCLI(t *testing.T, binaryPath, homeDir string) string {
	t.Helper()

	cmd := exec.Command(binaryPath, "config", "init", "--json")
	cmd.Dir = testutil.MustRepoRoot(t)
	cmd.Env = builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe config path through built CLI: %v\n%s", err, string(output))
	}
	var outcome struct {
		ConfigPath string `json:"configPath"`
	}
	if err := json.Unmarshal(output, &outcome); err != nil {
		t.Fatalf("decode built CLI config init JSON: %v\n%s", err, string(output))
	}
	if strings.TrimSpace(outcome.ConfigPath) == "" {
		t.Fatalf("built CLI config init returned empty configPath: %s", string(output))
	}
	return outcome.ConfigPath
}

func requirePathWithin(path, root string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("path %q is not a child of %q", path, root)
	}
	return nil
}

func TestInstallScript_FailsOnUnsupportedOperatingSystem(t *testing.T) {
	t.Parallel()

	skipIfInstallScriptUnsupported(t)

	output, err := runInstallScript(t, []string{
		"INFINITE_YOU_INSTALL_OS=solaris",
		"INFINITE_YOU_VERSION=1.2.3",
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

	archiveName := "you_1.2.3_linux_amd64.tar.gz"
	checksumName := "you_1.2.3_checksums.txt"
	archiveBytes := buildTarGzArchive(t, "you", readInstallTestBinary(t))
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
		"INFINITE_YOU_INSTALL_BASE_URL=" + server.URL + "/releases",
		"INFINITE_YOU_INSTALL_OS=linux",
		"INFINITE_YOU_INSTALL_ARCH=amd64",
	})
	if err != nil {
		t.Fatalf("run smoke-install.sh: %v\n%s", err, output)
	}

	installedBinary := filepath.Join(installDir, "you")
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

	cmd := exec.Command("sh", repoInstallScriptPath)
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

	contents, err := os.ReadFile(filepath.Join(testutil.MustRepoRoot(t), repoInstallScriptPath))
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	return contents
}

func readInstallTestBinary(t *testing.T) []byte {
	t.Helper()

	binaryPath := buildReleaseSmokeBinary(t)
	contents, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read install test binary: %v", err)
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
