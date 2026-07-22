// Package providersessions is the public Provider Sessions service boundary.
package providersessions

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"
)

// FileSystem is the exact provider-session storage boundary. Storage layout
// policy remains in Provider Sessions; Wire selects the concrete filesystem.
type FileSystem interface {
	Open(string) (io.ReadCloser, error)
	Stat(string) (fs.FileInfo, error)
}

// ResolveHomeDirectory supplies the process home used to derive provider-owned
// default storage roots.
type ResolveHomeDirectory func() (string, error)

// CodexWalkDirectory traverses the configured Codex session tree.
type CodexWalkDirectory func(string, fs.WalkDirFunc) error

// CodexResolveSymlinks resolves Codex session paths before containment checks.
type CodexResolveSymlinks func(string) (string, error)

// CursorWalkDirectory traverses the configured Cursor session storage tree.
type CursorWalkDirectory func(string, fs.WalkDirFunc) error

// CursorResolveSymlinks resolves Cursor storage paths before containment checks.
type CursorResolveSymlinks func(string) (string, error)

// CursorOpenSQLDatabase opens a Cursor database driver connection.
type CursorOpenSQLDatabase func(driverName, dataSourceName string) (*sql.DB, error)

// OperatingSystem identifies the platform whose provider storage convention
// should be selected.
type OperatingSystem string

// Service is the provider-independent session inspection contract.
type Service interface {
	Details(provider, kind, id string) (Detail, error)
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
