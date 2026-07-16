package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndReadFileReplaceSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "run.replay.json")
	for _, want := range []string{"first", "replacement"} {
		if err := WriteFile(path, []byte(want)); err != nil {
			t.Fatalf("WriteFile(%q): %v", want, err)
		}
		got, err := ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(): %v", err)
		}
		if string(got) != want {
			t.Fatalf("ReadFile() = %q, want %q", got, want)
		}
	}
}

func TestWriteAndReadFileFailuresAreActionable(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "parent-file")
	if err := os.WriteFile(parentFile, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}
	path := filepath.Join(parentFile, "run.replay.json")
	if err := WriteFile(path, []byte("{}")); err == nil || !strings.Contains(err.Error(), "create replay artifact directory") {
		t.Fatalf("WriteFile() error = %v, want directory context", err)
	}
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("ReadFile() error = nil, want missing-file error")
	}
}

func TestNormalizeHistoricalFailureDetails(t *testing.T) {
	input := []byte(`{"events":[{"payload":{"failureReason":"timeout","failureMessage":"provider timed out"}},{"payload":{"failureDetail":{"reason":"throttled","message":"retry later"},"failureReason":"ignored"}}]}`)
	got, err := NormalizeHistoricalFailureDetails(input)
	if err != nil {
		t.Fatalf("NormalizeHistoricalFailureDetails(): %v", err)
	}
	var decoded struct {
		Events []struct {
			Payload map[string]any `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal normalized payload: %v", err)
	}
	first := decoded.Events[0].Payload["failureDetail"].(map[string]any)
	if first["reason"] != "timeout" || first["message"] != "provider timed out" {
		t.Fatalf("translated failureDetail = %#v", first)
	}
	second := decoded.Events[1].Payload["failureDetail"].(map[string]any)
	if second["reason"] != "throttled" || second["message"] != "retry later" {
		t.Fatalf("preserved failureDetail = %#v", second)
	}
	for _, event := range decoded.Events {
		if _, exists := event.Payload["failureReason"]; exists {
			t.Fatalf("legacy failureReason retained: %#v", event.Payload)
		}
	}
}

func TestNormalizeHistoricalFailureDetailsRejectsInvalidJSON(t *testing.T) {
	if _, err := NormalizeHistoricalFailureDetails([]byte("{")); err == nil {
		t.Fatal("NormalizeHistoricalFailureDetails() error = nil, want malformed JSON error")
	}
}
