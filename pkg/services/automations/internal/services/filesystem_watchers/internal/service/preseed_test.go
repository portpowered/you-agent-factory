package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestPreseedInputs_EmptyDirectoryIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "request", "default"), 0o755); err != nil {
		t.Fatal(err)
	}

	mf := &recordingSubmitter{}
	fw := newTestWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir)

	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatalf("PreseedInputs() error = %v, want nil for empty watched root", err)
	}
	if got := mf.submitCallCount(); got != 0 {
		t.Fatalf("submit call count = %d, want 0 for empty preseed", got)
	}
}

func TestPreseedInputs_MultiFileMarkdownSubmitsOneBatchWithAllWorks(t *testing.T) {
	dir := setupWatchDir(t)
	files := map[string]string{
		"a-first.md":  "first payload",
		"b-second.md": "second payload",
		"c-third.md":  "third payload",
	}
	for name, content := range files {
		path := filepath.Join(dir, "request", "default", name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mf := &recordingSubmitter{}
	fw := newTestWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := mf.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1 combined batch for multiple markdown files", got)
	}
	requests := mf.getWorkRequests()
	if len(requests) != 1 {
		t.Fatalf("work request count = %d, want 1", len(requests))
	}
	if len(requests[0].Works) != len(files) {
		t.Fatalf("work count = %d, want %d", len(requests[0].Works), len(files))
	}
	for _, item := range requests[0].Works {
		if item.WorkTypeID != "request" {
			t.Fatalf("work %q work type = %q, want request", item.Name, item.WorkTypeID)
		}
		want := files[item.Name+".md"]
		if want == "" {
			t.Fatalf("unexpected work name %q", item.Name)
		}
		if string(item.Payload.([]byte)) != want {
			t.Fatalf("work %q payload = %q, want %q", item.Name, string(item.Payload.([]byte)), want)
		}
	}
}

func TestPreseedInputs_SubmitterCallCountMatchesValidatedRequests(t *testing.T) {
	dir := setupWatchDir(t)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "alpha.md"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "beta.md"), []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}

	explicitBatch := work.WorkRequest{
		RequestID: "explicit-batch",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "batch-work", WorkTypeID: "request", Payload: map[string]string{"step": "one"}},
		},
	}
	data, err := json.Marshal(explicitBatch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "batch.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &recordingSubmitter{}
	fw := newTestWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := mf.submitCallCount(); got != 2 {
		t.Fatalf("submit call count = %d, want 2 (combined markdown batch + explicit batch)", got)
	}
}

func TestPreseedInputs_ValidationFailurePerformsZeroAdmissions(t *testing.T) {
	dir := setupWatchDir(t)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "good.md"), []byte("valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	invalidBatch := []byte(`{
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "bad-state", "workTypeName": "request", "state": "not-a-real-state", "payload": {"step": "one"}}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "request", "default", "bad.json"), invalidBatch, 0o644); err != nil {
		t.Fatal(err)
	}

	mf := &recordingSubmitter{}
	fw := newTestWatcher(
		dir,
		mf,
		zap.NewNop(),
		[]string{"request"},
		map[string]map[string]bool{"request": {"queued": true, "complete": true}},
		localInputFiles{},
		filepath.WalkDir,
	)
	err := fw.PreseedInputs(context.Background())
	if err == nil {
		t.Fatal("PreseedInputs() error = nil, want validation failure")
	}
	if !strings.Contains(err.Error(), "preseed validate") {
		t.Fatalf("PreseedInputs() error = %q, want preseed validation failure", err.Error())
	}
	if got := mf.submitCallCount(); got != 0 {
		t.Fatalf("submit call count = %d, want 0 after validation failure", got)
	}
}

func TestPreseedInputs_SkipsIneligibleFilesWithoutFailing(t *testing.T) {
	dir := setupWatchDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "unknown", "default"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeCases := []struct {
		path    string
		content string
	}{
		{filepath.Join(dir, "request", "default", "accepted.md"), "accepted"},
		{filepath.Join(dir, "request", "default", "draft.tmp"), "temp"},
		{filepath.Join(dir, "request", "default", "notes.txt"), "unsupported"},
		{filepath.Join(dir, "unknown", "default", "ignored.md"), "unknown work type"},
	}
	for _, tc := range writeCases {
		if err := os.WriteFile(tc.path, []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mf := &recordingSubmitter{}
	fw := newTestWatcher(dir, mf, zap.NewNop(), []string{"request"}, nil, localInputFiles{}, filepath.WalkDir)
	if err := fw.PreseedInputs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := mf.submitCallCount(); got != 1 {
		t.Fatalf("submit call count = %d, want 1 for only eligible file", got)
	}
	requests := mf.getWorkRequests()
	if len(requests) != 1 || len(requests[0].Works) != 1 {
		t.Fatalf("submitted requests = %#v, want one accepted markdown work", requests)
	}
	if string(requests[0].Works[0].Payload.([]byte)) != "accepted" {
		t.Fatalf("payload = %q, want accepted", string(requests[0].Works[0].Payload.([]byte)))
	}
}
