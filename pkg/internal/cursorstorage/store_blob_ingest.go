package cursorstorage

// ingestDecodedBlobData maps decoded blob JSON into bubbles, composers, and token usage.
func ingestDecodedBlobData(
	blob BlobEntry,
	index int,
	data map[string]interface{},
	sessionID string,
	bubbles map[string]*RawBubble,
	composers *[]*RawComposer,
	tokenUsage *SessionTokenUsage,
) {
	if index < 5 {
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		LogInfo("Blob %d (key='%s') parsed successfully. Available fields: %v", index+1, blob.Key, keys)
	}
	mergeSessionTokenUsage(tokenUsage, tokenUsageFromData(data))

	if _, ok := data["bubbleId"].(string); ok {
		bubble, err := parseBubbleFromData(blob.Key, data, sessionID)
		if err == nil {
			bubbles[bubble.BubbleID] = bubble
		}
		return
	}

	if id, ok := data["id"].(string); ok {
		if role, hasRole := data["role"].(string); hasRole {
			bubble, err := parseMessageToBubble(blob.Key, id, role, data, sessionID)
			if err == nil {
				bubbles[bubble.BubbleID] = bubble
				LogInfo("Blob %d converted message (id='%s', role='%s') to bubble (bubbleId='%s')", index+1, id, role, bubble.BubbleID)
			} else {
				LogWarn("Blob %d failed to convert message to bubble: %v", index+1, err)
			}
		}
		return
	}

	if role, hasRole := data["role"].(string); hasRole {
		generatedID := blob.Key
		if len(blob.Key) > 16 {
			generatedID = blob.Key[:16]
		}
		bubble, err := parseMessageToBubble(blob.Key, generatedID, role, data, sessionID)
		if err == nil {
			bubbles[bubble.BubbleID] = bubble
			LogInfo("Blob %d converted message (no id, role='%s') to bubble (bubbleId='%s')", index+1, role, bubble.BubbleID)
		} else {
			LogWarn("Blob %d failed to convert message to bubble: %v", index+1, err)
		}
		return
	}

	if composerID, ok := data["composerId"].(string); ok {
		composer, err := parseComposerFromData(blob.Key, data)
		if err != nil {
			LogWarn("Failed to parse composer from blob key %s: %v", blob.Key, err)
			return
		}
		if composer.ComposerID == "" {
			LogWarn("Composer parsed but missing composerId. Blob key: %s", blob.Key)
			return
		}
		composer.ComposerID = composerID
		headerCount := len(composer.FullConversationHeadersOnly)
		LogInfo("Parsed composer %s: %d headers, name='%s'", composer.ComposerID, headerCount, composer.Name)
		*composers = append(*composers, composer)
	}
}
