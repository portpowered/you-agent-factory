package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
)

type watcherCheckpoint struct {
	Handled []string `json:"handled"`
}

func (s *service) ResumeWatcherFacts(
	identity filesystemwatchers.WatchIdentity,
	authoritative *filesystemwatchers.WatcherFacts,
	resume *filesystemwatchers.WatcherFacts,
) (filesystemwatchers.WatcherFacts, error) {
	if err := validateWatchIdentity(identity); err != nil {
		return filesystemwatchers.WatcherFacts{}, err
	}

	switch {
	case resume == nil && authoritative == nil:
		return filesystemwatchers.WatcherFacts{Identity: identity}, nil
	case resume == nil:
		if authoritative.Identity != identity {
			return filesystemwatchers.WatcherFacts{}, fmt.Errorf(
				"%w: authoritative identity mismatch",
				filesystemwatchers.ErrStaleResumeFacts,
			)
		}
		if err := validateWatcherResumeFacts(identity, *authoritative); err != nil {
			return filesystemwatchers.WatcherFacts{}, err
		}
		return normalizeWatcherFacts(*authoritative)
	case authoritative == nil:
		if err := validateWatcherResumeFacts(identity, *resume); err != nil {
			return filesystemwatchers.WatcherFacts{}, err
		}
		return normalizeWatcherFacts(*resume)
	default:
		if authoritative.Identity != identity {
			return filesystemwatchers.WatcherFacts{}, fmt.Errorf(
				"%w: authoritative identity mismatch",
				filesystemwatchers.ErrStaleResumeFacts,
			)
		}
		if err := validateWatcherResumeFacts(identity, *authoritative); err != nil {
			return filesystemwatchers.WatcherFacts{}, err
		}
		if err := validateWatcherResumeFacts(identity, *resume); err != nil {
			return filesystemwatchers.WatcherFacts{}, err
		}
		if !watcherFactsEquivalent(*authoritative, *resume) {
			return filesystemwatchers.WatcherFacts{}, fmt.Errorf(
				"%w: resume contradicts authoritative watcher facts",
				filesystemwatchers.ErrStaleResumeFacts,
			)
		}
		return normalizeWatcherFacts(*authoritative)
	}
}

func (s *service) ValidateExpectedCursor(
	authoritative filesystemwatchers.WatcherFacts,
	expected filesystemwatchers.Cursor,
) error {
	if strings.TrimSpace(string(expected)) == "" {
		return nil
	}
	if authoritative.Cursor != expected {
		return fmt.Errorf(
			"%w: expected cursor %q does not match authoritative cursor %q",
			filesystemwatchers.ErrStaleResumeFacts,
			expected,
			authoritative.Cursor,
		)
	}
	return nil
}

func (s *service) WatcherFactsFromCursor(
	identity filesystemwatchers.WatchIdentity,
	cursor filesystemwatchers.Cursor,
	checkpoint string,
) (filesystemwatchers.WatcherFacts, error) {
	if err := validateWatchIdentity(identity); err != nil {
		return filesystemwatchers.WatcherFacts{}, err
	}
	facts := filesystemwatchers.WatcherFacts{
		Identity:   identity,
		Cursor:     cursor,
		Checkpoint: checkpoint,
	}
	if err := validateWatcherResumeFacts(identity, facts); err != nil {
		return filesystemwatchers.WatcherFacts{}, err
	}
	return normalizeWatcherFacts(facts)
}

func (s *service) newHandledIdentities(
	facts filesystemwatchers.WatcherFacts,
	persist filesystemwatchers.CursorFactsPersist,
) (handledIdentities, error) {
	if persist == nil {
		return nil, fmt.Errorf("filesystem watcher cursor persist collaborator is required")
	}
	handled, err := decodeHandledIdentities(facts.Checkpoint)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeWatcherFacts(facts)
	if err != nil {
		return nil, err
	}
	return &cursorBackedHandledIdentities{
		facts:   normalized,
		handled: handled,
		persist: persist,
	}, nil
}

type cursorBackedHandledIdentities struct {
	mu      sync.Mutex
	facts   filesystemwatchers.WatcherFacts
	handled map[filesystemwatchers.ObservationIdentity]struct{}
	persist filesystemwatchers.CursorFactsPersist
}

func (s *cursorBackedHandledIdentities) Contains(identity filesystemwatchers.ObservationIdentity) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.handled[identity]
	return ok
}

func (s *cursorBackedHandledIdentities) Record(identity filesystemwatchers.ObservationIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.handled[identity]; ok {
		return nil
	}

	updatedHandled := make(map[filesystemwatchers.ObservationIdentity]struct{}, len(s.handled)+1)
	for existing := range s.handled {
		updatedHandled[existing] = struct{}{}
	}
	updatedHandled[identity] = struct{}{}

	nextCursor, checkpoint, err := encodeWatcherCheckpoint(s.facts.Cursor, updatedHandled)
	if err != nil {
		return err
	}
	updated := s.facts
	updated.Cursor = nextCursor
	updated.Checkpoint = checkpoint
	if err := s.persist(updated); err != nil {
		return fmt.Errorf("%w: %v", filesystemwatchers.ErrCursorPersistFailed, err)
	}

	s.facts = updated
	s.handled = updatedHandled
	return nil
}

