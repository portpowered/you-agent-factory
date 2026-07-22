package recordings

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const liveRecordingSessionToken = "__factory_session_id__"

// RecordingClock is the exact time source consumed by live recording target
// planning. Wire supplies the production clock.
type RecordingClock interface {
	Now() time.Time
}

// RecordingIdentityGenerator supplies an opaque uniqueness token for a live
// recording filename.
type RecordingIdentityGenerator func() string

// RecordingPathJoiner supplies platform-specific path joining mechanics.
type RecordingPathJoiner func(...string) string

// LiveRecordingTargetRequest identifies the customer edge used to place and
// report one automatically generated live recording.
type LiveRecordingTargetRequest struct {
	HomeDir           string
	ReportedSessionID string
}

// LiveRecordingTarget carries the runtime template path and the customer path
// reported for the selected Factory Session.
type LiveRecordingTarget struct {
	ServicePath  string
	ReportedPath string
}

// LiveRecordingTargetPlanner owns default live-recording layout, naming, and
// session-token interpretation.
type LiveRecordingTargetPlanner interface {
	PlanLiveRecordingTarget(LiveRecordingTargetRequest) (LiveRecordingTarget, error)
}

// LiveRecordingTargetPlannerFunc adapts a programmable exact operation.
type LiveRecordingTargetPlannerFunc func(LiveRecordingTargetRequest) (LiveRecordingTarget, error)

func (fn LiveRecordingTargetPlannerFunc) PlanLiveRecordingTarget(request LiveRecordingTargetRequest) (LiveRecordingTarget, error) {
	return fn(request)
}

type liveRecordingTargetPlanner struct {
	clock RecordingClock
	newID RecordingIdentityGenerator
	join  RecordingPathJoiner
}

// NewLiveRecordingTargetPlanner constructs the Recordings-owned target policy
// from exact mechanics selected by Wire.
func NewLiveRecordingTargetPlanner(
	clock RecordingClock,
	newID RecordingIdentityGenerator,
	join RecordingPathJoiner,
) LiveRecordingTargetPlanner {
	return &liveRecordingTargetPlanner{clock: clock, newID: newID, join: join}
}

func (planner *liveRecordingTargetPlanner) PlanLiveRecordingTarget(
	request LiveRecordingTargetRequest,
) (LiveRecordingTarget, error) {
	homeDir := strings.TrimSpace(request.HomeDir)
	if homeDir == "" {
		return LiveRecordingTarget{}, fmt.Errorf("resolve user home: home directory is required")
	}
	if planner == nil || planner.clock == nil {
		return LiveRecordingTarget{}, fmt.Errorf("Recordings live target clock is required")
	}
	if planner.newID == nil {
		return LiveRecordingTarget{}, fmt.Errorf("Recordings live target identity generator is required")
	}
	if planner.join == nil {
		return LiveRecordingTarget{}, fmt.Errorf("Recordings live target path joiner is required")
	}

	now := planner.clock.Now()
	identity := strings.TrimSpace(planner.newID())
	if identity == "" {
		return LiveRecordingTarget{}, fmt.Errorf("Recordings live target identity generator returned an empty identity")
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
	return LiveRecordingTarget{ServicePath: servicePath, ReportedPath: reportedPath}, nil
}

var _ RecordingPathJoiner = filepath.Join
