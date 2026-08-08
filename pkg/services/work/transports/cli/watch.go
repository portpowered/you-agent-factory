package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

// WatchSchemaVersion identifies the stable line-oriented Work watch contract.
const WatchSchemaVersion = "you.work.watch.v1"

// WatchConfig holds the public options shared by the Work watch command and
// the event-stream implementation that will consume them.
type WatchConfig struct {
	Context           context.Context
	Server            string
	SessionID         string
	SessionIDExplicit bool
	Follow            bool
	Output            io.Writer
	Diagnostics       io.Writer
	HTTP              clihttp.Protocol
	Verbose           bool
	Debug             bool
}

// WatchTransition is the transport-neutral transition data rendered by one
// Work watch NDJSON line.
type WatchTransition struct {
	SessionID     string
	EventID       string
	Sequence      int64
	EventTime     time.Time
	WorkID        string
	WorkTypeName  string
	FromState     string
	ToState       string
	Source        string
	Terminal      bool
	TriggerWorkID string
	Reason        string
}

type watchLine struct {
	SchemaVersion string    `json:"schemaVersion"`
	SessionID     string    `json:"sessionId"`
	EventID       string    `json:"eventId"`
	Sequence      int64     `json:"sequence"`
	EventTime     time.Time `json:"eventTime"`
	WorkID        string    `json:"workId"`
	WorkTypeName  string    `json:"workTypeName"`
	FromState     string    `json:"fromState"`
	ToState       string    `json:"toState"`
	Source        string    `json:"source"`
	Terminal      bool      `json:"terminal"`
	TriggerWorkID string    `json:"triggerWorkId,omitempty"`
	Reason        string    `json:"reason,omitempty"`
}

// ValidateWatchConfig validates command-owned options before a stream is
// opened. An omitted session intentionally means the compatibility session;
// an explicitly empty session is always an operator error.
func ValidateWatchConfig(cfg WatchConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("work watch context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("work watch output writer is required")
	}
	if cfg.SessionIDExplicit && strings.TrimSpace(cfg.SessionID) == "" {
		return fmt.Errorf("work watch --session must not be empty")
	}
	return nil
}

// RenderWatchTransition writes exactly one complete JSON line. The payload is
// marshalled before the writer is touched so validation and encoding failures
// cannot leave a partial transition record on stdout.
func RenderWatchTransition(output io.Writer, transition WatchTransition) error {
	if output == nil {
		return fmt.Errorf("render work watch transition: output writer is required")
	}
	if err := validateWatchTransition(transition); err != nil {
		return fmt.Errorf("render work watch transition: %w", err)
	}
	payload, err := json.Marshal(watchLine{
		SchemaVersion: WatchSchemaVersion,
		SessionID:     transition.SessionID,
		EventID:       transition.EventID,
		Sequence:      transition.Sequence,
		EventTime:     transition.EventTime,
		WorkID:        transition.WorkID,
		WorkTypeName:  transition.WorkTypeName,
		FromState:     transition.FromState,
		ToState:       transition.ToState,
		Source:        transition.Source,
		Terminal:      transition.Terminal,
		TriggerWorkID: transition.TriggerWorkID,
		Reason:        transition.Reason,
	})
	if err != nil {
		return fmt.Errorf("encode work watch transition: %w", err)
	}
	line := append(payload, '\n')
	written, err := output.Write(line)
	if err != nil {
		return fmt.Errorf("write work watch transition: %w", err)
	}
	if written != len(line) {
		return fmt.Errorf("write work watch transition: %w", io.ErrShortWrite)
	}
	return nil
}

func validateWatchTransition(transition WatchTransition) error {
	for field, value := range map[string]string{
		"sessionId":    transition.SessionID,
		"eventId":      transition.EventID,
		"workId":       transition.WorkID,
		"workTypeName": transition.WorkTypeName,
		"fromState":    transition.FromState,
		"toState":      transition.ToState,
		"source":       transition.Source,
	} {
		if value == "" {
			return fmt.Errorf("%s is required", field)
		}
	}
	if transition.Sequence < 0 {
		return fmt.Errorf("sequence must be non-negative")
	}
	if transition.EventTime.IsZero() {
		return fmt.Errorf("eventTime is required")
	}
	return nil
}
