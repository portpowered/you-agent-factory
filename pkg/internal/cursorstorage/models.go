package cursorstorage

import (
	"encoding/json"
	"fmt"
	"time"
)

// RawBubble represents a message bubble from the database
type RawBubble struct {
	BubbleID   string      `json:"bubbleId"`
	ChatID     string      `json:"chatId"`
	Text       string      `json:"text,omitempty"`
	RichText   string      `json:"richText,omitempty"`
	CodeBlocks []CodeBlock `json:"codeBlocks,omitempty"`
	Timestamp  int64       `json:"timestamp"`
	Type       int         `json:"type"` // 1=user, 2=assistant
}

// CodeBlock represents a code block in a message
type CodeBlock struct {
	Language string `json:"language,omitempty"`
	Content  string `json:"content"`
}

// RawComposer represents composer data from the database
type RawComposer struct {
	ComposerID                  string               `json:"composerId"`
	Name                        string               `json:"name,omitempty"`
	FullConversationHeadersOnly []ConversationHeader `json:"fullConversationHeadersOnly,omitempty"`
	LastUpdatedAt               int64                `json:"lastUpdatedAt,omitempty"`
	CreatedAt                   int64                `json:"createdAt,omitempty"`
}

// ConversationHeader represents a header in a conversation
type ConversationHeader struct {
	BubbleID string `json:"bubbleId"`
	Type     int    `json:"type"` // 1=user, 2=assistant
}

// MessageContext represents context data for a message
type MessageContext struct {
	BubbleID                      string        `json:"bubbleId"`
	ComposerID                    string        `json:"composerId"`
	ContextID                     string        `json:"contextId"`
	GitStatusRaw                  string        `json:"gitStatusRaw,omitempty"`
	TerminalFiles                 []string      `json:"terminalFiles,omitempty"`
	AttachedFoldersListDirResults []interface{} `json:"attachedFoldersListDirResults,omitempty"`
	CursorRules                   []interface{} `json:"cursorRules,omitempty"`
	ProjectLayouts                []string      `json:"projectLayouts,omitempty"`
}

// ParseRawBubble parses a JSON value into a RawBubble
func ParseRawBubble(key, value string) (*RawBubble, error) {
	// Extract chatId and bubbleId from key: bubbleId:<chatId>:<bubbleId>
	parts := splitKey(key, "bubbleId:")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid bubbleId key format: %s", key)
	}

	var bubble RawBubble
	if err := json.Unmarshal([]byte(value), &bubble); err != nil {
		return nil, fmt.Errorf("failed to parse bubble JSON: %w", err)
	}

	bubble.ChatID = parts[1]
	bubble.BubbleID = parts[2]

	return &bubble, nil
}

// ParseRawComposer parses a JSON value into a RawComposer
func ParseRawComposer(key, value string) (*RawComposer, error) {
	// Extract composerId from key: composerData:<composerId>
	parts := splitKey(key, "composerData:")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid composerData key format: %s", key)
	}

	var composer RawComposer
	if err := json.Unmarshal([]byte(value), &composer); err != nil {
		return nil, fmt.Errorf("failed to parse composer JSON: %w", err)
	}

	composer.ComposerID = parts[1]

	return &composer, nil
}

// ParseMessageContext parses a JSON value into a MessageContext
func ParseMessageContext(key, value string) (*MessageContext, error) {
	// Extract composerId and contextId from key: messageRequestContext:<composerId>:<contextId>
	parts := splitKey(key, "messageRequestContext:")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid messageRequestContext key format: %s", key)
	}

	var context MessageContext
	if err := json.Unmarshal([]byte(value), &context); err != nil {
		return nil, fmt.Errorf("failed to parse context JSON: %w", err)
	}

	context.ComposerID = parts[1]
	context.ContextID = parts[2]

	return &context, nil
}

// splitKey splits a key by prefix and returns the parts
func splitKey(key, prefix string) []string {
	if !startsWith(key, prefix) {
		return nil
	}
	remainder := key[len(prefix):]
	parts := split(remainder, ":")
	// Prepend empty string to match expected format: [prefix, part1, part2, ...]
	result := []string{""}
	result = append(result, parts...)
	return result
}

// Helper functions (simplified versions)
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func split(s, sep string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			parts = append(parts, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// ToIntermediaryJSON converts RawComposer to intermediary JSON format
func (rc *RawComposer) ToIntermediaryJSON() ([]byte, error) {
	return json.MarshalIndent(rc, "", "  ")
}

// ToIntermediaryYAML converts RawComposer to intermediary YAML format
func (rc *RawComposer) ToIntermediaryYAML() ([]byte, error) {
	// For now, we'll use JSON and convert later if needed
	// YAML support can be added with gopkg.in/yaml.v3
	return rc.ToIntermediaryJSON()
}

// GetTimestamp returns a time.Time from the timestamp
func (rb *RawBubble) GetTimestamp() time.Time {
	return time.Unix(0, rb.Timestamp*int64(time.Millisecond))
}

// GetTimestamp returns a time.Time from the timestamp
func (rc *RawComposer) GetCreatedAt() time.Time {
	if rc.CreatedAt == 0 {
		return time.Time{}
	}
	return time.Unix(0, rc.CreatedAt*int64(time.Millisecond))
}

// GetLastUpdatedAt returns a time.Time from the timestamp
func (rc *RawComposer) GetLastUpdatedAt() time.Time {
	if rc.LastUpdatedAt == 0 {
		return rc.GetCreatedAt()
	}
	return time.Unix(0, rc.LastUpdatedAt*int64(time.Millisecond))
}
