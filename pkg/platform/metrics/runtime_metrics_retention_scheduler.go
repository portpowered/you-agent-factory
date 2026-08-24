package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const runtimeMetricsRetentionInterval = time.Hour

// RuntimeMetricsRetentionTicker is the timer effect used by the periodic
// root-retention loop. The channel method keeps the production ticker and
// deterministic test tickers behind the same narrow boundary.
type RuntimeMetricsRetentionTicker interface {
	C() <-chan time.Time
	Stop()
}

// RuntimeMetricsRetentionTickerFactory creates one periodic retention timer.
// Tests can provide a manually ticked implementation without waiting.
type RuntimeMetricsRetentionTickerFactory func(time.Duration) RuntimeMetricsRetentionTicker

// RuntimeMetricsRetentionReporter receives every startup and periodic sweep.
// A non-nil error reports a sweep-level failure; candidate-level failures are
// included in the report's Failed totals and Failures fields.
type RuntimeMetricsRetentionReporter func(RuntimeMetricsRetentionReport, error)

// RuntimeMetricsRetentionLifecycle is the process-scoped lifecycle used by a
// metrics opener. Start performs the first sweep for a root and returns one
// reference whose close releases that root's periodic loop.
type RuntimeMetricsRetentionLifecycle interface {
	Start(context.Context, RuntimeMetricsRetentionRequest) (io.Closer, error)
	Close(context.Context) error
}

// RuntimeMetricsRetentionScheduler deduplicates one hourly loop per root for
// one process. The first active sink owns the startup sweep; later sinks share
// its loop and policy until the last sink releases its reference.
type RuntimeMetricsRetentionScheduler struct {
	retention     *RuntimeMetricsRetention
	tickerFactory RuntimeMetricsRetentionTickerFactory
	reporter      RuntimeMetricsRetentionReporter

	mu      sync.Mutex
	workers map[string]*runtimeMetricsRetentionWorker
	closed  bool
}

// NewRuntimeMetricsRetentionScheduler constructs a process-scoped scheduler.
// A nil ticker factory selects the production time.Ticker implementation. A
// nil reporter discards reports while the sweep still returns deterministic
// totals to direct callers.
func NewRuntimeMetricsRetentionScheduler(
	retention *RuntimeMetricsRetention,
	tickerFactory RuntimeMetricsRetentionTickerFactory,
	reporter RuntimeMetricsRetentionReporter,
) (*RuntimeMetricsRetentionScheduler, error) {
	if retention == nil {
		return nil, errors.New("construct runtime metrics retention scheduler: retention is required")
	}
	if tickerFactory == nil {
		tickerFactory = newRuntimeMetricsRetentionTicker
	}
	return &RuntimeMetricsRetentionScheduler{
		retention: retention, tickerFactory: tickerFactory, reporter: reporter,
		workers: make(map[string]*runtimeMetricsRetentionWorker),
	}, nil
}

// Start runs the startup sweep before the caller can reserve its live file.
// Candidate-level failures are reported and left for the next hourly sweep;
// only construction or cancellation failures prevent a new lifecycle lease.
func (scheduler *RuntimeMetricsRetentionScheduler) Start(
	ctx context.Context,
	request RuntimeMetricsRetentionRequest,
) (io.Closer, error) {
	if scheduler == nil || scheduler.retention == nil || scheduler.tickerFactory == nil {
		return nil, errors.New("start runtime metrics retention: scheduler is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("start runtime metrics retention: %w", err)
	}
	root, err := runtimeMetricsRetentionRoot(request.RootDirectory)
	if err != nil {
		return nil, err
	}
	request.RootDirectory = root

	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.closed {
		return nil, errors.New("start runtime metrics retention: scheduler is closed")
	}
	if worker := scheduler.workers[root]; worker != nil {
		worker.references++
		return &runtimeMetricsRetentionLease{scheduler: scheduler, root: root}, nil
	}

	if err := scheduler.ensureRoot(ctx, root); err != nil {
		return nil, fmt.Errorf("prepare runtime metrics root %q: %w", root, err)
	}
	report, sweepErr := scheduler.retention.Sweep(ctx, request)
	if report.RootDirectory == "" {
		report.RootDirectory = root
	}
	scheduler.publish(report, sweepErr)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("start runtime metrics retention: %w", err)
	}

	ticker := scheduler.tickerFactory(runtimeMetricsRetentionInterval)
	if ticker == nil || ticker.C() == nil {
		if ticker != nil {
			ticker.Stop()
		}
		return nil, errors.New("start runtime metrics retention: ticker is not configured")
	}
	workerContext, cancel := context.WithCancel(context.Background())
	worker := &runtimeMetricsRetentionWorker{
		scheduler:  scheduler,
		request:    request,
		ticker:     ticker,
		context:    workerContext,
		cancel:     cancel,
		done:       make(chan struct{}),
		references: 1,
	}
	scheduler.workers[root] = worker
	go worker.run()
	return &runtimeMetricsRetentionLease{scheduler: scheduler, root: root}, nil
}

