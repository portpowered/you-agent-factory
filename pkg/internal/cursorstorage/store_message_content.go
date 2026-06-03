package cursorstorage

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
