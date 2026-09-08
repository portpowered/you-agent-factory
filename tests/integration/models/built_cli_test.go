package models_test

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

const (
	story005ArtifactEnvironment        = "INFINITE_YOU_PREBUILT_ARTIFACT"
	story005RequireArtifactEnvironment = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	story005FactoryMainPackage         = "github.com/portpowered/infinite-you/cmd/factory"
)

type story005ArtifactIdentity struct {
	path      string
	size      int64
	sha256    string
	source    string
	head      string
	version   string
	goVersion string
	goos      string
	goarch    string
}

type story005ArtifactState struct {
	path     string
	identity story005ArtifactIdentity
}

var story001Binary story005ArtifactState

type story005ArtifactFailure struct {
	category string
	path     string
	err      error
}

func (failure *story005ArtifactFailure) Error() string {
	if failure == nil {
		return ""
	}
	if failure.err == nil {
		return fmt.Sprintf("story-005 prebuilt artifact %s: %q", failure.category, failure.path)
	}
	return fmt.Sprintf("story-005 prebuilt artifact %s: %q: %v", failure.category, failure.path, failure.err)
}

func (failure *story005ArtifactFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.err
}

// TestMain validates the one delivered CLI before any integration fixture or
// child process can be created. Optional local runs leave compiled cells to
// skip explicitly; they never manufacture a replacement artifact.
func TestMain(m *testing.M) {
	artifact, err := resolveStory005Artifact(
		strings.TrimSpace(os.Getenv(story005ArtifactEnvironment)),
		story005ArtifactRequired(os.Getenv(story005RequireArtifactEnvironment)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "STORY-005-ARTIFACT-SETUP failed: %v\n", err)
		os.Exit(1)
	}
	if artifact.path == "" {
		fmt.Fprintf(os.Stderr, "STORY-005-ARTIFACT status=optional-skip env=%s\n", story005ArtifactEnvironment)
	} else {
		story001Binary.path = artifact.path
		story001Binary.identity = artifact.identity
		fmt.Fprintf(os.Stderr, "STORY-005-ARTIFACT status=validated %s\n", formatStory005ArtifactIdentity(artifact.identity))
	}
	os.Exit(m.Run())
}

func story005ArtifactRequired(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func resolveStory005Artifact(path string, required bool) (story005ArtifactState, error) {
	if path == "" {
		if required {
			return story005ArtifactState{}, &story005ArtifactFailure{
				category: "missing",
				path:     path,
				err:      fmt.Errorf("%s is required", story005ArtifactEnvironment),
			}
		}
		return story005ArtifactState{}, nil
	}
	if !filepath.IsAbs(path) {
		return story005ArtifactState{}, &story005ArtifactFailure{
			category: "invalid-path",
			path:     path,
			err:      fmt.Errorf("path must be absolute"),
		}
	}
	fileInfo, err := statStory005Artifact(path)
	if err != nil {
		return story005ArtifactState{}, err
	}
	if err := validateStory005ArtifactMode(path, fileInfo); err != nil {
		return story005ArtifactState{}, err
	}
	digest, err := hashStory005Artifact(path, fileInfo.Size())
	if err != nil {
		return story005ArtifactState{}, err
	}
	build, err := readStory005BuildInfo(path)
	if err != nil {
		return story005ArtifactState{}, err
	}
	if err := validateStory005BuildInfo(path, build); err != nil {
		return story005ArtifactState{}, err
	}

	version := build.Main.Version
	if version == "" {
		version = "(unknown)"
	}
	head := buildSetting(build, "vcs.revision")
	if head == "" {
		head = "(unavailable)"
	}
	buildGOOS := buildSetting(build, "GOOS")
	buildGOARCH := buildSetting(build, "GOARCH")
	return story005ArtifactState{
		path: path,
		identity: story005ArtifactIdentity{
			path:      path,
			size:      fileInfo.Size(),
			sha256:    digest,
			source:    build.Path,
			head:      head,
			version:   version,
			goVersion: build.GoVersion,
			goos:      buildGOOS,
			goarch:    buildGOARCH,
		},
	}, nil
}

func statStory005Artifact(path string) (os.FileInfo, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		category := "unreadable"
		if os.IsNotExist(err) {
			category = "missing"
		}
		return nil, &story005ArtifactFailure{category: category, path: path, err: err}
	}
	if !fileInfo.Mode().IsRegular() {
		return nil, &story005ArtifactFailure{
			category: "non-regular",
			path:     path,
			err:      fmt.Errorf("mode=%s", fileInfo.Mode()),
		}
	}
	return fileInfo, nil
}

