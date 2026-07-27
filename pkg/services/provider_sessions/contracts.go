package providersessions

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service is the singular Provider Sessions root contract for cross-service
// peers. Published slices (detached-ref validation/inspection and normalized
// transcript/detail projection) are additive methods on this one named
// interface and use plain Provider Sessions-owned request, result, value, and
// typed-error contracts. Peers depend on Service rather than mixed string-keyed
// helpers, construction effect ports, or private Codex/Cursor reader types.
// Nested IMP-PSES reader cuts, CTR-PROV/IMP-PROV, Standardized Providers
// conductor/migration, CLI-manifest, workers construction, and OpenAPI
// package-motion edits remain out of scope for the root-contract packet.
type Service interface {
	// Details is the published provider-independent session inspection entry on
	// the singular root. Peers supply provider/kind/id identity strings and
	// receive a detached Detail value (transcript, parse, usage, and related
	// normalized facts) or a typed Provider Sessions failure such as
	// ErrUnsupportedProvider, ErrUnsupportedKind, ErrInvalidIdentifier,
	// ErrSessionNotFound, ErrAmbiguousSessionFile, and/or LookupError. Callers do
	// not supply filesystem/SQL/OS effect ports or Codex/Cursor reader types to
	// invoke this peer API. Additive typed SessionRef slices (Inspect, Project)
	// share this singular root without replacing Details.
	Details(provider, kind, id string) (Detail, error)

	// Inspect validates and inspects a detached typed SessionRef identity in the
	// providers.SessionRef vocabulary (provider + kind + id). Peers receive a
	// detached InspectResult or a typed Provider Sessions failure such as
	// ErrUnsupportedProvider, ErrUnsupportedKind, ErrInvalidIdentifier,
	// ErrSessionNotFound, ErrAmbiguousSessionFile, and/or LookupError. This slice
	// does not import Providers catalog/execution, enumeration, availability,
	// capability, or Workers selection-policy types.
	Inspect(InspectRequest) (InspectResult, error)

	// Project returns a provider-independent normalized transcript/detail
	// projection for a detached typed SessionRef. Peers receive a ProjectResult
	// whose Detail covers transcript entries, reasoning summaries,
	// tool/function-call facts, parse summary, and token usage, or a typed
	// Provider Sessions failure such as ErrUnsupportedProvider,
	// ErrUnsupportedKind, ErrSessionNotFound, and/or LookupError. Method
	// signatures and published values do not name private Codex/Cursor reader
	// types, filesystem/SQL/OS effect ports, or Providers execution types.
	Project(ProjectRequest) (ProjectResult, error)
}

// SessionRef is an alias for the Providers-owned canonical provider-session
// identity. Provider Sessions does not publish a second identity type.
type SessionRef = providers.SessionRef

// InspectRequest asks the root Service to validate and inspect one detached
// SessionRef without requiring filesystem/SQL/OS effect ports from the caller.
type InspectRequest struct {
	Session SessionRef
}

// InspectResult is the detached success outcome for typed SessionRef
// validation/inspection. Normalized transcript/detail projection is published
// as the additive Project companion slice on the same root Service.
type InspectResult struct {
	Session SessionRef
	Source  SourceMetadata
}

// ProjectRequest asks the root Service for a normalized transcript/detail
// projection for one detached SessionRef without requiring filesystem/SQL/OS
// effect ports from the caller.
type ProjectRequest struct {
	Session SessionRef
}

// ProjectResult is the detached Detail-shaped projection peers consume for
// transcript, reasoning, tool/function-call, parse, and usage facts through
// Provider Sessions root contracts only.
type ProjectResult struct {
	Session SessionRef
	Detail  Detail
}

type Provider string

const (
	ProviderCodex  Provider = "codex"
	ProviderCursor Provider = "cursor"

	SessionIDKind = "session_id"
)

type Detail struct {
	ProviderSession Ref
	Source          SourceMetadata
	Parse           ParseSummary
	Transcript      []TranscriptEntry
}

