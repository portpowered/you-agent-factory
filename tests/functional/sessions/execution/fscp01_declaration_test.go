package execution_test

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

type fscp01RunLocations struct {
	Env          []string
	Home         string
	State        string
	Cache        string
	PortLocation string
}

func newFSCP01RunLocations(t *testing.T) fscp01RunLocations {
	t.Helper()
	root := t.TempDir()
	locations := fscp01RunLocations{
		Home:         filepath.Join(root, "home"),
		State:        filepath.Join(root, "state"),
		Cache:        filepath.Join(root, "cache"),
		PortLocation: "127.0.0.1:0 (httptest OS-assigned)",
	}
	for _, path := range []string{locations.Home, locations.State, locations.Cache} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("create isolated FSCP-01 path %q: %v", path, err)
		}
	}

	blocked := map[string]struct{}{
		"APPDATA":                          {},
		"HOME":                             {},
		"HOMEDRIVE":                        {},
		"HOMEPATH":                         {},
		"INFINITE_YOU_OMNIVOICE_CACHE_DIR": {},
		"LOCALAPPDATA":                     {},
		"USERPROFILE":                      {},
		"XDG_CACHE_HOME":                   {},
		"XDG_STATE_HOME":                   {},
	}
	environment := make([]string, 0, len(os.Environ())+9)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, found := blocked[strings.ToUpper(name)]; found {
				continue
			}
		}
		environment = append(environment, entry)
	}
	locations.Env = append(environment,
		"HOME="+locations.Home,
		"USERPROFILE="+locations.Home,
		"HOMEDRIVE="+filepath.VolumeName(locations.Home),
		"HOMEPATH="+string(os.PathSeparator),
		"XDG_STATE_HOME="+locations.State,
		"XDG_CACHE_HOME="+locations.Cache,
		"APPDATA="+locations.State,
		"LOCALAPPDATA="+locations.Cache,
		"INFINITE_YOU_OMNIVOICE_CACHE_DIR="+filepath.Join(locations.Cache, "omnivoice"),
	)
	return locations
}

func logFSCP01RunDeclaration(
	t *testing.T,
	locations fscp01RunLocations,
	factoryDir, recordingPath, processLifetime string,
) {
	t.Helper()
	if strings.TrimSpace(recordingPath) == "" {
		recordingPath = "<none: --no-record>"
	}
	t.Logf(
		"FSCP-01 declaration: platform=%s commit=%s sourcePlanSHA256=%s isolatedHOME=%s isolatedUSERPROFILE=%s isolatedState=%s isolatedCache=%s isolatedFactoryDir=%s isolatedRecordingPath=%s portLocation=%s timeout=15m processLifetime=%s network=none retryBudget=0 providerCallBudget=1",
		runtime.GOOS,
		fscp01CurrentCommit(),
		fscp01SourcePlanSHA256,
		locations.Home,
		locations.Home,
		locations.State,
		locations.Cache,
		factoryDir,
		recordingPath,
		locations.PortLocation,
		processLifetime,
	)
}

func logFSCP01BoundPort(t *testing.T, serverURL string) {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse FSCP-01 server URL %q: %v", serverURL, err)
	}
	t.Logf("FSCP-01 bound port: location=%s", parsed.Host)
}

func fscp01CurrentCommit() string {
	for _, key := range []string{"UNIT_TIMING_COMMIT", "GITHUB_SHA"} {
		if commit := strings.TrimSpace(os.Getenv(key)); commit != "" {
			return commit
		}
	}
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" {
				if commit := strings.TrimSpace(setting.Value); commit != "" {
					return commit
				}
			}
		}
	}
	if workingDir, err := os.Getwd(); err == nil {
		if gitDir := fscp01FindGitDir(workingDir); gitDir != "" {
			if head, err := os.ReadFile(filepath.Join(gitDir, "HEAD")); err == nil {
				ref := strings.TrimSpace(string(head))
				if strings.HasPrefix(ref, "ref: ") {
					refName := strings.TrimSpace(strings.TrimPrefix(ref, "ref: "))
					if commit := fscp01ReadGitRef(gitDir, refName); commit != "" {
						return commit
					}
				} else if ref != "" {
					return ref
				}
			}
		}
	}
	return "UNAVAILABLE"
}

func fscp01FindGitDir(start string) string {
	for current := filepath.Clean(start); ; current = filepath.Dir(current) {
		gitPath := filepath.Join(current, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return gitPath
			}
			data, readErr := os.ReadFile(gitPath)
			line := strings.TrimSpace(string(data))
			if strings.HasPrefix(line, "gitdir: ") {
				resolved := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
				if !filepath.IsAbs(resolved) {
					resolved = filepath.Join(current, resolved)
				}
				return filepath.Clean(resolved)
			}
			if readErr != nil {
				return ""
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func fscp01ReadGitRef(gitDir, refName string) string {
	refPath := filepath.Join(gitDir, filepath.FromSlash(refName))
	if resolved, err := os.ReadFile(refPath); err == nil {
		return strings.TrimSpace(string(resolved))
	}
	commonDirData, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return ""
	}
	commonDir := strings.TrimSpace(string(commonDirData))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	resolved, err := os.ReadFile(filepath.Join(commonDir, filepath.FromSlash(refName)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(resolved))
}
