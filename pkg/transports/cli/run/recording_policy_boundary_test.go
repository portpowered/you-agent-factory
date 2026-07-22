package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTransportCannotOwnDefaultRecordingPolicyOrMutableBootstrapSeams(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob run sources: %v", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(source)
		for _, forbidden := range []string{
			"var bootstrapFactory",
			"defaultLiveRunRecordTime",
			"defaultLiveRunRecordUUID",
			"defaultpaths.RecordingsRoot",
			"defaultpaths.RecordingsDatedDir",
			"__factory_session_id__",
			"time.Now(",
			"time.Since(",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains forbidden transport-owned recording/bootstrap policy %q", file, forbidden)
			}
		}
	}
}
