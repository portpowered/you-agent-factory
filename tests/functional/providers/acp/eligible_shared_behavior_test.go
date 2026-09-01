package acp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedACPScenarioTimeout = 20 * time.Second

func runACPSharedEligibleBehavior(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	scenarios := []struct {
		name string
		run  func(*testing.T, *acpSharedProcessFixture)
	}{
		{"baseline/success-and-failure-isolation", runACPSharedBaseline},
		{"executor-provider-selection", runACPSharedExecutorProvider},
		{"generic-protocol-failure", runACPSharedGenericFailure},
		{"legacy-acp-executor-spelling", runACPSharedLegacyExecutor},
		{"operator-configured-provider", runACPSharedConfiguredProvider},
		{"self-reported-cancellation", runACPSharedCancellation},
		{"response-events-and-worker-session-history", runACPSharedResponseEvents},
		{"partial-output-failure", runACPSharedPartialFailure},
		{"authentication-required", runACPSharedAuthentication},
		{"advertised-model-config", runACPSharedModel},
		{"canonical-resource-link", runACPSharedResource},
		{"characterization-success-parity", runACPSharedBTRCSuccess},
		{"characterization-failure-parity", runACPSharedBTRCFailure},
		{"input-work-content", runACPSharedInputContent},
		{"catalog-invalid-mutations", runACPSharedInvalidCatalogMutations},
		{"catalog-persistence", runACPSharedCatalogPersistence},
		{"catalog-init-idempotency", runACPSharedCatalogInit},
		{"javascript-acp-run", runACPSharedJavaScriptACP},
		{"mixed-acp-and-legacy-routing", runACPSharedMixedRouting},
	}
	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			scenario.run(t, fixture)
		})
		if t.Failed() {
			return
		}
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		t.Fatalf("shared ACP root constructions = %d, want 1", got)
	}
	if got := fixture.peerStarts.Load(); got != 17 {
		t.Fatalf("shared ACP peer starts = %d, want 17 eligible ACP Work witnesses", got)
	}
	fixture.assertSessionTopology(t)
}

func runACPSharedBaseline(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("shared-spine")
	success := fixture.openSession(t)
	failure := fixture.openSession(t)

	successRun := success.run(t, "shared-success", "shared ACP success")
	assertACPSharedSuccess(t, successRun)

	failureRun := failure.run(t, "shared-failure", "shared ACP failure")
	assertACPSharedFailure(t, failureRun)
	assertACPSharedSessionIsolation(t, success, failure, successRun, failureRun)

	failure.close(t)
	success.close(t)
}

