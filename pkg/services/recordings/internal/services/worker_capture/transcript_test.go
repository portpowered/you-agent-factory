package worker_capture

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func transcriptRecord(position int, kind workers.Kind, phase workers.Phase, item, turn, payload string) WorkerCapturedRecord {
	draft := workers.Draft{Kind: kind, Phase: phase, ItemID: item, TurnID: turn, Payload: json.RawMessage(payload)}
	data, _ := json.Marshal(draft)
	stamp := time.Unix(int64(position), 0).UTC()
	return WorkerCapturedRecord{Record: events.Record{ID: events.RecordID{Position: events.AggregateSequence(position)}, SourceID: "provider", Payload: data}, CapturedAt: &stamp}
}

func TestProjectWorkerTranscriptNormalizesSnapshotsDeltasAndPublicSummaries(t *testing.T) {
	t.Parallel()
	records := []WorkerCapturedRecord{
		transcriptRecord(1, workers.KindTurn, workers.PhaseStarted, "", "turn", `{"turnIndex":2}`),
		transcriptRecord(2, workers.KindMessage, workers.PhaseStarted, "message", "turn", `{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"hel"}]}`),
		transcriptRecord(3, workers.KindMessage, workers.PhaseDelta, "message", "turn", `{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"lo"}`),
		transcriptRecord(4, workers.KindTool, workers.PhaseStarted, "tool", "turn", `{"toolCallId":"call","toolName":"search","argumentsSummary":{"query":"factory"}}`),
		transcriptRecord(5, workers.KindTool, workers.PhaseDelta, "tool", "turn", `{"toolCallId":"call","outputDelta":"partial"}`),
		transcriptRecord(6, workers.KindTool, workers.PhaseCompleted, "tool", "turn", `{"toolCallId":"call","toolName":"search","status":"completed","resultSummary":"tool result"}`),
		transcriptRecord(7, workers.KindMessage, workers.PhaseCompleted, "message", "turn", `{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"hello"}]}`),
		transcriptRecord(8, workers.KindReasoning, workers.PhaseDelta, "summary", "turn", `{"summaryDelta":"public ","encryptedContent":"private"}`),
		transcriptRecord(9, workers.KindReasoning, workers.PhaseCompleted, "summary", "turn", `{"summary":"public summary","privateReasoning":"secret"}`),
		transcriptRecord(10, workers.KindMessage, workers.PhaseCompleted, "user", "turn", `{"role":"user","contentBlocks":[{"kind":"TEXT","text":"follow-up"},{"kind":"STRUCTURED_OUTPUT","structuredOutput":{"secret":"never display"}}]}`),
	}
	got, err := (WorkerCapturedActivityPage{Records: records}).ProjectTranscript()
	if err != nil {
		t.Fatal(err)
	}

	expected := []WorkerTranscriptEntry{
		{Type: "assistant_message", Text: transcriptText("hello")},
		{Type: "tool_call", Name: transcriptText("search"), Arguments: transcriptText(`{"query":"factory"}`), CallID: transcriptText("call"), Status: transcriptText("completed")},
		{Type: "tool_output", Output: transcriptText("tool result"), CallID: transcriptText("call"), Status: transcriptText("completed")},
		{Type: "reasoning", Summary: transcriptText("public summary")},
		{Type: "user_message", Text: transcriptText("follow-up")},
	}
	stamps := []int64{2, 4, 5, 8, 10}
	for index := range expected {
		stamp, turn := time.Unix(stamps[index], 0).UTC(), 2
		expected[index].Order, expected[index].Timestamp, expected[index].TurnIndex = index+1, &stamp, &turn
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("captured normalization = %+v, want %+v", got, expected)
	}
	data, _ := json.Marshal(got)
	for _, private := range []string{"private", "secret", "never display"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("private facts leaked: %s", data)
		}
	}
	*got[0].Text = "mutated"
	*got[0].Timestamp = time.Time{}
	again, err := (WorkerCapturedActivityPage{Records: records}).ProjectTranscript()
	if err != nil || *again[0].Text != "hello" || again[0].Timestamp.IsZero() {
		t.Fatal("projection aliases captured data")
	}
}

func TestProjectWorkerTranscriptSeparatesTurnsSourcesAndAnonymousItems(t *testing.T) {
	t.Parallel()
	payload := `{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"answer"}]}`
	records := []WorkerCapturedRecord{
		transcriptRecord(1, workers.KindMessage, workers.PhaseCompleted, "same", "one", payload),
		transcriptRecord(2, workers.KindMessage, workers.PhaseCompleted, "same", "two", payload),
		transcriptRecord(3, workers.KindMessage, workers.PhaseCompleted, "same", "two", payload),
		transcriptRecord(4, workers.KindMessage, workers.PhaseCompleted, "", "two", payload),
		transcriptRecord(5, workers.KindMessage, workers.PhaseCompleted, "", "two", payload),
	}
	records[2].Record.SourceID = "other-provider"
	got, err := (WorkerCapturedActivityPage{Records: records}).ProjectTranscript()
	if err != nil || len(got) != 5 {
		t.Fatalf("identities merged: %+v %v", got, err)
	}
	for index, entry := range got {
		if entry.Order != index+1 {
			t.Fatalf("unstable order: %+v", got)
		}
	}
	// A finished snapshot with identical identity replaces rather than appends.
	records = append(records, transcriptRecord(6, workers.KindMessage, workers.PhaseCompleted, "same", "one", payload))
	again, err := (WorkerCapturedActivityPage{Records: records}).ProjectTranscript()
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatalf("snapshot repeated: %+v %v", again, err)
	}
}

func TestProjectWorkerTranscriptRejectsMalformedContentAndGaps(t *testing.T) {
	t.Parallel()
	for _, record := range []WorkerCapturedRecord{
		{Record: events.Record{Payload: []byte(`{`)}},
		transcriptRecord(1, workers.KindMessage, workers.PhaseCompleted, "message", "", `{"contentBlocks":"broken"}`),
		transcriptRecord(1, workers.KindStreamGap, workers.PhaseUpdated, "", "", `{}`),
	} {
		if _, err := (WorkerCapturedActivityPage{Records: []WorkerCapturedRecord{record}}).ProjectTranscript(); err == nil {
			t.Fatal("damaged history presented as complete")
		}
	}
}
