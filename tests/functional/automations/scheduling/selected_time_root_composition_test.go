package automations

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F-T01–F-T09 share one immutable default-role host. Sequencing is deliberate:
// each A/B stop/peer journey advances the same selected scheduler, so separate
// parallel leaves would change one another's eligibility. The whole journey
// runs in parallel with independent legacy/ingress hosts.
func TestAutomationsSelectedTimeControlsWorkAndJoinedShutdown(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	facts := platformclock.NewDeterministic(base.Add(time.Hour), time.Millisecond)
	scheduler := &selectedTimeScheduler{Deterministic: platformclock.NewDeterministic(base, time.Millisecond), registered: make(chan selectedTimeWait, 256), readiness: make(chan struct{}, 256)}
	files := &selectedTimeFiles{empty: make(map[string]int), reads: make(chan string, 16), walked: make(chan string, 64), admissions: make(map[string]int)}
	dirA, routeA := newScriptCycleFactory(t)
	dirB, routeB := newScriptCycleFactory(t)
	router := scriptCycleRouter{routes: map[string]*scriptCycleRoute{filepath.Clean(dirA): routeA, filepath.Clean(dirB): routeB}}
	cronRoutes := map[string]chan work.FactorySubmissionRecord{
		"owned-cron-A":      make(chan work.FactorySubmissionRecord, 8),
		"owned-cron-B":      make(chan work.FactorySubmissionRecord, 8),
		"owned-cron-jitter": make(chan work.FactorySubmissionRecord, 8),
	}
	var admissions atomic.Int32
	hostDir := support.ScaffoldFactory(t, map[string]any{"workTypes": []map[string]any{{"name": "idle", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"}}}}})
	support.ClearSeedInputs(t, hostDir)
	server := selectedTimeActivate(t, scheduler, func() *support.FunctionalAPIServer {
		return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir: hostDir,
			Edges: serviceedges.Edges{
				Clock: selectedTimeFacts{facts}, ProcessScheduler: scheduler, ScriptCommandRunner: router,
				FactoryRuntimeInputs:               files,
				FactoryRuntimeInputDirectoryWalker: files.walk,
				SubmissionRecorder: func(record work.FactorySubmissionRecord) {
					admissions.Add(1)
					files.admitted(record)
					if route := cronRoutes[record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation]]; route != nil {
						route <- record
					}
				},
			},
			BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
				if admissions.Load() != 0 || len(scheduler.registered) != 0 || len(routeA.entered) != 0 || len(routeB.entered) != 0 {
					tb.Fatal("construction activated a source or timer")
				}
				support.InitializeCustomerHomeWithProcess(tb, process, input.Env, hostDir)
			},
		})
	})
	t.Cleanup(func() { server.Stop(t) })
	assertSelectedScriptTime(t, server.URL(), facts, scheduler, dirA, dirB, routeA, routeB)
	assertSelectedCronTime(t, server.URL(), facts, scheduler, cronRoutes)
	assertSelectedWatcherTime(t, server.URL(), facts, scheduler, files)
}
