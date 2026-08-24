package service

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordedsessioninventory "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recorded_session_inventory"
)

type directoryReader func(string) ([]fs.DirEntry, error)

// Service implements the private Recordings history-inventory capability.
type Service struct {
	readDir      directoryReader
	replayInputs recordings.ReplayInputLoader
	logger       logging.Logger
}

var _ recordedsessioninventory.Service = (*Service)(nil)

// New constructs an inert recording inventory. Directory reads and replay
// decoding begin only when ListRecordedSessions is called.
func New(
	readDir func(string) ([]fs.DirEntry, error),
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
) *Service {
	return &Service{
		readDir:      directoryReader(readDir),
		replayInputs: replayInputs,
		logger:       logging.EnsureLogger(logger),
	}
}

func (inventory *Service) ListRecordedSessions(
	request recordings.RecordedSessionInventoryRequest,
) (recordings.RecordedSessionInventoryResult, error) {
	root := strings.TrimSpace(request.RecordingRoot)
	if root == "" {
		return recordings.RecordedSessionInventoryResult{}, fmt.Errorf("recording root is required")
	}
	if inventory == nil || inventory.readDir == nil {
		return recordings.RecordedSessionInventoryResult{}, fmt.Errorf("recording directory reader is required")
	}
	if inventory.replayInputs == nil {
		return recordings.RecordedSessionInventoryResult{}, fmt.Errorf("recording replay input loader is required")
	}

	inventory.logger.Info(
		"recordings session inventory accepted",
		"operation", "list_recorded_sessions",
	)
	paths, err := inventory.recordingPaths(root)
	if err != nil {
		inventory.logOutcome("failure", 0)
		return recordings.RecordedSessionInventoryResult{}, err
	}

	summaries := make([]recordings.RecordedSessionSummary, 0, len(paths))
	for _, path := range paths {
		summary, err := inventory.summaryForPath(root, path)
		if err != nil {
			inventory.logOutcome("failure", len(summaries))
			return recordings.RecordedSessionInventoryResult{}, err
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(left, right int) bool {
		if summaries[left].FactorySessionID != summaries[right].FactorySessionID {
			return summaries[left].FactorySessionID < summaries[right].FactorySessionID
		}
		return summaries[left].ArtifactReference < summaries[right].ArtifactReference
	})
	inventory.logOutcome("success", len(summaries))
	return recordings.RecordedSessionInventoryResult{Sessions: summaries}, nil
}

func (inventory *Service) recordingPaths(root string) ([]string, error) {
	paths := make([]string, 0)
	if err := inventory.walkRecordingRoot(root, root, true, &paths); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func (inventory *Service) walkRecordingRoot(
	root string,
	directory string,
	topLevel bool,
	paths *[]string,
) error {
	entries, err := inventory.readDir(directory)
	if err != nil {
		if topLevel && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read recording directory: %w", err)
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})
	for _, entry := range entries {
		if entry == nil || entry.Name() == "" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if entry.IsDir() {
			if err := inventory.walkRecordingRoot(root, path, false, paths); err != nil {
				return err
			}
			continue
		}
		if entry.Type()&fs.ModeSymlink != 0 || !isRecordingArtifact(root, path) {
			continue
		}
		*paths = append(*paths, path)
	}
	return nil
}

func (inventory *Service) summaryForPath(
	root string,
	path string,
) (recordings.RecordedSessionSummary, error) {
	reference, err := filepath.Rel(root, path)
	if err != nil {
		return recordings.RecordedSessionSummary{}, fmt.Errorf("reference recorded session artifact: %w", err)
	}
	reference = filepath.ToSlash(reference)
	format, err := recordingFormat(path)
	if err != nil {
		return recordings.RecordedSessionSummary{}, fmt.Errorf("classify recorded session artifact %q: %w", reference, err)
	}
	input, err := inventory.replayInputs.LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
	if err != nil {
		return recordings.RecordedSessionSummary{}, fmt.Errorf("load recorded session artifact %q: %w", reference, err)
	}
	id, err := canonicalSessionID(input, format, path)
	if err != nil {
		return recordings.RecordedSessionSummary{}, fmt.Errorf("identify recorded session artifact %q: %w", reference, err)
	}
	return recordings.RecordedSessionSummary{
		FactorySessionID:  id,
		ArtifactReference: reference,
		Format:            format,
	}, nil
}

func (inventory *Service) logOutcome(outcome string, count int) {
	inventory.logger.Info(
		"recordings session inventory outcome",
		"operation", "list_recorded_sessions",
		"outcome", outcome,
		"recorded_session_count", count,
	)
}

func isRecordingArtifact(root, path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".json" && extension != ".jsonl" {
		return false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	switch len(parts) {
	case 4:
		return isCalendarPart(parts[0], 4, 1, 9999) &&
			isCalendarPart(parts[1], 2, 1, 12) &&
			isCalendarPart(parts[2], 2, 1, 31)
	case 3:
		return isYearMonth(parts[0]) && isYearMonthDay(parts[1])
	default:
		return false
	}
}

func recordingFormat(path string) (recordings.RecordedSessionFormat, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return recordings.RecordedSessionFormatV1JSON, nil
	case ".jsonl":
		return recordings.RecordedSessionFormatV2JSONL, nil
	default:
		return "", fmt.Errorf("unsupported extension")
	}
}

func canonicalSessionID(
	input recordings.LoadReplayInputResult,
	format recordings.RecordedSessionFormat,
	path string,
) (string, error) {
	if input.Portable != nil && input.Legacy != nil {
		return "", fmt.Errorf("replay input returned both portable and legacy identities")
	}
	if input.Portable != nil {
		return requireSessionID(input.Portable.Session.ID)
	}
	if input.Legacy == nil {
		return "", fmt.Errorf("replay input did not contain a recording identity")
	}

	id, err := legacySessionID(input.Legacy)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	if format != recordings.RecordedSessionFormatV2JSONL {
		return "", fmt.Errorf("legacy replay input did not contain a Factory Session UUID")
	}

	// REC-4's v2 header owns the session identity and its filename is the
	// canonical UUID. The shared loader normalizes the v2 stream into the
	// legacy replay artifact shape, so an empty-event v2 artifact has no event
	// context from which to recover the header identity.
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if _, err := uuid.Parse(base); err != nil {
		return "", fmt.Errorf("v2 artifact filename %q is not a canonical Factory Session UUID", base)
	}
	return base, nil
}

func legacySessionID(artifact *recordings.ReplayArtifact) (string, error) {
	var sessionID string
	for _, event := range artifact.Events {
		if event.Context.SessionID == nil {
			continue
		}
		candidate := strings.TrimSpace(*event.Context.SessionID)
		if candidate == "" {
			continue
		}
		if sessionID != "" && sessionID != candidate {
			return "", fmt.Errorf("legacy replay input contains multiple Factory Session UUIDs")
		}
		sessionID = candidate
	}
	return sessionID, nil
}

func requireSessionID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return "", fmt.Errorf("Factory Session UUID is empty")
	}
	return id, nil
}

func isYearMonth(value string) bool {
	parts := strings.Split(value, "-")
	return len(parts) == 2 && isCalendarPart(parts[0], 4, 1, 9999) && isCalendarPart(parts[1], 2, 1, 12)
}

func isYearMonthDay(value string) bool {
	parts := strings.Split(value, "-")
	return len(parts) == 3 && isCalendarPart(parts[0], 4, 1, 9999) &&
		isCalendarPart(parts[1], 2, 1, 12) && isCalendarPart(parts[2], 2, 1, 31)
}

func isCalendarPart(value string, width, minimum, maximum int) bool {
	if len(value) != width {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	number, err := strconv.Atoi(value)
	return err == nil && number >= minimum && number <= maximum
}
