package kiro

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

const (
	providerIdentity    = "kiro"
	providerSessionKind = "session_id"
)

type sessionRecord struct {
	SessionID string `json:"session_id"`
}

// responseFromOutput preserves Kiro's final-only stdout and attaches only an
// explicit, canonical Kiro session. Free-form model text is never interpreted
// as provider-session metadata.
func responseFromOutput(
	stdout, stderr []byte,
	requested *inference.ProviderSession,
) inference.Response {
	session := validRequestedSession(requested)
	if emitted := providerSessionFromOutput(stdout, stderr); emitted != nil {
		session = emitted
	}
	return inference.NewResponse(inference.ResponseInput{
		Content:         string(stdout),
		ProviderSession: session,
	})
}

func providerSessionFromOutput(streams ...[]byte) *inference.ProviderSession {
	var latest *inference.ProviderSession
	for _, stream := range streams {
		normalized := bytes.ReplaceAll(stream, []byte("\r\n"), []byte("\n"))
		for _, line := range bytes.Split(normalized, []byte("\n")) {
			var record sessionRecord
			if json.Unmarshal(bytes.TrimSpace(line), &record) != nil {
				continue
			}
			if session := newProviderSession(record.SessionID); session != nil {
				latest = session
			}
		}
	}
	return latest
}

func validRequestedSession(session *inference.ProviderSession) *inference.ProviderSession {
	if session == nil ||
		!isKiroProvider(session.Provider()) ||
		strings.TrimSpace(session.Kind()) != providerSessionKind {
		return nil
	}
	return newProviderSession(session.ID())
}

func isKiroProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	return strings.EqualFold(provider, providerIdentity) ||
		strings.EqualFold(provider, string(modelprovider.ProviderKiro))
}

func newProviderSession(sessionID string) *inference.ProviderSession {
	sessionID = strings.TrimSpace(sessionID)
	parsed, err := uuid.Parse(sessionID)
	if err != nil || parsed == uuid.Nil || len(sessionID) != len(parsed.String()) ||
		!strings.EqualFold(sessionID, parsed.String()) {
		return nil
	}
	session := inference.NewProviderSession(
		providerIdentity,
		providerSessionKind,
		parsed.String(),
		nil,
	)
	return &session
}
