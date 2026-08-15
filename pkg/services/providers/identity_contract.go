package providers

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidID reports that a provider identity string is empty or otherwise
// unusable as a Providers-owned ID.
var ErrInvalidID = errors.New("provider id is invalid")

// ErrInvalidSessionRef reports that a detached SessionRef is missing required
// provider, kind, or id fields.
var ErrInvalidSessionRef = errors.New("provider session ref is invalid")

// ErrInvalidContinuationRef reports that a detached continuation reference is
// missing the provider or exact provider-session identity required to resume
// one prior attempt.
var ErrInvalidContinuationRef = errors.New("provider continuation ref is invalid")

// ID is the Providers-owned canonical provider identity. Peers enumerate and
// select providers through this typed vocabulary rather than Workers provider
// registry or manifest types.
type ID string

const (
	IDAntigravity ID = "antigravity"
	IDClaude      ID = "claude"
	IDCodex       ID = "codex"
	IDCursor      ID = "cursor"
	// Retired identities remain typed for persisted-data decoding and errors,
	// but are not present in the built-in catalog or execution registry.
	IDGemini   ID = "gemini"
	IDKiro     ID = "kiro"
	IDOpenCode ID = "opencode"
	IDPi       ID = "pi"
)

// SessionIDKind is the canonical session-ref kind for provider-issued session
// identifiers emitted by Execute and consumed by Provider Sessions.
const SessionIDKind = "session_id"

// SessionMetadata is the detached, provider-owned compatibility projection of
// a provider session identity. It is intentionally limited to identity facts;
// transcript, storage, and inspection state remain owned by Provider Sessions.
type SessionMetadata struct {
	Provider string `json:"provider,omitempty"`
	Kind     string `json:"kind,omitempty"`
	ID       string `json:"id,omitempty"`
}

// Clone returns a detached provider-session identity projection. Session
// storage and transcript state remain outside this value.
func (session *SessionMetadata) Clone() *SessionMetadata {
	if session == nil {
		return nil
	}
	clone := *session
	return &clone
}

// CanonicalSessionProvider maps legacy provider command names onto the stable
// provider identity used by persisted diagnostics and event projections.
func (id ID) CanonicalSessionProvider() string {
	trimmed := strings.TrimSpace(id.String())
	switch trimmed {
	case "", "cursor":
		return trimmed
	case "agent", "cursor-agent", "cursor-cli":
		return "cursor"
	default:
		return trimmed
	}
}

// ContinuationRef projects a detached session identity onto the opaque
// continuation vocabulary used across Worker boundaries.
func (session *SessionMetadata) ContinuationRef() *ContinuationRef {
	if session == nil {
		return nil
	}
	provider := ID(session.Provider).CanonicalSessionProvider()
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(session.ID) == "" {
		return nil
	}
	continuation := ContinuationRef{
		Provider:          provider,
		Kind:              strings.TrimSpace(session.Kind),
		ProviderSessionID: strings.TrimSpace(session.ID),
		ExternalRef:       strings.TrimSpace(session.ID),
	}.Normalize()
	return &continuation
}

// SessionMetadata projects an opaque continuation onto the detached identity
// shape required by canonical event and transport models.
func (reference *ContinuationRef) SessionMetadata() *SessionMetadata {
	if reference == nil {
		return nil
	}
	normalized := reference.Normalize()
	if strings.TrimSpace(normalized.Provider) == "" {
		return nil
	}
	identity := strings.TrimSpace(normalized.ProviderSessionID)
	if identity == "" {
		identity = strings.TrimSpace(normalized.ExternalRef)
	}
	if identity == "" {
		return nil
	}
	return &SessionMetadata{
		Provider: ID(normalized.Provider).CanonicalSessionProvider(),
		Kind:     normalized.Kind,
		ID:       identity,
	}
}

// Validate checks that the provider ID is non-empty after trimming.
func (id ID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return fmt.Errorf("%w: empty provider id", ErrInvalidID)
	}
	return nil
}

// String returns the canonical provider id string.
func (id ID) String() string {
	return string(id)
}

// Descriptor is the Providers-owned enumeration value for one catalog provider.
// It carries identity, availability, capability, and prerequisite facts peers
// need to list and select providers without importing Workers provider internals.
type Descriptor struct {
	ID                         ID
	Aliases                    []string
	DisplayName                string
	Availability               Availability
	Readiness                  Readiness
	TechnicalSupportLevel      TechnicalSupportLevel
	ImplementationAvailability ImplementationAvailability
	Prerequisites              []Prerequisite
	Models                     []ModelDescriptor
	Tools                      []Tool
	KnownLimits                []KnownLimit
	Capabilities               []Capability
}