func validateStory005ArtifactMode(path string, fileInfo os.FileInfo) error {
	if runtime.GOOS != "windows" {
		if fileInfo.Mode().Perm()&0o444 == 0 {
			return &story005ArtifactFailure{
				category: "unreadable",
				path:     path,
				err:      fmt.Errorf("mode=%s has no read permission", fileInfo.Mode()),
			}
		}
		if fileInfo.Mode().Perm()&0o111 == 0 {
			return &story005ArtifactFailure{
				category: "non-executable",
				path:     path,
				err:      fmt.Errorf("mode=%s has no execute permission", fileInfo.Mode()),
			}
		}
		return nil
	}
	if strings.EqualFold(filepath.Ext(path), ".exe") {
		return nil
	}
	return &story005ArtifactFailure{
		category: "non-executable",
		path:     path,
		err:      fmt.Errorf("Windows executable must use .exe"),
	}
}

func hashStory005Artifact(path string, expectedSize int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", &story005ArtifactFailure{category: "unreadable", path: path, err: err}
	}
	digest := sha256.New()
	_, copyErr := io.Copy(digest, file)
	closeErr := file.Close()
	if copyErr != nil {
		return "", &story005ArtifactFailure{category: "unreadable", path: path, err: copyErr}
	}
	if closeErr != nil {
		return "", &story005ArtifactFailure{category: "unreadable", path: path, err: closeErr}
	}
	finalInfo, err := os.Stat(path)
	if err != nil {
		return "", &story005ArtifactFailure{category: "unreadable", path: path, err: err}
	}
	if finalInfo.Size() != expectedSize {
		return "", &story005ArtifactFailure{
			category: "changed-during-validation",
			path:     path,
			err:      fmt.Errorf("size changed from %d to %d bytes", expectedSize, finalInfo.Size()),
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func readStory005BuildInfo(path string) (*debug.BuildInfo, error) {
	build, err := buildinfo.ReadFile(path)
	if err != nil {
		return nil, &story005ArtifactFailure{
			category: "non-executable",
			path:     path,
			err:      fmt.Errorf("read Go build identity: %w", err),
		}
	}
	return build, nil
}

func validateStory005BuildInfo(path string, build *debug.BuildInfo) error {
	if build.Path != story005FactoryMainPackage {
		return &story005ArtifactFailure{
			category: "non-executable",
			path:     path,
			err:      fmt.Errorf("main package=%q, want %q", build.Path, story005FactoryMainPackage),
		}
	}
	for key, want := range map[string]string{"GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH} {
		got := buildSetting(build, key)
		if got != "" && got != want {
			return &story005ArtifactFailure{
				category: "non-executable",
				path:     path,
				err:      fmt.Errorf("%s=%q, want %q", key, got, want),
			}
		}
	}
	return nil
}

func buildSetting(build *debug.BuildInfo, key string) string {
	if build == nil {
		return ""
	}
	for _, setting := range build.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

func formatStory005ArtifactIdentity(identity story005ArtifactIdentity) string {
	return fmt.Sprintf(
		"path=%q size=%d sha256=%s source=%q head=%q version=%q goVersion=%q platform=%s/%s",
		identity.path, identity.size, identity.sha256, identity.source, identity.head,
		identity.version, identity.goVersion, identity.goos, identity.goarch,
	)
}

// TestStory005PrebuiltArtifactContract keeps the artifact boundary explicit:
// a valid supplied executable is identified once, while invalid required
// inputs fail before any process-level fixture can be started.
func TestStory005PrebuiltArtifactContract(t *testing.T) {
	t.Run("valid supplied artifact identity is stable", func(t *testing.T) {
		if story001Binary.path == "" {
			t.Skipf("optional compiled-artifact mode: %s is unset", story005ArtifactEnvironment)
		}
		identity := story001Binary.identity
		if !filepath.IsAbs(identity.path) || identity.path != story001Binary.path {
			t.Fatalf("artifact path identity = %#v, state path=%q", identity, story001Binary.path)
		}
		if identity.size <= 0 || len(identity.sha256) != sha256.Size*2 || identity.source != story005FactoryMainPackage || identity.head == "" || identity.version == "" || identity.goVersion == "" {
			t.Fatalf("artifact identity = %#v, want size/digest/source/head/version/build identity", identity)
		}
		if got := buildStory001Binary(t); got != identity.path {
			t.Fatalf("first compiled cell path = %q, want %q", got, identity.path)
		}
		if got := buildStory001Binary(t); got != identity.path {
			t.Fatalf("second compiled cell path = %q, want %q", got, identity.path)
		}
		t.Logf("STORY-005-ARTIFACT-CONTRACT identity=%s", formatStory005ArtifactIdentity(identity))
	})

	t.Run("optional absence skips without replacement", func(t *testing.T) {
		state, err := resolveStory005Artifact("", false)
		if err != nil {
			t.Fatalf("optional absent artifact returned error: %v", err)
		}
		if state.path != "" || state.identity.path != "" {
			t.Fatalf("optional absent artifact state = %#v, want no artifact", state)
		}
	})

	t.Run("required absence fails closed", func(t *testing.T) {
		_, err := resolveStory005Artifact("", true)
		assertStory005ArtifactCategory(t, err, "missing")
	})

	t.Run("relative path fails before filesystem access", func(t *testing.T) {
		_, err := resolveStory005Artifact("you", true)
		assertStory005ArtifactCategory(t, err, "invalid-path")
	})

	t.Run("missing path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing-you")
		_, err := resolveStory005Artifact(path, true)
		assertStory005ArtifactCategory(t, err, "missing")
	})

	t.Run("non-regular path", func(t *testing.T) {
		path := t.TempDir()
		_, err := resolveStory005Artifact(path, true)
		assertStory005ArtifactCategory(t, err, "non-regular")
	})

	t.Run("unreadable path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unreadable-you")
		if runtime.GOOS == "windows" {
			// Windows does not expose portable read permission bits. An invalid
			// absolute path still exercises the setup error classification without
			// starting a fixture or child process.
			path += "\x00"
		} else {
			if err := os.WriteFile(path, []byte("not an executable"), 0o111); err != nil {
				t.Fatalf("write unreadable fixture: %v", err)
			}
			if err := os.Chmod(path, 0o111); err != nil {
				t.Fatalf("chmod unreadable fixture: %v", err)
			}
		}
		_, err := resolveStory005Artifact(path, true)
		assertStory005ArtifactCategory(t, err, "unreadable")
	})

	t.Run("non-executable path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "not-an-executable")
		if err := os.WriteFile(path, []byte("not an executable"), 0o644); err != nil {
			t.Fatalf("write non-executable fixture: %v", err)
		}
		_, err := resolveStory005Artifact(path, true)
		assertStory005ArtifactCategory(t, err, "non-executable")
	})
}

