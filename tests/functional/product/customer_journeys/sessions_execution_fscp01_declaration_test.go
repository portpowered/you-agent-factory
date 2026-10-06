package customer_journeys_test

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
	return "UNAVAILABLE"
}
