// Package wire supplies inert, focused Agent Message construction providers.
// The canonical application graph injects each component once and registers
// Startup separately; no constructor reads the journal or starts a lifecycle.
package wire

import (
	"crypto/sha256"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/admission"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// Construction aliases keep private components behind the owning Wire boundary.
// Peers consume agentmessages.Service, not these assembly capabilities.
type (
	Journal           = store.Journal
	Startup           = store.Startup
	Authority         = admission.Authority
	InputValidator    = admission.InputValidator
	Quota             = admission.Quota
	Ledger            = admission.Ledger
	EventAppender     = admission.EventAppender
	ObservationReader = admission.ObservationReader
)

func NewJournal(files agentmessages.StoreFileSystem, path string) (*Journal, error) {
	if files == nil || strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return nil, agentmessages.ErrBadRequest
	}
	return store.New(files, path), nil
}

func NewAuthority(validate workersessions.CallerValidator, read ObservationReader) (Authority, error) {
	if validate == nil || read == nil {
		return nil, agentmessages.ErrBadRequest
	}
	return admission.NewAuthorizer(validate, read), nil
}

func NewValidator(bodyBytes, defaultExpirySeconds int, redact func(recordings.RecordingRedactionRequest) (recordings.RecordingRedactionResult, error)) (InputValidator, error) {
	if bodyBytes < 1 || bodyBytes > admission.DefaultBodyBytes ||
		defaultExpirySeconds < admission.MinExpirySeconds || defaultExpirySeconds > admission.MaxExpirySeconds || redact == nil {
		return nil, agentmessages.ErrBadRequest
	}
	return admission.Validator{BodyBytes: bodyBytes, DefaultExpirySeconds: defaultExpirySeconds, Redact: redact}, nil
}

func NewQuota(sendsPerHour, threadMessages, maxHop int) (Quota, error) {
	if sendsPerHour < 1 || threadMessages < 1 || maxHop < 0 || maxHop > admission.MaxHop {
		return nil, agentmessages.ErrBadRequest
	}
	return admission.Limits{SendsPerHour: sendsPerHour, ThreadMessages: threadMessages, HopLimit: &maxHop}, nil
}

func NewService(enabled bool, authority Authority, validator InputValidator, quota Quota, ledger Ledger, now func() time.Time, newID func() string, cursorKey []byte, stream EventAppender, logger logging.Logger) (agentmessages.Service, error) {
	if authority == nil || validator == nil || quota == nil || ledger == nil || now == nil ||
		newID == nil || len(cursorKey) < sha256.Size || stream == nil || logger == nil {
		return nil, agentmessages.ErrBadRequest
	}
	return admission.NewEngine(enabled, authority, validator, quota, ledger, now, newID, cursorKey, stream, logger), nil
}

func NewStartup(enabled bool, journal *Journal, now func() time.Time, retention time.Duration, logger logging.Logger) (*Startup, error) {
	if journal == nil || now == nil || retention < 24*time.Hour || logger == nil {
		return nil, agentmessages.ErrBadRequest
	}
	return store.NewStartup(enabled, journal, now, retention, logger), nil
}
