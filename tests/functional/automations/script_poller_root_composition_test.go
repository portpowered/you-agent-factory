package automations

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// API-owned session opening activates each source without selecting ~default.
// The cells share the root-built host but own their command route, directory,
// session and cleanup. The next command proves the previous cycle completed;
// no internal cursor file or service pointer is used as a public observer.
func TestAutomationsSessionRecoveryAndIngress(t *testing.T) {
	t.Parallel()
	recoveryDir, recovery := newScriptCycleFactory(t)
	emptyDir, empty := newScriptCycleFactory(t)
	cursor, checkpoint := "cursor-雪-\\opaque", "checkpoint-λ-\"quoted\""
	output := scriptCycleOutput(t, cursor, checkpoint)
	restartDir, restart := newScriptCycleFactory(t)
	peerDir, peer := newScriptCycleFactory(t)
	failureDir, failure := newScriptCycleFactory(t)
	failurePeerDir, failurePeer := newScriptCycleFactory(t)
	files := &scriptReplacementFailure{directory: failureDir}
	watcherDir := newWatcherIngressFactory(t, interfaces.DefaultChannelName)
	executionWatcherDir := newWatcherIngressFactory(t, "owned-exec-preseed")
	duplicateWatcherDir := newWatcherIngressFactory(t, interfaces.DefaultChannelName)
	stoppedWatcherDir := newWatcherIngressFactory(t, interfaces.DefaultChannelName)
	watcherPeerDir := newWatcherIngressFactory(t, interfaces.DefaultChannelName)
	hosted := newHostedIngressScenarios(t)
	var stoppedWatcherAdmissions atomic.Int32
	router := scriptCycleRouter{routes: map[string]*scriptCycleRoute{
		filepath.Clean(recoveryDir):    recovery,
		filepath.Clean(emptyDir):       empty,
		filepath.Clean(restartDir):     restart,
		filepath.Clean(peerDir):        peer,
		filepath.Clean(failureDir):     failure,
		filepath.Clean(failurePeerDir): failurePeer,
	}}
	server := startScriptCycleHost(t, router, hosted.router, files, hosted.checkpoints, func(record work.FactorySubmissionRecord) {
		if record.Request.RequestID == "watcher-stopped-only" {
			stoppedWatcherAdmissions.Add(1)
		}
		if record.Request.WorkID == "linear:issue-stopped-only" {
			hosted.stoppedAdmissions.Add(1)
		}
	})
	hosted.run(t, server.URL())
	t.Run("watcher_stop_joins_while_peer_progresses_and_restart_admits_eligible_input", func(t *testing.T) {
		t.Parallel()
		assertWatcherIndependentStop(t, server.URL(), stoppedWatcherDir, watcherPeerDir, &stoppedWatcherAdmissions)
	})
	t.Run("watcher_retained_file_identity_does_not_duplicate_after_restart", func(t *testing.T) {
		t.Parallel()
		assertWatcherDuplicateRestart(t, server.URL(), duplicateWatcherDir)
	})
	t.Run("watcher_preseed_and_live_input_preserve_public_Work", func(t *testing.T) {
		t.Parallel()
		assertWatcherSessionIngress(t, server.URL(), watcherDir, interfaces.DefaultChannelName, interfaces.DefaultChannelName)
	})
	t.Run("watcher_preseed_and_new_execution_directory_preserve_correlation", func(t *testing.T) {
		t.Parallel()
		// The live channel directory is created only after preseed completion,
		// exercising supported dynamic channel discovery as well as startup.
		assertWatcherSessionIngress(t, server.URL(), executionWatcherDir, "owned-exec-preseed", "owned-exec-live")
	})
	t.Run("failed_replacement_retains_Work_and_prior_resume_while_peer_progresses", func(t *testing.T) {
		t.Parallel()
		assertScriptReplacementRecovery(t, server.URL(), failureDir, failurePeerDir,
			failure, failurePeer, output, cursor, checkpoint)
	})
	t.Run("stopped_source_joins_and_recovers_while_peer_progresses", func(t *testing.T) {
		t.Parallel()
		assertScriptSourceRestart(t, server.URL(), restartDir, peerDir, restart, peer, output, cursor, checkpoint)
	})
	for _, cell := range []struct {
		name  string
		dir   string
		route *scriptCycleRoute
	}{
		{"committed_opaque_facts_resume_with_public_Work", recoveryDir, recovery},
		{"completed_empty_cycle_admits_no_Work", emptyDir, empty},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			sessionID := support.OpenFactorySessionAt(t, server.URL(), cell.dir).Session.Id
			t.Cleanup(func() { support.CloseFactorySessionAt(t, server.URL(), sessionID) })
			first := awaitScriptCycleCommand(t, cell.route)
			if len(first.request.Env) != 0 {
				t.Fatalf("first command unexpectedly resumed: %#v", first.request.Env)
			}
			if cell.route == empty {
				first.output <- nil
			} else {
				first.output <- output
			}
			// The successor acknowledges parsing, admission and commit/empty handling.
			resumed := awaitScriptCycleCommand(t, cell.route)
			listed := support.GetJSON[factoryapi.ListWorkResponse](t,
				support.SessionWorkURL(server.URL(), sessionID, "/work"))
			if cell.route == empty {
				if len(listed.Results) != 0 || len(resumed.request.Env) != 0 {
					t.Fatalf("empty cycle Work=%#v env=%#v, want no admission/advancement", listed.Results, resumed.request.Env)
				}
				return
			}
			assertScriptResumeEnvironment(t, resumed.request, cursor, checkpoint)
			assertScriptIngressWork(t, readScriptQueuedWork(t, server.URL(), sessionID, scriptPollerExternalWorkID))
		})
	}
}

