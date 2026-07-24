package service_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	contentstaging "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_staging"
	contentstagingwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_staging/wire"
)

type testFileSystem struct {
	root        string
	mkdirErr    error
	writeErr    error
	statErr     error
	removeErr   error
	removed     []string
	written     map[string][]byte
	writtenMode fs.FileMode
}

func (f *testFileSystem) MkdirTemp(_ string, pattern string) (string, error) {
	if f.mkdirErr != nil {
		return "", f.mkdirErr
	}
	return os.MkdirTemp(f.root, pattern)
}

func (f *testFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.written == nil {
		f.written = make(map[string][]byte)
	}
	f.written[path] = append([]byte(nil), data...)
	f.writtenMode = mode
	return os.WriteFile(path, data, mode)
}

func (f *testFileSystem) Stat(path string) (fs.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return os.Stat(path)
}

func (f *testFileSystem) RemoveAll(path string) error {
	f.removed = append(f.removed, path)
	if f.removeErr != nil {
		return f.removeErr
	}
	return os.RemoveAll(path)
}

type testRandom struct {
	err   error
	value byte
}

func (r testRandom) Read(buffer []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	for index := range buffer {
		buffer[index] = r.value
	}
	return len(buffer), nil
}

type testClock struct {
	now time.Time
}

func (c *testClock) Now() time.Time {
	return c.now
}

func newNestedContentStagingForTest(
	t *testing.T,
) (contentstaging.Service, *testFileSystem, *testClock) {
	t.Helper()
	filesystem := &testFileSystem{root: t.TempDir()}
	clock := &testClock{now: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)}
	service, err := contentstagingwire.NewService(
		filesystem,
		testRandom{value: 0x2a},
		clock,
		time.Minute,
	)
	if err != nil {
		t.Fatalf("content_staging wire.NewService: %v", err)
	}
	return service, filesystem, clock
}

