package contentstaging

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestProductionAdaptersPerformExactEffects(t *testing.T) {
	filesystem := FileSystem{}
	stageDir, err := filesystem.MkdirTemp(t.TempDir(), "stage-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	path := filepath.Join(stageDir, "content.bin")
	if err := filesystem.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := filesystem.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.IsDir() || info.Size() != int64(len("content")) {
		t.Fatalf("staged info = %#v, want regular content file", info)
	}
	if err := filesystem.RemoveAll(stageDir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
		t.Fatalf("removed stage directory stat error = %v, want not-exist", err)
	}

	buffer := make([]byte, 32)
	if n, err := (Random{}).Read(buffer); err != nil || n != len(buffer) {
		t.Fatalf("Random.Read = (%d, %v), want (%d, nil)", n, err, len(buffer))
	}
	if bytes.Equal(buffer, make([]byte, len(buffer))) {
		t.Fatal("Random.Read returned an all-zero signing key")
	}
}
