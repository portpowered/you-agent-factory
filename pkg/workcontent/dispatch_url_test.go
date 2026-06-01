package workcontent_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workcontent"
)

func TestResolveDispatchContentURL_AbsoluteFileURLUnchanged(t *testing.T) {
	absPath := filepath.Join(t.TempDir(), "img.png")
	rawURL := "file://" + absPath

	got, err := workcontent.ResolveDispatchContentURL("/tmp/workspace", rawURL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != rawURL {
		t.Fatalf("url = %q, want %q", got, rawURL)
	}
}

func TestResolveDispatchContentURL_RelativeFileURLJoinsWorkingDirectory(t *testing.T) {
	workspace := t.TempDir()
	rawURL := "file://fixtures/ui.png"

	got, err := workcontent.ResolveDispatchContentURL(workspace, rawURL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := "file://" + filepath.Join(workspace, "fixtures", "ui.png")
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestResolveDispatchContentURL_RemoteURLUnchanged(t *testing.T) {
	rawURL := "https://cdn.example.com/image.png"
	got, err := workcontent.ResolveDispatchContentURL("/tmp/workspace", rawURL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != rawURL {
		t.Fatalf("url = %q, want %q", got, rawURL)
	}
}
