package release_test

import (
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	releasePrebuiltRequiredEnv     = "INFINITE_YOU_RELEASE_PREBUILT_REQUIRED"
	releasePrebuiltPathEnv         = "INFINITE_YOU_RELEASE_PREBUILT_PATH"
	releasePrebuiltSHA256Env       = "INFINITE_YOU_RELEASE_PREBUILT_SHA256"
	releasePrebuiltSizeEnv         = "INFINITE_YOU_RELEASE_PREBUILT_SIZE"
	releasePrebuiltSourceCommitEnv = "INFINITE_YOU_RELEASE_PREBUILT_SOURCE_COMMIT"
	releasePrebuiltSourceTreeEnv   = "INFINITE_YOU_RELEASE_PREBUILT_SOURCE_TREE"
	releasePrebuiltToolPathEnv     = "INFINITE_YOU_RELEASE_PREBUILT_TOOL_PATH"
	releasePrebuiltToolVersionEnv  = "INFINITE_YOU_RELEASE_PREBUILT_TOOL_VERSION"
	releasePrebuiltGOOSEnv         = "INFINITE_YOU_RELEASE_PREBUILT_GOOS"
	releasePrebuiltGOARCHEnv       = "INFINITE_YOU_RELEASE_PREBUILT_GOARCH"
	releaseLocalGoInstallSmokeEnv  = "INFINITE_YOU_RELEASE_LOCAL_GO_INSTALL_SMOKE"
	releasePublicGoInstallSmokeEnv = "INFINITE_YOU_RELEASE_PUBLIC_GO_INSTALL_SMOKE"
)

var releaseArtifactHexPattern = regexp.MustCompile(`^[0-9a-f]+$`)

type releasePrebuiltArtifact struct {
	Path         string
	SHA256       string
	Size         int64
	SourceCommit string
	SourceTree   string
	ToolPath     string
	ToolVersion  string
	GOOS         string
	GOARCH       string
}

type releasePrebuiltDescriptor struct {
	Path         string
	SHA256       string
	Size         string
	SourceCommit string
	SourceTree   string
	ToolPath     string
	ToolVersion  string
	GOOS         string
	GOARCH       string
}

func requireReleasePrebuiltArtifact(t *testing.T) releasePrebuiltArtifact {
	t.Helper()

	required, isCanonical := os.LookupEnv(releasePrebuiltRequiredEnv)
	if !isCanonical {
		t.Skip("compiled release cases require make test-release; canonical prebuilt descriptor is absent")
	}
	if required != "1" {
		t.Fatalf("%s = %q, want 1 in canonical release mode", releasePrebuiltRequiredEnv, required)
	}

	artifact, err := loadReleasePrebuiltArtifact(os.LookupEnv)
	if err != nil {
		t.Fatalf("validate canonical prebuilt artifact: %v", err)
	}
	t.Logf(
		"release prebuilt artifact path=%s size=%d sha256=%s source_commit=%s source_tree=%s tool_path=%s tool_version=%s goos=%s goarch=%s",
		artifact.Path,
		artifact.Size,
		artifact.SHA256,
		artifact.SourceCommit,
		artifact.SourceTree,
		artifact.ToolPath,
		artifact.ToolVersion,
		artifact.GOOS,
		artifact.GOARCH,
	)
	return artifact
}

func loadReleasePrebuiltArtifact(getenv func(string) (string, bool)) (releasePrebuiltArtifact, error) {
	descriptor, err := readReleasePrebuiltDescriptor(getenv)
	if err != nil {
		return releasePrebuiltArtifact{}, err
	}
	artifact, err := parseReleasePrebuiltDescriptor(descriptor)
	if err != nil {
		return releasePrebuiltArtifact{}, err
	}
	toolInfo, err := os.Stat(artifact.ToolPath)
	if err != nil {
		return releasePrebuiltArtifact{}, fmt.Errorf("stat %s: %w", releasePrebuiltToolPathEnv, err)
	}
	if !toolInfo.Mode().IsRegular() {
		return releasePrebuiltArtifact{}, fmt.Errorf("%s must name a regular file, got %q", releasePrebuiltToolPathEnv, artifact.ToolPath)
	}
	if err := artifact.validateFile(); err != nil {
		return releasePrebuiltArtifact{}, err
	}
	return artifact, nil
}