func runACPSharedExecutorProvider(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("1")
	legacyCalls := fixture.legacy.calls.Load()
	session := fixture.openSession(t)
	run := session.run(t, "acp-vertical-slice", "ACP vertical slice")
	if got := support.CountWorkAtCustomerState(run.listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(run.listed, "task:failed"); got != 0 {
		t.Fatalf("failed Work = %d, want 0", got)
	}
	assertACPProviderSession(t, run.events)
	if got := fixture.legacy.calls.Load(); got != legacyCalls {
		t.Fatalf("legacy provider calls = %d, want unchanged %d", got, legacyCalls)
	}
	session.close(t)
}

func runACPSharedGenericFailure(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("fail")
	session := fixture.openSession(t)
	run := session.run(t, "generic-protocol-failure", "generic protocol failure")
	assertACPSharedFailure(t, run)
	assertFactoryFailureReason(t, run.events, factoryapi.WorkFailureTypeUnknown)
	session.close(t)
}

func runACPSharedLegacyExecutor(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("1")
	session := fixture.openSessionWith(t, func(t *testing.T, dir string) {
		writeLegacyACPWorker(t, dir, "cursor-acp")
	})
	run := session.run(t, "legacy-acp-spelling", "legacy ACP spelling")
	assertACPSharedSuccess(t, run)
	session.close(t)
}

func runACPSharedConfiguredProvider(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("1")
	session := fixture.openSessionWith(t, func(t *testing.T, dir string) {
		writeACPWorker(t, dir, "custom-acp")
	})
	run := session.run(t, "configured-acp", "configured ACP provider")
	if got := support.CountWorkAtCustomerState(run.listed, "task:done"); got != 1 {
		t.Fatalf("configured ACP completed Work = %d, want 1; events=%#v", got, run.events)
	}
	assertProviderSession(t, run.events, "custom-acp")
	session.close(t)
}

func runACPSharedCancellation(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("cancelled-response")
	session := fixture.openSession(t)
	run := session.run(t, "self-cancel", "ACP self-reported cancellation")
	if got := support.CountWorkAtCustomerState(run.listed, "task:failed"); got != 1 {
		t.Fatalf("canceled Work = %d, want 1", got)
	}
	assertACPSharedCanceledResponse(t, run.responseEvents)
	session.close(t)
}

func runACPSharedResponseEvents(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("1")
	session := fixture.openSession(t)
	run := session.runRequest(t, sharedACPSubmitRequest("response-events", "ACP response events"), true)
	if got := support.CountWorkAtCustomerState(run.listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want 1", got)
	}
	assertACPProviderSession(t, run.events)
	assertResponseEventsStayOutOfFactoryReplay(t, run.events)
	assertACPResponseEventSequence(t, run.responseEvents)
	assertACPWorkerSessionHistory(t, run.workerEvents)
	session.close(t)
}

func runACPSharedPartialFailure(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("fail")
	session := fixture.openSession(t)
	run := session.run(t, "partial-failure", "ACP partial failure")
	if got := support.CountWorkAtCustomerState(run.listed, "task:failed"); got != 1 {
		t.Fatalf("failed Work = %d, want 1", got)
	}
	assertACPSharedTerminalError(t, run.responseEvents)
	session.close(t)
}

func runACPSharedAuthentication(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("auth")
	session := fixture.openSession(t)
	run := session.run(t, "authentication", "ACP auth")
	if got := support.CountWorkAtCustomerState(run.listed, "task:failed"); got != 1 {
		t.Fatalf("failed Work = %d, want 1", got)
	}
	for _, event := range run.events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.FailureDetail != nil &&
			payload.FailureDetail.Reason == factoryapi.WorkFailureTypeAuthFailure &&
			strings.Contains(payload.FailureDetail.Message, "Agent login") {
			session.close(t)
			return
		}
	}
	t.Fatalf("Factory events omitted actionable ACP authentication failure: %#v", run.events)
}

func runACPSharedModel(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("model")
	session := fixture.openSession(t)
	run := session.run(t, "model-config", "ACP model config")
	if got := support.CountWorkAtCustomerState(run.listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want 1; events=%#v", got, run.events)
	}
	session.close(t)
}

func runACPSharedResource(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("resource")
	contentType := "image/png"
	label := "fixture"
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
		Type:        factoryapi.WorkContentPartTypeImage,
		Url:         "https://example.test/fixture.png",
		Label:       &label,
		ContentType: &contentType,
	}); err != nil {
		t.Fatalf("build image Work content: %v", err)
	}
	content := factoryapi.WorkContent{part}
	session := fixture.openSession(t)
	name := "resource-input"
	run := session.runRequest(t, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Content:      &content,
	}, false)
	if got := support.CountWorkAtCustomerState(run.listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want 1; events=%#v", got, run.events)
	}
	session.close(t)
}

func runACPSharedBTRCSuccess(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("1")
	session := fixture.openSession(t)
	run := session.run(t, "btrc-success", "ACP target characterization")
	workID, dispatchID := assertBTRCACPDispatch(t, run.events, factoryapi.WorkOutcomeAccepted, "done")
	assertBTRCACPEventOrder(t, run.events)
	assertBTRCACPProviderSession(t, run.events, factoryapi.InferenceOutcomeSucceeded)
	assertBTRCACPResponseTerminal(t, run.responseEvents, "COMPLETED")
	assertBTRCACPCompletedTarget(t, run.session, run.listed, workID, dispatchID)
	session.close(t)
}

func runACPSharedBTRCFailure(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("fail")
	session := fixture.openSession(t)
	run := session.run(t, "btrc-failure", "ACP target characterization")
	workID, dispatchID := assertBTRCACPDispatch(t, run.events, factoryapi.WorkOutcomeFailed, "failed")
	assertBTRCACPEventOrder(t, run.events)
	assertBTRCACPProviderSession(t, run.events, factoryapi.InferenceOutcomeFailed)
	assertBTRCACPFailureDetail(t, run.events)
	assertBTRCACPResponseTerminal(t, run.responseEvents, "FAILED")
	assertBTRCACPFailedTarget(t, run.session, run.listed, workID, dispatchID)
	session.close(t)
}

func runACPSharedInputContent(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	const sentinel = "ACP_INPUT_WORK_CONTENT_9f31"
	fixture.usePeerContentMode(sentinel)
	session := fixture.openSession(t)
	run := session.run(t, "input-content", sentinel)
	if got := support.CountWorkAtCustomerState(run.listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want 1; events=%#v", got, run.events)
	}
	session.close(t)
}

