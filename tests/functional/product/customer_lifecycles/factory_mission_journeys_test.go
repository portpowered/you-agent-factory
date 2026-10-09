package customer_lifecycles_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const measuredMissionReply = `{"decision":"ACCEPTED","feedback":"measured","output":{"measurements":[{"name":"pending","value":0,"source":"local fixture"},{"name":"ready","value":false,"source":"read fixture"}]}}`

// One immutable process is shared with the retained proposal journeys. Each
// parallel cell owns a Session, directory, command route and correction count.
func TestMissionThoughtsJourneys(t *testing.T) {
	initializeFactoryreviewfailureroutingFixture(t)
	t.Parallel()
	for _, tc := range []struct {
		name, payload, first, final, state string
		correction, providerFailure        bool
	}{
		{"F01 measured zero", `{"mission":"measure pending and ready"}`, measuredMissionReply, "", "complete", false, false},
		{"F02 absent", `{}`, `{"decision":"ACCEPTED","feedback":"supervised","output":"hold"}`, "", "complete", false, false},
		{"F02 blank", `{"mission":""}`, `{"decision":"ACCEPTED","feedback":"supervised","output":"hold"}`, "", "complete", false, false},
		{"F02 whitespace", `{"mission":"  "}`, `{"decision":"ACCEPTED","feedback":"supervised","output":"hold"}`, "", "complete", false, false},
		{"F02 ordinary cron", `{"origin":"cron"}`, `{"decision":"ACCEPTED","feedback":"supervised","output":"hold"}`, "", "complete", false, false},
		{"F03 cron mission", `{"mission":"measure pending and ready","origin":"cron"}`, measuredMissionReply, "", "complete", false, false},
		{"F04 corrected", `{"mission":"measure pending and ready"}`, `{"decision":"ACCEPTED","feedback":"hold","output":"hold"}`, measuredMissionReply, "complete", true, false},
		{"F05 twice invalid", `{"mission":"measure pending and ready"}`, `{"decision":"ACCEPTED","feedback":"hold","output":{"measurements":[]}}`, "", "failed", true, false},
		{"F06 precondition", `{"mission":"measure pending and ready"}`, `{"decision":"ACCEPTED","feedback":"blocked","output":{"precondition":"required fixture unavailable","measurements":[{"name":"pending","value":0,"source":"fixture"}]}}`, "", "complete", false, false},
		{"F07 optional gap", `{"mission":"measure pending; optional GitHub read"}`, `{"decision":"ACCEPTED","feedback":"optional read exhausted","output":{"measurements":[{"name":"pending","value":0,"source":"fixture"}],"reads":[{"available":false,"required":false,"attempts":2}]}}`, "", "complete", false, false},
		{"F08 required gap", `{"mission":"measure pending; required GitHub read"}`, `{"decision":"FAILED","feedback":"required GitHub read unavailable","output":{"precondition":"required GitHub read unavailable","measurements":[{"name":"pending","value":0,"source":"fixture"}]}}`, "", "failed", false, false},
		{"F12 provider failure", `{"mission":"measure pending and ready"}`, "", "", "failed", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var providers, checks atomic.Int32
			scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
				provider: func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
					call := providers.Add(1)
					if tc.providerFailure {
						return platformprocess.CommandResult{}, errors.New("controlled mission provider failure")
					}
					reply := tc.first
					if call > 1 && tc.final != "" {
						reply = tc.final
					}
					return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(reply)}, nil
				},
				script: missionScriptResponse(t, &checks),
			})
			stream := scenario.eventStream(t)
			id := scenario.marker + "-mission"
			dependency := id + "-completed-fix"
			scenario.submit(t, dependency+"-request", reviewFailureSeed{Name: dependency, WorkID: dependency, WorkType: "task", State: "complete", TraceID: dependency + "-trace"})
			tags := map[string]string{"evidence": "retained-tag"}
			scenario.submit(t, id+"-request", reviewFailureSeed{Name: id, WorkID: id, WorkType: "thoughts", State: "init", TraceID: id + "-trace", Payload: tc.payload, Tags: tags, DependsOn: dependency, DependsOnState: "complete"})
			terminal := "report-thoughts-complete"
			if tc.state == "failed" {
				terminal = "report-thoughts-failure"
			}
			awaitReviewFailureDispatchResponses(t, stream, terminal, 1)
			awaitReviewFailureWorkStates(t, scenario, map[string]string{id: tc.state})
			mission := strings.Contains(tc.payload, "measure")
			wantProviders := int32(1)
			if tc.correction {
				wantProviders = 2
			}
			if providers.Load() != wantProviders {
				t.Fatalf("provider calls=%d, want %d", providers.Load(), wantProviders)
			}
			wantChecks := wantProviders
			if !mission || tc.providerFailure {
				wantChecks = 0
			}
			if checks.Load() != wantChecks {
				t.Fatalf("checker calls=%d, want %d", checks.Load(), wantChecks)
			}
			assertMissionJourneyEvidence(t, scenario, id, tc.payload, mission, tc.correction)
			assertMissionDispatchContract(t, scenario, id, mission, tc.correction, tc.state == "failed" && tc.correction)
			assertMissionFinalReply(t, scenario, id, tc.first, tc.final, tc.correction, tc.providerFailure)
		})
	}
	t.Run("F09-F11 gap handoff", testMissionGapHandoff)
}