func TestNestedContentStagingIssuesSecureStagedReference(t *testing.T) {
	service, filesystem, _ := newNestedContentStagingForTest(t)
	var _ contentstaging.Service = service
	var _ work.ContentStagingService = service

	staged, err := service.StageContent(context.Background(), work.StageContentRequest{
		ItemType:  "image",
		FileName:  "../ui.png",
		MediaType: "image/png",
		Content:   []byte("png-bytes"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef == "" || staged.URL == "" {
		t.Fatalf("stage result = %#v, want opaque staged reference and URL", staged)
	}
	if staged.FileName != "../ui.png" || staged.MediaType != "image/png" {
		t.Fatalf("stage identity fields = %#v", staged)
	}

	resolved, err := service.ResolveContent(context.Background(), staged.StagedFileRef)
	if err != nil {
		t.Fatalf("ResolveContent: %v", err)
	}
	if filepath.Base(resolved.Path) != "ui.png" {
		t.Fatalf("resolved path = %q, want detached base filename", resolved.Path)
	}
	if resolved.URL != staged.URL {
		t.Fatalf("resolved URL = %q, want stage URL %q", resolved.URL, staged.URL)
	}
	if got := string(filesystem.written[resolved.Path]); got != "png-bytes" {
		t.Fatalf("written content = %q, want png-bytes", got)
	}
	if filesystem.writtenMode != 0o600 {
		t.Fatalf("write mode = %#o, want 0600", filesystem.writtenMode)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestContentStagingOwnsPersistenceSignedResolutionAndCleanup(t *testing.T) {
	service, filesystem, _ := newNestedContentStagingForTest(t)
	ctx := context.Background()

	staged, err := service.StageContent(ctx, work.StageContentRequest{
		ItemType:  "image",
		FileName:  "../ui.png",
		MediaType: "image/png",
		Content:   []byte("png-bytes"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef == "" || staged.URL == "" {
		t.Fatalf("stage result = %#v, want signed reference and URL", staged)
	}
	resolved, err := service.ResolveContent(ctx, staged.StagedFileRef)
	if err != nil {
		t.Fatalf("ResolveContent: %v", err)
	}
	if filepath.Base(resolved.Path) != "ui.png" {
		t.Fatalf("resolved path = %q, want detached base filename", resolved.Path)
	}
	if resolved.URL != staged.URL {
		t.Fatalf("resolved URL = %q, want stage URL %q", resolved.URL, staged.URL)
	}
	if got := string(filesystem.written[resolved.Path]); got != "png-bytes" {
		t.Fatalf("written content = %q, want png-bytes", got)
	}
	if filesystem.writtenMode != 0o600 {
		t.Fatalf("write mode = %#o, want 0600", filesystem.writtenMode)
	}

	parts, err := service.PrepareContent(ctx, []work.StagedSubmissionItem{
		{ItemType: "text", Text: "Review this UI."},
		{
			ItemType: "image", StagedFileRef: staged.StagedFileRef,
			FileName: "customer-name.png", MediaType: "image/png",
		},
	})
	if err != nil {
		t.Fatalf("PrepareContent: %v", err)
	}
	if len(parts) != 2 || parts[0].Type != work.WorkContentPartTypeText ||
		parts[0].Text != "Review this UI." {
		t.Fatalf("prepared text = %#v", parts)
	}
	if parts[1].Type != work.WorkContentPartTypeImage || parts[1].URL != staged.URL ||
		parts[1].ContentType != "image/png" {
		t.Fatalf("prepared staged content = %#v", parts[1])
	}
	if parts[1].Metadata["submissionItemType"] != "image" ||
		parts[1].Metadata["fileName"] != "customer-name.png" {
		t.Fatalf("prepared metadata = %#v", parts[1].Metadata)
	}

	if err := service.CleanupContent(ctx, staged.StagedFileRef); err != nil {
		t.Fatalf("CleanupContent: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(resolved.Path)); !os.IsNotExist(err) {
		t.Fatalf("cleaned stage directory stat error = %v, want not-exist", err)
	}
}

func TestContentStagingRejectsTamperedExpiredAndMissingReferences(t *testing.T) {
	ctx := context.Background()

	t.Run("tampered", func(t *testing.T) {
		service, _, _ := newNestedContentStagingForTest(t)
		staged, err := service.StageContent(ctx, validStageContentRequest())
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		_, err = service.ResolveContent(ctx, staged.StagedFileRef+"tampered")
		if !errors.Is(err, work.ErrInvalidStagedContentRef) {
			t.Fatalf("ResolveContent error = %v, want invalid reference", err)
		}
	})

	t.Run("expired removes directory", func(t *testing.T) {
		service, filesystem, clock := newNestedContentStagingForTest(t)
		staged, err := service.StageContent(ctx, validStageContentRequest())
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		resolved, err := service.ResolveContent(ctx, staged.StagedFileRef)
		if err != nil {
			t.Fatalf("initial ResolveContent: %v", err)
		}
		clock.now = clock.now.Add(2 * time.Minute)
		_, err = service.ResolveContent(ctx, staged.StagedFileRef)
		if !errors.Is(err, work.ErrStagedContentExpired) {
			t.Fatalf("expired ResolveContent error = %v", err)
		}
		if len(filesystem.removed) == 0 ||
			filesystem.removed[len(filesystem.removed)-1] != filepath.Dir(resolved.Path) {
			t.Fatalf("removed paths = %#v, want expired stage directory", filesystem.removed)
		}
	})

	t.Run("missing", func(t *testing.T) {
		service, _, _ := newNestedContentStagingForTest(t)
		staged, err := service.StageContent(ctx, validStageContentRequest())
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		resolved, err := service.ResolveContent(ctx, staged.StagedFileRef)
		if err != nil {
			t.Fatalf("initial ResolveContent: %v", err)
		}
		if err := os.Remove(resolved.Path); err != nil {
			t.Fatalf("remove staged file: %v", err)
		}
		_, err = service.ResolveContent(ctx, staged.StagedFileRef)
		if !errors.Is(err, work.ErrStagedContentNotFound) {
			t.Fatalf("missing ResolveContent error = %v", err)
		}
	})
}

func TestContentStagingValidatesOwnedFilePolicy(t *testing.T) {
	tests := []struct {
		name    string
		request work.StageContentRequest
		message string
	}{
		{name: "text type", request: work.StageContentRequest{
			ItemType: "text", FileName: "notes.txt", MediaType: "text/plain", Content: []byte("x"),
		}, message: "itemType must be one of"},
		{name: "blank filename", request: work.StageContentRequest{
			ItemType: "document", FileName: "", MediaType: "application/pdf", Content: []byte("x"),
		}, message: "fileName must identify a file"},
		{name: "blank media", request: work.StageContentRequest{
			ItemType: "document", FileName: "spec.pdf", Content: []byte("x"),
		}, message: "mediaType must be a non-empty string"},
		{name: "image media", request: work.StageContentRequest{
			ItemType: "image", FileName: "ui.png", MediaType: "application/octet-stream", Content: []byte("x"),
		}, message: "mediaType must start with image/"},
		{name: "video media", request: work.StageContentRequest{
			ItemType: "video", FileName: "ui.mp4", MediaType: "application/octet-stream", Content: []byte("x"),
		}, message: "mediaType must start with video/"},
		{name: "audio media", request: work.StageContentRequest{
			ItemType: "audio", FileName: "ui.wav", MediaType: "application/octet-stream", Content: []byte("x"),
		}, message: "mediaType must start with audio/"},
		{name: "empty payload", request: work.StageContentRequest{
			ItemType: "document", FileName: "spec.pdf", MediaType: "application/pdf",
		}, message: "non-empty file payload"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _, _ := newNestedContentStagingForTest(t)
			_, err := service.StageContent(context.Background(), test.request)
			var validation *work.ContentStagingError
			if !errors.As(err, &validation) || !strings.Contains(validation.Message, test.message) {
				t.Fatalf("StageContent error = %v, want ContentStagingError containing %q", err, test.message)
			}
		})
	}
}

func TestContentStagingPrepareOwnsSubmissionMediaPolicy(t *testing.T) {
	service, _, _ := newNestedContentStagingForTest(t)
	staged, err := service.StageContent(context.Background(), work.StageContentRequest{
		ItemType:  "image",
		FileName:  "ui.png",
		MediaType: "image/png",
		Content:   []byte("png"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	_, err = service.PrepareContent(context.Background(), []work.StagedSubmissionItem{{
		ItemType:      "image",
		StagedFileRef: staged.StagedFileRef,
		FileName:      "ui.png",
		MediaType:     "audio/wav",
	}})
	var validation *work.ContentStagingError
	if !errors.As(err, &validation) ||
		!strings.Contains(validation.Message, "mediaType must start with image/") {
		t.Fatalf("PrepareContent error = %v, want typed image media validation", err)
	}
}

func TestContentStagingReportsInjectedEffectFailuresAndCleansPartialStage(t *testing.T) {
	entropyErr := errors.New("entropy unavailable")
	if _, err := contentstagingwire.NewService(
		&testFileSystem{root: t.TempDir()},
		testRandom{err: entropyErr},
		&testClock{now: time.Now()},
		time.Minute,
	); !errors.Is(err, entropyErr) {
		t.Fatalf("constructor error = %v, want entropy failure", err)
	}

	writeErr := errors.New("disk full")
	filesystem := &testFileSystem{root: t.TempDir(), writeErr: writeErr}
	service, err := contentstagingwire.NewService(
		filesystem,
		testRandom{value: 1},
		&testClock{now: time.Now()},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("content_staging wire.NewService: %v", err)
	}
	if _, err := service.StageContent(context.Background(), validStageContentRequest()); !errors.Is(err, writeErr) {
		t.Fatalf("StageContent error = %v, want write failure", err)
	}
	if len(filesystem.removed) != 1 {
		t.Fatalf("removed paths = %#v, want partial stage cleanup", filesystem.removed)
	}

	statErr := errors.New("stat unavailable")
	filesystem.statErr = statErr
	filesystem.writeErr = nil
	staged, err := service.StageContent(context.Background(), validStageContentRequest())
	if err != nil {
		t.Fatalf("StageContent after write recovery: %v", err)
	}
	if _, err := service.ResolveContent(context.Background(), staged.StagedFileRef); !errors.Is(err, work.ErrStagedContentNotFound) {
		t.Fatalf("ResolveContent stat error = %v, want customer-safe missing result", err)
	}
}

func validStageContentRequest() work.StageContentRequest {
	return work.StageContentRequest{
		ItemType:  "document",
		FileName:  "spec.pdf",
		MediaType: "application/pdf",
		Content:   []byte("pdf"),
	}
}

// TestNestedContentStagingSealsStagingOnlySurface seals IMP-WORK-02 nested
// ownership: content_staging remains a ContentStagingService and does not
// grow into the full Work root that owns materialization and state-access.
func TestNestedContentStagingSealsStagingOnlySurface(t *testing.T) {
	service, _, _ := newNestedContentStagingForTest(t)
	var _ contentstaging.Service = service
	var _ work.ContentStagingService = service
	if _, ok := any(service).(work.Service); ok {
		t.Fatal("content_staging must not satisfy work.Service (materialization/state-access stay out of this cut)")
	}

	ctx := context.Background()
	staged, err := service.StageContent(ctx, work.StageContentRequest{
		ItemType:  "image",
		FileName:  "seal.png",
		MediaType: "image/png",
		Content:   []byte("seal-bytes"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef == "" {
		t.Fatal("expected opaque staged reference")
	}
	if _, err := service.ResolveContent(ctx, staged.StagedFileRef+"tampered"); !errors.Is(err, work.ErrInvalidStagedContentRef) {
		t.Fatalf("tampered resolve error = %v, want ErrInvalidStagedContentRef", err)
	}
	if err := service.CleanupContent(ctx, staged.StagedFileRef); err != nil {
		t.Fatalf("CleanupContent: %v", err)
	}
}