// Close stops every root loop and waits for an in-flight sweep to unwind.
// Waiting for completion keeps timers, goroutines, and retention claims from
// surviving the owning application process lifecycle.
func (scheduler *RuntimeMetricsRetentionScheduler) Close(_ context.Context) error {
	if scheduler == nil {
		return nil
	}
	scheduler.mu.Lock()
	if scheduler.closed {
		scheduler.mu.Unlock()
		return nil
	}
	scheduler.closed = true
	workers := make([]*runtimeMetricsRetentionWorker, 0, len(scheduler.workers))
	for root, worker := range scheduler.workers {
		workers = append(workers, worker)
		delete(scheduler.workers, root)
	}
	scheduler.mu.Unlock()

	var returnErr error
	for _, worker := range workers {
		returnErr = errors.Join(returnErr, worker.close())
	}
	return returnErr
}

func (scheduler *RuntimeMetricsRetentionScheduler) ensureRoot(ctx context.Context, root string) error {
	lock, err := scheduler.retention.coordination.LockRoot(ctx, root)
	if err != nil {
		return err
	}
	return lock.Close()
}

func (scheduler *RuntimeMetricsRetentionScheduler) release(root string) error {
	scheduler.mu.Lock()
	worker := scheduler.workers[root]
	if worker == nil || worker.references == 0 {
		scheduler.mu.Unlock()
		return nil
	}
	worker.references--
	if worker.references > 0 {
		scheduler.mu.Unlock()
		return nil
	}
	delete(scheduler.workers, root)
	scheduler.mu.Unlock()
	return worker.close()
}

func (scheduler *RuntimeMetricsRetentionScheduler) sweep(
	ctx context.Context,
	request RuntimeMetricsRetentionRequest,
) {
	report, err := scheduler.retention.Sweep(ctx, request)
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return
	}
	if report.RootDirectory == "" {
		report.RootDirectory = request.RootDirectory
	}
	scheduler.publish(report, err)
}

func (scheduler *RuntimeMetricsRetentionScheduler) publish(
	report RuntimeMetricsRetentionReport,
	err error,
) {
	if scheduler.reporter != nil {
		scheduler.reporter(report, err)
	}
}

type runtimeMetricsRetentionWorker struct {
	scheduler  *RuntimeMetricsRetentionScheduler
	request    RuntimeMetricsRetentionRequest
	ticker     RuntimeMetricsRetentionTicker
	context    context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	references int
	once       sync.Once
}

func (worker *runtimeMetricsRetentionWorker) run() {
	defer close(worker.done)
	for {
		select {
		case <-worker.context.Done():
			return
		case <-worker.ticker.C():
			worker.scheduler.sweep(worker.context, worker.request)
		}
	}
}

func (worker *runtimeMetricsRetentionWorker) close() error {
	if worker == nil {
		return nil
	}
	worker.once.Do(func() {
		worker.ticker.Stop()
		worker.cancel()
	})
	<-worker.done
	return nil
}

type runtimeMetricsRetentionLease struct {
	scheduler *RuntimeMetricsRetentionScheduler
	root      string
	once      sync.Once
	err       error
}

func (lease *runtimeMetricsRetentionLease) Close() error {
	if lease == nil {
		return nil
	}
	lease.once.Do(func() {
		if lease.scheduler != nil {
			lease.err = lease.scheduler.release(lease.root)
		}
	})
	return lease.err
}

type runtimeMetricsRetentionTicker struct {
	ticker *time.Ticker
}

func newRuntimeMetricsRetentionTicker(interval time.Duration) RuntimeMetricsRetentionTicker {
	return runtimeMetricsRetentionTicker{ticker: time.NewTicker(interval)}
}

func (ticker runtimeMetricsRetentionTicker) C() <-chan time.Time {
	if ticker.ticker == nil {
		return nil
	}
	return ticker.ticker.C
}

func (ticker runtimeMetricsRetentionTicker) Stop() {
	if ticker.ticker != nil {
		ticker.ticker.Stop()
	}
}

func runtimeMetricsRetentionRoot(root string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return "", errors.New("start runtime metrics retention: root is required")
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return "", errors.New("start runtime metrics retention: root is required")
	}
	return cleaned, nil
}
