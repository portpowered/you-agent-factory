package content_test

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/work/content"
)

func TestFilesystemPathToContentURL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	absPath := filepath.Join(dir, "img.png")
	absURLPath := filepath.ToSlash(absPath)
	if volume := filepath.VolumeName(absPath); volume != "" && !strings.HasPrefix(absURLPath, "/") {
		absURLPath = "/" + absURLPath
	}
	wantAbsoluteURL := (&url.URL{Scheme: "file", Path: absURLPath}).String()

	tests := []struct {
		name    string
		path    string
		wantURL string
	}{
		{name: "relative", path: "fixtures/ui.png", wantURL: "file://fixtures/ui.png"},
		{name: "absolute", path: absPath, wantURL: wantAbsoluteURL},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := content.FilesystemPathToContentURL(tc.path)
			if err != nil {
				t.Fatalf("FilesystemPathToContentURL: %v", err)
			}
			if got != tc.wantURL {
				t.Fatalf("url = %q, want %q", got, tc.wantURL)
			}
		})
	}
}

func TestNormalizeFileBackedContentPart_LegacyFileOnly(t *testing.T) {
	t.Parallel()

	part, err := content.NormalizeFileBackedContentPart(work.WorkContentPart{
		Type: work.WorkContentPartTypeImage,
		File: "fixtures/ui.png",
	})
	if err != nil {
		t.Fatalf("NormalizeFileBackedContentPart: %v", err)
	}
	if part.URL != "file://fixtures/ui.png" {
		t.Fatalf("url = %q, want file://fixtures/ui.png", part.URL)
	}
	if part.File != "" {
		t.Fatalf("file = %q, want empty canonical file field", part.File)
	}
}

func TestNormalizeFileBackedContentPart_URLAndFileConflict(t *testing.T) {
	t.Parallel()

	_, err := content.NormalizeFileBackedContentPart(work.WorkContentPart{
		Type: work.WorkContentPartTypeAudio,
		URL:  "file://voice.wav",
		File: "voice.wav",
	})
	if err == nil || !strings.Contains(err.Error(), "url and file cannot both be set") {
		t.Fatalf("error = %v, want url/file conflict", err)
	}
}
