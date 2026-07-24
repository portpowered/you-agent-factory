package work_test

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
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

type publicSeamFileSystem struct {
	root     string
	writeErr error
	removed  []string
	written  map[string][]byte
}

func (f *publicSeamFileSystem) MkdirTemp(_ string, pattern string) (string, error) {
	return os.MkdirTemp(f.root, pattern)
}

func (f *publicSeamFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.written == nil {
		f.written = make(map[string][]byte)
	}
	f.written[path] = append([]byte(nil), data...)
	return os.WriteFile(path, data, mode)
}

func (f *publicSeamFileSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func (f *publicSeamFileSystem) RemoveAll(path string) error {
	f.removed = append(f.removed, path)
	return os.RemoveAll(path)
}

type publicSeamRandom struct{ value byte }

func (r publicSeamRandom) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = r.value
	}
	return len(buffer), nil
}

type publicSeamClock struct{ now time.Time }

func (c *publicSeamClock) Now() time.Time { return c.now }

func newPublicWorkRootStaging(
	t *testing.T,
	clock *publicSeamClock,
) work.ContentStagingService {
	t.Helper()
	if clock == nil {
		clock = &publicSeamClock{now: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)}
	}
	staging, err := workwire.NewContentStagingService(
		&publicSeamFileSystem{root: t.TempDir()},
		publicSeamRandom{value: 0x11},
		clock,
		time.Minute,
	)
	if err != nil {
		t.Fatalf("work wire.NewContentStagingService: %v", err)
	}
	return staging
}

