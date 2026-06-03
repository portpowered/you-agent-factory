package cursorstorage

import (
	"fmt"
)

// LoadSessionFromStoreDB loads session data from a single store.db file.
func LoadSessionFromStoreDB(dbPath string) (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, SessionTokenUsage, error) {
	db, err := OpenDatabase(dbPath)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to open store.db: %w", err)
	}
	defer func() { _ = db.Close() }()

	blobs, err := QueryBlobsTable(db)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to query blobs table: %w", err)
	}

	meta, err := QueryMetaTable(db)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to query meta table: %w", err)
	}

	sessionID := extractSessionIDFromPath(dbPath)
	bubbles := make(map[string]*RawBubble)
	var composers []*RawComposer
	contexts := make(map[string][]*MessageContext)
	var sessionTokenUsage SessionTokenUsage

	jsonParseFailures := loadSessionBlobs(blobs, sessionID, bubbles, &composers, &sessionTokenUsage)
	if jsonParseFailures > 0 {
		LogWarn("Failed to parse %d/%d blobs as JSON", jsonParseFailures, len(blobs))
	}

	var sessionMeta sessionMetaFields
	metaParseFailures := loadSessionMeta(meta, contexts, &sessionTokenUsage, &sessionMeta)
	if metaParseFailures > 0 {
		LogWarn("Failed to parse %d/%d meta entries as JSON", metaParseFailures, len(meta))
	}

	applySessionMetadata(bubbles, composers, sessionMeta)

	LogInfo("LoadSessionFromStoreDB summary: %d blobs queried, %d meta queried, %d bubbles extracted, %d composers extracted, %d contexts extracted",
		len(blobs), len(meta), len(bubbles), len(composers), len(contexts))

	return bubbles, composers, contexts, sessionTokenUsage, nil
}

func loadSessionBlobs(
	blobs []BlobEntry,
	sessionID string,
	bubbles map[string]*RawBubble,
	composers *[]*RawComposer,
	tokenUsage *SessionTokenUsage,
) int {
	jsonParseFailures := 0
	for i, blob := range blobs {
		data, earlyBubble, ok := decodeBlobEntryValue(blob, i, sessionID, &jsonParseFailures)
		if !ok {
			continue
		}
		if earlyBubble != nil {
			bubbles[earlyBubble.BubbleID] = earlyBubble
			continue
		}
		ingestDecodedBlobData(blob, i, data, sessionID, bubbles, composers, tokenUsage)
	}
	return jsonParseFailures
}

func loadSessionMeta(
	meta []MetaEntry,
	contexts map[string][]*MessageContext,
	tokenUsage *SessionTokenUsage,
	sessionMeta *sessionMetaFields,
) int {
	metaJSONParseFailures := 0
	for i, entry := range meta {
		data, ok := decodeMetaEntryValue(entry, i, &metaJSONParseFailures)
		if !ok {
			continue
		}

		if i < 3 {
			keys := make([]string, 0, len(data))
			for k := range data {
				keys = append(keys, k)
			}
			LogInfo("Meta %d (key='%s') parsed successfully. Available fields: %v", i+1, entry.Key, keys)
		}
		mergeSessionTokenUsage(tokenUsage, tokenUsageFromData(data))

		fields := extractSessionMetaFromEntry(entry, data)
		if fields.createdAt > 0 {
			sessionMeta.createdAt = fields.createdAt
		}
		if fields.agentID != "" {
			sessionMeta.agentID = fields.agentID
		}
		if fields.name != "" {
			sessionMeta.name = fields.name
		}

		if _, ok := data["contextId"].(string); ok {
			context, err := parseContextFromData(entry.Key, data)
			if err == nil {
				composerID := context.ComposerID
				if composerID == "" {
					if cid, ok := data["composerId"].(string); ok {
						composerID = cid
						context.ComposerID = composerID
					}
				}
				if composerID != "" {
					contexts[composerID] = append(contexts[composerID], context)
				}
			}
		}
	}
	return metaJSONParseFailures
}

func applySessionMetadata(bubbles map[string]*RawBubble, composers []*RawComposer, sessionMeta sessionMetaFields) {
	if sessionMeta.createdAt > 0 {
		for bubbleID, bubble := range bubbles {
			if bubble.Timestamp == 0 {
				bubble.Timestamp = sessionMeta.createdAt
				bubbles[bubbleID] = bubble
				LogInfo("Applied session createdAt (%d) to bubble %s (was missing timestamp)", sessionMeta.createdAt, bubbleID)
			}
		}
	}

	if sessionMeta.createdAt == 0 && sessionMeta.name == "" {
		return
	}

	for i := range composers {
		if sessionMeta.createdAt > 0 && composers[i].CreatedAt == 0 {
			composers[i].CreatedAt = sessionMeta.createdAt
			LogInfo("Applied session createdAt (%d) to composer %s", sessionMeta.createdAt, composers[i].ComposerID)
		}
		if sessionMeta.name != "" && composers[i].Name == "" {
			composers[i].Name = sessionMeta.name
			LogInfo("Applied session name (%s) to composer %s", sessionMeta.name, composers[i].ComposerID)
		}
	}
}

// LoadAllSessionsFromAgentStorage loads all sessions from all store.db files.
func (r *AgentStorageReader) LoadAllSessionsFromAgentStorage() (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, error) {
	allBubbles := make(map[string]*RawBubble)
	var allComposers []*RawComposer
	allContexts := make(map[string][]*MessageContext)

	for _, dbPath := range r.storeDBPaths {
		bubbles, composers, contexts, _, err := LoadSessionFromStoreDB(dbPath)
		if err != nil {
			LogWarn("Failed to load session from %s: %v", dbPath, err)
			continue
		}

		for id, bubble := range bubbles {
			allBubbles[id] = bubble
		}
		allComposers = append(allComposers, composers...)
		LogInfo("Loaded from %s: %d bubbles, %d composers, %d context entries", dbPath, len(bubbles), len(composers), len(contexts))

		for composerID, ctxList := range contexts {
			allContexts[composerID] = append(allContexts[composerID], ctxList...)
		}
	}

	LogInfo("Total loaded from agent storage: %d bubbles, %d composers, %d context groups", len(allBubbles), len(allComposers), len(allContexts))
	return allBubbles, allComposers, allContexts, nil
}