func assertStory005ArtifactCategory(t testing.TB, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("artifact validation succeeded, want category %q", want)
	}
	var failure *story005ArtifactFailure
	if !errors.As(err, &failure) {
		t.Fatalf("artifact validation error = %v, want story-005 failure category %q", err, want)
	}
	if failure.category != want {
		t.Fatalf("artifact validation category = %q, want %q (error=%v)", failure.category, want, err)
	}
}

func buildStory001Binary(t testing.TB) string {
	t.Helper()
	if story001Binary.path == "" {
		t.Skipf("compiled-artifact cell skipped: %s is unset in optional mode; no in-test build fallback", story005ArtifactEnvironment)
	}
	return story001Binary.path
}

func story001EnvironmentWithBrowserStub(t testing.TB, home, cache, endpoint string) []string {
	t.Helper()
	environment := story001Environment(home, cache, endpoint)
	binDir := filepath.Join(home, "story-001-browser-stub")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create story-001 browser stub directory: %v", err)
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("locate Go executable for story-001 browser stub: %v", err)
	}
	stubName := "xdg-open"
	if runtime.GOOS == "windows" {
		stubName = "rundll32.exe"
	} else if runtime.GOOS == "darwin" {
		stubName = "open"
	}
	stubPath := filepath.Join(binDir, stubName)
	goBinary, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatalf("read Go executable for story-001 browser stub: %v", err)
	}
	if err := os.WriteFile(stubPath, goBinary, 0o755); err != nil {
		t.Fatalf("install story-001 browser stub: %v", err)
	}
	return prependStory001Path(environment, binDir)
}

func prependStory001Path(environment []string, directory string) []string {
	pathValue := ""
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "PATH") {
			pathValue = value
			continue
		}
		filtered = append(filtered, entry)
	}
	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	return append(filtered, "PATH="+directory+string(os.PathListSeparator)+pathValue)
}
