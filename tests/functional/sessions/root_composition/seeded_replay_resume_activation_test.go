package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession
// exercises the assembled replay/resume path through the customer process. The
// in-flight artifact is intentionally unfinalized, while the finished artifact
// retains its terminal Work state.
func TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	reusable := newSeededReplayResumeProcess(t)

	for _, test := range []struct {
		name     string
		finished bool
	}{
		{
			name: "in-flight tail",
		}, {
			name:     "finished recording",
			finished: true,
		}} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			artifactPayload := seededReplayResumeArtifactPayload(t, test.finished)
			factoryDir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			artifactPath := filepath.Join(factoryDir, "seeded-replay-resume.json")
			if err := os.WriteFile(artifactPath, artifactPayload, 0o644); err != nil {
				t.Fatalf("write replay artifact: %v", err)
			}
			running := reusable.run(t, factoryDir, artifactPath)

			stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(running.url, running.sessionID))
			waitForSeededReplayRuntimeStart(t, stream)
			status := support.GetJSON[factoryapi.StatusResponse](
				t,
				strings.TrimSuffix(running.url, "/")+"/factory-sessions/"+running.sessionID+"/status",
			)

			if status.TotalTokens != 1 {
				t.Fatalf("replayed status totalTokens = %d, want one Work token", status.TotalTokens)
			}
			if test.finished {
				if status.Categories.Terminal != 1 {
					t.Fatalf("finished replay terminal count = %d, want 1", status.Categories.Terminal)
				}
			} else if status.Categories.Initial != 1 {
				t.Fatalf("in-flight replay initial count = %d, want 1", status.Categories.Initial)
			}

			listed := support.GetJSON[factoryapi.ListWorkResponse](
				t,
				support.SessionWorkURL(running.url, running.sessionID, "/work"),
			)
			if len(listed.Results) != 1 {
				t.Fatalf("replayed Work listing length = %d, want one Work: %#v", len(listed.Results), listed.Results)
			}
			wantLocation := "task:init"
			if test.finished {
				wantLocation = "task:complete"
			}
			if !support.HasWorkAtCustomerState(listed, "work-seeded-replay-resume", wantLocation) {
				t.Fatalf("replayed Work is not at %s: %#v", wantLocation, listed.Results)
			}

			for _, event := range support.GetFactoryEventsForSessionAt(t, running.url, running.sessionID) {
				if event.Type == factoryapi.FactoryEventTypeDispatchRequest {
					t.Fatalf("replayed Work unexpectedly produced a dispatch request: %#v", event)
				}
			}
			running.daemon.Stop(t)
		})
	}
}

type seededReplayResumeProcess struct {
	process support.Process

	mu             sync.RWMutex
	serversByPort  map[int]*support.ProcessAPIServer
	payloadsByPath map[string][]byte
	nextPort       atomic.Int32
}

type seededReplayResumeRun struct {
	url       string
	sessionID string
	daemon    *support.ProcessCommand
}

func newSeededReplayResumeProcess(t *testing.T) *seededReplayResumeProcess {
	t.Helper()
	reusable := &seededReplayResumeProcess{
		serversByPort:  make(map[int]*support.ProcessAPIServer),
		payloadsByPath: make(map[string][]byte),
	}
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:                    reusable.startAPIServer,
		FactorySessionReplayRecordingReader: reusable.readReplayRecording,
		ProviderCommandRunner: testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout("unexpected replay dispatch COMPLETE"),
		}),
	})
	support.CleanupProcess(t, process)
	reusable.process = process
	return reusable
}

func (reusable *seededReplayResumeProcess) run(
	t *testing.T,
	factoryDir string,
	artifactPath string,
) seededReplayResumeRun {
	t.Helper()
	api := support.NewProcessAPIServer()
	port := 22000 + int(reusable.nextPort.Add(1))
	reusable.mu.Lock()
	reusable.serversByPort[port] = api
	reusable.payloadsByPath[filepath.Clean(artifactPath)] = append([]byte(nil), mustReadSeededReplayArtifact(t, artifactPath)...)
	reusable.mu.Unlock()
	t.Cleanup(func() {
		reusable.mu.Lock()
		delete(reusable.serversByPort, port)
		delete(reusable.payloadsByPath, filepath.Clean(artifactPath))
		reusable.mu.Unlock()
	})
	sessionID := uuid.NewString()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--session", sessionID,
		"--continuously", "--with-server", "--quiet",
		"--listen", fmt.Sprintf("127.0.0.1:%d", port),
		"--dir", factoryDir,
		"--provider", "CODEX", "--model", "gpt-5-codex",
		"--replay", artifactPath, "--no-record",
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = factoryDir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("seeded replay daemon stderr: %s", stderr)
		}
	})
	daemon := support.StartProcessCommand(t, reusable.process, inputs.Input)
	return seededReplayResumeRun{url: api.WaitForURL(t), sessionID: sessionID, daemon: daemon}
}

