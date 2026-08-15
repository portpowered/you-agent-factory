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