func readReleasePrebuiltDescriptor(getenv func(string) (string, bool)) (releasePrebuiltDescriptor, error) {
	values := make(map[string]string, 9)
	for _, name := range []string{
		releasePrebuiltPathEnv,
		releasePrebuiltSHA256Env,
		releasePrebuiltSizeEnv,
		releasePrebuiltSourceCommitEnv,
		releasePrebuiltSourceTreeEnv,
		releasePrebuiltToolPathEnv,
		releasePrebuiltToolVersionEnv,
		releasePrebuiltGOOSEnv,
		releasePrebuiltGOARCHEnv,
	} {
		value, err := requiredReleaseEnv(getenv, name)
		if err != nil {
			return releasePrebuiltDescriptor{}, err
		}
		values[name] = value
	}
	return releasePrebuiltDescriptor{
		Path:         values[releasePrebuiltPathEnv],
		SHA256:       values[releasePrebuiltSHA256Env],
		Size:         values[releasePrebuiltSizeEnv],
		SourceCommit: values[releasePrebuiltSourceCommitEnv],
		SourceTree:   values[releasePrebuiltSourceTreeEnv],
		ToolPath:     values[releasePrebuiltToolPathEnv],
		ToolVersion:  values[releasePrebuiltToolVersionEnv],
		GOOS:         values[releasePrebuiltGOOSEnv],
		GOARCH:       values[releasePrebuiltGOARCHEnv],
	}, nil
}

func parseReleasePrebuiltDescriptor(descriptor releasePrebuiltDescriptor) (releasePrebuiltArtifact, error) {
	if !filepath.IsAbs(descriptor.Path) {
		return releasePrebuiltArtifact{}, fmt.Errorf("%s must be absolute, got %q", releasePrebuiltPathEnv, descriptor.Path)
	}
	if !filepath.IsAbs(descriptor.ToolPath) {
		return releasePrebuiltArtifact{}, fmt.Errorf("%s must be absolute, got %q", releasePrebuiltToolPathEnv, descriptor.ToolPath)
	}
	if err := validateReleaseHex(releasePrebuiltSHA256Env, descriptor.SHA256, sha256.Size*2); err != nil {
		return releasePrebuiltArtifact{}, err
	}
	if err := validateReleaseHex(releasePrebuiltSourceCommitEnv, descriptor.SourceCommit, 40); err != nil {
		return releasePrebuiltArtifact{}, err
	}
	if err := validateReleaseHex(releasePrebuiltSourceTreeEnv, descriptor.SourceTree, 40); err != nil {
		return releasePrebuiltArtifact{}, err
	}
	size, err := strconv.ParseInt(descriptor.Size, 10, 64)
	if err != nil || size <= 0 || strconv.FormatInt(size, 10) != descriptor.Size {
		return releasePrebuiltArtifact{}, fmt.Errorf("%s must be a positive decimal byte count, got %q", releasePrebuiltSizeEnv, descriptor.Size)
	}
	if strings.ContainsAny(descriptor.ToolVersion, "\r\n") || strings.TrimSpace(descriptor.ToolVersion) == "" {
		return releasePrebuiltArtifact{}, fmt.Errorf("%s must be a non-empty single line", releasePrebuiltToolVersionEnv)
	}
	if err := validateReleasePlatformToken(releasePrebuiltGOOSEnv, descriptor.GOOS); err != nil {
		return releasePrebuiltArtifact{}, err
	}
	if err := validateReleasePlatformToken(releasePrebuiltGOARCHEnv, descriptor.GOARCH); err != nil {
		return releasePrebuiltArtifact{}, err
	}
	return releasePrebuiltArtifact{
		Path:         filepath.Clean(descriptor.Path),
		SHA256:       descriptor.SHA256,
		Size:         size,
		SourceCommit: descriptor.SourceCommit,
		SourceTree:   descriptor.SourceTree,
		ToolPath:     filepath.Clean(descriptor.ToolPath),
		ToolVersion:  descriptor.ToolVersion,
		GOOS:         descriptor.GOOS,
		GOARCH:       descriptor.GOARCH,
	}, nil
}

