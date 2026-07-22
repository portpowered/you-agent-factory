package cursor

import (
	"fmt"
	"sort"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// SessionData is parsed cursor-agent CLI storage for one session store.db file.
type SessionData struct {
	SessionID   string
	StoreDBPath string
	Bubbles     map[string]*RawBubble
	Composers   []*RawComposer
	Contexts    map[string][]*MessageContext
	ParseStats  SessionParseStats
	TokenUsage  SessionTokenUsage
}

// SessionParseStats summarizes readable vs unavailable blob records while parsing.
type SessionParseStats struct {
	BlobCount            int
	ReadableBlobCount    int
	UnavailableBlobCount int
	MalformedBlobCount   int
	MetaCount            int
	MalformedMetaCount   int
}

// LoadSessionData opens a resolved store.db and parses bubbles, composers, and contexts in-process.
func LoadSessionData(files providersessions.FileSystem, openSQLDatabase providersessions.CursorOpenSQLDatabase, resolved ResolvedStoreDB) (*SessionData, error) {
	if resolved.AbsolutePath == "" {
		return nil, fmt.Errorf("cursor session path is empty")
	}
	info, err := files.Stat(resolved.AbsolutePath)
	if err != nil {
		return nil, fmt.Errorf("stat cursor session store: %w", err)
	}
	_ = info

	bubbles, composers, contexts, stats, tokenUsage, err := loadSessionFromStoreDBWithStats(files, openSQLDatabase, resolved.AbsolutePath)
	if err != nil {
		return nil, err
	}
	return &SessionData{
		SessionID:   resolved.SessionID,
		StoreDBPath: resolved.AbsolutePath,
		Bubbles:     bubbles,
		Composers:   composers,
		Contexts:    contexts,
		ParseStats:  stats,
		TokenUsage:  tokenUsage,
	}, nil
}

// OrderedBubbles returns bubbles sorted by timestamp then bubble id for stable transcript ordering.
func (s *SessionData) OrderedBubbles() []*RawBubble {
	if s == nil || len(s.Bubbles) == 0 {
		return nil
	}
	ordered := make([]*RawBubble, 0, len(s.Bubbles))
	for _, bubble := range s.Bubbles {
		ordered = append(ordered, bubble)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i].GetTimestamp()
		right := ordered[j].GetTimestamp()
		if !left.Equal(right) {
			return left.Before(right)
		}
		return ordered[i].BubbleID < ordered[j].BubbleID
	})
	return ordered
}

func loadSessionFromStoreDBWithStats(files providersessions.FileSystem, openSQLDatabase providersessions.CursorOpenSQLDatabase, dbPath string) (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, SessionParseStats, SessionTokenUsage, error) {
	db, err := OpenDatabase(files, openSQLDatabase, dbPath)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}
	defer func() { _ = db.Close() }()

	blobs, err := QueryBlobsTable(db)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}
	meta, err := QueryMetaTable(db)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}

	bubbles, composers, contexts, tokenUsage, err := LoadSessionFromStoreDB(files, openSQLDatabase, dbPath)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}

	stats := SessionParseStats{
		BlobCount:            len(blobs),
		ReadableBlobCount:    len(bubbles),
		UnavailableBlobCount: max(0, len(blobs)-len(bubbles)),
		MetaCount:            len(meta),
	}
	return bubbles, composers, contexts, stats, tokenUsage, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
