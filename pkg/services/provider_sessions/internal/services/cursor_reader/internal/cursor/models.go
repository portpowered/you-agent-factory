package cursor

import (
	"encoding/json"
	"strings"
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
	content    []rawContentItem
}

type rawContentItem struct {
	kind      string
	text      string
	summary   string
	name      string
	callID    string
	arguments string
	output    string
	status    string
	encrypted bool
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

func parseMessageContentItems(content []interface{}) []rawContentItem {
	items := make([]rawContentItem, 0, len(content))
	for _, value := range content {
		data, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		items = append(items, parseMessageContentItem(data))
	}
	return items
}

func parseMessageContentItem(data map[string]interface{}) rawContentItem {
	kind, _ := data["type"].(string)
	item := rawContentItem{
		kind:    strings.ToLower(strings.TrimSpace(kind)),
		text:    firstString(data, "text"),
		summary: firstString(data, "summary"),
		name:    firstString(data, "name"),
		callID:  firstString(data, "tool_call_id", "call_id", "id"),
		output:  firstString(data, "output", "result", "content"),
		status:  firstString(data, "status"),
	}
	item.arguments = normalizedArguments(data["arguments"])
	if item.kind == "redacted-reasoning" || item.kind == "redacted_reasoning" {
		item.encrypted = true
		item.text = ""
		if item.summary == "" {
			item.summary = "encrypted reasoning unavailable"
		}
		return item
	}
	if item.text == "" && isReasoningKind(item.kind) {
		item.text = firstString(data, "data")
	}
	return item
}

func normalizedArguments(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]interface{}, []interface{}:
		encoded, err := json.Marshal(typed)
		if err == nil {
			return string(encoded)
		}
	}
	return ""
}

func firstString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok {
			return value
		}
	}
	return ""
}

func plainMessageText(items []rawContentItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if isTextKind(item.kind) && item.text != "" {
			parts = append(parts, item.text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func isTextKind(kind string) bool {
	switch kind {
	case "", "text", "input_text", "output_text":
		return true
	default:
		return false
	}
}

func isReasoningKind(kind string) bool {
	switch kind {
	case "reasoning", "thinking", "redacted-reasoning", "redacted_reasoning":
		return true
	default:
		return false
	}
}
