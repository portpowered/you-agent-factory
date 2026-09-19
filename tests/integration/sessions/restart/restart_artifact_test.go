package restart_test

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const (
	restartArtifactEnvironment         = "INFINITE_YOU_INTEGRATION_BINARY"
	restartArtifactFallbackEnvironment = "INFINITE_YOU_PREBUILT_ARTIFACT"
	restartArtifactRequiredEnvironment = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	restartSourceHeadEnvironment       = "INFINITE_YOU_SOURCE_HEAD"
	restartEvidenceOutputEnvironment   = "INFINITE_YOU_RESTART_EVIDENCE_OUTPUT"
	restartCLIMainPackage              = "github.com/portpowered/infinite-you/cmd/factory"
	restartCLIModule                   = "github.com/portpowered/infinite-you"
)

type restartCLIArtifactIdentity struct {
	Path                         string `json:"path"`
	SHA256                       string `json:"sha256"`
	SizeBytes                    int64  `json:"sizeBytes"`
	BuildIdentity                string `json:"buildIdentity"`
	SourceHead                   string `json:"sourceHead"`
	EmbeddedVCSRevision          string `json:"embeddedVcsRevision"`
	VCSRevisionMatchesSourceHead bool   `json:"vcsRevisionMatchesSourceHead"`
	VCSModified                  string `json:"vcsModified,omitempty"`
	MainPackage                  string `json:"mainPackage"`
	Module                       string `json:"module"`
	GoVersion                    string `json:"goVersion"`
	BuildGOOS                    string `json:"buildGOOS"`
	BuildGOARCH                  string `json:"buildGOARCH"`
	TestGOOS                     string `json:"testGOOS"`
	TestGOARCH                   string `json:"testGOARCH"`
}

var (
	restartCLIArtifact    *restartCLIArtifactIdentity
	restartEvidenceOutput string
	restartEvidenceMu     sync.Mutex
	restartRunEvidence    *restartBaselineEvidence
)