// The script edge supplies controlled branch responses; U1 proves the actual
// Python parser. Here script stdin, checker argv and persisted feedback are observed.
func missionScriptResponse(t *testing.T, checks *atomic.Int32) reviewFailureCommandResponder {
	return func(_ context.Context, request platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
		for index, arg := range request.Args {
			if strings.HasSuffix(arg, "route-thoughts.py") {
				if index+1 >= len(request.Args) || request.Args[index+1] != "--payload-stdin" || len(request.Stdin) == 0 {
					t.Error("route stdin payload or flag absent")
					return platformprocess.CommandResult{}, errors.New("payload absent")
				}
				label := "supervision"
				if strings.Contains(string(request.Stdin), "measure") {
					label = "mission"
				}
				return platformprocess.CommandResult{Stdout: []byte(label)}, nil
			}
			if strings.HasSuffix(arg, "check-mission-output.py") {
				checks.Add(1)
				if index+2 >= len(request.Args) {
					t.Error("checker argv absent")
					return platformprocess.CommandResult{}, errors.New("checker argv absent")
				}
				raw, feedback := request.Args[index+1], request.Args[index+2]
				var reply map[string]any
				_ = json.Unmarshal([]byte(raw), &reply)
				if strings.Contains(raw, `"output":"hold"`) || strings.Contains(raw, `"measurements":[]`) {
					decision := "REJECTED"
					if strings.Contains(feedback, "mission-output-invalid:") {
						decision = "FAILED"
					}
					reply = map[string]any{"decision": decision, "feedback": "mission-output-invalid: output requires non-empty measurements", "output": map[string]string{"invalidReply": raw}}
				}
				encoded, _ := json.Marshal(reply)
				return platformprocess.CommandResult{Stdout: encoded}, nil
			}
		}
		return platformprocess.CommandResult{Stdout: []byte("script-ok")}, nil
	}
}

func assertMissionJourneyEvidence(t *testing.T, scenario *reviewFailureScenario, id, payload string, mission, corrected bool) {
	t.Helper()
	work := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(scenario.fixture.baseURL, scenario.sessionID, "/work/"+id))
	assertPayloadHasSentinel(t, "original payload", work.Payload, payload)
	assertPayloadHasSentinel(t, "original tag", work.Tags, "retained-tag")
	requests := reviewFailureProviderRequests(scenario.fixture.router.requestsFor(scenario.factoryDir))
	for _, request := range requests {
		prompt := providerCommandPrompt(request)
		if mission {
			for _, value := range []string{"Bound mission verification", id, scenario.sessionID, "retained-tag", "measure"} {
				if !strings.Contains(prompt, value) {
					t.Errorf("mission prompt missing %q", value)
				}
			}
			if strings.Contains(prompt, "Portfolio supervision") {
				t.Error("mission dispatched to supervisor")
			}
		} else if !strings.Contains(prompt, "Portfolio supervision") {
			t.Error("ordinary thoughts did not dispatch to supervisor")
		}
	}
	if corrected && !strings.Contains(providerCommandPrompt(requests[1]), "mission-output-invalid:") {
		t.Error("correction feedback did not reach same worker")
	}
	dispatches := reviewFailureDispatches(t, scenario)
	if len(dispatchesWithTransition(dispatches, "route-thoughts")) != 1 {
		t.Error("thoughts routed more than once")
	}
	if mission && len(dispatchesWithTransition(dispatches, "ideafy")) != 0 {
		t.Error("mission consumed supervisor workstation")
	}
	assertNoIncompleteReviewFailureDispatches(t, dispatches)
}

