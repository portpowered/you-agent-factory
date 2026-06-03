package cursorstorage

import (
	"fmt"
	"os"
	"sort"
	"time"
)

// SessionData is parsed cursor-agent CLI storage for one session store.db file.
type SessionData struct {
	SessionID          string
	StoreDBPath        string
	Bubbles            map[string]*RawBubble
	Composers          []*RawComposer
	Contexts           map[string][]*MessageContext
	ParseStats         SessionParseStats
	TokenUsage         SessionTokenUsage
}

// SessionParseStats summarizes readable vs unavailable blob records while parsing.
type SessionParseStats struct {
	BlobCount              int
	ReadableBlobCount      int
	UnavailableBlobCount   int
	MalformedBlobCount     int
	MetaCount              int
	MalformedMetaCount     int
}

// LoadSessionData opens a resolved store.db and parses bubbles, composers, and contexts in-process.
func LoadSessionData(resolved ResolvedStoreDB) (*SessionData, error) {
	if resolved.AbsolutePath == "" {
		return nil, fmt.Errorf("cursor session path is empty")
	}
	info, err := os.Stat(resolved.AbsolutePath)
	if err != nil {
		return nil, fmt.Errorf("stat cursor session store: %w", err)
	}
	_ = info

	bubbles, composers, contexts, stats, tokenUsage, err := loadSessionFromStoreDBWithStats(resolved.AbsolutePath)
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

// LatestActivityAt returns the newest bubble or composer timestamp when available.
func (s *SessionData) LatestActivityAt() *time.Time {
	if s == nil {
		return nil
	}
	var latest time.Time
	var found bool
	for _, bubble := range s.Bubbles {
		ts := bubble.GetTimestamp()
		if ts.IsZero() {
			continue
		}
		if !found || ts.After(latest) {
			latest = ts
			found = true
		}
	}
	for _, composer := range s.Composers {
		ts := composer.GetLastUpdatedAt()
		if ts.IsZero() {
			ts = composer.GetCreatedAt()
		}
		if ts.IsZero() {
			continue
		}
		if !found || ts.After(latest) {
			latest = ts
			found = true
		}
	}
	if !found {
		return nil
	}
	utc := latest.UTC()
	return &utc
}