func scriptCycleOutput(t *testing.T, cursor, checkpoint string) []byte {
	t.Helper()
	output, err := json.Marshal(map[string]any{
		"request": json.RawMessage(scriptPollerExternalWorkRequestJSON(t)),
		"cursor":  cursor, "checkpoint": checkpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func newScriptCycleFactory(t *testing.T) (string, *scriptCycleRoute) {
	t.Helper()
	dir := support.ScaffoldFactory(t, scriptPollerFactoryConfig())
	support.ClearSeedInputs(t, dir)
	return dir, newScriptCycleRoute()
}

func startScriptCycleHost(t *testing.T, router scriptCycleRouter, hosted hostedCycleRouter, files *scriptReplacementFailure,
	checkpoints automations.HostedLinearCheckpointStore,
	observeSubmission func(work.FactorySubmissionRecord),
) *support.FunctionalAPIServer {
	t.Helper()
	var admissions atomic.Int32
	hostDir := support.ScaffoldFactory(t, map[string]any{
		"name": "idle-automation-host", "workTypes": []map[string]any{{
			"name": "idle", "states": []map[string]string{
				{"name": "init", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"},
			},
		}},
	})
	support.ClearSeedInputs(t, hostDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: hostDir,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: router, AutomationsCursorFileSystem: files,
			HostedHTTPClient: hosted, HostedLinearEndpoint: "https://owned-hosted.invalid/graphql",
			HostedLinearCheckpointStore: checkpoints,
			SubmissionRecorder: func(record work.FactorySubmissionRecord) {
				admissions.Add(1)
				observeSubmission(record)
			},
		},
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			// Construction has not activated any sessions, so there are no peer
			// admissions in this observation window. Retain watcher inertness
			// alongside source-command inertness without a second root build.
			if count := admissions.Load(); count != 0 {
				tb.Fatalf("BuildProcess admitted %d Work items before activation", count)
			}
			for dir, route := range router.routes {
				select {
				case <-route.entered:
					tb.Fatalf("BuildProcess invoked a script source at %q before activation", dir)
				default:
				}
			}
			for _, route := range hosted.routes {
				select {
				case <-route.entered:
					tb.Fatal("BuildProcess invoked hosted HTTP before activation")
				default:
				}
			}
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, hostDir)
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	return server
}

func assertScriptReplacementRecovery(t *testing.T, baseURL, sourceDir, peerDir string,
	source, peer *scriptCycleRoute, output []byte, cursor, checkpoint string,
) {
	t.Helper()
	sourceID := support.OpenFactorySessionAt(t, baseURL, sourceDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, sourceID) })
	peerID := support.OpenFactorySessionAt(t, baseURL, peerDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, peerID) })
	first, peerFirst := awaitScriptCycleCommand(t, source), awaitScriptCycleCommand(t, peer)
	first.output <- output
	replacement := awaitScriptCycleCommand(t, source)
	assertScriptResumeEnvironment(t, replacement.request, cursor, checkpoint)
	// Admission precedes durable replacement. The fault is at the existing
	// filesystem edge, after a real prior commit, and scoped to this directory.
	var next map[string]any
	if err := json.Unmarshal(output, &next); err != nil {
		t.Fatal(err)
	}
	next["request"] = json.RawMessage(strings.ReplaceAll(
		string(scriptPollerNamedWorkRequestJSON(t, "admitted-before-replacement-failure")),
		scriptPollerExternalRequestID, "replacement-failure-request"))
	next["cursor"], next["checkpoint"] = "uncommitted-next-cursor", "uncommitted-next-checkpoint"
	failedOutput, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	replacement.output <- failedOutput
	resumed := awaitScriptCycleCommand(t, source)
	assertScriptResumeEnvironment(t, resumed.request, cursor, checkpoint)
	listed := readScriptQueuedWork(t, baseURL, sourceID, scriptPollerExternalWorkID, "admitted-before-replacement-failure")
	location := support.WorkCustomerLocation(scriptPollerWorkTypeName, scriptPollerOutputStateName)
	if len(listed.Results) != 2 || !support.HasWorkAtCustomerState(listed, scriptPollerExternalWorkID, location) ||
		!support.HasWorkAtCustomerState(listed, "admitted-before-replacement-failure", location) {
		t.Fatalf("Work after failed replacement = %#v, want both admitted items retained", listed.Results)
	}
	peerFirst.output <- output
	peerNext := awaitScriptCycleCommand(t, peer)
	assertScriptResumeEnvironment(t, peerNext.request, cursor, checkpoint)
	assertScriptIngressWork(t, readScriptQueuedWork(t, baseURL, peerID, scriptPollerExternalWorkID))
}