func mustReadSeededReplayArtifact(t testing.TB, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replay artifact: %v", err)
	}
	return payload
}

func (reusable *seededReplayResumeProcess) readReplayRecording(path string) ([]byte, error) {
	reusable.mu.RLock()
	payload := reusable.payloadsByPath[filepath.Clean(path)]
	reusable.mu.RUnlock()
	if len(payload) == 0 {
		return nil, errors.New("seeded replay payload was not registered for this invocation")
	}
	return append([]byte(nil), payload...), nil
}

func (reusable *seededReplayResumeProcess) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	reusable.mu.RLock()
	server := reusable.serversByPort[request.Port]
	reusable.mu.RUnlock()
	if server == nil {
		return fmt.Errorf("seeded replay API server is not registered for requested port %d", request.Port)
	}
	return server.Start(ctx, request)
}

func waitForSeededReplayRuntimeStart(t *testing.T, stream *support.FactoryEventStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if event.Type == factoryapi.FactoryEventTypeFactoryStateResponse {
			return
		}
	}
}

func seededReplayResumeFactoryConfig() map[string]any {
	return map[string]any{
		"name": "seeded-replay-resume",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func seededReplayResumeArtifactPayload(t *testing.T, finished bool) []byte {
	t.Helper()
	const (
		workID    = "work-seeded-replay-resume"
		requestID = "request-seeded-replay-resume"
		traceID   = "trace-seeded-replay-resume"
	)
	snapshot, err := factorydefinitions.NewFactorySnapshot(seededReplayResumeFactoryConfig())
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	requestIDValue := requestID
	workIDValue := []string{workID}
	traceIDValue := []string{traceID}
	requestSourceValue := "external-submit"
	sourceValue := requestSourceValue
	events := []factorydefinitions.FactoryEvent{
		seededReplayResumeEvent(t, "run-request", 0, 0, base, factorydefinitions.FactoryEventTypeRunRequest, factorydefinitions.RunRequestEventPayload{
			Factory:    snapshot,
			RecordedAt: base,
		}),
		seededReplayResumeEventWithContext(t, "work-request", 1, 1, base.Add(time.Second), factorydefinitions.FactoryEventTypeWorkRequest, work.WorkRequestEventPayload{
			Source: requestSourceValue,
			Type:   work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.WorkRequestEventWork{{
				Name:       "recorded-work",
				WorkID:     workID,
				RequestID:  requestID,
				WorkTypeID: "task",
				State:      &work.WorkEventState{Name: "init", Type: "INITIAL"},
				TraceID:    traceID,
			}},
		}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
	}
	if finished {
		finishedSource := work.WorkStateChangeSourceAPI
		events = append(events,
			seededReplayResumeEventWithContext(t, "work-state-change", 2, 2, base.Add(2*time.Second), factorydefinitions.FactoryEventTypeWorkStateChange, factorydefinitions.WorkStateChangeEventPayload{
				FromPlaceID:  "task:init",
				FromState:    "init",
				Source:       finishedSource,
				ToPlaceID:    "task:complete",
				ToState:      "complete",
				WorkID:       workID,
				WorkTypeName: "task",
			}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
			seededReplayResumeEvent(t, "run-response", 3, 3, base.Add(3*time.Second), factorydefinitions.FactoryEventTypeRunResponse, func() factorydefinitions.RunResponseEventPayload {
				state := factorydefinitions.FactoryStateCompleted
				return factorydefinitions.RunResponseEventPayload{State: &state}
			}()),
		)
	}

	artifact := factorydefinitions.ReplayArtifact{
		SchemaVersion: factorydefinitions.ReplayV1SourceFormat,
		RecordedAt:    base,
		Events:        events,
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal seeded replay artifact: %v", err)
	}
	return payload
}

func seededReplayResumeEvent(
	t *testing.T,
	id string,
	sequence int,
	tick int,
	eventTime time.Time,
	eventType factorydefinitions.FactoryEventType,
	payload any,
) factorydefinitions.FactoryEvent {
	return seededReplayResumeEventWithContext(t, id, sequence, tick, eventTime, eventType, payload, nil, nil, nil, nil)
}

func seededReplayResumeEventWithContext(
	t *testing.T,
	id string,
	sequence int,
	tick int,
	eventTime time.Time,
	eventType factorydefinitions.FactoryEventType,
	payload any,
	source *string,
	contextRequestID *string,
	workIDs *[]string,
	traceIDs *[]string,
) factorydefinitions.FactoryEvent {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal replay event %q payload: %v", id, err)
	}
	return factorydefinitions.FactoryEvent{
		Id:            id,
		Payload:       payloadBytes,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          eventType,
		Context: factorydefinitions.FactoryEventContext{
			EventTime: eventTime,
			RequestID: contextRequestID,
			Sequence:  sequence,
			Source:    source,
			Tick:      tick,
			TraceIDs:  traceIDs,
			WorkIDs:   workIDs,
		},
	}
}
