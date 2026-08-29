package internal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// resumeSourceCanonicalSessionID extracts only the canonical identity that a
// resumed runtime may retain for metrics. Public selectors and historical
// aliases are deliberately not promoted to a canonical scope.
func resumeSourceCanonicalSessionID(input recordings.LoadReplayInputResult) (string, error) {
	if input.Portable != nil && input.Legacy != nil {
		return "", fmt.Errorf("replay input returned both portable and legacy identities")
	}

	if metadataID := strings.TrimSpace(replayInputMetadataID(input.Metadata)); isCanonicalSessionUUID(metadataID) {
		for _, candidate := range replayInputCanonicalIDs(input) {
			if candidate != metadataID {
				return "", fmt.Errorf("replay input contains conflicting canonical Factory Session identities")
			}
		}
		return metadataID, nil
	}

	canonicalIDs := replayInputCanonicalIDs(input)
	if len(canonicalIDs) > 1 {
		return "", fmt.Errorf("replay input contains conflicting Factory Session identities")
	}
	if len(canonicalIDs) == 1 {
		return canonicalIDs[0], nil
	}
	return "", nil
}

// resumeSourceCanonicalSessionIDForPath combines identities carried by the
// decoded input with the canonical name of a Recordings-owned automatic
// target. Legacy JSON artifacts can contain only the public ~default event
// scope, while the automatic dated target already preserves the canonical
// UUID in its filename. Arbitrary explicit paths are intentionally ignored.
func resumeSourceCanonicalSessionIDForPath(
	input recordings.LoadReplayInputResult,
	path string,
) (string, error) {
	inputID, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		return "", err
	}
	pathID := automaticRecordingPathCanonicalSessionID(path)
	if inputID != "" && pathID != "" && inputID != pathID {
		return "", fmt.Errorf("replay input contains conflicting canonical Factory Session identities")
	}
	if inputID != "" {
		return inputID, nil
	}
	return pathID, nil
}

func automaticRecordingPathCanonicalSessionID(path string) string {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "." || !strings.EqualFold(filepath.Ext(cleanPath), ".json") {
		return ""
	}
	day := filepath.Dir(cleanPath)
	month := filepath.Dir(day)
	year := filepath.Dir(month)
	recordingsRoot := filepath.Dir(year)
	if strings.ToLower(filepath.Base(recordingsRoot)) != "recordings" ||
		strings.ToLower(filepath.Base(filepath.Dir(recordingsRoot))) != ".you-agent-factory" ||
		!isDatePathComponent(filepath.Base(year), 4) ||
		!isDatePathComponent(filepath.Base(month), 2) ||
		!isDatePathComponent(filepath.Base(day), 2) {
		return ""
	}
	canonicalID := strings.TrimSuffix(filepath.Base(cleanPath), filepath.Ext(cleanPath))
	if !isCanonicalSessionUUID(canonicalID) {
		return ""
	}
	return canonicalID
}

func isDatePathComponent(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func replayInputCanonicalIDs(input recordings.LoadReplayInputResult) []string {
	ids := make([]string, 0, 1)
	appendID := func(value string) {
		value = strings.TrimSpace(value)
		if !isCanonicalSessionUUID(value) {
			return
		}
		for _, existing := range ids {
			if existing == value {
				return
			}
		}
		ids = append(ids, value)
	}
	if input.Portable != nil {
		appendID(input.Portable.Session.ID)
	}
	if input.Legacy != nil {
		for _, event := range input.Legacy.Events {
			if event.Context.SessionID != nil {
				appendID(*event.Context.SessionID)
			}
		}
	}
	return ids
}

func replayInputMetadataID(metadata *recordings.ReplayInputMetadata) string {
	if metadata == nil {
		return ""
	}
	return metadata.FactorySessionID
}

func isCanonicalSessionUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}
