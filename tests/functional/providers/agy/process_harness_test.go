package agy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const agyFunctionalModel = "gemini-pro"

// TestAgyConductorSuccessThroughRootBuildProcess proves successful Agy PTY
// execution through the customer process boundary and Providers-backed adapter.
func TestAgyConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, agyFunctionalModel),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy conductor success"}`))

	executablePath := writeAgyFixtureExecutable(t)
	ptyHost := newFunctionalPTYHost([]byte("agy functional answer COMPLETE"), 0)
	clock := platformclock.NewDeterministic(time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC), time.Millisecond)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		agyFunctionalEdges(executablePath, ptyHost, clock),
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if ptyHost.callCount() != 1 {
		t.Fatalf("agy PTY host starts = %d, want 1 through Providers path", ptyHost.callCount())
	}
	launch := ptyHost.lastLaunch()
	if launch.Executable != executablePath {
		t.Fatalf("executable = %q, want fixture %q", launch.Executable, executablePath)
	}
	if !containsArgPair(launch.Argv, "--model", agyFunctionalModel) {
		t.Fatalf("argv = %#v, want --model %s", launch.Argv, agyFunctionalModel)
	}
	if !containsArg(launch.Argv, "chat") || !containsArg(launch.Argv, "--headless") {
		t.Fatalf("argv = %#v, want chat and --headless", launch.Argv)
	}
	assertAgyFinalOnlyCompletion(t, events, responseEvents, "agy functional answer COMPLETE")
}

// TestAgyNativeFailureThroughRootBuildProcessIsSafe proves native Agy failures
// remain safe and observable through the customer process boundary.
func TestAgyNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		agyFunctionalModel,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy native failure"}`))

	const leaked = "/tmp/secret-key"
	executablePath := writeAgyFixtureExecutable(t)
	ptyHost := newFunctionalPTYHost([]byte("authentication failed: token path "+leaked+" leaked"), 1)
	clock := platformclock.NewDeterministic(time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC), time.Millisecond)

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		agyFunctionalEdges(executablePath, ptyHost, clock),
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0 after native failure", got)
	}
	if ptyHost.callCount() != 1 {
		t.Fatalf("agy PTY host starts = %d, want 1", ptyHost.callCount())
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) || strings.Contains(payload, "secret-key") {
		t.Fatalf("Factory events leaked unsafe Agy failure detail: %s", payload)
	}
	assertAgyProviderSession(t, events, factoryapi.InferenceOutcomeFailed, string(modelprovider.ProviderAntigravity))
}

// TestAgyTimeoutFailureThroughRootBuildProcess proves timeout normalization
// through the customer process boundary without leaking partial output.
func TestAgyTimeoutFailureThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		agyFunctionalModel,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy timeout failure"}`))

	executablePath := writeAgyFixtureExecutable(t)
	ptyHost := newFunctionalPTYHost([]byte("partial answer before timeout"), 124)
	clock := platformclock.NewDeterministic(time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC), time.Millisecond)

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		agyFunctionalEdges(executablePath, ptyHost, clock),
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0 after timeout", got)
	}
	reason := terminalFailureReason(t, events)
	if reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want %q", reason, factoryapi.WorkFailureTypeTimeout)
	}
	assertAgyProviderSession(t, events, factoryapi.InferenceOutcomeFailed, string(modelprovider.ProviderAntigravity))
}

// TestAgyCommandCancellationThroughRootBuildProcessIsCanonical proves
// cancellation returns the canonical outcome through the Providers PTY adapter.
func TestAgyCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		agyFunctionalModel,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy command cancel"}`))

	executablePath := writeAgyFixtureExecutable(t)
	ptyHost := &canceledPTYHost{}
	clock := platformclock.NewDeterministic(time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC), time.Millisecond)

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		agyFunctionalEdges(executablePath, ptyHost, clock),
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if ptyHost.callCount() != 1 {
		t.Fatalf("agy PTY host starts = %d, want 1", ptyHost.callCount())
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if !strings.Contains(payload, "provider invocation was canceled") {
		t.Fatalf("Factory events missing canonical cancellation outcome: %s", payload)
	}
}

func agyFunctionalEdges(
	executablePath string,
	ptyHost platformpty.Host,
	clock platformclock.Source,
) serviceedges.Edges {
	return serviceedges.Edges{
		AgyPTYHost:               ptyHost,
		AgyPTYClock:              clock,
		WorkersExecutableLocator: fixedExecutableLocator{path: executablePath},
		WorkersResolveSymlinks:   identityResolveSymlinks,
	}
}