const scriptReplacementFailureDetail = "owned-script-cursor-replacement-unavailable"

type scriptReplacementFailure struct {
	platformfilesystem.Local
	directory    string
	replacements atomic.Int32
}

func (f *scriptReplacementFailure) Rename(from, to string) error {
	if strings.HasPrefix(filepath.Clean(to), filepath.Clean(f.directory)+string(filepath.Separator)) && f.replacements.Add(1) > 1 {
		return fmt.Errorf("%s", scriptReplacementFailureDetail)
	}
	return f.Local.Rename(from, to)
}

func assertScriptSourceRestart(t *testing.T, baseURL, sourceDir, peerDir string,
	source, peer *scriptCycleRoute, output []byte, cursor, checkpoint string,
) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, baseURL, sourceDir)
	sessionID := opened.Session.Id
	t.Cleanup(func() {
		if sessionID != "" {
			support.CloseFactorySessionAt(t, baseURL, sessionID)
		}
	})
	peerID := support.OpenFactorySessionAt(t, baseURL, peerDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, peerID) })
	if sessionID == peerID || sessionID == "~default" || peerID == "~default" {
		t.Fatalf("source/peer sessions = %q/%q, want distinct explicit identities", sessionID, peerID)
	}
	first := awaitScriptCycleCommand(t, source)
	peerFirst := awaitScriptCycleCommand(t, peer)
	first.output <- output
	pending := awaitScriptCycleCommand(t, source)
	assertScriptResumeEnvironment(t, pending.request, cursor, checkpoint)
	assertScriptIngressWork(t, readScriptQueuedWork(t, baseURL, sessionID, scriptPollerExternalWorkID))

	// Termination stops execution; closing also deactivates and joins source
	// sidecars. Use that full existing control before claiming source shutdown.
	support.CloseFactorySessionAt(t, baseURL, sessionID)
	select {
	case <-pending.canceled:
	case <-time.After(10 * time.Second):
		t.Fatal("stopped source command did not acknowledge cancellation")
	}
	// Deliver a late result to the canceled invocation only. It must never reach
	// admission or be consumed by a new invocation after the source restarts.
	pending.output <- scriptPollerNamedWorkRequestJSON(t, "stopped-interval-work")
	peerFirst.output <- output
	peerNext := awaitScriptCycleCommand(t, peer)
	assertScriptResumeEnvironment(t, peerNext.request, cursor, checkpoint)
	assertScriptIngressWork(t, readScriptQueuedWork(t, baseURL, peerID, scriptPollerExternalWorkID))
	select {
	case command := <-source.entered:
		t.Fatalf("stopped source executed another command: %#v", command.request)
	default:
	}

	// Deleting the joined live session and opening the same folder/target is the
	// supported logical-target restart; the new live UUID is intentionally distinct.
	priorSessionID := sessionID
	sessionID = ""
	reopened := support.OpenFactorySessionAt(t, baseURL, sourceDir)
	if reopened.Session.Id == priorSessionID || reopened.Session.FactoryDir != opened.Session.FactoryDir ||
		reopened.Session.FolderPath != opened.Session.FolderPath {
		t.Fatalf("reopened session = %#v, want same logical target and new live identity", reopened.Session)
	}
	sessionID = reopened.Session.Id
	resumed := awaitScriptCycleCommand(t, source)
	assertScriptResumeEnvironment(t, resumed.request, cursor, checkpoint)
	resumed.output <- scriptPollerNamedWorkRequestJSON(t, "reactivated-work")
	_ = awaitScriptCycleCommand(t, source)
	listed := readScriptQueuedWork(t, baseURL, sessionID, "reactivated-work")
	location := support.WorkCustomerLocation(scriptPollerWorkTypeName, scriptPollerOutputStateName)
	if !support.HasWorkAtCustomerState(listed, "reactivated-work", location) ||
		support.HasWorkAtCustomerState(listed, "stopped-interval-work", location) {
		t.Fatalf("restarted session Work = %#v, want eligible Work and no late stopped result", listed.Results)
	}
}

