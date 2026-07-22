package cursor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

func parseBubbleFromData(key string, data map[string]interface{}, sessionID string) (*RawBubble, error) {
	bubble := &RawBubble{}

	// Extract bubbleId
	if id, ok := data["bubbleId"].(string); ok {
		bubble.BubbleID = id
	} else {
		return nil, fmt.Errorf("missing bubbleId in data")
	}

	// Extract chatId (use sessionID if not present)
	if chatID, ok := data["chatId"].(string); ok {
		bubble.ChatID = chatID
	} else {
		bubble.ChatID = sessionID
	}

	// Extract text
	if text, ok := data["text"].(string); ok {
		bubble.Text = text
	}

	// Extract richText
	if richText, ok := data["richText"].(string); ok {
		bubble.RichText = richText
	}

	// Extract codeBlocks
	if codeBlocks, ok := data["codeBlocks"].([]interface{}); ok {
		for _, cb := range codeBlocks {
			if cbMap, ok := cb.(map[string]interface{}); ok {
				codeBlock := CodeBlock{}
				if lang, ok := cbMap["language"].(string); ok {
					codeBlock.Language = lang
				}
				if content, ok := cbMap["content"].(string); ok {
					codeBlock.Content = content
				}
				bubble.CodeBlocks = append(bubble.CodeBlocks, codeBlock)
			}
		}
	}

	// Extract timestamp
	// NOTE: cursor-agent does NOT store timestamps in individual bubble blobs.
	// Timestamps are only available at the session level in the meta table (key="0", field="createdAt").
	// Individual bubbles inherit the session createdAt timestamp.
	// Normalize to milliseconds (formatTimestamp expects milliseconds)
	if ts, ok := data["timestamp"].(float64); ok {
		bubble.Timestamp = normalizeTimestamp(int64(ts))
	} else if ts, ok := data["timestamp"].(int64); ok {
		bubble.Timestamp = normalizeTimestamp(ts)
	} else {
		bubble.Timestamp = 0
	}

	// Extract type
	if t, ok := data["type"].(float64); ok {
		bubble.Type = int(t)
	} else if t, ok := data["type"].(int); ok {
		bubble.Type = t
	}

	return bubble, nil
}

// isValidUUID checks if a string is a valid UUID format
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isValidUUID(s string) bool {
	return uuidRegex.MatchString(strings.ToLower(s))
}

// isReadableText checks if text is mostly readable (not garbled binary data)
// Returns true if text is valid UTF-8 and has a reasonable ratio of printable characters
func isReadableText(text string) bool {
	// Must be valid UTF-8
	if !utf8.ValidString(text) {
		return false
	}

	// Empty text is not readable
	if len(text) == 0 {
		return false
	}

	// Count printable vs non-printable characters
	printableCount := 0
	totalRunes := 0
	for _, r := range text {
		totalRunes++
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			printableCount++
		}
	}

	// If we have very few runes, require all to be printable
	if totalRunes < 5 {
		return printableCount == totalRunes
	}

	// For longer text, require at least 70% printable characters
	// This filters out binary data that happens to contain a $ character
	printableRatio := float64(printableCount) / float64(totalRunes)
	return printableRatio >= 0.70
}

