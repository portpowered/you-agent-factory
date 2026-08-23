package run

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func prepareCanonicalSessionIDForRun(cfg RunConfig) (RunConfig, error) {
	if !usesAutomaticRecording(cfg) || strings.TrimSpace(cfg.CanonicalSessionID) != "" {
		return cfg, nil
	}
	generator := cfg.CanonicalSessionIDGenerator
	if generator == nil {
		generator = uuid.NewString
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