func assertMissionDispatchContract(t *testing.T, scenario *reviewFailureScenario, id string, mission, corrected, exhausted bool) {
	t.Helper()
	rejections := 0
	for _, dispatch := range reviewFailureDispatches(t, scenario) {
		step := dispatch.Request.TransitionId
		if step == "verify-mission" || step == "ideafy" {
			assertMissionWorkerDispatch(t, dispatch.Request, id, mission)
		}
		if step != "check-mission-output" || dispatch.Response == nil {
			continue
		}
		rejections += assertMissionCheckerResponse(t, *dispatch.Response, exhausted)
	}
	if (rejections == 1) != corrected {
		t.Fatalf("rejections=%d corrected=%v", rejections, corrected)
	}
	work := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(scenario.fixture.baseURL, scenario.sessionID, "/work/"+id))
	if work.Relations == nil || len(*work.Relations) == 0 {
		t.Fatal("dependency relation lost")
	}
	assertPayloadHasSentinel(t, "dependency identity", work.Relations, id+"-completed-fix")
}

func testMissionGapHandoff(t *testing.T) {
	t.Parallel()
	for _, tagged := range []bool{false, true} {
		for _, failed := range []bool{false, true} {
			name := "untagged"
			if tagged {
				name = "tagged"
			}
			if failed {
				name += " dry-run failure"
			}
			t.Run(name, func(t *testing.T) { testMissionGapCase(t, tagged, failed) })
		}
	}
}

func testMissionGapCase(t *testing.T, tagged, failed bool) {
	t.Parallel()
	var checks atomic.Int32
	var artifact string
	var protocol func() map[string]any
	wake := openProjectWakeScenarioWithConfig(t, reviewFailureRouteConfig{
		provider: missionGapProvider(t, &artifact, &protocol, failed),
		script:   missionScriptResponse(t, &checks),
	})
	artifact = filepath.Join(wake.rootDir, "docs", "temp", "projects", wake.ownerName, "proposals", "loopback.json")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o755); err != nil {
		t.Fatal(err)
	}
	proposal := `{"type":"FACTORY_REQUEST_BATCH","requestId":"stable-fixture","works":[{"name":"fix","workTypeName":"idea","state":"to-complete","payload":"controlled correction held before completion"},{"name":"loopback","workTypeName":"thoughts","payload":{"mission":"measure corrected result"}}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"loopback","targetWorkName":"fix","requiredState":"complete"}]}`
	if err := os.WriteFile(artifact, []byte(proposal), 0o600); err != nil {
		t.Fatal(err)
	}
	protocol = func() map[string]any { return runMissionGapProtocol(t, wake, artifact, tagged, failed) }
	wake.admitOwner(t)
	stream := wake.eventStream(t)
	id := wake.marker + "-gap"
	tags := map[string]string{"evidence": "retained-tag"}
	if tagged {
		tags["project"] = wake.ownerName
	}
	wake.submit(t, id+"-request", reviewFailureSeed{Name: id, WorkID: id, WorkType: "thoughts", State: "init", TraceID: id + "-trace", Payload: `{"mission":"measure corrected result"}`, Tags: tags})
	terminal, state := "report-thoughts-complete", "complete"
	if failed {
		terminal, state = "report-thoughts-failure", "failed"
	}
	if tagged {
		assertProjectLeadWakes(t, wake, stream, id)
	} else {
		awaitReviewFailureDispatchResponses(t, stream, terminal, 1)
	}
	awaitReviewFailureWorkStates(t, wake.reviewFailureScenario, map[string]string{id: state})
	assertLoopbackReportOrigin(t, wake.listWorks(t), id, !tagged)
	work := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(wake.fixture.baseURL, wake.sessionID, "/work/"+id))
	output := missionLastOutput(t, work)
	var native map[string]any
	if err := json.Unmarshal([]byte(output), &native); err != nil {
		t.Fatal(err)
	}
	if native["proposal"] != artifact {
		t.Fatalf("proposal = %v, want %s", native["proposal"], artifact)
	}
	filing := native["filing"].(map[string]any)
	if filing["submitted"] != (!tagged && !failed) || filing["receiptVerified"] != (!tagged && !failed) || filing["dryRun"] != !failed {
		t.Fatalf("filing evidence = %#v", filing)
	}
	measured := native["measurements"].([]any)[0].(map[string]any)
	if measured["value"] != float64(0) {
		t.Fatalf("zero lost: %v", measured)
	}
}