// parseTextMessageFormat parses cursor-agent's text message format: "text$uuid"
// Returns a RawBubble if the format matches and data is valid, nil otherwise
// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session text bubble parsing stays grouped until extraction refactor.
// Handles format like: "hello$027f8b2f-d09c-4a69-98b0-b53f0118605d" (may have control chars)
func parseTextMessageFormat(key, value, sessionID string) *RawBubble {
	// First, check if value is valid UTF-8 - if not, it's likely binary data
	if !utf8.ValidString(value) {
		return nil
	}

	// First, aggressively remove all control characters except newlines/tabs/carriage returns
	// This handles cases where the value starts with control chars like \x05, \n, etc.
	cleaned := strings.Map(func(r rune) rune {
		// Keep printable characters, newlines, tabs, carriage returns, and space
		if r >= 32 || r == '\n' || r == '\r' || r == '\t' {
			return r
		}
		// Remove all other control characters
		return -1
	}, value)

	// Trim whitespace from both ends
	cleaned = strings.TrimSpace(cleaned)

	// Check if value matches pattern: text$uuid
	// Example: "hello$027f8b2f-d09c-4a69-98b0-b53f0118605d"
	dollarIdx := strings.Index(cleaned, "$")
	if dollarIdx == -1 || dollarIdx == 0 {
		return nil // No $ found or $ is at start
	}

	// Extract text before $ and clean it
	text := strings.TrimSpace(cleaned[:dollarIdx])
	// Remove any remaining control characters (shouldn't be any after first pass, but be safe)
	text = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1 // Remove control characters except newlines/tabs
		}
		return r
	}, text)
	text = strings.TrimSpace(text)

	// Trim quotes from beginning and end (both single and double quotes)
	// This handles cases like: "text$uuid or 'text$uuid
	text = strings.Trim(text, `"'`)

	if text == "" {
		return nil // No text before $
	}

	// Validate that the text is actually readable (not garbled binary data)
	if !isReadableText(text) {
		return nil // Text is not readable, likely binary data
	}

	// Extract UUID after $ (optional, but useful for bubble ID)
	uuidPart := ""
	if dollarIdx+1 < len(cleaned) {
		uuidPart = strings.TrimSpace(cleaned[dollarIdx+1:])
		// Remove control characters from UUID
		uuidPart = strings.Map(func(r rune) rune {
			if r < 32 {
				return -1
			}
			return r
		}, uuidPart)
	}

	// If UUID part exists, validate it's a proper UUID format
	if uuidPart != "" && !isValidUUID(uuidPart) {
		// If it looks like it should be a UUID but isn't valid, this might be garbled data
		// Only accept if the text part is clearly readable
		if len(text) < 10 {
			return nil // Short text with invalid UUID is suspicious
		}
		// For longer readable text, we'll accept it but use a generated ID
		uuidPart = ""
	}

	// Use UUID as bubble ID if available, otherwise use a hash of the text
	bubbleID := uuidPart
	if bubbleID == "" {
		// Generate a simple ID from the blob key (first 8 chars)
		if len(key) >= 8 {
			bubbleID = key[:8]
		} else {
			bubbleID = key
		}
	}

	// Create user bubble (type=1 for user messages)
	bubble := &RawBubble{
		BubbleID:  bubbleID,
		ChatID:    sessionID,
		Type:      1, // User message
		Text:      text,
		Timestamp: 0, // The text blob format does not carry a timestamp.
	}

	return bubble
}

// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session composer parsing stays grouped until extraction refactor.
func parseComposerFromData(key string, data map[string]interface{}) (*RawComposer, error) {
	composer := &RawComposer{}

	// Extract composerId
	if id, ok := data["composerId"].(string); ok {
		composer.ComposerID = id
	}

	// Extract name
	if name, ok := data["name"].(string); ok {
		composer.Name = name
	}

	// Extract fullConversationHeadersOnly
	if headers, ok := data["fullConversationHeadersOnly"].([]interface{}); ok {
		for _, h := range headers {
			if hMap, ok := h.(map[string]interface{}); ok {
				header := ConversationHeader{}
				if bubbleID, ok := hMap["bubbleId"].(string); ok {
					header.BubbleID = bubbleID
				}
				if t, ok := hMap["type"].(float64); ok {
					header.Type = int(t)
				} else if t, ok := hMap["type"].(int); ok {
					header.Type = t
				}
				composer.FullConversationHeadersOnly = append(composer.FullConversationHeadersOnly, header)
			}
		}
	}

	// Fallback to legacy format: conversation[] array
	if len(composer.FullConversationHeadersOnly) == 0 {
		// Try legacy format: conversation[] array
		if convArray, ok := data["conversation"].([]interface{}); ok && len(convArray) > 0 {
			LogInfo("Composer %s: Using legacy conversation[] format (found %d entries)", composer.ComposerID, len(convArray))
			// Convert legacy format to headers
			for _, entry := range convArray {
				if entryMap, ok := entry.(map[string]interface{}); ok {
					header := ConversationHeader{}
					if bubbleID, ok := entryMap["bubbleId"].(string); ok {
						header.BubbleID = bubbleID
					}
					if t, ok := entryMap["type"].(float64); ok {
						header.Type = int(t)
					} else if t, ok := entryMap["type"].(int); ok {
						header.Type = t
					}
					if header.BubbleID != "" {
						composer.FullConversationHeadersOnly = append(composer.FullConversationHeadersOnly, header)
					}
				}
			}
		} else {
			// Log available fields for debugging
			keys := make([]string, 0, len(data))
			for k := range data {
				keys = append(keys, k)
			}
			LogWarn("Composer %s: No conversation data found. Available fields: %v", composer.ComposerID, keys)
		}
	}

	// Extract timestamps
	if ts, ok := data["createdAt"].(float64); ok {
		composer.CreatedAt = int64(ts)
	} else if ts, ok := data["createdAt"].(int64); ok {
		composer.CreatedAt = ts
	}

	if ts, ok := data["lastUpdatedAt"].(float64); ok {
		composer.LastUpdatedAt = int64(ts)
	} else if ts, ok := data["lastUpdatedAt"].(int64); ok {
		composer.LastUpdatedAt = ts
	}

	return composer, nil
}

