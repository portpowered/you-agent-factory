package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/work/content"
	"github.com/portpowered/infinite-you/pkg/work/materialize"
)

// T9: WORK_REQUEST after submit serializes canonical url on content parts, not dispatch temps.
func TestFactoryEventHistory_RecordWorkRequest_SerializesContentURLNotMaterializedPaths(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "submit.png")
	if err := os.WriteFile(localPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write local image: %v", err)
	}
	localURL, err := content.FilesystemPathToContentURL(localPath)
	if err != nil {
		t.Fatalf("local content url: %v", err)
	}
	remoteURL := "https://cdn.example.test/assets/review.png"

	// Simulate dispatch-time materialization; event history must never persist this path.
	materializedPath, cleanup, err := materialize.MaterializeContentURL(
		t.Context(),
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
		nil,
	)
	if err != nil {
		t.Fatalf("materialize data url: %v", err)
	}
	defer cleanup()

	record := work.WorkRequestRecord{
		RequestID: "request-url-wire",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-url-wire",
		Source:    "external-submit",
		WorkItems: []work.FactoryWorkItem{{
			ID:          "work-url-wire",
			WorkTypeID:  "task",
			DisplayName: "url-wire",
			TraceID:     "trace-url-wire",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "review images"},
				{Type: work.WorkContentPartTypeImage, URL: localURL},
				{Type: work.WorkContentPartTypeImage, URL: remoteURL},
			},
		}},
	}

	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time {
		return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	})
	history.RecordWorkRequest(3, record, time.Date(2026, 6, 1, 12, 0, 1, 0, time.UTC))

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}

	data, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal WORK_REQUEST: %v", err)
	}
	assertWorkRequestWireJSON(t, string(data), localURL, remoteURL, materializedPath)
	assertWorkRequestWireContentParts(t, events[0], localURL, remoteURL)
}

func assertWorkRequestWireJSON(t *testing.T, payload, localURL, remoteURL, materializedPath string) {
	t.Helper()
	for _, wantURL := range []string{localURL, remoteURL} {
		if !strings.Contains(payload, wantURL) {
			t.Fatalf("WORK_REQUEST JSON missing url %q: %s", wantURL, payload)
		}
	}
	for _, reject := range []string{
		materializedPath,
		filepath.Base(materializedPath),
		"workcontent-",
	} {
		if strings.Contains(payload, reject) {
			t.Fatalf("WORK_REQUEST JSON must not contain materialized path fragment %q: %s", reject, payload)
		}
	}
}

func assertWorkRequestWireContentParts(t *testing.T, event factoryapi.FactoryEvent, localURL, remoteURL string) {
	t.Helper()
	payloadStruct, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	if payloadStruct.Works == nil || len(*payloadStruct.Works) != 1 {
		t.Fatalf("works = %#v, want one work item", payloadStruct.Works)
	}
	work := (*payloadStruct.Works)[0]
	if work.Content == nil || len(*work.Content) != 3 {
		t.Fatalf("content count = %d, want 3 parts", len(*work.Content))
	}
	for i, wantURL := range []string{localURL, remoteURL} {
		imagePart, err := (*work.Content)[i+1].AsWorkImageContentPart()
		if err != nil {
			t.Fatalf("decode image %d: %v", i+1, err)
		}
		if string(imagePart.Url) != wantURL {
			t.Fatalf("image %d url = %q, want %q", i+1, imagePart.Url, wantURL)
		}
		if imagePart.File != nil {
			t.Fatalf("image %d file = %#v, want omitted canonical file field", i+1, imagePart.File)
		}
	}
}
