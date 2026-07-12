package factory

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

func TestCreateFromFile_WritesHumanReadableConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	var out strings.Builder
	if err := CreateFromFile(CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: &out,
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	wantDir := filepath.Join(rootDir, "gamma")
	want := "Created factory gamma\nDirectory: " + wantDir + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestCreateFromFile_SetCurrentUpdatesPointer(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	if err := CreateFromFile(CreateFromFileConfig{
		Name:       "gamma",
		From:       from,
		Dir:        rootDir,
		SetCurrent: true,
		Output:     ioDiscard(t),
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	current, err := factoryconfig.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if current != "gamma" {
		t.Fatalf("current = %q, want gamma", current)
	}
}

func TestCreateFromFile_JSONEmitsStructuredConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	var out bytes.Buffer
	if err := CreateFromFile(CreateFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		JSON:   true,
		Output: &out,
	}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}

	var result CreateFromFileResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Name != "gamma" || result.FactoryDir != filepath.Join(rootDir, "gamma") {
		t.Fatalf("result = %#v, want gamma factory directory", result)
	}
}