func parseContextFromData(key string, data map[string]interface{}) (*MessageContext, error) {
	context := &MessageContext{}

	// Extract contextId
	if id, ok := data["contextId"].(string); ok {
		context.ContextID = id
	}

	// Extract bubbleId
	if id, ok := data["bubbleId"].(string); ok {
		context.BubbleID = id
	}

	// Extract composerId
	if id, ok := data["composerId"].(string); ok {
		context.ComposerID = id
	}

	// Extract other optional fields
	if gitStatus, ok := data["gitStatusRaw"].(string); ok {
		context.GitStatusRaw = gitStatus
	}

	if terminalFiles, ok := data["terminalFiles"].([]interface{}); ok {
		for _, tf := range terminalFiles {
			if str, ok := tf.(string); ok {
				context.TerminalFiles = append(context.TerminalFiles, str)
			}
		}
	}

	if projectLayouts, ok := data["projectLayouts"].([]interface{}); ok {
		for _, pl := range projectLayouts {
			if str, ok := pl.(string); ok {
				context.ProjectLayouts = append(context.ProjectLayouts, str)
			}
		}
	}

	return context, nil
}

func messageBubbleID(key, id string) string {
	if len(key) >= 8 {
		return id + "-" + key[:8]
	}
	return id + "-" + key
}

func messageBubbleRoleType(role string) int {
	switch role {
	case "user":
		return 1
	case "assistant":
		return 2
	default:
		return 2
	}
}

func messageContentArrayToText(content []interface{}) string {
	var textParts []string
	for _, item := range content {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if part := messageContentItemText(itemMap); part != "" {
				textParts = append(textParts, part)
			}
		}
	}
	return strings.Join(textParts, "\n\n")
}

func messageContentItemText(itemMap map[string]interface{}) string {
	itemType, _ := itemMap["type"].(string)

	if itemType == "tool_call" || itemType == "function_call" {
		return messageToolCallText(itemMap)
	}
	if itemType == "tool" {
		return messageToolResponseText(itemMap)
	}
	if text, ok := itemMap["text"].(string); ok {
		return text
	}
	if data, ok := itemMap["data"].(string); ok {
		return messageDataFieldText(itemType, data)
	}
	return messageUnknownContentText(itemMap, itemType)
}

func messageToolCallText(itemMap map[string]interface{}) string {
	toolCallParts := []string{"[Tool Call]"}
	if name, ok := itemMap["name"].(string); ok {
		toolCallParts = append(toolCallParts, fmt.Sprintf("Tool: %s", name))
	}
	if toolCallID, ok := itemMap["tool_call_id"].(string); ok {
		toolCallParts = append(toolCallParts, fmt.Sprintf("ID: %s", toolCallID))
	}
	if args, ok := itemMap["arguments"].(string); ok {
		toolCallParts = append(toolCallParts, fmt.Sprintf("Arguments: %s", args))
	} else if argsMap, ok := itemMap["arguments"].(map[string]interface{}); ok {
		argsJSON, err := json.MarshalIndent(argsMap, "  ", "  ")
		if err == nil {
			toolCallParts = append(toolCallParts, fmt.Sprintf("Arguments:\n%s", string(argsJSON)))
		}
	}
	return strings.Join(toolCallParts, "\n")
}

