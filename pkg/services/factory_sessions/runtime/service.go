// Package runtime owns mutable live Factory Session registration and lookup.
// Runtime engines remain opaque handles; this service controls their session
// identity, selection, and cleanup without importing a process host.
package runtime

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
)

// Registration contains the host-independent state needed to register one
// live Factory Session runtime.
type Registration struct {
	SessionID            string
	FactoryDir           string
	FolderPath           string
	ExecutionBaseDir     string
	Target               factorysessions.TargetRef
	Handle               any
	Runtime              *factorysessions.LiveRuntime
	Default              bool
	Project              string
	Select               bool
	AllocateDefaultID    bool
	AddEventTypeRecorder func(func(interfaces.FactoryEventType))
}

// RegistrationInput contains raw runtime and target paths used to normalize a
// host-independent live-session registration.
type RegistrationInput struct {
	FactoryRootDir string
	BundleDir      string
	BundleFolder   string
	RuntimeBaseDir string
	Target         factorysessions.Target
	PreparedSpec   any
}

// RegistrationMetadata is the normalized path and target portion of a live
// session registration.
type RegistrationMetadata struct {
	FactoryDir       string
	FolderPath       string
	ExecutionBaseDir string
	Target           factorysessions.TargetRef
	Project          string
	PreparedSpec     any
}

// NormalizeRegistration applies the canonical live-session path and target defaults.
func NormalizeRegistration(input RegistrationInput) RegistrationMetadata {
	metadata := RegistrationMetadata{
		FactoryDir:   strings.TrimSpace(input.Target.FactoryDir),
		FolderPath:   strings.TrimSpace(input.Target.FolderPath),
		Target:       input.Target.Ref,
		Project:      strings.TrimSpace(input.Target.Project),
		PreparedSpec: input.PreparedSpec,
	}
	if metadata.FactoryDir == "" {
		metadata.FactoryDir = strings.TrimSpace(input.BundleDir)
	}
	if metadata.FolderPath == "" {
		metadata.FolderPath = strings.TrimSpace(input.FactoryRootDir)
	}
	if metadata.FolderPath == "" {
		metadata.FolderPath = strings.TrimSpace(input.BundleFolder)
	}
	if metadata.FolderPath == "" {
		metadata.FolderPath = metadata.FactoryDir
	}
	if metadata.Target.Kind == "" {
		metadata.Target = factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}
	}
	if metadata.Project == "" {
		metadata.Project = filepath.Base(metadata.FolderPath)
	}
	metadata.ExecutionBaseDir = strings.TrimSpace(input.RuntimeBaseDir)
	if metadata.ExecutionBaseDir == "" {
		metadata.ExecutionBaseDir = metadata.FolderPath
	}
	if metadata.ExecutionBaseDir == "" {
		metadata.ExecutionBaseDir = metadata.FactoryDir
	}
	return metadata
}

// DefaultTarget returns the canonical default-session target for one runtime bundle.
func DefaultTarget(bundleDir, bundleFolder, factoryRootDir string) factorysessions.Target {
	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		FactoryDir: bundleDir, FolderPath: bundleFolder,
	}
	if strings.TrimSpace(target.FolderPath) == "" {
		target.FolderPath = factoryRootDir
	}
	if strings.TrimSpace(target.Project) == "" && target.FolderPath != "" {
		target.Project = filepath.Base(target.FolderPath)
	}
	return target
}

// Service is the authoritative mutable registry for one process's live
// Factory Sessions.
type Service struct {
	registry   factorysessions.Registry
	responses  *responsestream.Registry
	close      func(*factorysessions.LiveSession)
	clock      factoryruntime.Clock
	eventIDs   factorysessions.ResponseEventIDGenerator
	sessionIDs factorysessions.SessionIDGenerator
	activation sync.RWMutex
}

// CurrentRuntime returns the selected session's host-independent runtime view.
func (s *Service) CurrentRuntime() *factorysessions.LiveRuntime {
	session := s.Current()
	if session == nil {
		session = s.Default()
	}
	if session == nil {
		return nil
	}
	return session.Runtime
}

// WithRuntimeRead protects runtime operations against concurrent definition activation.
func (s *Service) WithRuntimeRead(fn func(*factorysessions.LiveRuntime) error) error {
	if s == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	s.activation.RLock()
	defer s.activation.RUnlock()
	runtime := s.CurrentRuntime()
	if runtime == nil || runtime.Factory == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return fn(runtime)
}

// WithActivationLock serializes definition activation against runtime operations.
func (s *Service) WithActivationLock(fn func() error) error {
	if s == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	s.activation.Lock()
	defer s.activation.Unlock()
	return fn()
}

// New constructs the session runtime with explicit live-session, response-stream,
// cleanup, and clock dependencies.
func New(
	registry factorysessions.Registry,
	responses *responsestream.Registry,
	closeSession func(*factorysessions.LiveSession),
	clock factoryruntime.Clock,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
) *Service {
	return NewWithResponseStreams(registry, responses, closeSession, clock, eventIDs, sessionIDs)
}

