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
	IDAgy      ID = "agy"
	IDClaude   ID = "claude"
	IDCodex    ID = "codex"
	IDCursor   ID = "agent"
	IDGemini   ID = "gemini"
	IDKiro     ID = "kiro-cli"
	IDOpenCode ID = "opencode"
	IDPi       ID = "pi"
)

// SessionIDKind is the canonical session-ref kind for provider-issued session
// identifiers emitted by Execute and consumed by Provider Sessions.
const SessionIDKind = "session_id"

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
// Availability, capability, and prerequisite facts publish on later catalog
// slices; this value carries the identity facts peers need to list and name
// providers without importing Workers provider internals.
type Descriptor struct {
	ID          ID
	Aliases     []string
	DisplayName string
}

// Clone returns a detached descriptor copy.
func (descriptor Descriptor) Clone() Descriptor {
	cloned := descriptor
	cloned.Aliases = append([]string(nil), descriptor.Aliases...)
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