func runACPSharedInvalidCatalogMutations(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	home := t.TempDir()
	working := t.TempDir()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing name", args: []string{"workers", "acp", "add", "--argument", "agent acp"}, want: "required flag"},
		{name: "non canonical name", args: []string{"workers", "acp", "add", "--name", "Cursor ACP", "--argument", "agent acp"}, want: "lowercase"},
		{name: "unsupported transport", args: []string{"workers", "acp", "add", "--name", "custom-acp", "--transport", "tcp", "--argument", "agent acp"}, want: "must be stdio"},
		{name: "empty command", args: []string{"workers", "acp", "add", "--name", "custom-acp", "--argument", ""}, want: "command is required"},
		{name: "missing delete name", args: []string{"workers", "acp", "delete"}, want: "required flag"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			inputs, err := executeACPSharedCLI(t, fixture, home, working, test.args...)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()+inputs.Stderr()), strings.ToLower(test.want)) {
				t.Fatalf("execute %v error = %v, stderr=%q; want %q", test.args, err, inputs.Stderr(), test.want)
			}
		})
	}
	if _, err := os.Stat(operatorACPConfigPath(home)); !os.IsNotExist(err) {
		t.Fatalf("invalid ACP mutations changed operator settings: stat error = %v", err)
	}
}

func runACPSharedCatalogPersistence(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	home := t.TempDir()
	working := t.TempDir()
	add := executeACPCommand(t, fixture.process, home, working,
		"workers", "acp", "add",
		"--name", "custom-acp", "--transport", "stdio", "--argument", `custom-agent --profile "team alpha" acp`,
	)
	if !strings.Contains(add, "install succeeded: custom-acp") {
		t.Fatalf("add output = %q", add)
	}
	unified := executeACPCommand(t, fixture.process, home, working, "workers", "list")
	for _, want := range []string{"NAME", "TYPE", "codex", "AGENT", "cursor-acp", "AGENT-ACP", "custom-acp", "custom"} {
		if !strings.Contains(unified, want) {
			t.Fatalf("unified workers list omitted %q: %q", want, unified)
		}
	}
	deleted := executeACPCommand(t, fixture.process, home, working, "workers", "acp", "delete", "--name", "custom-acp")
	if !strings.Contains(deleted, "deleted ACP provider custom-acp") {
		t.Fatalf("delete output = %q", deleted)
	}
	unified = executeACPCommand(t, fixture.process, home, working, "workers", "list")
	if strings.Contains(unified, "custom-acp") {
		t.Fatalf("unified list after delete retained configured provider: %q", unified)
	}
	data, err := os.ReadFile(filepath.Join(home, ".you-agent-factory", "config.json"))
	if err != nil {
		t.Fatalf("read persisted operator config: %v", err)
	}
	if strings.Contains(string(data), "permission") || strings.Contains(string(data), "timeout") {
		t.Fatalf("ACP settings persisted forbidden policy fields: %s", data)
	}
}