// NewWithResponseStreams constructs the session runtime with both canonical
// live-session and response-stream registries.
func NewWithResponseStreams(
	registry factorysessions.Registry,
	responses *responsestream.Registry,
	closeSession func(*factorysessions.LiveSession),
	clock factoryruntime.Clock,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
) *Service {
	if registry == nil || responses == nil || clock == nil || eventIDs == nil || sessionIDs == nil {
		return nil
	}
	return &Service{registry: registry, responses: responses, close: closeSession, clock: clock, eventIDs: eventIDs, sessionIDs: sessionIDs}
}

// ResponseStreams returns the response-stream registry paired with this live
// session runtime.
func (s *Service) ResponseStreams() *responsestream.Registry {
	if s == nil {
		return nil
	}
	return s.responses
}

// ResponseStreamsForSession returns the canonical registry-owned stream set
// for one live session.
func (s *Service) ResponseStreamsForSession(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if s == nil || s.responses == nil || session == nil {
		return nil
	}
	return s.responses.Streams(factorysessions.CanonicalFactorySessionID(session))
}

// CloseResponseStreams closes response events and every dispatch stream owned
// by one live session.
func (s *Service) CloseResponseStreams(session *factorysessions.LiveSession) {
	if s == nil || s.responses == nil || session == nil {
		return
	}
	session.CloseResponseEvents()
	s.responses.Close(factorysessions.CanonicalFactorySessionID(session))
}

// Registry exposes the canonical registry to bounded compatibility adapters.
func (s *Service) Registry() factorysessions.Registry {
	if s == nil {
		return nil
	}
	return s.registry
}

// Register creates and selects a live session according to the registration.
func (s *Service) Register(registration Registration) string {
	if s == nil || s.registry == nil || registration.Handle == nil {
		return ""
	}
	sessionID := strings.TrimSpace(registration.SessionID)
	if sessionID == "" {
		return ""
	}
	isDefault := registration.Default || factorysessions.IsDefaultSessionSelector(sessionID)
	if isDefault && registration.AllocateDefaultID {
		if existing := s.registry.DefaultSession(); existing != nil {
			sessionID = existing.ID
		} else {
			sessionID = strings.TrimSpace(s.sessionIDs())
			if sessionID == "" {
				return ""
			}
		}
	}
	session := factorysessions.NewLiveSession(
		sessionID,
		strings.TrimSpace(registration.FactoryDir),
		strings.TrimSpace(registration.FolderPath),
		strings.TrimSpace(registration.ExecutionBaseDir),
		registration.Target,
		registration.Handle,
		isDefault,
		strings.TrimSpace(registration.Project),
		s.clock,
		s.sessionIDs,
		s.eventIDs,
	)
	session.Runtime = registration.Runtime
	factorysessions.BindResponseEventCompletion(session, registration.AddEventTypeRecorder)
	s.registry.Upsert(session, registration.Select)
	return sessionID
}

// Unregister closes session-owned streams and removes the live session.
func (s *Service) Unregister(sessionID string) {
	if s == nil || s.registry == nil {
		return
	}
	session := s.Resolve(sessionID)
	if session == nil {
		return
	}
	if s.close != nil {
		s.close(session)
	}
	s.CloseResponseStreams(session)
	s.registry.Remove(session.ID)
}

// Current returns the selected live session.
func (s *Service) Current() *factorysessions.LiveSession {
	if s == nil || s.registry == nil {
		return nil
	}
	return s.registry.Current()
}

// Default returns the compatibility default live session.
func (s *Service) Default() *factorysessions.LiveSession {
	if s == nil || s.registry == nil {
		return nil
	}
	return s.registry.DefaultSession()
}

// Resolve accepts registry IDs, the default selector, and canonical runtime IDs.
func (s *Service) Resolve(sessionID string) *factorysessions.LiveSession {
	if s == nil || s.registry == nil {
		return nil
	}
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return nil
	}
	if session := s.registry.Get(trimmed); session != nil {
		return session
	}
	for _, id := range s.registry.IDs() {
		session := s.registry.Get(id)
		if session != nil && factorysessions.CanonicalFactorySessionID(session) == trimmed {
			return session
		}
	}
	return nil
}

// Require resolves a live session and verifies its opaque runtime handle.
func (s *Service) Require(sessionID string, validHandle func(any) bool) (*factorysessions.LiveSession, error) {
	session := s.Resolve(sessionID)
	if session == nil || (validHandle != nil && !validHandle(session.Handle)) {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	return session, nil
}

// RequireSession resolves a session for domain services that do not interpret
// the runtime-owned opaque handle.
func (s *Service) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	return s.Require(sessionID, nil)
}

// GetLiveSession resolves registry aliases and canonical runtime IDs.
func (s *Service) GetLiveSession(sessionID string) *factorysessions.LiveSession {
	return s.Resolve(sessionID)
}