func TestPublicWorkRootStagingSeamIssuesSecureStagedReference(t *testing.T) {
	filesystem := &publicSeamFileSystem{root: t.TempDir()}
	staging, err := workwire.NewContentStagingService(
		filesystem,
		publicSeamRandom{value: 0x11},
		&publicSeamClock{now: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("work wire.NewContentStagingService: %v", err)
	}

	staged, err := staging.StageContent(context.Background(), work.StageContentRequest{
		ItemType:  "image",
		FileName:  "../escape/ui.png",
		MediaType: "image/png",
		Content:   []byte("png-bytes"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef == "" || staged.URL == "" {
		t.Fatalf("stage result = %#v, want opaque staged reference and URL", staged)
	}
	resolved, err := staging.ResolveContent(context.Background(), staged.StagedFileRef)
	if err != nil {
		t.Fatalf("ResolveContent: %v", err)
	}
	if filepath.Base(resolved.Path) != "ui.png" {
		t.Fatalf("resolved path = %q, want detached base filename", resolved.Path)
	}
	if got := string(filesystem.written[resolved.Path]); got != "png-bytes" {
		t.Fatalf("written content = %q, want png-bytes", got)
	}
}

func TestPublicWorkRootStagingSeamRejectsTamperedExpiredAndMissingReferences(t *testing.T) {
	ctx := context.Background()
	validRequest := work.StageContentRequest{
		ItemType:  "image",
		FileName:  "ui.png",
		MediaType: "image/png",
		Content:   []byte("png-bytes"),
	}

	t.Run("tampered", func(t *testing.T) {
		staging := newPublicWorkRootStaging(t, nil)
		staged, err := staging.StageContent(ctx, validRequest)
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		_, err = staging.ResolveContent(ctx, staged.StagedFileRef+"tampered")
		if !errors.Is(err, work.ErrInvalidStagedContentRef) {
			t.Fatalf("ResolveContent error = %v, want ErrInvalidStagedContentRef", err)
		}
	})

	t.Run("expired removes directory", func(t *testing.T) {
		clock := &publicSeamClock{now: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)}
		staging := newPublicWorkRootStaging(t, clock)
		staged, err := staging.StageContent(ctx, validRequest)
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		resolved, err := staging.ResolveContent(ctx, staged.StagedFileRef)
		if err != nil {
			t.Fatalf("initial ResolveContent: %v", err)
		}
		stageDir := filepath.Dir(resolved.Path)
		clock.now = clock.now.Add(2 * time.Minute)
		_, err = staging.ResolveContent(ctx, staged.StagedFileRef)
		if !errors.Is(err, work.ErrStagedContentExpired) {
			t.Fatalf("expired ResolveContent error = %v, want ErrStagedContentExpired", err)
		}
		if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
			t.Fatalf("expired stage directory stat = %v, want not-exist", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		staging := newPublicWorkRootStaging(t, nil)
		staged, err := staging.StageContent(ctx, validRequest)
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		resolved, err := staging.ResolveContent(ctx, staged.StagedFileRef)
		if err != nil {
			t.Fatalf("initial ResolveContent: %v", err)
		}
		if err := os.Remove(resolved.Path); err != nil {
			t.Fatalf("remove staged file: %v", err)
		}
		_, err = staging.ResolveContent(ctx, staged.StagedFileRef)
		if !errors.Is(err, work.ErrStagedContentNotFound) {
			t.Fatalf("missing ResolveContent error = %v, want ErrStagedContentNotFound", err)
		}
	})
}

func TestPublicWorkRootStagingSeamCleansUpStagedAndPartialStages(t *testing.T) {
	ctx := context.Background()
	validRequest := work.StageContentRequest{
		ItemType:  "image",
		FileName:  "ui.png",
		MediaType: "image/png",
		Content:   []byte("png-bytes"),
	}

	t.Run("explicit cleanup removes directory", func(t *testing.T) {
		staging := newPublicWorkRootStaging(t, nil)
		staged, err := staging.StageContent(ctx, validRequest)
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		resolved, err := staging.ResolveContent(ctx, staged.StagedFileRef)
		if err != nil {
			t.Fatalf("ResolveContent: %v", err)
		}
		stageDir := filepath.Dir(resolved.Path)
		if err := staging.CleanupContent(ctx, staged.StagedFileRef); err != nil {
			t.Fatalf("CleanupContent: %v", err)
		}
		if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
			t.Fatalf("cleaned stage directory stat = %v, want not-exist", err)
		}
	})

	t.Run("partial stage cleanup on write failure", func(t *testing.T) {
		writeErr := errors.New("disk full")
		filesystem := &publicSeamFileSystem{root: t.TempDir(), writeErr: writeErr}
		staging, err := workwire.NewContentStagingService(
			filesystem,
			publicSeamRandom{value: 0x11},
			&publicSeamClock{now: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)},
			time.Minute,
		)
		if err != nil {
			t.Fatalf("work wire.NewContentStagingService: %v", err)
		}
		if _, err := staging.StageContent(ctx, validRequest); !errors.Is(err, writeErr) {
			t.Fatalf("StageContent error = %v, want write failure", err)
		}
		if len(filesystem.removed) != 1 {
			t.Fatalf("removed paths = %#v, want partial stage cleanup", filesystem.removed)
		}
		if _, err := os.Stat(filesystem.removed[0]); !os.IsNotExist(err) {
			t.Fatalf("partial stage directory stat = %v, want not-exist", err)
		}
	})
}

func TestPublicWorkRootStagingSeamPreservesPrepareAndValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("prepare mixes text and staged media", func(t *testing.T) {
		staging := newPublicWorkRootStaging(t, nil)
		image, err := staging.StageContent(ctx, work.StageContentRequest{
			ItemType:  "image",
			FileName:  "ui.png",
			MediaType: "image/png",
			Content:   []byte("png-bytes"),
		})
		if err != nil {
			t.Fatalf("StageContent image: %v", err)
		}
		document, err := staging.StageContent(ctx, work.StageContentRequest{
			ItemType:  "document",
			FileName:  "spec.pdf",
			MediaType: "application/pdf",
			Content:   []byte("pdf-bytes"),
		})
		if err != nil {
			t.Fatalf("StageContent document: %v", err)
		}

		parts, err := staging.PrepareContent(ctx, []work.StagedSubmissionItem{
			{ItemType: "text", Text: "Review this UI."},
			{
				ItemType: "image", StagedFileRef: image.StagedFileRef,
				FileName: "customer-name.png", MediaType: "image/png",
			},
			{
				ItemType: "document", StagedFileRef: document.StagedFileRef,
				FileName: "customer-spec.pdf", MediaType: "application/pdf",
			},
		})
		if err != nil {
			t.Fatalf("PrepareContent: %v", err)
		}
		if len(parts) != 3 {
			t.Fatalf("prepared parts len = %d, want 3", len(parts))
		}
		if parts[0].Type != work.WorkContentPartTypeText || parts[0].Text != "Review this UI." {
			t.Fatalf("prepared text = %#v", parts[0])
		}
		if parts[1].Type != work.WorkContentPartTypeImage || parts[1].URL != image.URL ||
			parts[1].ContentType != "image/png" {
			t.Fatalf("prepared image = %#v", parts[1])
		}
		if parts[1].Metadata["submissionItemType"] != "image" ||
			parts[1].Metadata["fileName"] != "customer-name.png" {
			t.Fatalf("prepared image metadata = %#v", parts[1].Metadata)
		}
		if parts[2].Type != work.WorkContentPartTypeBinary || parts[2].URL != document.URL ||
			parts[2].ContentType != "application/pdf" {
			t.Fatalf("prepared document = %#v", parts[2])
		}
		if parts[2].Metadata["submissionItemType"] != "document" ||
			parts[2].Metadata["fileName"] != "customer-spec.pdf" {
			t.Fatalf("prepared document metadata = %#v", parts[2].Metadata)
		}
	})

	t.Run("invalid stage requests return typed validation", func(t *testing.T) {
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
			{name: "empty payload", request: work.StageContentRequest{
				ItemType: "document", FileName: "spec.pdf", MediaType: "application/pdf",
			}, message: "non-empty file payload"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				staging := newPublicWorkRootStaging(t, nil)
				_, err := staging.StageContent(ctx, test.request)
				var validation *work.ContentStagingError
				if !errors.As(err, &validation) || !strings.Contains(validation.Message, test.message) {
					t.Fatalf("StageContent error = %v, want ContentStagingError containing %q", err, test.message)
				}
			})
		}
	})

	t.Run("prepare rejects media-type mismatch", func(t *testing.T) {
		staging := newPublicWorkRootStaging(t, nil)
		staged, err := staging.StageContent(ctx, work.StageContentRequest{
			ItemType:  "image",
			FileName:  "ui.png",
			MediaType: "image/png",
			Content:   []byte("png"),
		})
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		_, err = staging.PrepareContent(ctx, []work.StagedSubmissionItem{{
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
	})
}

// TestPublicWorkRootStagingSeamSealsContentStagingCutWithoutMaterializationOrRuntime
// seals IMP-WORK-02: the nested content_staging capability remains a staging-only
// ContentStagingService behind CTR-WORK, not the full Work root that owns
// materialization and state-access.
func TestPublicWorkRootStagingSeamSealsContentStagingCutWithoutMaterializationOrRuntime(t *testing.T) {
	ctx := context.Background()
	staging := newPublicWorkRootStaging(t, nil)

	if _, ok := any(staging).(work.Service); ok {
		t.Fatal("content_staging must not satisfy work.Service (materialization/state-access stay out of this cut)")
	}

	staged, err := staging.StageContent(ctx, work.StageContentRequest{
		ItemType:  "image",
		FileName:  "seal.png",
		MediaType: "image/png",
		Content:   []byte("seal-bytes"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef == "" || staged.URL == "" {
		t.Fatalf("stage result = %#v, want opaque staged reference", staged)
	}

	resolved, err := staging.ResolveContent(ctx, staged.StagedFileRef)
	if err != nil {
		t.Fatalf("ResolveContent: %v", err)
	}
	if resolved.URL != staged.URL {
		t.Fatalf("resolved URL = %q, want %q", resolved.URL, staged.URL)
	}

	parts, err := staging.PrepareContent(ctx, []work.StagedSubmissionItem{{
		ItemType: "text", Text: "seal prepare",
	}, {
		ItemType: "image", StagedFileRef: staged.StagedFileRef,
		FileName: "seal.png", MediaType: "image/png",
	}})
	if err != nil {
		t.Fatalf("PrepareContent: %v", err)
	}
	if len(parts) != 2 || parts[0].Text != "seal prepare" || parts[1].URL != staged.URL {
		t.Fatalf("prepared parts = %#v", parts)
	}

	if _, err := staging.ResolveContent(ctx, staged.StagedFileRef+"tampered"); !errors.Is(err, work.ErrInvalidStagedContentRef) {
		t.Fatalf("tampered resolve error = %v, want ErrInvalidStagedContentRef", err)
	}

	if err := staging.CleanupContent(ctx, staged.StagedFileRef); err != nil {
		t.Fatalf("CleanupContent: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(resolved.Path)); !os.IsNotExist(err) {
		t.Fatalf("stage directory still present after cleanup: %v", err)
	}
}
