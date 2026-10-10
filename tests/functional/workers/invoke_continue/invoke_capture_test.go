package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestRecordingContentDirectEchoAndNonEcho(t *testing.T) {
	for _, name := range []string{"recording-echo", "recording-other", "recording-equal", "recording-snapshot"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := ensureInvokeContinuePackageFixture(t)
			scenario := fixture.scenario(t, name)
			defer scenario.close(t)
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			runner := scenario.providerRunner.(*t7GatedProviderRunner)
			defer t7ReleaseAndJoin(t, ctx, runner)()
			id := scenarioScopedID(scenario, "content")
			prompt := "ordinary nonoverlapping prompt"
			if name == "recording-echo" {
				prompt = "DIRECT_CAPTURE_BETA"
			}
			path := filepath.Join(scenario.workingDirectory, "content.json")
			writeInvokeContinueJSON(t, path, invokeContinueExecutionDocument(invokeContinueExecutionSpec{
				requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
				workingDirectory: scenario.workingDirectory, userMessage: prompt,
			}))
			start := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--async", "--execution", path)
			if err := fixture.process.Execute(start.Input); err != nil {
				t.Fatalf("invoke: %v: %s", err, start.Stderr())
			}
			t19AwaitSignal(t, ctx, runner.started, "ordinary output")
			// The durable reader commits asynchronously after provider observation.
			// Await this session's public prefix rather than provider callback timing.
			liveMarker := "DIRECT_CAPTURE_BETA"
			if name == "recording-snapshot" {
				liveMarker = "DIRECT_CAPTURE_"
			}
			if body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
				_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
				return body, nil
			}, func(body string) bool { return strings.Contains(body, liveMarker) }); err != nil {
				t.Fatalf("live content: %v: %s", err, body)
			}
			close(runner.release)
			join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
			if err := fixture.process.Execute(join.Input); err != nil {
				t.Fatalf("join: %v: %s", err, join.Stderr())
			}
			_, summary := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id, nil)
			var observation factoryapi.WorkerSessionObservation
			if err := json.Unmarshal([]byte(summary), &observation); err != nil {
				t.Fatal(err)
			}
			if observation.TokenUsage != nil {
				t.Fatalf("unknown native-path usage became known: %+v", observation.TokenUsage)
			}
			page := recordingContentPages(t, ctx, fixture.baseURL, id)
			assertRecordingMessageProvenance(t, page, false)
			for _, view := range []string{"logs", "transcript"} {
				read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", view, "--worker-session-id", id)
				if err := fixture.process.Execute(read.Input); err != nil {
					t.Fatalf("read %s: %v: %s", view, err, read.Stderr())
				}
				status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/"+view, nil)
				if status != http.StatusOK || !strings.Contains(read.Stdout(), "DIRECT_CAPTURE_BETA") || !strings.Contains(body, "DIRECT_CAPTURE_BETA") {
					t.Fatalf("completed %s lost ordinary output: %d %s / %s", view, status, read.Stdout(), body)
				}
				if view == "transcript" {
					var cli, httpResult factoryapi.WorkerSessionTranscriptResponse
					if err := json.Unmarshal([]byte(read.Stdout()), &cli); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal([]byte(body), &httpResult); err != nil {
						t.Fatal(err)
					}
					wantEntries := 1
					if name == "recording-equal" {
						wantEntries = 2
					}
					if !reflect.DeepEqual(cli, httpResult) || len(cli.Entries) != wantEntries || cli.Entries[0].Text == nil || *cli.Entries[0].Text != "DIRECT_CAPTURE_BETA" {
						t.Fatalf("transcript parity/content = %+v / %+v", cli, httpResult)
					}
					for i, entry := range cli.Entries {
						if entry.Order != i+1 || entry.Text == nil || *entry.Text != "DIRECT_CAPTURE_BETA" || entry.Timestamp == nil {
							t.Fatalf("distinct ordered messages lost: %+v", cli.Entries)
						}
					}
				}
			}
		})
	}
}

