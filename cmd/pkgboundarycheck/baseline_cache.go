package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The base-tree scan is a pure function of the base commit's tree and of this
// checker build. Extracting and rescanning that tree costs more than the scan
// of the working tree, so the resulting fingerprints are memoized under
// -baseline-cache-dir. The key names the resolved base commit, the package
// root, and a digest of the running executable, so any checker change or base
// move misses the cache and recomputes. Cache failures are never errors.
const baselineCacheVersion = "v1"

func baselineCacheFile(cfg config, repoRoot, ref string) string {
	cacheDir := resolveBaselineCacheDir(cfg.baselineCacheDir)
	if cacheDir == "" {
		return ""
	}
	output, ok := runGit(repoRoot, "rev-parse", "--verify", ref+"^{commit}")
	if !ok {
		return ""
	}
	executableDigest, ok := currentExecutableDigest()
	if !ok {
		return ""
	}
	key := sha256.Sum256([]byte(strings.Join([]string{
		baselineCacheVersion, strings.TrimSpace(string(output)), cfg.packageRoot, executableDigest,
	}, "\x00")))
	return filepath.Join(cacheDir, "pkgboundary-base-"+hex.EncodeToString(key[:16])+".json")
}

// resolveBaselineCacheDir maps "auto" to a per-user cache directory shared by
// every worktree and leaves an empty value disabled.
func resolveBaselineCacheDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir != "auto" {
		return dir
	}
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		return ""
	}
	return filepath.Join(base, "you-lint", "pkgboundary")
}

func currentExecutableDigest() (string, bool) {
	path, err := os.Executable()
	if err != nil {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", false
	}
	return hex.EncodeToString(digest.Sum(nil)), true
}

func readBaselineCache(path string) (map[string]struct{}, bool) {
	if path == "" {
		return nil, false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var fingerprints []string
	if err := json.Unmarshal(content, &fingerprints); err != nil {
		return nil, false
	}
	result := make(map[string]struct{}, len(fingerprints))
	for _, fingerprint := range fingerprints {
		result[fingerprint] = struct{}{}
	}
	return result, true
}

func writeBaselineCache(path string, fingerprints map[string]struct{}) {
	if path == "" {
		return
	}
	sorted := make([]string, 0, len(fingerprints))
	for fingerprint := range fingerprints {
		sorted = append(sorted, fingerprint)
	}
	sort.Strings(sorted)
	content, err := json.Marshal(sorted)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pkgboundary-base-*")
	if err != nil {
		return
	}
	temporaryPath := temporary.Name()
	_, writeErr := temporary.Write(content)
	closeErr := temporary.Close()
	if writeErr != nil || closeErr != nil || os.Rename(temporaryPath, path) != nil {
		_ = os.Remove(temporaryPath)
	}
}
