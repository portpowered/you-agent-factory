//go:build functionallong

package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// WriteSeedMarkdownFile writes raw content as a .md seed file with the given
// filename (without extension). The file watcher derives the SubmitRequest Name
// from the filename, so this exercises the non-JSON submission path.
func WriteSeedMarkdownFile(t *testing.T, dir, workType, name string, content []byte) {
	t.Helper()
	inputDir := filepath.Join(dir, interfaces.InputsDir, workType, interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("WriteSeedMarkdownFile: create input dir: %v", err)
	}
	filename := fmt.Sprintf("%s.md", name)
	if err := os.WriteFile(filepath.Join(inputDir, filename), content, 0o644); err != nil {
		t.Fatalf("WriteSeedMarkdownFile: write file: %v", err)
	}
}

// WriteSeedBatchFile writes a canonical FACTORY_REQUEST_BATCH watched-file input
// into inputs/BATCH/default so functional tests exercise the public mixed-work-
// type file-watcher boundary instead of direct API or runtime helpers.
func WriteSeedBatchFile(t *testing.T, dir string, request interfaces.WorkRequest) {
	t.Helper()

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("WriteSeedBatchFile: marshal: %v", err)
	}

	inputDir := filepath.Join(dir, interfaces.InputsDir, "BATCH", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("WriteSeedBatchFile: create input dir: %v", err)
	}

	filename := request.RequestID
	if filename == "" {
		filename = fmt.Sprintf("batch-%d", seedFileCounter.Add(1))
	}
	if err := os.WriteFile(filepath.Join(inputDir, filename+".json"), data, 0o644); err != nil {
		t.Fatalf("WriteSeedBatchFile: write file: %v", err)
	}
}