// Clone returns a detached descriptor copy.
func (descriptor Descriptor) Clone() Descriptor {
	cloned := descriptor
	cloned.Aliases = append([]string(nil), descriptor.Aliases...)
	cloned.Prerequisites = clonePrerequisites(descriptor.Prerequisites)
	cloned.Models = make([]ModelDescriptor, len(descriptor.Models))
	for index, model := range descriptor.Models {
		cloned.Models[index] = model.Clone()
	}
	cloned.Tools = append([]Tool(nil), descriptor.Tools...)
	cloned.KnownLimits = make([]KnownLimit, len(descriptor.KnownLimits))
	for index, limit := range descriptor.KnownLimits {
		cloned.KnownLimits[index] = limit.Clone()
	}
	cloned.Capabilities = append([]Capability(nil), descriptor.Capabilities...)
	return cloned
}

// SessionRef is the detached typed provider-session identity in the
// Providers-owned vocabulary (provider + kind + id). It does not embed Workers
// provider, Petri/JavaScript, concrete adapter, transport, or UI types.
type SessionRef struct {
	Provider ID
	Kind     string
	ID       string
}

// Validate checks that provider, kind, and id are all non-empty after trimming.
func (ref SessionRef) Validate() error {
	if err := ref.Provider.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(ref.Kind) == "" {
		return fmt.Errorf("%w: empty session kind", ErrInvalidSessionRef)
	}
	if strings.TrimSpace(ref.ID) == "" {
		return fmt.Errorf("%w: empty session id", ErrInvalidSessionRef)
	}
	return nil
}

// Clone returns a detached session-ref copy.
func (ref SessionRef) Clone() SessionRef {
	return ref
}

// ContinuationRef is the Providers-owned opaque continuation value carried
// across Worker and Runtime boundaries. It contains only detached identity
// facts; provider clients, transcripts, processes, and mutable session state
// remain behind Providers and Provider Sessions.
//
// ProviderSessionID is the canonical provider-session identity. ExternalRef
// preserves a provider-specific opaque reference when one is available. The
// legacy session-ref adapter uses ProviderSessionID first and ExternalRef as a
// compatibility fallback.
type ContinuationRef struct {
	Provider          string
	Kind              string
	ProviderSessionID string
	ExternalRef       string
}

// Validate checks that a continuation has a provider and at least one exact
// provider-session identity. An omitted Kind is accepted for persisted
// compatibility and is normalized to SessionIDKind when adapted to SessionRef.
func (ref ContinuationRef) Validate() error {
	if strings.TrimSpace(ref.Provider) == "" {
		return fmt.Errorf("%w: empty provider", ErrInvalidContinuationRef)
	}
	if strings.TrimSpace(ref.ProviderSessionID) == "" &&
		strings.TrimSpace(ref.ExternalRef) == "" {
		return fmt.Errorf("%w: empty provider session identity", ErrInvalidContinuationRef)
	}
	return nil
}

// Normalize returns a detached, trimmed continuation value and supplies the
// compatibility session-id kind when older callers omitted it.
func (ref ContinuationRef) Normalize() ContinuationRef {
	ref.Provider = strings.TrimSpace(ref.Provider)
	ref.Kind = strings.TrimSpace(ref.Kind)
	if ref.Kind == "" {
		ref.Kind = SessionIDKind
	}
	ref.ProviderSessionID = strings.TrimSpace(ref.ProviderSessionID)
	ref.ExternalRef = strings.TrimSpace(ref.ExternalRef)
	return ref
}

// ToSessionRef adapts a valid continuation into the Providers-owned exact
// session identity used by the existing provider continuation capability.
func (ref ContinuationRef) ToSessionRef() (SessionRef, error) {
	if err := ref.Validate(); err != nil {
		return SessionRef{}, err
	}
	normalized := ref.Normalize()
	identity := normalized.ProviderSessionID
	if identity == "" {
		identity = normalized.ExternalRef
	}
	session := SessionRef{
		Provider: ID(normalized.Provider),
		Kind:     normalized.Kind,
		ID:       identity,
	}
	if err := session.Validate(); err != nil {
		return SessionRef{}, fmt.Errorf("%w: %v", ErrInvalidContinuationRef, err)
	}
	return session, nil
}

// ContinuationRef projects this exact Providers session identity onto the
// detached continuation vocabulary. Both identity fields are populated so a
// compatibility boundary that understands either spelling retains the same
// exact session.
func (ref SessionRef) ContinuationRef() ContinuationRef {
	return ContinuationRef{
		Provider:          ref.Provider.CanonicalSessionProvider(),
		Kind:              ref.Kind,
		ProviderSessionID: ref.ID,
		ExternalRef:       ref.ID,
	}
}

// Clone returns a detached continuation-reference copy.
func (ref ContinuationRef) Clone() ContinuationRef {
	return ref
}

// ClonePtr returns a detached pointer copy for optional continuation fields.
func (ref *ContinuationRef) ClonePtr() *ContinuationRef {
	if ref == nil {
		return nil
	}
	clone := ref.Clone()
	return &clone
}