func scriptPollerNamedWorkRequestJSON(t *testing.T, workID string) []byte {
	t.Helper()
	return []byte(strings.ReplaceAll(string(scriptPollerExternalWorkRequestJSON(t)), scriptPollerExternalWorkID, workID))
}

func assertScriptResumeEnvironment(t *testing.T, request platformprocess.CommandRequest, cursor, checkpoint string) {
	t.Helper()
	for _, expected := range []string{
		"INFINITE_YOU_SCRIPT_POLLER_CURSOR=" + cursor,
		"INFINITE_YOU_SCRIPT_POLLER_CHECKPOINT=" + checkpoint,
	} {
		if !slices.Contains(request.Env, expected) {
			t.Fatalf("resumed source command missing exact %q", expected)
		}
	}
}

// A resumed poll proves source-cycle completion, not downstream worker completion.
// Observe the retained public dispatch completion event before reading queued Work.
func readScriptQueuedWork(t *testing.T, baseURL, sessionID string, workIDs ...string) factoryapi.ListWorkResponse {
	t.Helper()
	return readAutomationCompletedWork(t, baseURL, sessionID, scriptPollerOutputStateName, workIDs...)
}

func readAutomationCompletedWork(t *testing.T, baseURL, sessionID, state string, workIDs ...string) factoryapi.ListWorkResponse {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, sessionID))
	defer stream.Close()
	ctx, cancel := context.WithTimeout(t.Context(), support.ScaledTimeout(10*time.Second))
	defer cancel()
	pending := make(map[string]bool, len(workIDs))
	for _, id := range workIDs {
		pending[id] = true
	}
	for len(pending) > 0 {
		event := stream.NextEventContext(ctx)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("dispatch event belongs to session %v, want %q", event.Context.SessionId, sessionID)
		}
		if payload.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("automation dispatch outcome=%q, want accepted", payload.Outcome)
		}
		if payload.OutputWork != nil {
			for _, output := range *payload.OutputWork {
				if output.WorkId != nil && output.State != nil && output.State.Name == state {
					delete(pending, *output.WorkId)
				}
			}
		}
	}
	listed, err := readWorkAtState(ctx, support.SessionWorkURL(baseURL, sessionID, "/work"), state, workIDs...)
	if err != nil {
		t.Fatalf("read completed automation Work: %v", err)
	}
	return listed
}