func messageToolResponseText(itemMap map[string]interface{}) string {
	toolParts := []string{"[Tool Response]"}
	if name, ok := itemMap["name"].(string); ok {
		toolParts = append(toolParts, fmt.Sprintf("Tool: %s", name))
	}
	if toolCallID, ok := itemMap["tool_call_id"].(string); ok {
		toolParts = append(toolParts, fmt.Sprintf("Call ID: %s", toolCallID))
	}
	if content, ok := itemMap["content"].(string); ok {
		toolParts = append(toolParts, fmt.Sprintf("Content: %s", content))
	}
	return strings.Join(toolParts, "\n")
}

func messageDataFieldText(itemType, data string) string {
	if itemType == "redacted-reasoning" {
		decoded, wasDecoded := decodeRedactedReasoning(data)
		if wasDecoded {
			return fmt.Sprintf("```\n[Redacted Reasoning - Decoded]\n%s\n```", decoded)
		}
		if strings.Contains(decoded, "[Encrypted:") {
			return fmt.Sprintf("```\n%s\n```", decoded)
		}
		return fmt.Sprintf("```\n[Redacted Reasoning]\n%s\n```", data)
	}
	return data
}

// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session message content formatting stays grouped until extraction refactor.
func messageUnknownContentText(itemMap map[string]interface{}, itemType string) string {
	var unknownParts []string
	if itemType != "" {
		unknownParts = append(unknownParts, fmt.Sprintf("[%s]", itemType))
	}
	if name, ok := itemMap["name"].(string); ok && name != "" {
		unknownParts = append(unknownParts, fmt.Sprintf("Name: %s", name))
	}
	if id, ok := itemMap["id"].(string); ok && id != "" {
		unknownParts = append(unknownParts, fmt.Sprintf("ID: %s", id))
	}
	if toolCallID, ok := itemMap["tool_call_id"].(string); ok && toolCallID != "" {
		unknownParts = append(unknownParts, fmt.Sprintf("Tool Call ID: %s", toolCallID))
	}
	if content, ok := itemMap["content"].(string); ok && content != "" {
		contentPreview := content
		if len(contentPreview) > 500 {
			contentPreview = contentPreview[:500] + "..."
		}
		unknownParts = append(unknownParts, fmt.Sprintf("Content: %s", contentPreview))
	}
	if args, ok := itemMap["arguments"].(string); ok && args != "" {
		unknownParts = append(unknownParts, fmt.Sprintf("Arguments: %s", args))
	} else if argsMap, ok := itemMap["arguments"].(map[string]interface{}); ok {
		argsJSON, err := json.MarshalIndent(argsMap, "  ", "  ")
		if err == nil {
			argsStr := string(argsJSON)
			if len(argsStr) > 500 {
				argsStr = argsStr[:500] + "..."
			}
			unknownParts = append(unknownParts, fmt.Sprintf("Arguments:\n%s", argsStr))
		}
	}
	if len(unknownParts) > 1 || (len(unknownParts) == 1 && !strings.Contains(unknownParts[0], "content]")) {
		return strings.Join(unknownParts, "\n")
	}
	if len(unknownParts) == 1 {
		return unknownParts[0]
	}
	return ""
}

// parseMessageToBubble converts a message format (id, role, content) to a RawBubble.
func parseMessageToBubble(key, id, role string, data map[string]interface{}, sessionID string) (*RawBubble, error) {
	bubble := &RawBubble{
		BubbleID: messageBubbleID(key, id),
		ChatID:   sessionID,
		Type:     messageBubbleRoleType(role),
	}

	if content, ok := data["content"].([]interface{}); ok {
		bubble.Text = messageContentArrayToText(content)
	}

	if ts, ok := data["timestamp"].(float64); ok {
		bubble.Timestamp = normalizeTimestamp(int64(ts))
	} else if ts, ok := data["timestamp"].(int64); ok {
		bubble.Timestamp = normalizeTimestamp(ts)
	}

	return bubble, nil
}
