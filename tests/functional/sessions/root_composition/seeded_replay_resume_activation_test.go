package root_composition_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	acquireRootCompositionFixtureSlot(t)
	fixture := ensureRootCompositionFixture(t)

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
			artifactPayload := seededReplayResumeArtifactPayload(t, test.finished)
			factoryDir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			artifactPath := filepath.Join(factoryDir, "seeded-replay-resume.json")
			if err := os.WriteFile(artifactPath, artifactPayload, 0o644); err != nil {
				t.Fatalf("write replay artifact: %v", err)
			}
			homeDir := t.TempDir()
			label := "seeded-replay-resume-in-flight"
			if test.finished {
				label = "seeded-replay-resume-finished"
			}
			fixture.withRootCompositionRoute(t, rootCompositionRouteSpec{
				label:      label,
				homeDir:    homeDir,
				workingDir: factoryDir,
				extraPaths: []string{artifactPath},
			}, func() {
				running := startSeededReplayResumeRun(t, fixture, factoryDir, artifactPath)

				stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(running.url))
				waitForSeededReplayRuntimeStart(t, stream)
				status := support.GetJSON[factoryapi.StatusResponse](t, strings.TrimSuffix(running.url, "/")+"/status")

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

				listed := support.ListDefaultSessionWork(t, running.url)
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

				for _, event := range support.GetFactoryEventsAt(t, running.url) {
					if event.Type == factoryapi.FactoryEventTypeDispatchRequest {
						t.Fatalf("replayed Work unexpectedly produced a dispatch request: %#v", event)
					}
				}
				running.daemon.Stop(t)
			})
		})
	}
}

type seededReplayResumeRun struct {
	url    string
	daemon *rootCompositionServer
}

func startSeededReplayResumeRun(
	t *testing.T,
	fixture *rootCompositionFixture,
	factoryDir string,
	artifactPath string,
) seededReplayResumeRun {
	t.Helper()
	server := startRootCompositionServer(t, fixture, support.NewProcessAPIServer(), []string{
		"you", "run",
		"--continuously", "--with-server", "--quiet",
		"--dir", factoryDir,
		"--provider", "CODEX", "--model", "gpt-5-codex",
		"--replay", artifactPath, "--no-record",
	}, nil, factoryDir)
	return seededReplayResumeRun{url: server.URL(t), daemon: server}
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