func assertScriptIngressWork(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	location := support.WorkCustomerLocation(scriptPollerWorkTypeName, scriptPollerOutputStateName)
	if len(listed.Results) != 1 || !support.HasWorkAtCustomerState(listed, scriptPollerExternalWorkID, location) {
		t.Fatalf("session Work=%#v, want one %q at %q", listed.Results, scriptPollerExternalWorkID, location)
	}
	payload, ok := listed.Results[0].Payload.(map[string]any)
	if !ok || payload["id"] != "ISSUE-101" || payload["title"] != "External ingress item" {
		t.Fatalf("admitted payload=%#v, want preserved external item", listed.Results[0].Payload)
	}
}

type scriptCycleRouter struct{ routes map[string]*scriptCycleRoute }

func (r scriptCycleRouter) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	// A POLLER workstation also executes its SCRIPT_WORKER for admitted Work.
	// Worker attempts have an execution scope and receive Work on stdin; source
	// polling has neither. Keep the real worker path separate from source gates.
	if request.ExecutionScopeID != "" {
		return platformprocess.CommandResult{Stdout: request.Stdin}, nil
	}
	route := r.routes[filepath.Clean(request.WorkDir)]
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("unowned script command directory %q", request.WorkDir)
	}
	return route.Run(ctx, request)
}

type scriptCycleRoute struct {
	entered chan scriptCycleCommand
}

type scriptCycleCommand struct {
	request  platformprocess.CommandRequest
	output   chan []byte
	canceled chan struct{}
}

func newScriptCycleRoute() *scriptCycleRoute {
	return &scriptCycleRoute{entered: make(chan scriptCycleCommand, 4)}
}

func (r *scriptCycleRoute) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	command := scriptCycleCommand{request: request, output: make(chan []byte, 1), canceled: make(chan struct{})}
	select {
	case r.entered <- command:
	case <-ctx.Done():
		close(command.canceled)
		return platformprocess.CommandResult{}, ctx.Err()
	}
	select {
	case output := <-command.output:
		return platformprocess.CommandResult{Stdout: output}, nil
	case <-ctx.Done():
		close(command.canceled)
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func awaitScriptCycleCommand(t *testing.T, route *scriptCycleRoute) scriptCycleCommand {
	t.Helper()
	select {
	case request := <-route.entered:
		return request
	case <-time.After(10 * time.Second):
		t.Fatal("session source did not reach its owned script command edge")
		return scriptCycleCommand{}
	}
}

const (
	scriptPollerWorkTypeName      = "story"
	scriptPollerOutputStateName   = "queued"
	scriptPollerExternalWorkID    = "external-issue-101"
	scriptPollerExternalRequestID = "poller-external-batch-1"
	scriptPollerScriptCommand     = "factory/scripts/poller.sh"
	scriptPollerWorkstationName   = "poll-tasks"
	scriptPollerWorkerName        = "script-poller"
)

func scriptPollerFactoryConfig() map[string]any {
	return map[string]any{
		"name": "poller-external-items",
		"workTypes": []map[string]any{{
			"name": scriptPollerWorkTypeName,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": scriptPollerOutputStateName, "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{
			"name":    scriptPollerWorkerName,
			"type":    "SCRIPT_WORKER",
			"command": scriptPollerScriptCommand,
		}},
		"workstations": []map[string]any{{
			"name":      scriptPollerWorkstationName,
			"behavior":  "POLLER",
			"worker":    scriptPollerWorkerName,
			"inputs":    []map[string]string{{"workType": scriptPollerWorkTypeName, "state": "init"}},
			"outputs":   []map[string]string{{"workType": scriptPollerWorkTypeName, "state": scriptPollerOutputStateName}},
			"onFailure": []map[string]string{{"workType": scriptPollerWorkTypeName, "state": "failed"}},
		}},
	}
}

func scriptPollerExternalWorkRequestJSON(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"requestId": scriptPollerExternalRequestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         "external-issue-101",
			"workId":       scriptPollerExternalWorkID,
			"workTypeName": scriptPollerWorkTypeName,
			"payload": map[string]string{
				"id":    "ISSUE-101",
				"title": "External ingress item",
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal script poller work request: %v", err)
	}
	return payload
}
