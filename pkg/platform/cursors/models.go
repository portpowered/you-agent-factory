package cursors

import (
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

// GetTimestamp returns a time.Time from the timestamp
func (rb *RawBubble) GetTimestamp() time.Time {
	return time.Unix(0, rb.Timestamp*int64(time.Millisecond))
}

// DisplayText returns the best-effort plaintext body for transcript mapping.
func (rb *RawBubble) DisplayText() string {
	if rb == nil {
		return ""
	}
	if rb.Text != "" {
		return rb.Text
	}
	return rb.RichText
}

// TranscriptEntryType maps cursor-agent bubble type to provider-neutral transcript types.
func (rb *RawBubble) TranscriptEntryType() string {
	if rb != nil && rb.Type == 1 {
		return "user_message"
	}
	return "assistant_message"
}
