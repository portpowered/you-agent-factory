// Package wire supplies inert, focused Agent Message construction providers.
// The canonical application graph injects each component once and registers
// Startup separately; no constructor reads the journal or starts a lifecycle.
package wire

import (
	"crypto/sha256"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/admission"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
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
	Engine            = admission.Engine
	ContinueOperation = admission.ContinueOperation
	SessionReader     = admission.SessionReader
	ConfiguredService = admission.Configured
)

func NewJournal(files agentmessages.StoreFileSystem, path string) (*Journal, error) {
	// An empty path defers selection to Startup.StartAt. Construction occurs
	// before invocation profile selection and never discovers ambient home.
	if files == nil || (path != "" && (strings.TrimSpace(path) == "" || !filepath.IsAbs(path))) {
		return nil, agentmessages.ErrBadRequest
	}
	return store.New(files, path), nil
}

func NewAuthority(validate workersessions.CallerValidator, read, captured ObservationReader) (Authority, error) {
	if validate == nil || read == nil || captured == nil {
		return nil, agentmessages.ErrBadRequest
	}
	return admission.NewAuthorizer(validate, read, captured), nil
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

func NewService(enabled bool, authority Authority, validator InputValidator, quota Quota, ledger Ledger, now func() time.Time, scheduler platformclock.TimerSource, newID func() string, cursorKey []byte, stream EventAppender, logger logging.Logger, continueSession ContinueOperation, readSession SessionReader) (*Engine, error) {
	if authority == nil || validator == nil || quota == nil || ledger == nil || now == nil || scheduler == nil ||
		newID == nil || len(cursorKey) < sha256.Size || stream == nil || logger == nil || continueSession == nil || readSession == nil {
		return nil, agentmessages.ErrBadRequest
	}
	return admission.NewEngine(enabled, authority, validator, quota, ledger, now, scheduler, newID, cursorKey, stream, logger, continueSession, readSession), nil
}

// NewConfiguredService supplies selected-profile lifecycle over the same Engine
// injected into Messaging's transports. It constructs no collaborators later.
func NewConfiguredService(engine *Engine, startup *Startup, load operatorsettings.ConfigLoader) (*ConfiguredService, error) {
	if engine == nil || startup == nil || load == nil {
		return nil, agentmessages.ErrBadRequest
	}
	return admission.NewConfigured(engine, startup, load)
}

func NewStartup(enabled bool, journal *Journal, now func() time.Time, retention time.Duration, logger logging.Logger) (*Startup, error) {
	if journal == nil || now == nil || retention < 24*time.Hour || logger == nil {
		return nil, agentmessages.ErrBadRequest
	}
	return store.NewStartup(enabled, journal, now, retention, logger), nil
}