// FC2 crosses actual Factory admission and the agent loop using the shared
// process. The command gate lets the observer read the first committed item.
func TestRecordingContentFactoryResultAppearsOnce(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "recording-factory")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	t7WriteFactorySibling(t, scenario.workingDirectory)
	// FC2 requires the Factory agent loop, which is selected by both kinds.
	for path, replacements := range map[string][2]string{
		filepath.Join("workers", "worker", "AGENTS.md"):       {"MODEL_WORKER", "AGENT_WORKER"},
		filepath.Join("workstations", "process", "AGENTS.md"): {"MODEL_WORKSTATION", "AGENT_RUN"},
	} {
		file := filepath.Join(scenario.workingDirectory, path)
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(strings.ReplaceAll(string(body), replacements[0], replacements[1])), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.workingDirectory)
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	item := support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Payload: "FACTORY_CAPTURE_ALPHA COMPLETE",
	})
	if item.WorkId == nil {
		t.Fatal("Factory Work has no identity")
	}
	t19AwaitSignal(t, ctx, runner.started, "Factory content")
	rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, fixture.baseURL+"/factory-sessions/"+opened.Session.Id+"/worker-sessions?workId="+*item.WorkId)
	if len(rows.Sessions) != 1 {
		t.Fatalf("Factory attempts = %+v", rows)
	}
	id := rows.Sessions[0].WorkerSessionId
	_, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		return body, nil
	}, func(body string) bool { return strings.Contains(body, "FACTORY_CAPTURE_ALPHA COMPLETE") })
	if err != nil {
		t.Fatal(err)
	}
	close(runner.release)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, opened.Session.Id, 30*time.Second)
	read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", "transcript", "--worker-session-id", id)
	if err := fixture.process.Execute(read.Input); err != nil {
		t.Fatalf("Factory transcript: %v %s", err, read.Stderr())
	}
	var transcript factoryapi.WorkerSessionTranscriptResponse
	if err := json.Unmarshal([]byte(read.Stdout()), &transcript); err != nil {
		t.Fatal(err)
	}
	httpTranscript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, fixture.baseURL+"/worker-sessions/"+id+"/transcript")
	if !reflect.DeepEqual(transcript, httpTranscript) || len(transcript.Entries) != 1 ||
		transcript.Entries[0].Text == nil || *transcript.Entries[0].Text != "FACTORY_CAPTURE_ALPHA COMPLETE" || transcript.Entries[0].Order != 1 {
		t.Fatalf("Factory result identity/parity: %+v / %+v", transcript, httpTranscript)
	}
	assertRecordingMessageProvenance(t, recordingContentPages(t, ctx, fixture.baseURL, id), true)
}

func recordingContentPages(t *testing.T, ctx context.Context, baseURL, id string) factoryapi.WorkerSessionLogPage {
	t.Helper()
	endpoint := baseURL + "/worker-sessions/" + id + "/logs?limit=2"
	var result factoryapi.WorkerSessionLogPage
	for pages := 0; pages < 100; pages++ {
		status, body := t7HTTP(t, ctx, http.MethodGet, endpoint, nil)
		var page factoryapi.WorkerSessionLogPage
		if err := json.Unmarshal([]byte(body), &page); err != nil || status != http.StatusOK || page.Health != "COMPLETE" {
			t.Fatalf("committed logs: status=%d error=%v body=%s", status, err, body)
		}
		if pages == 0 {
			result = page
			result.Events = nil
		} else if page.CommittedPosition != result.CommittedPosition || page.RecordingGenerationId != result.RecordingGenerationId {
			t.Fatal("capture watermark changed while paging completed recording")
		}
		for _, event := range page.Events {
			if event.Event.Position != int64(len(result.Events)+1) || event.Event.CapturedAt == nil || event.Event.SourceEventId == "" {
				t.Fatalf("committed order/identity/time: %+v", event)
			}
			result.Events = append(result.Events, event)
		}
		if page.NextToken == nil {
			if int64(len(result.Events)) != page.CommittedPosition {
				t.Fatal("paging lost committed records")
			}
			result.NextToken = nil
			return result
		}
		endpoint = baseURL + "/worker-sessions/" + id + "/logs?limit=2&nextToken=" + *page.NextToken
	}
	t.Fatal("completed recording did not finish paging")
	return result
}

