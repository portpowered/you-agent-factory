package inference

import (
	"errors"
	"testing"
)

func TestTargetErrorPreservesCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("provider failed")
	err := &TargetError{ModelName: "model-a", WorkerName: "worker-a", Operation: "TTS", Cause: cause}
	if err.Error() != cause.Error() || !errors.Is(err, cause) {
		t.Fatalf("TargetError = %v, want preserved cause", err)
	}
}

func TestNilTargetErrorIsSafe(t *testing.T) {
	t.Parallel()

	var err *TargetError
	if err.Error() != "" || err.Unwrap() != nil {
		t.Fatalf("nil TargetError = (%q, %v), want empty and nil", err.Error(), err.Unwrap())
	}
	if got := (&TargetError{}).Error(); got != "" {
		t.Fatalf("empty TargetError.Error() = %q, want empty", got)
	}
}