func TestMain(m *testing.M) {
	// The SCRIPT_WORKER fixture intentionally uses the Go test executable as a
	// controlled worker. The parent suite has already validated and recorded
	// the prebuilt CLI before starting that child.
	if os.Getenv(boardPersistenceHelperEnv) == boardPersistenceHelperEnvValue {
		os.Exit(m.Run())
	}

	artifactPath := restartArtifactPathFromEnvironment()
	required := strings.EqualFold(strings.TrimSpace(os.Getenv(restartArtifactRequiredEnvironment)), "1")
	identity, err := resolveRestartCLIArtifact(artifactPath, required)
	if err != nil {
		fmt.Fprintf(os.Stderr, "restart integration artifact setup failed: %v\n", err)
		os.Exit(2)
	}
	restartCLIArtifact = identity

	restartEvidenceOutput = strings.TrimSpace(os.Getenv(restartEvidenceOutputEnvironment))
	if required && restartEvidenceOutput == "" {
		fmt.Fprintf(os.Stderr, "%s is required when %s=1\n", restartEvidenceOutputEnvironment, restartArtifactRequiredEnvironment)
		os.Exit(2)
	}
	if restartEvidenceOutput != "" {
		absolutePath, err := filepath.Abs(restartEvidenceOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve restart evidence output %q: %v\n", restartEvidenceOutput, err)
			os.Exit(2)
		}
		restartEvidenceOutput = absolutePath
		if identity != nil && filepath.Clean(restartEvidenceOutput) == filepath.Clean(identity.Path) {
			fmt.Fprintln(os.Stderr, "restart evidence output must not replace the prebuilt CLI artifact")
			os.Exit(2)
		}
		if err := os.MkdirAll(filepath.Dir(restartEvidenceOutput), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "create restart evidence output directory: %v\n", err)
			os.Exit(2)
		}
	}

	if identity != nil {
		encoded, _ := json.Marshal(identity)
		fmt.Fprintf(os.Stderr, "restart prebuilt CLI identity: %s\n", encoded)
	}

	exitCode := m.Run()
	if restartEvidenceOutput != "" {
		if err := writeRestartBaselineEvidence(restartEvidenceOutput, exitCode); err != nil {
			fmt.Fprintf(os.Stderr, "write restart baseline evidence: %v\n", err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func restartArtifactPathFromEnvironment() string {
	path := strings.TrimSpace(os.Getenv(restartArtifactEnvironment))
	if path == "" {
		path = strings.TrimSpace(os.Getenv(restartArtifactFallbackEnvironment))
	}
	return path
}

func resolveRestartCLIArtifact(path string, required bool) (*restartCLIArtifactIdentity, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		if required {
			return nil, fmt.Errorf("%s or %s is required; the restart integration suite never builds a replacement CLI", restartArtifactEnvironment, restartArtifactFallbackEnvironment)
		}
		return nil, nil
	}
	if required && strings.TrimSpace(os.Getenv(restartSourceHeadEnvironment)) == "" {
		return nil, fmt.Errorf("%s is required when %s=1 so the built source head is recorded", restartSourceHeadEnvironment, restartArtifactRequiredEnvironment)
	}

	identity, err := inspectRestartCLIArtifact(path, os.Getenv(restartSourceHeadEnvironment))
	if err != nil {
		return nil, fmt.Errorf("invalid prebuilt CLI artifact %q: %w", path, err)
	}
	return &identity, nil
}

func inspectRestartCLIArtifact(path, sourceHead string) (restartCLIArtifactIdentity, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return restartCLIArtifactIdentity{}, fmt.Errorf("resolve absolute path: %w", err)
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return restartCLIArtifactIdentity{}, fmt.Errorf("stat artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return restartCLIArtifactIdentity{}, errors.New("artifact must be a non-empty regular file")
	}

	file, err := os.Open(absolutePath)
	if err != nil {
		return restartCLIArtifactIdentity{}, fmt.Errorf("open artifact: %w", err)
	}
	hash := sha256.New()
	bytesRead, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return restartCLIArtifactIdentity{}, fmt.Errorf("hash artifact: %w", copyErr)
	}
	if closeErr != nil {
		return restartCLIArtifactIdentity{}, fmt.Errorf("close artifact after hashing: %w", closeErr)
	}
	if bytesRead != info.Size() {
		return restartCLIArtifactIdentity{}, fmt.Errorf("artifact size changed while hashing: stat=%d hashed=%d", info.Size(), bytesRead)
	}

	build, err := buildinfo.ReadFile(absolutePath)
	if err != nil {
		return restartCLIArtifactIdentity{}, fmt.Errorf("read Go build identity: %w", err)
	}
	if build.Path != restartCLIMainPackage || build.Main.Path != restartCLIModule {
		return restartCLIArtifactIdentity{}, fmt.Errorf("artifact build target is %q with module %q, want %q from %q", build.Path, build.Main.Path, restartCLIMainPackage, restartCLIModule)
	}

	settings := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
	}
	embeddedVCSRevision := strings.TrimSpace(settings["vcs.revision"])
	if embeddedVCSRevision == "" || embeddedVCSRevision == "-" {
		return restartCLIArtifactIdentity{}, errors.New("artifact build has no vcs.revision; build with source metadata enabled")
	}
	sourceHead = strings.TrimSpace(sourceHead)
	if sourceHead == "" {
		sourceHead = embeddedVCSRevision
	}
	if len(sourceHead) != 40 && len(sourceHead) != 64 {
		return restartCLIArtifactIdentity{}, fmt.Errorf("source head %q is not a full Git object ID", sourceHead)
	}
	if _, err := hex.DecodeString(sourceHead); err != nil {
		return restartCLIArtifactIdentity{}, fmt.Errorf("source head is not hexadecimal: %w", err)
	}
	buildGOOS := strings.TrimSpace(settings["GOOS"])
	buildGOARCH := strings.TrimSpace(settings["GOARCH"])
	if buildGOOS == "" || buildGOARCH == "" {
		return restartCLIArtifactIdentity{}, errors.New("artifact build is missing GOOS or GOARCH metadata")
	}

	sha := hex.EncodeToString(hash.Sum(nil))
	identity := restartCLIArtifactIdentity{
		Path:                         absolutePath,
		SHA256:                       sha,
		SizeBytes:                    bytesRead,
		SourceHead:                   sourceHead,
		EmbeddedVCSRevision:          embeddedVCSRevision,
		VCSRevisionMatchesSourceHead: embeddedVCSRevision == sourceHead,
		VCSModified:                  strings.TrimSpace(settings["vcs.modified"]),
		MainPackage:                  build.Path,
		Module:                       build.Main.Path,
		GoVersion:                    build.GoVersion,
		BuildGOOS:                    buildGOOS,
		BuildGOARCH:                  buildGOARCH,
		TestGOOS:                     runtime.GOOS,
		TestGOARCH:                   runtime.GOARCH,
	}
	identity.BuildIdentity = fmt.Sprintf("sha256:%s;head:%s;vcs:%s;target:%s;%s/%s;%s", sha, sourceHead, embeddedVCSRevision, build.Path, buildGOOS, buildGOARCH, build.GoVersion)
	return identity, nil
}

func requireRestartCLIArtifact(t *testing.T) string {
	t.Helper()
	if restartCLIArtifact == nil {
		t.Skipf("prebuilt restart CLI unavailable; set %s or %s", restartArtifactEnvironment, restartArtifactFallbackEnvironment)
	}
	return restartCLIArtifact.Path
}

func currentRestartWorkerExecutable(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve controlled restart worker executable: %v", err)
	}
	return path
}
