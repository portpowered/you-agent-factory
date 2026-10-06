package cli_rest_journeys_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type fscp01ResponseRunLocations struct {
	Env          []string
	Home         string
	State        string
	Cache        string
	PortLocation string
}

func newFSCP01ResponseRunLocations(t *testing.T) fscp01ResponseRunLocations {
	t.Helper()
	root := t.TempDir()
	locations := fscp01ResponseRunLocations{
		Home:         filepath.Join(root, "home"),
		State:        filepath.Join(root, "state"),
		Cache:        filepath.Join(root, "cache"),
		PortLocation: "127.0.0.1:0 (httptest OS-assigned)",
	}
	for _, path := range []string{locations.Home, locations.State, locations.Cache} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("create isolated FSCP-01 response path %q: %v", path, err)
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

func logFSCP01ResponseRunDeclaration(
	t *testing.T,
	locations fscp01ResponseRunLocations,
	factoryDir, processLifetime string,
) {
	t.Helper()
	t.Logf(
		"FSCP-01 declaration: platform=%s commit=%s sourcePlanSHA256=%s isolatedHOME=%s isolatedUSERPROFILE=%s isolatedState=%s isolatedCache=%s isolatedFactoryDir=%s isolatedRecordingPath=<none: --no-record> portLocation=%s timeout=15m processLifetime=%s network=none retryBudget=0 providerCallBudget=2",
		runtime.GOOS,
		fscp01ResponseCurrentCommit(),
		fscp01ResponseSourcePlanSHA256,
		locations.Home,
		locations.Home,
		locations.State,
		locations.Cache,
		factoryDir,
		locations.PortLocation,
		processLifetime,
	)
}

type fscp01FirstThenGatedRunner struct {
	delegate      *support.ShapedProviderCommandRunner
	gate          <-chan struct{}
	callCount     atomic.Int32
	secondStarted chan struct{}
	secondOnce    atomic.Bool
}

func newFSCP01FirstThenGatedRunner(gate <-chan struct{}) *fscp01FirstThenGatedRunner {
	return &fscp01FirstThenGatedRunner{
		delegate:      support.NewShapedProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("fscp01 response teardown provider COMPLETE")}),
		gate:          gate,
		secondStarted: make(chan struct{}),
	}
}

func (r *fscp01FirstThenGatedRunner) Run(
	ctx context.Context,
	req platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	call := r.callCount.Add(1)
	if call >= 2 && r.secondOnce.CompareAndSwap(false, true) {
		close(r.secondStarted)
	}
	if call >= 2 {
		select {
		case <-r.gate:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return r.delegate.Run(ctx, req)
}

func (r *fscp01FirstThenGatedRunner) WaitForSecondCall(t *testing.T) {
	t.Helper()
	select {
	case <-r.secondStarted:
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for second controlled provider command to enter its gate")
	}
}

func (r *fscp01FirstThenGatedRunner) CallCount() int32 {
	return r.callCount.Load()
}

func logFSCP01ResponseBoundPort(t *testing.T, serverURL string) {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse FSCP-01 response server URL %q: %v", serverURL, err)
	}
	t.Logf("FSCP-01 bound port: location=%s", parsed.Host)
}

const fscp01ResponseSourcePlanSHA256 = "058bf1a1e74cbc64dfedb89bb83f0cbc3b805f941d489bb24bd207e00371794a"

func fscp01ResponseCurrentCommit() string {
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