func assertRecordingMessageProvenance(t *testing.T, page factoryapi.WorkerSessionLogPage, agentLoop bool) {
	t.Helper()
	var native, synthesized []workers.Draft
	for _, event := range page.Events {
		encoded, err := json.Marshal(event.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var draft workers.Draft
		if err := json.Unmarshal(encoded, &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Kind != workers.KindMessage {
			continue
		}
		switch draft.Provenance.Delivery {
		case workers.DeliveryNativeStream:
			native = append(native, draft)
		case workers.DeliverySynthesized:
			synthesized = append(synthesized, draft)
		}
	}
	wantSynthesized := 0
	if agentLoop {
		wantSynthesized = 1
	}
	if len(native) < 2 || len(synthesized) != wantSynthesized {
		t.Fatalf("stream/final provenance: native=%+v synthesized=%+v", native, synthesized)
	}
	if len(synthesized) == 0 {
		// Direct provider execution has no agent-loop final snapshot.
		if native[len(native)-1].Phase != workers.PhaseCompleted {
			t.Fatal("direct stream lost completed message")
		}
		return
	}
	last, final := native[len(native)-1], synthesized[0]
	if last.ItemID == "" || last.ItemID != final.ItemID || last.TurnID != final.TurnID || last.RunID != final.RunID || last.DispatchID != final.DispatchID || final.Provenance.Fidelity != workers.FidelityNormalized {
		t.Fatalf("final snapshot lost last stream identity: %+v / %+v", last, final)
	}
}

const (
	t7SecretPrompt = "synthetic-private-t7-user-message"
	t7SecretSystem = "synthetic-private-t7-system-prompt"
	t7SecretToken  = "synthetic-private-t7-api-token"
)

// F6-10 observes the delivered redaction policy after the native adapter
// echoes classified request values in otherwise public tool progress.
func TestT7CapturedProgressRedactsRequestSecrets(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.read", "rest/readWorkerSessionLogs")
	runRecordingContentSecrets(t, "t7-secrets")
}

func TestRecordingContentFailedPrefixRedactsSecrets(t *testing.T) {
	t.Parallel()
	runRecordingContentSecrets(t, "recording-failure")
}

func runRecordingContentSecrets(t *testing.T, name string) {
	t.Helper()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, name)
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	id := scenarioScopedID(scenario, "private-session")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: t7SecretPrompt,
	})
	execution := document["execution"].(map[string]any)
	execution["systemPrompt"] = t7SecretSystem
	// These prompt values are explicitly classified secrets in this cell.
	// Ordinary prompts are exercised separately by RecordingContent.
	execution["envVars"] = map[string]string{"T7_API_TOKEN": t7SecretToken, "T7_PROMPT_SECRET": t7SecretPrompt, "T7_SYSTEM_SECRET": t7SecretSystem}
	path := filepath.Join(scenario.workingDirectory, "private.json")
	writeInvokeContinueJSON(t, path, document)
	start := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--async", "--execution", path)
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatalf("private invoke: %v: %s", err, start.Stderr())
	}
	t19AwaitSignal(t, ctx, runner.started, "private progress")
	// Capture is asynchronous; await only this session's committed progress.
	body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		return body, nil
	}, func(body string) bool { return strings.Contains(body, "public progress") })
	if err != nil {
		t.Fatalf("private capture: %v: %s", err, body)
	}
	t7AssertSecretsAbsent(t, body)
	if !strings.Contains(body, "redacted") {
		t.Fatalf("progress did not retain redaction marker: %s", body)
	}
	close(runner.release)
	join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(join.Input); (err != nil) != (name == "recording-failure") {
		t.Fatalf("private completion: %v: %s", err, join.Stderr())
	}
	logs := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", "logs", "--worker-session-id", id)
	if err := fixture.process.Execute(logs.Input); err != nil {
		t.Fatalf("private archived logs: %v", err)
	}
	t7AssertSecretsAbsent(t, logs.Stdout()+logs.Stderr()+start.Stdout()+start.Stderr()+join.Stdout()+join.Stderr())
	status, observation := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id, nil)
	if status != http.StatusOK || runner.CallCount() != 1 {
		t.Fatalf("private observation = %d, calls = %d", status, runner.CallCount())
	}
	t7AssertSecretsAbsent(t, observation)
	if name == "recording-failure" {
		var summary factoryapi.WorkerSessionObservation
		if err := json.Unmarshal([]byte(observation), &summary); err != nil || summary.State != "FAILED" || summary.Failure == nil {
			t.Fatalf("failed prefix fabricated completion: error=%v summary=%s", err, observation)
		}
		page := recordingContentPages(t, ctx, fixture.baseURL, id)
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		t7AssertSecretsAbsent(t, string(encoded))
		if !strings.Contains(string(encoded), "ordinary prefix") || !strings.Contains(string(encoded), "public progress") {
			t.Fatal("failed attempt lost ordinary captured neighbors")
		}
		read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--view", "transcript", "--worker-session-id", id)
		if err := fixture.process.Execute(read.Input); err != nil {
			t.Fatalf("failed prefix transcript: %v %s", err, read.Stderr())
		}
		httpTranscript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, fixture.baseURL+"/worker-sessions/"+id+"/transcript")
		var transcript factoryapi.WorkerSessionTranscriptResponse
		if err := json.Unmarshal([]byte(read.Stdout()), &transcript); err != nil || !reflect.DeepEqual(transcript, httpTranscript) || transcript.State != "FAILED" {
			t.Fatalf("failed transcript parity: error=%v transcript=%s", err, read.Stdout())
		}
		t7AssertSecretsAbsent(t, read.Stdout())
		if !strings.Contains(read.Stdout(), "ordinary prefix") || !strings.Contains(read.Stdout(), "public progress") {
			t.Fatal("failed transcript lost committed prefix")
		}
	}
}