type fixedExecutableLocator struct {
	path string
}

func (l fixedExecutableLocator) LookPath(file string) (string, error) {
	if file == "agy" {
		return l.path, nil
	}
	return "", fmt.Errorf("executable %q not found", file)
}

func identityResolveSymlinks(path string) (string, error) {
	return path, nil
}

func writeAgyFixtureExecutable(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("agy-fixture-executable\n"), 0o755); err != nil {
		t.Fatalf("write agy fixture executable: %v", err)
	}
	return path
}

type functionalPTYHost struct {
	stdout   []byte
	exitCode int
	starts   int
	launches []platformpty.ProcessLaunch
}

func newFunctionalPTYHost(stdout []byte, exitCode int) *functionalPTYHost {
	return &functionalPTYHost{
		stdout:   append([]byte(nil), stdout...),
		exitCode: exitCode,
	}
}

func (h *functionalPTYHost) callCount() int {
	return h.starts
}

func (h *functionalPTYHost) lastLaunch() platformpty.ProcessLaunch {
	if len(h.launches) == 0 {
		return platformpty.ProcessLaunch{}
	}
	return h.launches[len(h.launches)-1]
}

func (h *functionalPTYHost) Allocate(context.Context) (platformpty.Allocation, error) {
	return functionalPTYAllocation{}, nil
}

func (h *functionalPTYHost) Start(
	launch platformpty.ProcessLaunch,
	_ platformpty.Allocation,
) (platformpty.Process, io.ReadCloser, error) {
	h.starts++
	h.launches = append(h.launches, launch)
	return &functionalPTYProcess{exitCode: h.exitCode}, io.NopCloser(bytes.NewReader(h.stdout)), nil
}

type canceledPTYHost struct {
	starts int
}

func (h *canceledPTYHost) callCount() int {
	return h.starts
}

func (h *canceledPTYHost) Allocate(context.Context) (platformpty.Allocation, error) {
	return functionalPTYAllocation{}, nil
}

func (h *canceledPTYHost) Start(
	_ platformpty.ProcessLaunch,
	_ platformpty.Allocation,
) (platformpty.Process, io.ReadCloser, error) {
	h.starts++
	return nil, nil, context.Canceled
}

type functionalPTYAllocation struct{}

func (functionalPTYAllocation) Close() error           { return nil }
func (functionalPTYAllocation) Kind() platformpty.Kind { return platformpty.KindPOSIX }

type functionalPTYProcess struct {
	exitCode int
}

func (p *functionalPTYProcess) Wait() error      { return nil }
func (p *functionalPTYProcess) Terminate() error { return nil }
func (*functionalPTYProcess) Close()             {}
func (*functionalPTYProcess) PID() int           { return 0 }
func (p *functionalPTYProcess) ExitCode() int    { return p.exitCode }

func assertAgyFinalOnlyCompletion(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
	wantOutput string,
) {
	t.Helper()

	var completedMessages int
	for _, event := range responseEvents {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase == factoryapi.FactoryResponseEventPhaseDelta {
				t.Fatalf("final-only Agy replay fabricated message delta: %#v", event)
			}
			if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
				completedMessages++
			}
		case factoryapi.FactoryResponseEventKindTool, factoryapi.FactoryResponseEventKindUsage:
			t.Fatalf("final-only Agy replay fabricated lifecycle: %#v", event)
		}
	}
	if completedMessages != 1 {
		t.Fatalf("completed message events = %d, want exactly one terminal result", completedMessages)
	}

	dispatchOutput := ""
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output != nil && *payload.Output != "" {
			dispatchOutput = *payload.Output
		}
	}
	if dispatchOutput != wantOutput {
		t.Fatalf("dispatch output = %q, want %q", dispatchOutput, wantOutput)
	}
}

func assertAgyProviderSession(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantOutcome factoryapi.InferenceOutcome,
	wantProvider string,
) {
	t.Helper()

	var found bool
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome != wantOutcome {
			continue
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Provider == nil {
			t.Fatal("inference response missing provider session metadata")
		}
		if got := support.StringPointerValue(payload.ProviderSession.Provider); got != wantProvider {
			t.Fatalf("provider session provider = %q, want %q", got, wantProvider)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("missing inference response with outcome %q", wantOutcome)
	}
}

func terminalFailureReason(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.WorkFailureType {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome == factoryapi.InferenceOutcomeFailed && payload.FailureDetail != nil {
			return payload.FailureDetail.Reason
		}
	}
	t.Fatal("missing failed inference response with failure detail")
	return ""
}

func containsArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
