package kiro

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type sessionRecord struct {
	SessionID string `json:"session_id"`
}

func sessionRefFromOutput(
	stdout, stderr []byte,
	resume *providers.SessionRef,
) *providers.SessionRef {
	requested := validRequestedSessionRef(resume)
	if emitted := providerSessionFromOutput(stdout, stderr); emitted != nil {
		return emitted
	}
	return requested
}

func providerSessionFromOutput(streams ...[]byte) *providers.SessionRef {
	var latest *providers.SessionRef
	for _, stream := range streams {
		normalized := bytes.ReplaceAll(stream, []byte("\r\n"), []byte("\n"))
		for _, line := range bytes.Split(normalized, []byte("\n")) {
			var record sessionRecord
			if json.Unmarshal(bytes.TrimSpace(line), &record) != nil {
				continue
			}
			if ref := newSessionRef(record.SessionID); ref != nil {
				latest = ref
			}
		}
	}
	return latest
}

func validRequestedSessionRef(ref *providers.SessionRef) *providers.SessionRef {
	if ref == nil ||
		!isKiroProvider(ref.Provider) ||
		strings.TrimSpace(ref.Kind) != providers.SessionIDKind {
		return nil
	}
	return newSessionRef(ref.ID)
}

func isKiroProvider(provider providers.ID) bool {
	switch strings.TrimSpace(string(provider)) {
	case string(providers.IDKiro), "kiro":
		return true
	default:
		return false
	}
}

// SessionRefFromOutputForTest exposes session resolution for adapter tests.
func SessionRefFromOutputForTest(
	stdout, stderr []byte,
	resume *providers.SessionRef,
) *providers.SessionRef {
	return sessionRefFromOutput(stdout, stderr, resume)
}

func newSessionRef(sessionID string) *providers.SessionRef {
	sessionID = strings.TrimSpace(sessionID)
	parsed, err := uuid.Parse(sessionID)
	if err != nil || parsed == uuid.Nil || len(sessionID) != len(parsed.String()) ||
		!strings.EqualFold(sessionID, parsed.String()) {
		return nil
	}
	ref := providers.SessionRef{
		Provider: providers.IDKiro,
		Kind:     providers.SessionIDKind,
		ID:       parsed.String(),
	}
	return &ref
}