func runACPSharedCatalogInit(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	home := t.TempDir()
	working := t.TempDir()
	executeACPCommand(t, fixture.process, home, working, "init", "--provider", "codex")
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read initialized config: %v", err)
	}
	for _, want := range []string{`"cursor-acp"`, `"cursor-agent acp"`, `"opencode-acp"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("initialized config omitted %s: %s", want, data)
		}
	}
	executeACPCommand(t, fixture.process, home, working, "workers", "acp", "add", "--name", "custom-acp", "--argument", "custom-agent acp")
	executeACPCommand(t, fixture.process, home, working, "init", "--provider", "codex")
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read reinitialized config: %v", err)
	}
	if !strings.Contains(string(data), `"custom-acp"`) || strings.Count(string(data), `"name": "cursor-acp"`) != 1 {
		t.Fatalf("re-running init did not preserve exact integrations: %s", data)
	}
}

func runACPSharedJavaScriptACP(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("1")
	dir := writeACPJavaScriptFactory(t)
	inputs := sharedACPJavaScriptInputs(t, fixture, dir, "you", "--json", "run", "--factory", "./acp.js", "--no-record")
	starts := fixture.peerStarts.Load()
	legacyCalls := fixture.legacy.calls.Load()
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(JavaScript ACP Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	waitForACPSharedPeerExits(t, fixture)
	if got := fixture.peerStarts.Load() - starts; got != 1 {
		t.Fatalf("JavaScript ACP process starts = %d, want 1", got)
	}
	if fixture.legacy.calls.Load() != legacyCalls {
		t.Fatalf("JavaScript ACP legacy provider calls = %d, want unchanged %d", fixture.legacy.calls.Load(), legacyCalls)
	}
	if !strings.Contains(inputs.Stdout(), "ACP root execution COMPLETE") ||
		!strings.Contains(inputs.Stdout(), `"providerSessionRef":"acp-session-functional-1"`) {
		t.Fatalf("JavaScript ACP result omitted content/session evidence: %s", inputs.Stdout())
	}
}

func runACPSharedMixedRouting(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	fixture.usePeerMode("1")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	factory := `{
  "name":"mixed-acp-native",
  "workTypes":[
    {"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]},
    {"name":"native","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}
  ],
  "workers":[{"name":"worker"},{"name":"native-worker"}],
  "workstations":[
    {"name":"process-acp","worker":"worker","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"done"}],"onFailure":[{"workType":"task","state":"failed"}]},
    {"name":"process-native","worker":"native-worker","inputs":[{"workType":"native","state":"init"}],"outputs":[{"workType":"native","state":"done"}],"onFailure":[{"workType":"native","state":"failed"}]}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(factory), 0o600); err != nil {
		t.Fatalf("write mixed Factory: %v", err)
	}
	writeACPWorker(t, dir, "cursor-acp")
	writeWorkerDefinition(t, dir, "native-worker", "SCRIPT_WRAP")
	writeWorkstationDefinition(t, dir, "process-acp")
	writeWorkstationDefinition(t, dir, "process-native")
	session := fixture.openSessionDir(t, dir)
	acpName := "mixed-acp"
	nativeName := "mixed-native"
	beforeStarts := fixture.peerStarts.Load()
	beforeLegacy := fixture.legacy.calls.Load()
	acpSubmission := support.SubmitSessionWorkAt(t, fixture.baseURL, session.id, factoryapi.SubmitWorkRequest{
		Name: &acpName, WorkTypeName: "task", Payload: map[string]string{"title": "ACP branch"},
	})
	nativeSubmission := support.SubmitSessionWorkAt(t, fixture.baseURL, session.id, factoryapi.SubmitWorkRequest{
		Name: &nativeName, WorkTypeName: "native", Payload: map[string]string{"title": "native branch"},
	})
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, session.id, sharedACPScenarioTimeout)
	waitForACPSharedPeerExits(t, fixture)
	listed := sharedACPWork(t, fixture.baseURL, session.id)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("ACP completed Work = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "native:done"); got != 1 {
		t.Fatalf("native completed Work = %d, want 1", got)
	}
	if support.StringPointerValue(acpSubmission.WorkId) == "" || support.StringPointerValue(nativeSubmission.WorkId) == "" {
		t.Fatalf("mixed submissions = ACP:%#v native:%#v, want Work identities", acpSubmission, nativeSubmission)
	}
	if got := fixture.peerStarts.Load() - beforeStarts; got != 1 {
		t.Fatalf("mixed ACP peer starts = %d, want 1", got)
	}
	if got := fixture.legacy.calls.Load() - beforeLegacy; got != 1 {
		t.Fatalf("mixed legacy provider calls = %d, want 1", got)
	}
	session.close(t)
}

func waitForACPSharedPeerExits(t *testing.T, fixture *acpSharedProcessFixture) {
	t.Helper()
	want := fixture.peerStarts.Load()
	if want == 0 {
		return
	}
	releaseACPSharedPeers(t, fixture, want)
	_, err := support.WaitForObservation(
		sharedACPScenarioTimeout,
		func() (int32, error) {
			pids, readErr := readACPSharedPeerPIDs(fixture)
			if readErr != nil {
				return 0, readErr
			}
			var confirmed int32
			for _, pid := range pids {
				hasExited, processErr := acpHelperProcessExited(pid)
				if processErr != nil {
					return confirmed, fmt.Errorf("inspect ACP helper PID %d: %w", pid, processErr)
				}
				if !hasExited {
					continue
				}
				confirmed++
			}
			return confirmed, nil
		},
		func(got int32) bool { return got >= want },
	)
	if err != nil {
		startPIDs, startErr := readACPHelperPIDs(fixture.peerStartMarker)
		readyRecords, readyErr := readACPHelperReadyRecords(fixture.peerReady)
		exitPIDs, exitErr := readACPHelperPIDs(fixture.peerExits)
		t.Fatalf("wait for shared ACP peer exits: %v; want at least %d exits; starts=%#v (err=%v); ready=%#v (err=%v); exits=%#v (err=%v)", err, want, startPIDs, startErr, readyRecords, readyErr, exitPIDs, exitErr)
	}
}

func readACPSharedPeerPIDs(fixture *acpSharedProcessFixture) ([]int, error) {
	startPIDs, err := readACPHelperPIDs(fixture.peerStartMarker)
	if err != nil {
		return nil, fmt.Errorf("read ACP helper starts: %w", err)
	}
	readyRecords, err := readACPHelperReadyRecords(fixture.peerReady)
	if err != nil {
		return nil, fmt.Errorf("read ACP helper readiness: %w", err)
	}
	exitPIDs, err := readACPHelperPIDs(fixture.peerExits)
	if err != nil {
		return nil, fmt.Errorf("read ACP helper exits: %w", err)
	}
	seen := make(map[int]struct{}, len(startPIDs)+len(readyRecords)+len(exitPIDs))
	pids := make([]int, 0, len(startPIDs)+len(readyRecords)+len(exitPIDs))
	for _, pid := range startPIDs {
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	for _, record := range readyRecords {
		if _, ok := seen[record.pid]; ok {
			continue
		}
		seen[record.pid] = struct{}{}
		pids = append(pids, record.pid)
	}
	for _, pid := range exitPIDs {
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	return pids, nil
}

func releaseACPSharedPeers(t *testing.T, fixture *acpSharedProcessFixture, want int32) {
	t.Helper()
	readySeen := fixture.peerReadySeen.Load()
	_, err := support.WaitForObservation(
		sharedACPScenarioTimeout,
		func() (int32, error) {
			readyRecords, readyErr := readACPHelperReadyRecords(fixture.peerReady)
			if readyErr != nil {
				return 0, readyErr
			}
			if readySeen > int32(len(readyRecords)) {
				return readySeen, fmt.Errorf("ACP helper readiness observations regressed from %d to %d", readySeen, len(readyRecords))
			}
			for _, record := range readyRecords[readySeen:] {
				file, openErr := os.OpenFile(fixture.peerReady, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
				if openErr != nil {
					return readySeen, fmt.Errorf("release ACP helper %s: %w", record.token, openErr)
				}
				_, writeErr := fmt.Fprintf(file, "release %s\n", record.token)
				closeErr := file.Close()
				if writeErr != nil {
					return readySeen, fmt.Errorf("release ACP helper %s: %w", record.token, writeErr)
				}
				if closeErr != nil {
					return readySeen, fmt.Errorf("close ACP helper release for %s: %w", record.token, closeErr)
				}
				readySeen++
			}

			// A start marker only proves that the process exists. The peer writes
			// its readiness record later, after the prompt response has flushed.
			// Returning on the cumulative start count can therefore miss the newest
			// record and leave that peer blocked forever waiting for its release.
			exitPIDs, exitErr := readACPHelperPIDs(fixture.peerExits)
			if exitErr != nil {
				return 0, exitErr
			}
			observed := make(map[int]struct{}, len(readyRecords)+len(exitPIDs))
			for _, record := range readyRecords {
				observed[record.pid] = struct{}{}
			}
			for _, pid := range exitPIDs {
				observed[pid] = struct{}{}
			}
			return int32(len(observed)), nil
		},
		func(got int32) bool { return got >= want },
	)
	if err != nil {
		startPIDs, startErr := readACPHelperPIDs(fixture.peerStartMarker)
		readyRecords, readyErr := readACPHelperReadyRecords(fixture.peerReady)
		exitPIDs, exitErr := readACPHelperPIDs(fixture.peerExits)
		t.Fatalf("wait for shared ACP peer readiness or exit: %v; want at least %d peers; starts=%#v (err=%v); ready=%#v (err=%v); exits=%#v (err=%v)", err, want, startPIDs, startErr, readyRecords, readyErr, exitPIDs, exitErr)
	}
	fixture.peerReadySeen.Store(readySeen)
}

type acpHelperReadyRecord struct {
	token string
	pid   int
}

func readACPHelperReadyRecords(path string) ([]acpHelperReadyRecord, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	contents := strings.TrimSpace(string(data))
	if contents == "" {
		return nil, nil
	}
	lines := strings.Split(contents, "\n")
	records := make([]acpHelperReadyRecord, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "release" {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid ACP helper readiness record %q", line)
		}
		pid, pidErr := strconv.Atoi(fields[1])
		if fields[0] == "" || pidErr != nil || pid <= 0 {
			return nil, fmt.Errorf("invalid ACP helper readiness record %q", line)
		}
		records = append(records, acpHelperReadyRecord{token: fields[0], pid: pid})
	}
	return records, nil
}

func readACPHelperPIDs(path string) ([]int, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	contents := strings.TrimSpace(string(data))
	if contents == "" {
		return nil, nil
	}
	lines := strings.Split(contents, "\n")
	pids := make([]int, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, parseErr := strconv.Atoi(line)
		if parseErr != nil || pid <= 0 {
			return nil, fmt.Errorf("invalid ACP helper PID %q", line)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func sharedACPSubmitRequest(name, title string) factoryapi.SubmitWorkRequest {
	return factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": title},
	}
}

func assertACPSharedCanceledResponse(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()
	for _, event := range events {
		if event.Kind != "ERROR" || event.Phase != factoryapi.FactoryResponseEventPhaseFailed || event.Provenance.Provider != "cursor-acp" {
			continue
		}
		payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
		if err != nil {
			t.Fatalf("decode ACP canceled response: %v", err)
		}
		if strings.Contains(strings.ToLower(payload.Message), "canceled") {
			return
		}
	}
	t.Fatalf("ACP response stream omitted the canceled attempt failure: %#v", events)
}

func assertACPSharedTerminalError(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()
	for _, event := range events {
		if event.Provenance.Provider != "cursor-acp" || event.Kind != "ERROR" || event.Phase != factoryapi.FactoryResponseEventPhaseFailed {
			continue
		}
		payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
		if err != nil {
			t.Fatalf("decode ACP terminal error: %v", err)
		}
		if strings.TrimSpace(payload.Code) != "" && strings.TrimSpace(payload.Message) != "" {
			return
		}
	}
	t.Fatalf("terminal ACP error missing; events=%#v", events)
}

func installSharedACPIntegration(t *testing.T, home string) {
	t.Helper()
	configDir := filepath.Join(home, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create shared operator config directory: %v", err)
	}
	path := filepath.Join(configDir, "config.json")
	config := []byte(`{"workers":{"acp":{"integrations":[{"id":"entry-1","name":"custom-acp","transport":"stdio","command":"custom-agent acp"}]}}}`)
	if err := os.WriteFile(path, config, 0o600); err != nil {
		t.Fatalf("write shared operator config: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove shared operator config: %v", err)
		}
	})
}

func executeACPSharedCLI(
	t testing.TB,
	fixture *acpSharedProcessFixture,
	home, working string,
	args ...string,
) (*support.CapturedInputs, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you"}, args...))
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = working
	return inputs, fixture.process.Execute(inputs.Input)
}

func sharedACPJavaScriptInputs(
	t *testing.T,
	fixture *acpSharedProcessFixture,
	dir string,
	args ...string,
) *support.CapturedInputs {
	t.Helper()
	support.SetWorkingDirectory(t, dir)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = append(os.Environ(), "HOME="+fixture.homeDir, "USERPROFILE="+fixture.homeDir)
	return inputs
}

func getACPSharedWorkerSessionEvents(
	t *testing.T,
	baseURL, sessionID, workID string,
) []factoryapi.WorkerSessionEvent {
	t.Helper()
	listEndpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID)
	list := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, listEndpoint)
	if len(list.Sessions) != 1 || strings.TrimSpace(list.Sessions[0].WorkerSessionId) == "" {
		t.Fatalf("shared ACP Worker Sessions = %#v, want one session for Work %q", list.Sessions, workID)
	}
	workerSessionID := list.Sessions[0].WorkerSessionId
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions/" + url.PathEscape(workerSessionID) + "/events?replayOnly=true"
	ctx, cancel := context.WithTimeout(context.Background(), sharedACPScenarioTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build shared ACP Worker Session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET shared ACP Worker Session events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET shared ACP Worker Session events status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	scanner := bufio.NewScanner(response.Body)
	var events []factoryapi.WorkerSessionEvent
	complete := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatalf("decode shared ACP Worker Session event: %v", err)
		}
		events = append(events, event)
		if event.ReplaySummary != nil && event.ReplaySummary.Complete {
			complete = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read shared ACP Worker Session events: %v", err)
	}
	if !complete {
		t.Fatalf("shared ACP Worker Session replay = %#v, want complete replay summary", events)
	}
	return events
}
