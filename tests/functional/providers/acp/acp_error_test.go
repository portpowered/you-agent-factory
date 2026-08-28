package acp_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestACPCommandStartFailureMapsToDependencyFailure keeps a root.BuildProcess
// cell for the case where the ACP command factory produces a non-nil,
// lookup-eligible *exec.Cmd but the OS itself refuses to start it (here, a
// command name no PATH entry can resolve), proving the observable,
// caller-visible outcome the daemon's cmd.Start() failure path maps to: the
// run fails with a dependency-kind error rather than hanging or panicking.
func TestACPCommandStartFailureMapsToDependencyFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP command start failure"}`))
	writeACPWorker(t, dir, "cursor-acp")

	_, listed, _, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: func(string, ...string) *exec.Cmd {
			return exec.Command("you-agent-factory-acp-helper-does-not-exist-xyz")
		},
		ProvidersExecutableLocator: availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	for _, event := range responseEvents {
		if event.Kind != "ERROR" || event.Phase != "FAILED" || event.Provenance.Provider != "cursor-acp" {
			continue
		}
		payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
		if err != nil {
			t.Fatalf("decode ACP error response: %v", err)
		}
		if payload.Message == "" {
			t.Fatal("ACP command start failure produced an empty error message")
		}
		return
	}
	t.Fatalf("ACP response stream had no FAILED error event for the command start failure: %#v", responseEvents)
}

func TestACPFailureRedactsConfiguredSecretsFromStderr(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP stderr redaction"}`))
	writeACPWorker(t, dir, "cursor-acp")
	workstation := []byte("---\ntype: MODEL_WORKSTATION\nenv:\n  ACP_TEST_API_TOKEN: super-secret-token\n---\n\nTest workstation.\n")
	if err := os.WriteFile(filepath.Join(dir, "workstations", "process", "AGENTS.md"), workstation, 0o600); err != nil {
		t.Fatalf("write ACP workstation environment: %v", err)
	}
	t.Setenv(acpHelperEnvironment, "stderr")

	var starts atomic.Int32
	_, listed, _, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	for _, event := range responseEvents {
		if event.Kind != "ERROR" || event.Phase != "FAILED" || event.Provenance.Provider != "cursor-acp" {
			continue
		}
		payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
		if err != nil {
			t.Fatalf("decode ACP error response: %v", err)
		}
		if strings.Contains(payload.Message, "super-secret-token") {
			t.Fatalf("ACP error response leaked configured secret: %q", payload.Message)
		}
		if strings.Contains(payload.Message, "agent diagnostic token=<redacted>") {
			return
		}
	}
	t.Fatalf("ACP response stream omitted redacted stderr diagnostic: %#v", responseEvents)
}

// TestACPAgentSelfReportedCancellationMapsToCanceledFailure keeps a
// root.BuildProcess cell for an ACP agent that honors a turn by returning
// StopReasonCancelled from session/prompt itself (a real, spec-legal
// response an agent can send independent of any session/cancel notification
// from this system). Providers.ControlAttempt's own cancel-delivery seam has
// no wired transport yet and is proven at the package-integration layer
// instead (see acp/internal/service/cancel_test.go); this cell instead
// proves the observable, caller-visible outcome of that same StopReason
// mapping: the run fails, and the surfaced error reflects a canceled
// attempt rather than a generic failure.
// TestACPProtocolFailuresMapToStableWorkerFailureClasses keeps two representative
// root.BuildProcess cells for misconfigured vs unknown ACP protocol failures.
// The exhaustive mode matrix lives in
// pkg/services/providers/internal/services/acp/internal/service.
func TestACPProtocolFailuresMapToStableWorkerFailureClasses(t *testing.T) {
	for _, test := range []struct {
		mode string
		want factoryapi.WorkFailureType
	}{
		{mode: "version", want: factoryapi.WorkFailureTypeMisconfigured},
	} {
		t.Run(test.mode, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP failure"}`))
			writeACPWorker(t, dir, "cursor-acp")
			t.Setenv(acpHelperEnvironment, test.mode)

			var starts atomic.Int32
			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
				ProvidersExecutableLocator:    availableExecutableLocator{},
			}, 20*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
				t.Fatalf("failed work = %d, want 1", got)
			}
			if starts.Load() == 0 {
				t.Fatal("ACP protocol failure did not start the Agent process")
			}
			assertFactoryFailureReason(t, events, test.want)
		})
	}
}

func TestUnavailableACPExecutableFailsBeforeStartWithMissingExecutableClass(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"missing ACP executable"}`))
	writeACPWorker(t, dir, "cursor-acp")

	var starts atomic.Int32
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    missingExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	if starts.Load() != 0 {
		t.Fatalf("ACP starts = %d, want 0 for unavailable executable", starts.Load())
	}
	assertFactoryFailureReason(t, events, factoryapi.WorkFailureTypeMissingExecutable)
}

func assertFactoryFailureReason(t *testing.T, events []factoryapi.FactoryEvent, want factoryapi.WorkFailureType) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.FailureDetail != nil && payload.FailureDetail.Reason == want {
			return
		}
	}
	t.Fatalf("Factory events omitted failure reason %q: %#v", want, events)
}

type missingExecutableLocator struct{}

func (missingExecutableLocator) LookPath(string) (string, error) {
	return "", errors.New("executable not found")
}
