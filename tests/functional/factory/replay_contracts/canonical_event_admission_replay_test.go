package replay_contracts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCanonicalRecordReplayPreservesAdmittedFacts uses the assembled process
// to compare the public live stream with the persisted canonical artifact and
// then sends a malformed event through the same replay admission boundary.
func TestCanonicalRecordReplayPreservesAdmittedFacts(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, replayContractFactoryConfig())
	testutil.WriteSeedFile(t, factoryDir, "task", []byte(`{"title":"canonical Recordings fact"}`))
	artifactPath := filepath.Join(t.TempDir(), "canonical-facts.replay.json")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		Args:                      []string{"--record", artifactPath},
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: support.NewStaticSuccessCommandRunner("canonical fact provider COMPLETE"),
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 15*time.Second)
	liveEvents := server.GetFactoryEvents(t)
	server.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertCanonicalFactsMatchLiveStream(t, artifact, liveEvents)
	assertMalformedReplayIsRejected(t, artifactPath, artifact)
}

func assertCanonicalFactsMatchLiveStream(
	t *testing.T,
	artifact *factorydefinitions.ReplayArtifact,
	liveEvents []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(artifact.Events) == 0 || len(liveEvents) == 0 {
		t.Fatalf("canonical artifact/live event counts = %d/%d, want non-zero histories", len(artifact.Events), len(liveEvents))
	}
	if artifact.Events[len(artifact.Events)-1].Context.Sequence != len(artifact.Events)-1 {
		t.Fatalf("artifact terminal sequence = %d, want %d", artifact.Events[len(artifact.Events)-1].Context.Sequence, len(artifact.Events)-1)
	}
	byID := make(map[string]factorydefinitions.FactoryEvent, len(artifact.Events))
	for _, recorded := range artifact.Events {
		byID[recorded.Id] = recorded
	}
	for index, live := range liveEvents {
		recorded, ok := byID[live.Id]
		if !ok || recorded.Context.Sequence != live.Context.Sequence || string(recorded.Type) != string(live.Type) {
			t.Fatalf("live event[%d] = %#v has no matching canonical fact", index, live)
		}
		if canonicalPayloadIsPubliclyComparable(recorded.Type) {
			livePayload, err := json.Marshal(live.Payload)
			if err != nil {
				t.Fatalf("marshal live event[%d] payload: %v", index, err)
			}
			if !sameJSON(recorded.Payload, livePayload) {
				t.Fatalf("event[%d] payloads differ: recorded=%s live=%s", index, recorded.Payload, livePayload)
			}
		}
		assertEventScope(t, recorded, live)
	}
	for _, want := range []factorydefinitions.FactoryEventType{
		factorydefinitions.FactoryEventTypeRunResponse,
		factorydefinitions.FactoryEventTypeSessionCompleted,
	} {
		if !hasCanonicalEventType(artifact.Events, want) {
			t.Fatalf("canonical artifact is missing terminal %s fact", want)
		}
	}
}

func canonicalPayloadIsPubliclyComparable(kind factorydefinitions.FactoryEventType) bool {
	switch kind {
	case factorydefinitions.FactoryEventTypeWorkRequest,
		factorydefinitions.FactoryEventTypeDispatchRequest,
		factorydefinitions.FactoryEventTypeDispatchResponse:
		return true
	default:
		return false
	}
}

func hasCanonicalEventType(events []factorydefinitions.FactoryEvent, want factorydefinitions.FactoryEventType) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}

func assertEventScope(t *testing.T, recorded factorydefinitions.FactoryEvent, live factoryapi.FactoryEvent) {
	t.Helper()
	switch recorded.Type {
	case factorydefinitions.FactoryEventTypeSessionStarted,
		factorydefinitions.FactoryEventTypeSessionResultUpdated,
		factorydefinitions.FactoryEventTypeSessionCompleted:
		if value := stringPointerValue(recorded.Context.SessionID); value != "~default" || stringPointerValue(live.Context.SessionId) != value {
			t.Fatalf("%s session scope = recorded %q/live %q, want ~default", recorded.Type, value, stringPointerValue(live.Context.SessionId))
		}
	}
}

func assertMalformedReplayIsRejected(
	t *testing.T,
	artifactPath string,
	artifact *factorydefinitions.ReplayArtifact,
) {
	t.Helper()
	invalid := *artifact
	invalid.Events = append([]factorydefinitions.FactoryEvent(nil), artifact.Events...)
	for index := range invalid.Events {
		if invalid.Events[index].Type == factorydefinitions.FactoryEventTypeWorkRequest {
			invalid.Events[index].Payload = json.RawMessage(`[]`)
			break
		}
		if index == len(invalid.Events)-1 {
			t.Fatal("recorded artifact has no WORK_REQUEST event to invalidate")
		}
	}
	invalidPath := filepath.Join(t.TempDir(), "malformed.replay.json")
	payload, err := json.Marshal(invalid)
	if err != nil {
		t.Fatalf("marshal malformed replay artifact: %v", err)
	}
	if err := os.WriteFile(invalidPath, payload, 0o600); err != nil {
		t.Fatalf("write malformed replay artifact: %v", err)
	}
	runner := &rejectingReplayRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			if path != invalidPath {
				return nil, fmt.Errorf("unexpected replay path %q", path)
			}
			return os.ReadFile(path)
		},
		ProviderCommandRunner: runner,
	})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", t.TempDir(), "--replay", invalidPath, "--no-record", "--quiet",
	})
	inputs.Input.Env = isolatedReplayEnvironment(t)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("malformed canonical replay succeeded")
	}
	if runner.calls.Load() != 0 {
		t.Fatalf("provider calls during malformed replay = %d, want 0", runner.calls.Load())
	}
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("source artifact after malformed replay: %v", err)
	}
}

func sameJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right))
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type rejectingReplayRunner struct {
	calls atomic.Int32
}

func (runner *rejectingReplayRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{}, errors.New("provider execution is not valid during malformed replay")
}

var _ platformprocess.CommandRunner = (*rejectingReplayRunner)(nil)

// TestRootComposedCanonicalAppendRejectsContractOnlyKinds exercises the
// public Recordings append capability in the same root-built process as the
// functional test. The CLI/API daemon scenarios above intentionally remain
// process-boundary tests, so this narrower public-root seam is needed to
// observe the canonical admission result without importing owner internals.
func TestRootComposedCanonicalAppendRejectsContractOnlyKinds(t *testing.T) {
	var recordingsRoot recordings.Service
	process := support.BuildProcess(t, serviceedges.Edges{
		RecordingsRootObserver: func(root recordings.Service) {
			recordingsRoot = root
		},
	})
	support.CleanupProcess(t, process)
	if recordingsRoot == nil {
		t.Fatal("RecordingsRootObserver was not invoked")
	}

	first := canonicalAppendEvent("functional-canonical-first")
	acceptedFirst, err := recordingsRoot.Append(recordings.AppendRecordedEventRequest{Event: first})
	if err != nil {
		t.Fatalf("public Recordings Append(first): %v", err)
	}
	if acceptedFirst.Event.ID != first.ID || acceptedFirst.Event.Sequence != 0 {
		t.Fatalf("first accepted canonical event = %#v, want id %q at sequence 0", acceptedFirst.Event, first.ID)
	}

	invalid := first
	invalid.ID = "functional-canonical-contract-only"
	invalid.Kind = recordings.CanonicalEventKind(recordings.FactoryEventTypeJavaScriptCheckpointRef)
	if _, err := recordingsRoot.Append(recordings.AppendRecordedEventRequest{Event: invalid}); !errors.Is(err, recordings.ErrInvalidAppendEvent) {
		t.Fatalf("public Recordings Append(contract-only kind) error = %v, want %v", err, recordings.ErrInvalidAppendEvent)
	}

	second := canonicalAppendEvent("functional-canonical-second")
	acceptedSecond, err := recordingsRoot.Append(recordings.AppendRecordedEventRequest{Event: second})
	if err != nil {
		t.Fatalf("public Recordings Append(second): %v", err)
	}
	if acceptedSecond.Event.Sequence != 1 {
		t.Fatalf("second accepted canonical event sequence = %d, want 1 after rejected append", acceptedSecond.Event.Sequence)
	}

	subscribed, err := recordingsRoot.SubscribeFrom(t.Context(), recordings.SubscribeRequest{})
	if err != nil {
		t.Fatalf("public Recordings SubscribeFrom: %v", err)
	}
	for index, want := range []recordings.CanonicalEvent{acceptedFirst.Event, acceptedSecond.Event} {
		outcome := subscribed.Subscription.Next(t.Context())
		if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != want.ID || outcome.Event.Sequence != recordings.CanonicalEventSequence(index) {
			t.Fatalf("canonical subscription event[%d] = %#v, want %q at sequence %d", index, outcome, want.ID, index)
		}
	}
}

// TestRootComposedRecordingsValidatesOpenAPIEventKindParity exercises the
// existing parity guard through the composed public root. The guard is a
// Recordings-owned contract check, so this functional scenario keeps the
// coverage on the same root seam used by the application.
func TestRootComposedRecordingsValidatesOpenAPIEventKindParity(t *testing.T) {
	var recordingsRoot recordings.Service
	process := support.BuildProcess(t, serviceedges.Edges{
		RecordingsRootObserver: func(root recordings.Service) {
			recordingsRoot = root
		},
	})
	support.CleanupProcess(t, process)
	if recordingsRoot == nil {
		t.Fatal("RecordingsRootObserver was not invoked")
	}
	validator, ok := recordingsRoot.(interface {
		ValidateFactoryEventKindParity([]byte) error
	})
	if !ok {
		t.Fatal("composed Recordings root does not expose event-kind parity validation")
	}
	openAPIYAML, err := os.ReadFile(testutil.MustRepoPath(t, "api/openapi.yaml"))
	if err != nil {
		t.Fatalf("read bundled OpenAPI contract: %v", err)
	}
	if err := validator.ValidateFactoryEventKindParity(openAPIYAML); err != nil {
		t.Fatalf("ValidateFactoryEventKindParity: %v", err)
	}
}

func canonicalAppendEvent(id string) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:         recordings.CanonicalEventID(id),
		Kind:       recordings.CanonicalEventKind(recordings.FactoryEventTypeRunRequest),
		RecordedAt: time.Date(2026, time.August, 22, 20, 0, 0, 0, time.UTC),
		Payload:    `{}`,
	}
}