// Controlled transcripts and real test-owned proposal artifacts protect the
// handoff contract. VAL1 owns actual agent filing and remote receipt usability.
func missionGapProvider(t *testing.T, artifact *string, protocol *func() map[string]any, failed bool) reviewFailureCommandResponder {
	return func(_ context.Context, request platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
		prompt := providerCommandPrompt(request)
		if strings.Contains(prompt, "Project Lead wake") {
			return reviewFailureAccepted("reconciled"), nil
		}
		for _, clause := range []string{"stable request ID, dry-run, idempotent submission", "verified receipt", "dependent loopback", "submit no Project children", "proposals/<loopback-name>.json in the main checkout"} {
			if !strings.Contains(strings.Join(strings.Fields(prompt), " "), clause) {
				t.Errorf("gap protocol missing %q", clause)
			}
		}
		var reply map[string]any
		_ = json.Unmarshal([]byte(measuredMissionReply), &reply)
		output := reply["output"].(map[string]any)
		output["proposal"] = *artifact
		output["filing"] = (*protocol)()
		if failed {
			reply["decision"] = "FAILED"
			reply["feedback"] = "required dry-run unavailable"
		}
		encoded, _ := json.Marshal(reply)
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(string(encoded))}, nil
	}
}

func missionLastOutput(t *testing.T, work factoryapi.Work) string {
	t.Helper()
	tagsJSON, err := json.Marshal(work.Tags)
	if err != nil {
		t.Fatal(err)
	}
	var tags map[string]string
	if err := json.Unmarshal(tagsJSON, &tags); err != nil {
		t.Fatal(err)
	}
	return tags["_last_output"]
}

func assertMissionOutput(t *testing.T, work factoryapi.Work, expected string) {
	t.Helper()
	actual := missionLastOutput(t, work)
	var got, want any
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	if _, ok := want.(string); ok {
		if actual != want {
			t.Fatalf("output=%q, want %q", actual, want)
		}
		return
	}
	if err := json.Unmarshal([]byte(actual), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("output=%#v, want %#v", got, want)
	}
}

func assertMissionFinalReply(t *testing.T, scenario *reviewFailureScenario, id, first, final string, corrected, providerFailure bool) {
	t.Helper()
	if providerFailure || (corrected && final == "") {
		return
	}
	work := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(scenario.fixture.baseURL, scenario.sessionID, "/work/"+id))
	if final == "" {
		final = first
	}
	var reply map[string]any
	if err := json.Unmarshal([]byte(final), &reply); err != nil {
		t.Fatal(err)
	}
	output, _ := json.Marshal(reply["output"])
	assertMissionOutput(t, work, string(output))
}

