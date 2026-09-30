package acp_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each scenario owns its provider process because its installed Pi version
// and startup response are deliberately different external effect outcomes.
func TestPiACPRejectsIncompatibleVersionsAndSurfacesModelFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, version, mode, diagnostic string }{
		{"minimum supported", "pi v0.81.0", "pi-startup", ""},
		{"new major", "1.0.0", "pi-startup", ""},
		{"new minor", "0.82.0", "pi-startup", ""},
		{"old minor", "0.80.99", "pi-startup", "Pi 0.81.0 or newer"},
		{"malformed", "not-a-version", "pi-startup", "Pi 0.81.0 or newer"},
		{"invalid component", "0.x.0", "pi-startup", "Pi 0.81.0 or newer"},
		{"negative component", "0.-1.0", "pi-startup", "Pi 0.81.0 or newer"},
		{"missing executable", "missing", "pi-startup", "Pi executable is unavailable"},
		{"probe start failure", "probe-error", "pi-startup", "Pi executable is unavailable"},
		{"missing command", "nil-command", "pi-startup", "Pi 0.81.0 or newer"},
		{"model connection failure", "0.81.0", "pi-failure", "could not connect to the selected model endpoint"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"Pi ACP startup compatibility"}`))
			writeACPWorker(t, dir, "pi")
			var starts atomic.Int32
			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: piACPCommandFactory(&starts, test.version, test.mode),
				ProvidersExecutableLocator:    piACPExecutableLocator{missing: test.version == "missing"},
			}, 20*time.Second)
			state := "task:done"
			if test.diagnostic != "" {
				state = "task:failed"
			}
			if got := support.CountWorkAtCustomerState(listed, state); got != 1 {
				t.Fatalf("Work at %s = %d, want 1; %s", state, got, acpFailureDiagnostics(events))
			}
			if test.diagnostic != "" {
				encoded, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(encoded), test.diagnostic) {
					t.Fatalf("missing customer diagnostic %q: %s", test.diagnostic, encoded)
				}
			}
		})
	}
}

type piACPExecutableLocator struct{ missing bool }

func (piACPExecutableLocator) CurrentExecutable() (string, error) { return os.Executable() }

func (locator piACPExecutableLocator) LookPath(name string) (string, error) {
	if name == "pi" && locator.missing {
		return "", errors.New("Pi is not installed")
	}
	return name, nil
}

func piACPCommandFactory(starts *atomic.Int32, version, mode string) platformprocess.CommandFactory {
	provider := acpHelperCommandFactory(starts, functionalACPFixture(mode))
	versionFixture := functionalACPFixture("pi-version")
	versionFixture.SessionID = version
	probe := acpHelperCommandFactory(new(atomic.Int32), versionFixture)
	return func(name string, args ...string) *exec.Cmd {
		if name == "pi" && sameStringSlice(args, []string{"--version"}) {
			if version == "nil-command" {
				return nil
			}
			if version == "probe-error" {
				return probe("you-test-unavailable-pi-version-command")
			}
			return probe("cursor-agent", "acp")
		}
		if sameStringSlice(args, []string{"pi-acp"}) {
			return provider("cursor-agent", "acp")
		}
		return provider(name, args...)
	}
}

// Isolation: isolated-with-reason - pinned wire peer; each failure branch
// requires a fresh golden subprocess and exact session/config RPC diagnostics.
func TestYouRunMapsGoldenSessionAndConfigRPCFailuresToTerminalWork(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		mode       string
		diagnostic string
	}{
		{mode: "new-fail", diagnostic: "golden session/new failure"},
		{mode: "config-fail", diagnostic: "golden model config failure"},
	} {
		test := test
		t.Run(test.mode, func(t *testing.T) {
			t.Parallel()
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"golden ACP failure"}`))
			writeACPWorker(t, dir, "cursor")
			writeGoldenSentinelWorkstation(t, dir)
			fixture := goldenACPFixture(test.mode)

			var starts atomic.Int32
			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: goldenACPCommandFactory(&starts, fixture),
				ProvidersExecutableLocator:    availableExecutableLocator{},
			}, 20*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
				t.Fatalf("failed work = %d, want 1", got)
			}
			if starts.Load() != 1 {
				t.Fatalf("ACP process starts = %d, want 1", starts.Load())
			}
			encoded, err := json.Marshal(events)
			if err != nil {
				t.Fatalf("marshal Factory events: %v", err)
			}
			if !strings.Contains(string(encoded), test.diagnostic) {
				t.Fatalf("Factory events omitted RPC diagnostic %q: %s", test.diagnostic, encoded)
			}
		})
	}
}