func validateWatchIdentity(identity filesystemwatchers.WatchIdentity) error {
	if strings.TrimSpace(identity.AutomationID) == "" || strings.TrimSpace(identity.WatchRoot) == "" {
		return fmt.Errorf(
			"%w: automation id and watch root are required",
			filesystemwatchers.ErrInvalidResumeFacts,
		)
	}
	return nil
}

func validateWatcherResumeFacts(
	identity filesystemwatchers.WatchIdentity,
	resume filesystemwatchers.WatcherFacts,
) error {
	if resume.Identity != identity {
		return fmt.Errorf("%w: resume identity mismatch", filesystemwatchers.ErrStaleResumeFacts)
	}
	if strings.TrimSpace(resume.Checkpoint) != "" {
		if _, err := decodeHandledIdentities(resume.Checkpoint); err != nil {
			return err
		}
	}
	if cursor := strings.TrimSpace(string(resume.Cursor)); cursor != "" {
		if _, err := strconv.ParseUint(cursor, 10, 64); err != nil {
			return fmt.Errorf(
				"%w: cursor %q is not a valid version token",
				filesystemwatchers.ErrInvalidResumeFacts,
				resume.Cursor,
			)
		}
	}
	if strings.TrimSpace(string(resume.Cursor)) != "" && strings.TrimSpace(resume.Checkpoint) == "" {
		return fmt.Errorf(
			"%w: cursor requires checkpoint",
			filesystemwatchers.ErrInvalidResumeFacts,
		)
	}
	if strings.TrimSpace(string(resume.Cursor)) == "" && strings.TrimSpace(resume.Checkpoint) != "" {
		return fmt.Errorf(
			"%w: checkpoint requires cursor",
			filesystemwatchers.ErrInvalidResumeFacts,
		)
	}
	return nil
}

func normalizeWatcherFacts(
	facts filesystemwatchers.WatcherFacts,
) (filesystemwatchers.WatcherFacts, error) {
	normalized := facts
	normalized.Identity.AutomationID = strings.TrimSpace(facts.Identity.AutomationID)
	normalized.Identity.WatchRoot = filepathSlash(strings.TrimSpace(facts.Identity.WatchRoot))
	normalized.Cursor = filesystemwatchers.Cursor(strings.TrimSpace(string(facts.Cursor)))
	normalized.Checkpoint = strings.TrimSpace(facts.Checkpoint)
	if normalized.Checkpoint != "" {
		if _, err := decodeHandledIdentities(normalized.Checkpoint); err != nil {
			return filesystemwatchers.WatcherFacts{}, err
		}
	}
	return normalized, nil
}

func watcherFactsEquivalent(
	left filesystemwatchers.WatcherFacts,
	right filesystemwatchers.WatcherFacts,
) bool {
	return left.Identity == right.Identity &&
		left.Cursor == right.Cursor &&
		left.Checkpoint == right.Checkpoint
}

func decodeHandledIdentities(checkpoint string) (map[filesystemwatchers.ObservationIdentity]struct{}, error) {
	if strings.TrimSpace(checkpoint) == "" {
		return make(map[filesystemwatchers.ObservationIdentity]struct{}), nil
	}
	var payload watcherCheckpoint
	if err := json.Unmarshal([]byte(checkpoint), &payload); err != nil {
		return nil, fmt.Errorf("%w: decode checkpoint: %v", filesystemwatchers.ErrInvalidResumeFacts, err)
	}
	handled := make(map[filesystemwatchers.ObservationIdentity]struct{}, len(payload.Handled))
	for _, identity := range payload.Handled {
		trimmed := strings.TrimSpace(identity)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: checkpoint contains empty identity", filesystemwatchers.ErrInvalidResumeFacts)
		}
		handled[filesystemwatchers.ObservationIdentity(trimmed)] = struct{}{}
	}
	return handled, nil
}

func encodeWatcherCheckpoint(
	current filesystemwatchers.Cursor,
	handled map[filesystemwatchers.ObservationIdentity]struct{},
) (filesystemwatchers.Cursor, string, error) {
	identities := make([]string, 0, len(handled))
	for identity := range handled {
		identities = append(identities, string(identity))
	}
	sort.Strings(identities)
	payload, err := json.Marshal(watcherCheckpoint{Handled: identities})
	if err != nil {
		return "", "", fmt.Errorf("encode watcher checkpoint: %w", err)
	}
	nextCursor, err := nextWatcherCursor(current)
	if err != nil {
		return "", "", err
	}
	return nextCursor, string(payload), nil
}

func nextWatcherCursor(current filesystemwatchers.Cursor) (filesystemwatchers.Cursor, error) {
	if strings.TrimSpace(string(current)) == "" {
		return filesystemwatchers.Cursor("1"), nil
	}
	version, err := strconv.ParseUint(string(current), 10, 64)
	if err != nil {
		return "", fmt.Errorf("%w: cursor %q is not a valid version token", filesystemwatchers.ErrInvalidResumeFacts, current)
	}
	return filesystemwatchers.Cursor(strconv.FormatUint(version+1, 10)), nil
}

func filepathSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