func validateReleasePlatformToken(name, value string) error {
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\t ") {
		return fmt.Errorf("%s must be a single platform token, got %q", name, value)
	}
	return nil
}

func requiredReleaseEnv(getenv func(string) (string, bool), name string) (string, error) {
	value, ok := getenv(name)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func validateReleaseHex(name, value string, length int) error {
	if len(value) != length || !releaseArtifactHexPattern.MatchString(value) {
		return fmt.Errorf("%s must be %d lowercase hexadecimal characters, got %q", name, length, value)
	}
	return nil
}

func (artifact releasePrebuiltArtifact) validateFile() error {
	info, err := os.Lstat(artifact.Path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", artifact.Path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("artifact path %s must not be a symlink", artifact.Path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("artifact path %s is not a regular file", artifact.Path)
	}
	if info.Size() != artifact.Size {
		return fmt.Errorf("artifact size for %s = %d, want %d", artifact.Path, info.Size(), artifact.Size)
	}
	if info.Mode().Perm()&0o222 != 0 {
		return fmt.Errorf("artifact path %s must be read-only, mode=%#o", artifact.Path, info.Mode().Perm())
	}
	if err := validateReleaseArtifactLaunchability(artifact, info); err != nil {
		return err
	}

	file, err := os.Open(artifact.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", artifact.Path, err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("hash %s: %w", artifact.Path, err)
	}
	observed := hex.EncodeToString(hasher.Sum(nil))
	if observed != artifact.SHA256 {
		return fmt.Errorf("artifact SHA-256 for %s = %s, want %s", artifact.Path, observed, artifact.SHA256)
	}
	return nil
}

func validateReleaseArtifactLaunchability(artifact releasePrebuiltArtifact, info os.FileInfo) error {
	if artifact.GOOS == "windows" {
		if !strings.EqualFold(filepath.Ext(artifact.Path), ".exe") {
			return fmt.Errorf("artifact path %s must use the .exe extension for Windows launchability", artifact.Path)
		}
		image, err := pe.Open(artifact.Path)
		if err != nil {
			return fmt.Errorf("artifact path %s is not a launchable Windows executable: %w", artifact.Path, err)
		}
		if err := image.Close(); err != nil {
			return fmt.Errorf("close Windows executable inspection for %s: %w", artifact.Path, err)
		}
		return nil
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("artifact path %s must be executable, mode=%#o", artifact.Path, info.Mode().Perm())
	}
	return nil
}

func (artifact releasePrebuiltArtifact) readBytes() ([]byte, error) {
	if err := artifact.validateFile(); err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(artifact.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", artifact.Path, err)
	}
	if int64(len(contents)) != artifact.Size {
		return nil, fmt.Errorf("read artifact size for %s = %d, want %d", artifact.Path, len(contents), artifact.Size)
	}
	observed := sha256.Sum256(contents)
	if hex.EncodeToString(observed[:]) != artifact.SHA256 {
		return nil, fmt.Errorf("artifact bytes changed while reading: %s", artifact.Path)
	}
	return contents, nil
}

func TestReleasePrebuiltArtifactValidation(t *testing.T) {
	t.Parallel()

	for _, test := range releaseArtifactValidationCases() {
		t.Run(test.name, func(t *testing.T) {
			env := validReleaseArtifactEnvironment(t)
			wantErr, wantText := test.prepare(t, env)
			artifact, err := loadReleasePrebuiltArtifact(func(name string) (string, bool) {
				value, ok := env[name]
				return value, ok
			})
			if wantErr {
				if err == nil || !strings.Contains(err.Error(), wantText) {
					t.Fatalf("load artifact error = %v, want substring %q", err, wantText)
				}
				return
			}
			if err != nil {
				t.Fatalf("load valid artifact: %v", err)
			}
			if artifact.Path != env[releasePrebuiltPathEnv] {
				t.Fatalf("artifact path = %q, want %q", artifact.Path, env[releasePrebuiltPathEnv])
			}
		})
	}
}

type releaseArtifactValidationCase struct {
	name    string
	prepare func(t *testing.T, env map[string]string) (bool, string)
}

func releaseArtifactValidationCases() []releaseArtifactValidationCase {
	return []releaseArtifactValidationCase{
		{name: "absent required field", prepare: func(_ *testing.T, env map[string]string) (bool, string) {
			delete(env, releasePrebuiltPathEnv)
			return true, releasePrebuiltPathEnv
		}},
		{name: "malformed digest", prepare: func(_ *testing.T, env map[string]string) (bool, string) {
			env[releasePrebuiltSHA256Env] = strings.Repeat("A", sha256.Size*2)
			return true, releasePrebuiltSHA256Env
		}},
		{name: "size mismatch", prepare: func(_ *testing.T, env map[string]string) (bool, string) {
			env[releasePrebuiltSizeEnv] = "1"
			return true, "artifact size"
		}},
		{name: "digest mismatch", prepare: func(_ *testing.T, env map[string]string) (bool, string) {
			env[releasePrebuiltSHA256Env] = strings.Repeat("0", sha256.Size*2)
			return true, "artifact SHA-256"
		}},
		{name: "non-executable Unix artifact", prepare: func(t *testing.T, env map[string]string) (bool, string) {
			env[releasePrebuiltGOOSEnv] = "linux"
			if err := os.Chmod(env[releasePrebuiltPathEnv], 0o444); err != nil {
				t.Fatalf("remove artifact execute permission: %v", err)
			}
			return true, "executable"
		}},
		{name: "non-launchable Windows artifact", prepare: func(t *testing.T, env map[string]string) (bool, string) {
			path := filepath.Join(t.TempDir(), "not-launchable.exe")
			contents := []byte("not a Windows executable")
			if err := os.WriteFile(path, contents, 0o555); err != nil {
				t.Fatalf("write non-launchable Windows fixture: %v", err)
			}
			digest := sha256.Sum256(contents)
			env[releasePrebuiltPathEnv] = path
			env[releasePrebuiltSHA256Env] = hex.EncodeToString(digest[:])
			env[releasePrebuiltSizeEnv] = strconv.Itoa(len(contents))
			env[releasePrebuiltGOOSEnv] = "windows"
			return true, "launchable Windows executable"
		}},
		{name: "mutable file", prepare: func(t *testing.T, env map[string]string) (bool, string) {
			if err := os.Chmod(env[releasePrebuiltPathEnv], 0o755); err != nil {
				t.Fatalf("make artifact mutable: %v", err)
			}
			return true, "read-only"
		}},
		{name: "valid descriptor", prepare: func(_ *testing.T, _ map[string]string) (bool, string) {
			return false, ""
		}},
	}
}

func validReleaseArtifactEnvironment(t *testing.T) map[string]string {
	t.Helper()

	sourcePath, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	contents, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read test executable: %v", err)
	}
	name := "you"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, contents, 0o555); err != nil {
		t.Fatalf("write artifact fixture: %v", err)
	}
	if err := os.Chmod(path, 0o555); err != nil {
		t.Fatalf("make artifact read-only: %v", err)
	}
	digest := sha256.Sum256(contents)
	return map[string]string{
		releasePrebuiltPathEnv:         path,
		releasePrebuiltSHA256Env:       hex.EncodeToString(digest[:]),
		releasePrebuiltSizeEnv:         strconv.Itoa(len(contents)),
		releasePrebuiltSourceCommitEnv: strings.Repeat("a", 40),
		releasePrebuiltSourceTreeEnv:   strings.Repeat("b", 40),
		releasePrebuiltToolPathEnv:     path,
		releasePrebuiltToolVersionEnv:  "go version " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH,
		releasePrebuiltGOOSEnv:         runtime.GOOS,
		releasePrebuiltGOARCHEnv:       runtime.GOARCH,
	}
}