func t7AssertSecretsAbsent(t *testing.T, body string) {
	t.Helper()
	for _, secret := range []string{t7SecretPrompt, t7SecretSystem, t7SecretToken} {
		if strings.Contains(body, secret) {
			t.Fatal("public capture or diagnostics exposed a classified synthetic secret")
		}
	}
}

// F6-10 injects one selected session's append failure at the existing durable
// writer edge. Its opening remains real and stopping still joins the attempt.
func TestT7CaptureLossRetainsPrefixAndLiveStop(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "cli/you.worker-sessions.terminate", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "t7-degraded")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	id, _ := t7StartGatedAsync(t, ctx, fixture, scenario)
	t19AwaitSignal(t, ctx, runner.started, "capture-loss attempt progress")
	// Capture consumes the Events topic asynchronously. Polling the public
	// projection is required to observe its committed prefix independently
	// of the provider command callback; no shared state is inspected.
	if body, err := support.WaitForObservation(60*time.Second, func() (string, error) {
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		if status != http.StatusOK {
			return "", nil
		}
		return body, nil
	}, func(body string) bool {
		return strings.Contains(body, `"health":"INCOMPLETE"`) && strings.Contains(body, `"committedPosition":2`)
	}); err != nil {
		t.Fatalf("direct capture retains live prefix: %v: %s", err, body)
	}
	stop := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "terminate", id)
	if err := fixture.process.Execute(stop.Input); err != nil {
		t.Fatalf("degraded attempt stop: %v: %s", err, stop.Stderr())
	}
	t7AssertStopResult(t, stop.Stdout(), id, "terminate")
	t19AwaitSignal(t, ctx, runner.stopped, "degraded attempt joined")
	status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(body, `"health":"DEGRADED"`) || !strings.Contains(body, `"position":1`) || strings.Contains(body, "private-t7-capture-failure") {
		t.Fatalf("degraded prefix lost or health fabricated: %d %s", status, body)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("degraded attempt provider calls = %d", runner.CallCount())
	}
}
