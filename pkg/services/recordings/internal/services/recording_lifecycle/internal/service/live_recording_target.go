package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

type liveRecordingTargetPlanner struct {
	clock   recordings.RecordingClock
	reserve recordings.RecordingNamedPathReserver
	join    recordings.RecordingPathJoiner
}

// NewTargetPlanner constructs the Recordings-owned automatic target planner.
// The planner consumes the canonical Factory Session UUID supplied by the
// caller and reserves that exact name through the shared dated-path policy.
func NewTargetPlanner(
	clock recordings.RecordingClock,
	reserve recordings.RecordingNamedPathReserver,
	join recordings.RecordingPathJoiner,
) recordings.LiveRecordingTargetPlanner {
	return &liveRecordingTargetPlanner{clock: clock, reserve: reserve, join: join}
}

func (planner *liveRecordingTargetPlanner) PlanLiveRecordingTarget(
	request recordings.LiveRecordingTargetRequest,
) (recordings.LiveRecordingTarget, error) {
	if planner == nil || planner.clock == nil {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("Recordings live target clock is required")
	}
	if planner.reserve == nil {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("Recordings live target named path reserver is required")
	}
	if planner.join == nil {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("Recordings live target path joiner is required")
	}
	homeDir := strings.TrimSpace(request.HomeDir)
	if homeDir == "" {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("Recordings live target home directory is required")
	}
	canonicalID := strings.TrimSpace(request.CanonicalSessionID)
	if canonicalID == "" || !isUUID(canonicalID) {
		return recordings.LiveRecordingTarget{}, fmt.Errorf(
			"Recordings live target canonical Factory Session ID %q must be a UUID",
			canonicalID,
		)
	}
	now := planner.clock.Now().UTC()
	recordingsRoot := planner.join(homeDir, ".you-agent-factory", "recordings")
	reservedPath, err := planner.reserve.ReserveNamed(recordingsRoot, now, canonicalID, ".json")
	if err != nil {
		return recordings.LiveRecordingTarget{}, fmt.Errorf(
			"reserve recording target for canonical Factory Session ID %q: %w",
			canonicalID,
			err,
		)
	}
	wantPath := planner.join(
		recordingsRoot,
		now.Format("2006"),
		now.Format("01"),
		now.Format("02"),
		canonicalID+".json",
	)
	if reservedPath != wantPath {
		return recordings.LiveRecordingTarget{}, fmt.Errorf(
			"reserve recording target for canonical Factory Session ID %q returned %q; want exact path %q",
			canonicalID,
			reservedPath,
			wantPath,
		)
	}
	return recordings.LiveRecordingTarget{ServicePath: reservedPath, ReportedPath: reservedPath}, nil
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

var _ recordings.RecordingPathJoiner = filepath.Join
