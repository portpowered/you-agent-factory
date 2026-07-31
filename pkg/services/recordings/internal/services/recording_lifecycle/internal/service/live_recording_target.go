package service

import (
	"fmt"
	"path/filepath"
	"strings"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

const liveRecordingSessionToken = "__factory_session_id__"

type liveRecordingTargetPlanner struct {
	clock recordings.RecordingClock
	newID recordings.RecordingIdentityGenerator
	join  recordings.RecordingPathJoiner
}

// NewTargetPlanner constructs the Recordings-owned target policy from exact
// mechanics selected by Wire.
func NewTargetPlanner(
	clock recordings.RecordingClock,
	newID recordings.RecordingIdentityGenerator,
	join recordings.RecordingPathJoiner,
) recordings.LiveRecordingTargetPlanner {
	return &liveRecordingTargetPlanner{clock: clock, newID: newID, join: join}
}

func (planner *liveRecordingTargetPlanner) PlanLiveRecordingTarget(
	request recordings.LiveRecordingTargetRequest,
) (recordings.LiveRecordingTarget, error) {
	homeDir := strings.TrimSpace(request.HomeDir)
	if homeDir == "" {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("resolve user home: home directory is required")
	}
	if planner == nil || planner.clock == nil {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("Recordings live target clock is required")
	}
	if planner.newID == nil {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("Recordings live target identity generator is required")
	}
	if planner.join == nil {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("Recordings live target path joiner is required")
	}

	now := planner.clock.Now()
	identity := strings.TrimSpace(planner.newID())
	if identity == "" {
		return recordings.LiveRecordingTarget{}, fmt.Errorf("Recordings live target identity generator returned an empty identity")
	}
	recordingsRoot := planner.join(homeDir, ".you-agent-factory", "recordings")
	recordingsDir := planner.join(recordingsRoot, now.Format("2006-01"), now.Format("2006-01-02"))
	filename := fmt.Sprintf(
		"factory-session-%s-%s-%s.json",
		liveRecordingSessionToken,
		now.Format("150405"),
		identity,
	)
	servicePath := planner.join(recordingsDir, filename)
	reportedPath := servicePath
	if sessionID := strings.TrimSpace(request.ReportedSessionID); sessionID != "" {
		reportedPath = strings.ReplaceAll(servicePath, liveRecordingSessionToken, sessionID)
	}
	return recordings.LiveRecordingTarget{ServicePath: servicePath, ReportedPath: reportedPath}, nil
}

var _ recordings.RecordingPathJoiner = filepath.Join