func assertMissionWorkerDispatch(t *testing.T, request factoryapi.DispatchRequestEventPayload, id string, mission bool) {
	t.Helper()
	if len(request.Inputs) != 1 || request.Inputs[0].WorkId != id {
		t.Fatalf("dispatch lost Work identity: %#v", request.Inputs)
	}
	supervisor := false
	if request.Resources != nil {
		for _, resource := range *request.Resources {
			supervisor = supervisor || resource.Name == "supervisor-slot"
		}
	}
	if supervisor == mission {
		t.Fatalf("supervisor-slot=%v for mission=%v", supervisor, mission)
	}
}

func assertMissionCheckerResponse(t *testing.T, response factoryapi.DispatchResponseEventPayload, exhausted bool) int {
	t.Helper()
	const reason = "mission-output-invalid: output requires non-empty measurements"
	if response.Outcome == factoryapi.WorkOutcomeRejected {
		if response.Feedback == nil || *response.Feedback != reason {
			t.Fatalf("correction reason=%v", response.Feedback)
		}
		return 1
	}
	if exhausted && response.Outcome == factoryapi.WorkOutcomeFailed {
		if response.Error == nil {
			t.Fatal("final failure missing reason")
		}
		if *response.Error != reason {
			t.Fatalf("final failure reason=%q", *response.Error)
		}
	}
	return 0
}

// The fixture drives the documented commands through the shared public process.
// Corrections are deliberately staged before completion so their dependent
// loopback cannot execute while the original verification checks its receipt.
func runMissionGapProtocol(t *testing.T, wake *projectWakeScenario, artifact string, tagged, failed bool) map[string]any {
	t.Helper()
	result := map[string]any{"dryRun": false, "submitted": false, "receiptVerified": false, "dependentLoopback": true}
	path := artifact
	if failed {
		path += ".unavailable"
	}
	args := []string{"you", "--server", wake.fixture.baseURL, "--json", "submit", "batch", "--session", wake.sessionID}
	dryRun := support.FakeInputs(context.Background(), append(append([]string(nil), args...), "--dry-run", path))
	err := wake.fixture.process.Execute(dryRun.Input)
	if failed {
		if err == nil {
			t.Error("missing proposal dry-run unexpectedly succeeded")
		}
		return result
	}
	if err != nil {
		t.Errorf("proposal dry-run: %v; %s", err, dryRun.Stderr())
		return result
	}
	result["dryRun"] = true
	if tagged {
		return result
	}
	submitted := support.FakeInputs(context.Background(), append(append([]string(nil), args...), artifact))
	if err := wake.fixture.process.Execute(submitted.Input); err != nil {
		t.Errorf("submit corrective batch: %v; %s", err, submitted.Stderr())
		return result
	}
	result["submitted"] = true
	result["requestId"] = "stable-fixture"
	// Verify receipt fields and inspect the admitted Works, then repeat the exact
	// stable request to prove idempotency instead of creating another batch.
	if !strings.Contains(submitted.Stdout(), "stable-fixture") || !strings.Contains(submitted.Stdout(), wake.sessionID) {
		t.Errorf("receipt missing request/session: %s", submitted.Stdout())
	}
	repeat := support.FakeInputs(context.Background(), append(append([]string(nil), args...), artifact))
	if err := wake.fixture.process.Execute(repeat.Input); err != nil {
		t.Errorf("idempotent repeat: %v", err)
		return result
	}
	result["receiptVerified"] = verifyMissionCorrectionReceipt(t, wake)

	return result
}

func verifyMissionCorrectionReceipt(t *testing.T, wake *projectWakeScenario) bool {
	t.Helper()
	count := 0
	for _, work := range wake.listWorks(t) {
		if work.RequestId == nil || *work.RequestId != "stable-fixture" {
			continue
		}
		count++
		if work.WorkId == nil || *work.WorkId == "" {
			t.Error("receipt Work missing ID")
		}
		if work.Name == "loopback" {
			if work.Relations == nil || len(*work.Relations) != 1 {
				t.Errorf("dependent loopback relations=%#v", work.Relations)
			}
		}
	}
	if count != 2 {
		t.Errorf("admitted corrective Work count=%d, want 2", count)
	}
	return count == 2
}
