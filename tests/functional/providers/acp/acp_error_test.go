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
// Isolation: isolated-with-reason - OS start failure; sharing would replace
// the refused real command boundary with an already-started peer.
func TestACPCommandStartFailureMapsToDependencyFailure(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP command start failure"}`))
	writeACPWorker(t, dir, "cursor")

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
		if event.Kind != "ERROR" || event.Phase != "FAILED" || event.Provenance.Provider != "cursor" {
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

// Isolation: isolated-with-reason - subprocess stderr; sharing would make the
// configured secret and peer-owned diagnostic stream non-independent.
func TestACPFailureRedactsConfiguredSecretsFromStderr(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP stderr redaction"}`))
	writeACPWorker(t, dir, "cursor")
	workstation := []byte("---\ntype: MODEL_WORKSTATION\nenv:\n  ACP_TEST_API_TOKEN: super-secret-token\n---\n\nTest workstation.\n")
	if err := os.WriteFile(filepath.Join(dir, "workstations", "process", "AGENTS.md"), workstation, 0o600); err != nil {
		t.Fatalf("write ACP workstation environment: %v", err)
	}
	fixture := functionalACPFixture("stderr")

	var starts atomic.Int32
	_, listed, _, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts, fixture),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	for _, event := range responseEvents {
		if event.Kind != "ERROR" || event.Phase != "FAILED" || event.Provenance.Provider != "cursor" {
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

// The peer receives this configured value; failure diagnostics must not echo it.
const acpProtocolFailureSecret = "acp-protocol-failure-super-secret-token"

// Isolation: isolated-with-reason - scenario-owned protocol negotiation.
// Authentication failures must explain all advertised login methods.
func TestACPProtocolFailuresMapToStableWorkerFailureClasses(t *testing.T) {
	for _, test := range []struct {
		mode       string
		want       factoryapi.WorkFailureType
		wantDetail []string
	}{
		{mode: "version", want: factoryapi.WorkFailureTypeMisconfigured},
		{
			mode: "auth",
			want: factoryapi.WorkFailureTypeAuthFailure,
			wantDetail: []string{
				"ACP authentication required",
				"Agent login",
				"Env var login",
				"Terminal login",
			},
		},
	} {
		t.Run(test.mode, func(t *testing.T) {
			t.Parallel()
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP failure"}`))
			writeACPWorker(t, dir, "cursor")
			support.WriteWorkstationConfig(t, dir, "process",
				"---\ntype: MODEL_WORKSTATION\nenv:\n  ACP_TEST_API_TOKEN: "+acpProtocolFailureSecret+"\n---\n\nTest workstation.\n")
			fixture := functionalACPFixture(test.mode)

			var starts atomic.Int32
			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: acpHelperCommandFactory(&starts, fixture),
				ProvidersExecutableLocator:    availableExecutableLocator{},
			}, 20*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
				t.Fatalf("failed work = %d, want 1", got)
			}
			if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
				t.Fatalf("completed work = %d, want 0 for a provider the Factory could not use", got)
			}
			if starts.Load() == 0 {
				t.Fatal("ACP protocol failure did not start the Agent process")
			}
			assertFactoryFailureReason(t, events, test.want, test.wantDetail...)

		})
	}
}

// Isolation: isolated-with-reason - executable lookup; sharing would remove
// the zero-start proof that lookup fails before the ACP process boundary.
func TestUnavailableACPExecutableFailsBeforeStartWithMissingExecutableClass(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"missing ACP executable"}`))
	writeACPWorker(t, dir, "cursor")

	var starts atomic.Int32
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts, functionalACPFixture("1")),
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

// Pin the public failure, absence of an answer, and optional login guidance.
func assertFactoryFailureReason(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want factoryapi.WorkFailureType,
	wantDetail ...string,
) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.FailureDetail == nil || payload.FailureDetail.Reason != want {
			continue
		}
		if strings.Contains(payload.FailureDetail.Message, acpProtocolFailureSecret) {
			t.Fatalf("public failure detail leaked a configured invocation secret: %q", payload.FailureDetail.Message)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeFailed {
			t.Fatalf("failed dispatch outcome = %q, want %q", payload.Outcome, factoryapi.InferenceOutcomeFailed)
		}
		if payload.OutputContent != nil && len(*payload.OutputContent) > 0 {
			t.Fatalf("failed dispatch published a primary answer: %#v", *payload.OutputContent)
		}
		for _, fragment := range wantDetail {
			if !strings.Contains(payload.FailureDetail.Message, fragment) {
				t.Fatalf("public failure detail %q omits required fragment %q", payload.FailureDetail.Message, fragment)
			}
		}
		return
	}
	t.Fatalf("Factory events omitted failure reason %q: %#v", want, events)
}

type missingExecutableLocator struct{}

func (missingExecutableLocator) LookPath(string) (string, error) {
	return "", errors.New("executable not found")
}
