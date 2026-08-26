package process_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// youPackageArtifact is the one lazily built, immutable ./cmd/factory binary
// shared by every executable-boundary row in this package. Rows that claim a
// built-executable, stream, OS-exit, supported-signal, PID, or cleanup property
// must start this exact artifact as a real OS child process.
type youPackageArtifact struct {
	Path       string
	Absolute   string
	SHA256     string
	SizeBytes  int64
	Revision   string
	GOOS       string
	GOARCH     string
	GoVersion  string
	SourceRoot string
}

var (
	artifactOnce   sync.Once
	artifactValue  youPackageArtifact
	artifactErr    error
	artifactTmpDir string
)

// packageArtifact returns the package-built you artifact, building it exactly
// once per test-binary process with concurrency-safe publication.
func packageArtifact(t testing.TB) youPackageArtifact {
	t.Helper()
	artifactOnce.Do(func() {
		repoRoot, err := findRepoRootFromTest()
		if err != nil {
			artifactErr = fmt.Errorf("resolve repo root: %w", err)
			return
		}
		var dirErr error
		artifactTmpDir, dirErr = os.MkdirTemp("", "you-cli-functional-process-artifact-")
		if dirErr != nil {
			artifactErr = fmt.Errorf("create artifact directory: %w", dirErr)
			return
		}
		binaryName := "you"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		finalPath := filepath.Join(artifactTmpDir, binaryName)
		stagingPath := finalPath + ".staging"
		build := exec.Command("go", "build", "-o", stagingPath, "./cmd/factory")
		build.Dir = repoRoot
		if buildLog, buildErr := build.CombinedOutput(); buildErr != nil {
			artifactErr = fmt.Errorf("build you CLI: %v\n%s", buildErr, buildLog)
			return
		}
		if renameErr := os.Rename(stagingPath, finalPath); renameErr != nil {
			artifactErr = fmt.Errorf("publish artifact atomically: %w", renameErr)
			return
		}
		info, statErr := os.Stat(finalPath)
		if statErr != nil {
			artifactErr = fmt.Errorf("stat artifact: %w", statErr)
			return
		}
		contents, readErr := os.ReadFile(finalPath)
		if readErr != nil {
			artifactErr = fmt.Errorf("read artifact for digest: %w", readErr)
			return
		}
		digest := sha256.Sum256(contents)
		versionOutput, verErr := exec.Command("go", "version", finalPath).CombinedOutput()
		if verErr != nil {
			artifactErr = fmt.Errorf("probe artifact go version: %v\n%s", verErr, versionOutput)
			return
		}
		fields := strings.Fields(string(versionOutput))
		goVersion := ""
		for _, field := range fields {
			if strings.HasPrefix(field, "go") && strings.Contains(field, ".") {
				goVersion = field
				break
			}
		}
		modOutput, modErr := exec.Command("go", "version", "-m", finalPath).CombinedOutput()
		if modErr != nil {
			artifactErr = fmt.Errorf("probe artifact build metadata: %v\n%s", modErr, modOutput)
			return
		}
		revision := ""
		for _, line := range strings.Split(string(modOutput), "\n") {
			if value, ok := strings.CutPrefix(strings.TrimSpace(line), "build\t-ldflags"); ok && strings.Contains(value, "revision") {
				continue
			}
			if _, settings, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
				for _, setting := range strings.Split(settings, " ") {
					if revision == "" {
						revision = parseVCSSetting(setting, "vcs.revision")
					}
				}
			} else if parsed := parseVCSSetting(strings.TrimSpace(line), "vcs.revision"); parsed != "" {
				revision = parsed
			}
		}
		absolute, absErr := filepath.Abs(finalPath)
		if absErr != nil {
			artifactErr = fmt.Errorf("resolve artifact absolute path: %w", absErr)
			return
		}
		artifactValue = youPackageArtifact{
			Path:       finalPath,
			Absolute:   absolute,
			SHA256:     hex.EncodeToString(digest[:]),
			SizeBytes:  info.Size(),
			Revision:   revision,
			GOOS:       runtime.GOOS,
			GOARCH:     runtime.GOARCH,
			GoVersion:  goVersion,
			SourceRoot: repoRoot,
		}
	})
	if artifactErr != nil {
		t.Fatalf("package you artifact unavailable: %v", artifactErr)
	}
	return artifactValue
}

// findRepoRootFromTest walks upward from this file to find the repo root.
func findRepoRootFromTest() (string, error) {
	_, callerFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot determine caller file path")
	}
	current := filepath.Clean(filepath.Dir(callerFile))
	for {
		goModPath := filepath.Join(current, "go.mod")
		if info, err := os.Stat(goModPath); err == nil && !info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}

func parseVCSSetting(field, key string) string {
	value, ok := strings.CutPrefix(field, key+"=")
	if !ok {
		return ""
	}
	return value
}

// releasePackageArtifact removes the artifact directory after the package
// finishes so no temporary artifact leaks past the test binary.
func releasePackageArtifact() {
	if artifactTmpDir != "" {
		_ = os.RemoveAll(artifactTmpDir)
	}
}
