package replay_contracts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

type replayFunctionalServer struct {
	httpSrv *httptest.Server
	service *service.FactoryService
	cancel  context.CancelFunc
	done    chan struct{}
}

func startReplayFunctionalServerWithConfig(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	configure func(*service.FactoryServiceConfig),
	extraOpts ...factory.FactoryOption,
) *replayFunctionalServer {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	var handler http.Handler
	readyCh := make(chan struct{})

	cfg := &service.FactoryServiceConfig{
		Dir:          factoryDir,
		Port:         1,
		Logger:       zap.NewNop(),
		ExtraOptions: extraOpts,
		APIServerStarter: func(ctx context.Context, f apisurface.APISurface, port int, l *zap.Logger) error {
			handler = api.NewServer(f, 0, l).Handler()
			close(readyCh)
			<-ctx.Done()
			return nil
		},
	}
	if useMockWorkers {
		cfg.MockWorkersConfig = config.NewEmptyMockWorkersConfig()
	}
	if configure != nil {
		configure(cfg)
	}

	svc, err := service.BuildFactoryService(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatalf("BuildFactoryService: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := svc.Run(ctx); err != nil && err != context.Canceled {
			fmt.Printf("replay_contracts FunctionalServer: svc.Run ended: %v\n", err)
		}
	}()

	select {
	case <-readyCh:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("FunctionalServer: timed out waiting for API handler")
	}

	httpSrv := httptest.NewServer(handler)
	server := &replayFunctionalServer{
		httpSrv: httpSrv,
		service: svc,
		cancel:  cancel,
		done:    done,
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		httpSrv.Close()
	})
	return server
}

func (fs *replayFunctionalServer) URL() string {
	return fs.httpSrv.URL
}

func (fs *replayFunctionalServer) GetDashboard(t *testing.T) DashboardResponse {
	t.Helper()

	snapshot, err := fs.service.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	events, err := fs.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("get factory events: %v", err)
	}

	worldState, err := projections.ReconstructFactoryWorldState(events, snapshot.TickCount)
	if err != nil {
		t.Fatalf("reconstruct world state: %v", err)
	}
	worldView := projections.BuildFactoryWorldViewWithActiveThrottlePauses(worldState, snapshot.ActiveThrottlePauses)

	var out DashboardResponse
	out.FactoryState = snapshot.FactoryState
	out.TickCount = snapshot.TickCount
	out.Runtime.InFlightDispatchCount = worldView.Runtime.InFlightDispatchCount
	out.Runtime.Session.CompletedCount = worldView.Runtime.Session.CompletedCount
	out.Runtime.Session.DispatchedCount = worldView.Runtime.Session.DispatchedCount
	out.Runtime.Session.FailedCount = worldView.Runtime.Session.FailedCount
	return out
}

func (fs *replayFunctionalServer) GetEngineStateSnapshot(t *testing.T) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	snapshot, err := fs.service.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snapshot
}

type DashboardResponse struct {
	FactoryState string `json:"factory_state"`
	TickCount    int    `json:"tick_count"`
	Runtime      struct {
		InFlightDispatchCount int `json:"in_flight_dispatch_count"`
		Session               struct {
			CompletedCount  int `json:"completed_count"`
			DispatchedCount int `json:"dispatched_count"`
			FailedCount     int `json:"failed_count"`
		} `json:"session"`
	} `json:"runtime"`
}

type factoryEventHTTPStream struct {
	t      *testing.T
	cancel context.CancelFunc
	done   chan struct{}
	events chan factoryapi.FactoryEvent
	errs   chan error
}

func openFactoryEventHTTPStream(t *testing.T, endpoint string) *factoryEventHTTPStream {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build /events request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET /events: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		cancel()
		t.Fatalf("GET /events status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		defer resp.Body.Close()
		cancel()
		t.Fatalf("GET /events content type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	stream := &factoryEventHTTPStream{
		t:      t,
		cancel: cancel,
		done:   make(chan struct{}),
		events: make(chan factoryapi.FactoryEvent, 4096),
		errs:   make(chan error, 1),
	}
	go stream.read(resp)
	t.Cleanup(stream.close)
	return stream
}

func (s *factoryEventHTTPStream) read(resp *http.Response) {
	defer close(s.done)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			select {
			case s.errs <- fmt.Errorf("decode /events payload: %w", err):
			default:
			}
			return
		}
		select {
		case s.events <- event:
		default:
			select {
			case s.errs <- fmt.Errorf("/events test buffer overflow"):
			default:
			}
		}
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			select {
			case s.errs <- fmt.Errorf("/events emitted named SSE event line %q", line):
			default:
			}
			return
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		select {
		case s.errs <- err:
		default:
		}
	}
}

func (s *factoryEventHTTPStream) next(timeout time.Duration) factoryapi.FactoryEvent {
	s.t.Helper()
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	select {
	case event := <-s.events:
		return event
	case err := <-s.errs:
		s.t.Fatalf("/events stream error: %v", err)
	case <-time.After(timeout):
		s.t.Fatalf("timed out waiting for /events payload within %s", timeout)
	}
	return factoryapi.FactoryEvent{}
}

func (s *factoryEventHTTPStream) close() {
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

func requireFunctionalEventStreamPrelude(
	t *testing.T,
	stream *factoryEventHTTPStream,
) (factoryapi.FactoryEvent, factoryapi.FactoryEvent) {
	t.Helper()

	runStarted := stream.next(5 * time.Second)
	if runStarted.Type != factoryapi.FactoryEventTypeRunRequest || runStarted.Context.Tick != 0 {
		t.Fatalf("first /events payload = %#v, want run-request at tick 0", runStarted)
	}

	initialStructure := stream.next(5 * time.Second)
	if initialStructure.Type != factoryapi.FactoryEventTypeInitialStructureRequest || initialStructure.Context.Tick != 0 {
		t.Fatalf("second /events payload = %#v, want initial structure at tick 0", initialStructure)
	}
	if initialStructure.Context.Sequence <= runStarted.Context.Sequence {
		t.Fatalf(
			"/events prelude sequences = run_request:%d initial_structure_request:%d, want increasing order",
			runStarted.Context.Sequence,
			initialStructure.Context.Sequence,
		)
	}

	return runStarted, initialStructure
}

func collectUnifiedSmokeEventsUntilRunResponse(t *testing.T, stream *factoryEventHTTPStream, initialEvents []factoryapi.FactoryEvent, timeout time.Duration) []factoryapi.FactoryEvent {
	t.Helper()

	events := append([]factoryapi.FactoryEvent(nil), initialEvents...)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		events = append(events, event)
		if event.Type == factoryapi.FactoryEventTypeRunResponse {
			return events
		}
	}
	t.Fatalf("timed out waiting for RUN_RESPONSE in live /events timeline: %#v", unifiedSmokeEventSummaries(events))
	return nil
}

func maxUnifiedSmokeTick(events []factoryapi.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
}

func unifiedSmokeEventSummaries(events []factoryapi.FactoryEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, string(event.Type)+"@"+event.Id)
	}
	return out
}

func lastIndexOfFunctionalEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			return i
		}
	}
	return -1
}
