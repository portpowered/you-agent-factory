package run

import (
	"errors"
	"fmt"
	"strings"
)

// prepareCanonicalSessionIDForRun allocates the identity used by both the
// automatic recording target and the opened runtime. Keeping allocation at
// the CLI boundary means the recording path can be reserved before runtime
// construction without deriving a second identity later.
func prepareCanonicalSessionIDForRun(cfg RunConfig) (RunConfig, error) {
	if !usesAutomaticRecording(cfg) || strings.TrimSpace(cfg.CanonicalSessionID) != "" {
		return cfg, nil
	}
	generator := cfg.CanonicalSessionIDGenerator
	if generator == nil {
		return RunConfig{}, errors.New("prepare automatic recording: canonical Factory Session ID generator is required")
	}
	canonicalID := strings.TrimSpace(generator())
	if canonicalID == "" {
		return RunConfig{}, fmt.Errorf("canonical Factory Session ID generator returned an empty identity")
	}
	cfg.CanonicalSessionID = canonicalID
	return cfg, nil
}

func usesAutomaticRecording(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.RecordPath) == "" &&
		!cfg.DisableDefaultRecording &&
		strings.TrimSpace(cfg.ReplayPath) == ""
}
