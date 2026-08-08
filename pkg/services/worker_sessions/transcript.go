package workersessions

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// ReadTranscriptRequest identifies one exact Provider Session whose normalized
// transcript should be returned.
type ReadTranscriptRequest struct {
	ProviderSession providers.SessionRef
}

// Validate reports whether the request carries a complete typed identity.
func (r ReadTranscriptRequest) Validate() error {
	if err := r.ProviderSession.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
	}
	return nil
}

// ReadTranscriptResult is the detached Worker Session envelope and ordered
// normalized transcript returned for a finished Provider Session.
type ReadTranscriptResult struct {
	WorkerSessionID string
	ProviderSession providers.SessionRef
	WorkIDs         []string
	TurnID          string
	AttemptID       string
	State           State
	Entries         []TranscriptEntry
}

// Clone returns a detached transcript result.
func (r ReadTranscriptResult) Clone() ReadTranscriptResult {
	clone := r
	clone.ProviderSession = r.ProviderSession.Clone()
	clone.WorkIDs = append([]string(nil), r.WorkIDs...)
	clone.Entries = make([]TranscriptEntry, len(r.Entries))
	for index, entry := range r.Entries {
		clone.Entries[index] = entry.Clone()
	}
	return clone
}

// Validate reports whether the transcript envelope has a coherent identity,
// terminal lifecycle state, and ordered entries.
func (r ReadTranscriptResult) Validate() error {
	if strings.TrimSpace(r.WorkerSessionID) == "" {
		return ErrInvalidObservationIdentity
	}
	if err := r.ProviderSession.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidObservationIdentity, err)
	}
	if !r.State.Valid() {
		return ErrInvalidState
	}
	if !r.State.Terminal() {
		return ErrObservationTranscriptActive
	}
	if strings.TrimSpace(r.AttemptID) == "" {
		return ErrInvalidObservationAttempt
	}
	for index, entry := range r.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("transcript entry %d: %w", index+1, err)
		}
	}
	return nil
}

// TranscriptEntryType is the normalized provider-neutral role or activity
// represented by one transcript entry.
type TranscriptEntryType string

const (
	TranscriptAssistantMessage TranscriptEntryType = "assistant_message"
	TranscriptReasoning        TranscriptEntryType = "reasoning"
	TranscriptSystemEvent      TranscriptEntryType = "system_event"
	TranscriptToolCall         TranscriptEntryType = "tool_call"
	TranscriptToolOutput       TranscriptEntryType = "tool_output"
	TranscriptUserMessage      TranscriptEntryType = "user_message"
)

// TranscriptEntry is one normalized, bounded transcript item. Optional
// fields remain nil when the Provider Sessions projection cannot supply them.
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

// Validate reports whether the entry has a stable order and supported
// normalized type. Provider-specific payloads are intentionally not accepted.
func (e TranscriptEntry) Validate() error {
	if e.Order < 0 {
		return fmt.Errorf("transcript entry order must not be negative")
	}
	if strings.TrimSpace(string(e.Type)) == "" {
		return fmt.Errorf("transcript entry type is required")
	}
	return nil
}

// Clone returns a detached transcript entry.
func (e TranscriptEntry) Clone() TranscriptEntry {
	clone := e
	clone.Arguments = cloneString(e.Arguments)
	clone.CallID = cloneString(e.CallID)
	clone.Encrypted = cloneBool(e.Encrypted)
	clone.EncryptedContent = cloneString(e.EncryptedContent)
	clone.LineNumber = cloneInt(e.LineNumber)
	clone.Name = cloneString(e.Name)
	clone.Output = cloneString(e.Output)
	clone.SourceType = cloneString(e.SourceType)
	clone.Status = cloneString(e.Status)
	clone.Summary = cloneString(e.Summary)
	clone.Text = cloneString(e.Text)
	clone.Timestamp = cloneTime(e.Timestamp)
	clone.TurnIndex = cloneInt(e.TurnIndex)
	return clone
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

var (
	// ErrObservationTranscriptActive means the requested session has not
	// reached an absorbing lifecycle state, so its transcript is not final.
	ErrObservationTranscriptActive = errors.New("worker session transcript: session is active")
	// ErrObservationTranscriptUnavailable means no normalized transcript is
	// available for an otherwise terminal Worker Session.
	ErrObservationTranscriptUnavailable = errors.New("worker session transcript: transcript unavailable")
	// ErrObservationTranscriptProjectionUnavailable means Provider Sessions
	// could not project the normalized transcript source.
	ErrObservationTranscriptProjectionUnavailable = errors.New("worker session transcript: projection unavailable")
)
