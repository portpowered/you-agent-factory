package automations_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations/service"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type diskFilesystemInputReader struct{}

func (diskFilesystemInputReader) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (diskFilesystemInputReader) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (diskFilesystemInputReader) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func newFilesystemWatcherBoundaryService() *automationservice.Service {
	return automationservice.New(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		"factory/main",
		"",
		nil,
		nil,
		nil,
	)
}

func setupFilesystemWatcherBoundaryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "request", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newFilesystemWatcherForBoundary(
	t *testing.T,
	svc *automationservice.Service,
	dir string,
	submitter automations.WorkRequestSubmitter,
) automations.FilesystemWatcher {
	t.Helper()
	watcher := svc.NewFilesystemWatcher(automations.FilesystemWatcherConfig{
		Dir:            dir,
		Logger:         zap.NewNop(),
		KnownWorkTypes: []string{"request"},
		Files:          diskFilesystemInputReader{},
		WalkDirectory:  filepath.WalkDir,
		WorkRequestIDs: func() string { return "generated-request-id" },
		Submitter:      submitter,
	})
	if watcher == nil {
		t.Fatal("NewFilesystemWatcher returned nil")
	}
	return watcher
}

// TestFilesystemWatcherPreseed_HandsWorkRootJSONBatchToAutomationsSubmitter proves
// filesystem watcher JSON batch parsing uses Work root helpers and hands a
// work.WorkRequest to the Automations WorkRequestSubmitter contract.
func TestFilesystemWatcherPreseed_HandsWorkRootJSONBatchToAutomationsSubmitter(t *testing.T) {
	t.Parallel()

	dir := setupFilesystemWatcherBoundaryDir(t)
	batch := work.WorkRequest{
		RequestID: "request-batch-boundary",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{
				Name:       "first",
				WorkTypeID: "request",
				TraceID:    "trace-boundary",
				Payload:    map[string]string{"step": "first"},
			},
			{
				Name:       "second",
				WorkTypeID: "request",
				Payload:    map[string]string{"step": "second"},
			},
		},
		Relations: []work.WorkRelation{
			{
				Type:           work.WorkRelationDependsOn,
				SourceWorkName: "second",
				TargetWorkName: "first",
				RequiredState:  "complete",
			},
		},
	}
	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := newFilesystemWatcherBoundaryService()
	var submitCalls int
	var submitted work.WorkRequest
	submitter := automations.WorkRequestSubmitter(func(_ context.Context, request work.WorkRequest) error {
		submitCalls++
		submitted = request
		return nil
	})
	watcher := newFilesystemWatcherForBoundary(t, svc, dir, submitter)

	if err := watcher.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("PreseedInputs: %v", err)
	}
	if submitCalls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitCalls)
	}
	if submitted.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", submitted.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if submitted.RequestID != batch.RequestID {
		t.Fatalf("request ID = %q, want %q", submitted.RequestID, batch.RequestID)
	}
	if len(submitted.Works) != 2 {
		t.Fatalf("works count = %d, want 2", len(submitted.Works))
	}
	if submitted.Works[0].WorkTypeID != "request" || submitted.Works[1].WorkTypeID != "request" {
		t.Fatalf("work types = %#v, want request for both works", submitted.Works)
	}
	if submitted.Works[0].TraceID != "trace-boundary" {
		t.Fatalf("trace ID = %q, want trace-boundary", submitted.Works[0].TraceID)
	}
	if len(submitted.Relations) != 1 {
		t.Fatalf("relations count = %d, want 1", len(submitted.Relations))
	}
	if submitted.Relations[0].Type != work.WorkRelationDependsOn {
		t.Fatalf("relation type = %q, want %q", submitted.Relations[0].Type, work.WorkRelationDependsOn)
	}
	if submitted.Relations[0].TargetWorkName != "first" {
		t.Fatalf("relation target = %q, want first", submitted.Relations[0].TargetWorkName)
	}
}

// TestFilesystemWatcherPreseed_HandsWorkRootMarkdownBatchToAutomationsSubmitter proves
// single-file markdown admission wraps content into a Work root WorkRequest batch
// before submitter handoff.
func TestFilesystemWatcherPreseed_HandsWorkRootMarkdownBatchToAutomationsSubmitter(t *testing.T) {
	t.Parallel()

	dir := setupFilesystemWatcherBoundaryDir(t)
	content := []byte("# boundary markdown work")
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "story.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := newFilesystemWatcherBoundaryService()
	var submitted work.WorkRequest
	submitter := automations.WorkRequestSubmitter(func(_ context.Context, request work.WorkRequest) error {
		submitted = request
		return nil
	})
	watcher := newFilesystemWatcherForBoundary(t, svc, dir, submitter)

	if err := watcher.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("PreseedInputs: %v", err)
	}
	if submitted.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", submitted.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if len(submitted.Works) != 1 {
		t.Fatalf("works count = %d, want 1", len(submitted.Works))
	}
	workItem := submitted.Works[0]
	if workItem.Name != "story" {
		t.Fatalf("work name = %q, want story", workItem.Name)
	}
	if workItem.WorkTypeID != "request" {
		t.Fatalf("work type = %q, want request", workItem.WorkTypeID)
	}
	payloadBytes, ok := workItem.Payload.([]byte)
	if !ok {
		t.Fatalf("payload type = %T, want []byte", workItem.Payload)
	}
	if string(payloadBytes) != string(content) {
		t.Fatalf("payload = %q, want %q", payloadBytes, content)
	}
}

// TestFilesystemWatcherPreseed_PreservesDeterministicWorkRootIdentityForEquivalentInputs
// proves equivalent watched inputs still produce the same observable Work Request
// identity fields before submitter handoff.
func TestFilesystemWatcherPreseed_PreservesDeterministicWorkRootIdentityForEquivalentInputs(t *testing.T) {
	t.Parallel()

	capture := func(t *testing.T) work.WorkRequest {
		t.Helper()
		dir := setupFilesystemWatcherBoundaryDir(t)
		batch := work.WorkRequest{
			RequestID: "request-equivalence",
			Type:      work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.Work{
				{Name: "alpha", WorkTypeID: "request", Payload: map[string]string{"step": "one"}},
			},
		}
		data, err := json.Marshal(batch)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		svc := newFilesystemWatcherBoundaryService()
		var submitted work.WorkRequest
		submitter := automations.WorkRequestSubmitter(func(_ context.Context, request work.WorkRequest) error {
			submitted = request
			return nil
		})
		watcher := newFilesystemWatcherForBoundary(t, svc, dir, submitter)
		if err := watcher.PreseedInputs(context.Background()); err != nil {
			t.Fatalf("PreseedInputs: %v", err)
		}
		return submitted
	}

	first := capture(t)
	second := capture(t)
	if first.RequestID != second.RequestID {
		t.Fatalf("request ID changed: first=%q second=%q", first.RequestID, second.RequestID)
	}
	if len(first.Works) != 1 || len(second.Works) != 1 {
		t.Fatal("expected one work item per batch request")
	}
	if first.Works[0].Name != second.Works[0].Name {
		t.Fatalf("work name changed: first=%q second=%q", first.Works[0].Name, second.Works[0].Name)
	}
	if first.Works[0].WorkTypeID != second.Works[0].WorkTypeID {
		t.Fatalf("work type changed: first=%q second=%q", first.Works[0].WorkTypeID, second.Works[0].WorkTypeID)
	}
}