type Ref struct {
	Provider Provider
	Kind     string
	ID       string
}

// Metadata carries a stable provider rollout/session identity across service
// boundaries.
type Metadata struct {
	Provider string `json:"provider,omitempty"`
	Kind     string `json:"kind,omitempty"`
	ID       string `json:"id,omitempty"`
}

// CanonicalProvider normalizes provider aliases onto their stable session
// identity.
func CanonicalProvider(provider string) string {
	trimmed := strings.TrimSpace(provider)
	switch trimmed {
	case "", "cursor":
		return trimmed
	case "agent", "cursor-agent":
		return "cursor"
	default:
		return trimmed
	}
}

func CloneMetadata(session *Metadata) *Metadata {
	if session == nil {
		return nil
	}
	clone := *session
	return &clone
}

type SourceMetadata struct {
	ModifiedAt   *time.Time
	RelativePath string
	SizeBytes    int64
}

type ParseSummary struct {
	EventCount         int
	FunctionCalls      []FunctionCallSummary
	LineCount          int
	MalformedLineCount int
	ParseErrors        []LineError
	Reasoning          []ReasoningSummary
	TokenUsage         *TokenUsage
	Turns              []TurnSummary
	UnknownEventCount  int
	UnknownEvents      []UnknownEvent
}

type FunctionCallSummary struct {
	Arguments *string
	CallID    *string
	Name      *string
	Order     int
	Output    *string
	Status    *string
	TurnIndex *int
	Type      string
}

type LineError struct {
	LineNumber int
	Message    string
}

type ReasoningSummary struct {
	Encrypted        *bool
	EncryptedContent *string
	Order            int
	SourceType       string
	Summary          *string
	Text             *string
	TurnIndex        *int
}

type TokenUsage struct {
	CacheWriteTokens      *int
	CachedInputTokens     *int
	InputTokens           *int
	OutputTokens          *int
	ReasoningOutputTokens *int
	TotalTokens           *int
}

type TranscriptEntryType string

const (
	TranscriptAssistantMessage TranscriptEntryType = "assistant_message"
	TranscriptReasoning        TranscriptEntryType = "reasoning"
	TranscriptSystemEvent      TranscriptEntryType = "system_event"
	TranscriptToolCall         TranscriptEntryType = "tool_call"
	TranscriptToolOutput       TranscriptEntryType = "tool_output"
	TranscriptUserMessage      TranscriptEntryType = "user_message"
)

type TranscriptEntry struct {
	Arguments        *string
	CallID           *string
	Encrypted        *bool
	EncryptedContent *string
	LineNumber       *int
	Name             *string
	Order            int
	Output           *string
	SourceType       *string
	Status           *string
	Summary          *string
	Text             *string
	Timestamp        *time.Time
	TurnIndex        *int
	Type             TranscriptEntryType
}

type TurnSummary struct {
	EventCount        int
	FunctionCallCount int
	Index             int
	ReasoningCount    int
	ResponseItemCount int
	StartedAt         *time.Time
}

type UnknownEvent struct {
	LineNumber  int
	PayloadType *string
	Type        *string
}

var (
	ErrAmbiguousSessionFile = errors.New("ambiguous provider session file")
	ErrInvalidIdentifier    = errors.New("invalid provider session identifier")
	ErrSessionNotFound      = errors.New("provider session not found")
	ErrUnsupportedKind      = errors.New("unsupported provider session kind")
	ErrUnsupportedProvider  = errors.New("unsupported provider session provider")
)

// LookupError retains normalized provider and root context without exposing
// provider-specific storage details through the service API.
type LookupError struct {
	Provider Provider
	Root     string
	Err      error
}

func (e *LookupError) Error() string {
	return fmt.Sprintf("load %s provider session: %v", e.Provider, e.Err)
}

func (e *LookupError) Unwrap() error {
	return e.Err
}
