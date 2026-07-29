package agypty

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
)

const ptyCaptureDrainTimeout = 250 * time.Millisecond

// platformSession holds an allocated platform PTY for one supervised Agy child.
// Story 002 implements Run with capture, timeout, and cleanup.
type platformSession struct {
	launch ProcessLaunch
	cfg    SessionConfig
	kind   PTYKind
	pty    platformpty.Allocation
	host   platformpty.Host
	clock  platformclock.Source
	mu     sync.Mutex
	closed bool
}

func (s *platformSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.pty == nil {
		return nil
	}
	return s.pty.Close()
}

func (s *platformSession) PTYKind() PTYKind {
	if s == nil {
		return PTYKindUnknown
	}
	return s.kind
}

func newPlatformSession(
	launch ProcessLaunch,
	cfg SessionConfig,
	kind PTYKind,
	pty platformpty.Allocation,
	host platformpty.Host,
	clock platformclock.Source,
) (*platformSession, error) {
	if pty == nil {
		return nil, errors.New("agypty: PTY allocation is required")
	}
	if clock == nil {
		return nil, ErrClockRequired
	}
	if host == nil {
		return nil, ErrHostRequired
	}
	return &platformSession{
		launch: launch,
		cfg:    cfg,
		kind:   kind,
		pty:    pty,
		host:   host,
		clock:  clock,
	}, nil
}

func exitCodeFromWait(waitErr error, proc platformpty.Process) int {
	if waitErr != nil {
		return -1
	}
	if proc != nil {
		return proc.ExitCode()
	}
	return 0
}

func (s *platformSession) Run(ctx context.Context) (SessionResult, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return SessionResult{}, errors.New("agypty: session closed")
	}
	s.mu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	if s.host == nil {
		return SessionResult{}, ErrHostRequired
	}
	defer closeSessionPTY(s)
	proc, reader, err := s.host.Start(platformProcessLaunch(s.launch), s.pty)
	if err != nil {
		return SessionResult{}, err
	}
	return executeSessionRun(ctx, s.cfg, reader, proc, s.clock)
}

func executeSessionRun(
	ctx context.Context,
	cfg SessionConfig,
	reader io.ReadCloser,
	proc platformpty.Process,
	clock platformclock.Source,
) (SessionResult, error) {
	if reader == nil {
		return SessionResult{}, errors.New("agypty: PTY reader is required")
	}
	if proc == nil {
		return SessionResult{}, errors.New("agypty: supervised process is required")
	}
	if clock == nil {
		return SessionResult{}, ErrClockRequired
	}

	defer closePTYReader(reader)
	defer proc.Close()

	var (
		mu          sync.Mutex
		buf         []byte
		capacityHit bool
		lastByteAt  = clock.Now()
	)

	hardDeadline := clock.Now().Add(cfg.HardTimeout)

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- proc.Wait()
	}()

	readDone := startPTYCapture(reader, cfg, &mu, &buf, &capacityHit, &lastByteAt, clock)

	timer := time.NewTimer(timeUntilTimeout(clock.Now(), readLastByteAt(&mu, &lastByteAt), hardDeadline, cfg))
	defer timer.Stop()

	var (
		timedOut bool
		waitErr  error
		runErr   error
	)

	for {
		select {
		case waitErr = <-waitDone:
			timer.Stop()
			return finishSessionRun(reader, readDone, &mu, &buf, &capacityHit, timedOut, waitErr, proc, runErr)
		case <-ctx.Done():
			timer.Stop()
			_ = proc.Terminate()
			waitErr = <-waitDone
			return finishSessionRun(reader, readDone, &mu, &buf, &capacityHit, timedOut, waitErr, proc, ctx.Err())
		case <-timer.C:
			lastByte := readLastByteAt(&mu, &lastByteAt)
			now := clock.Now()
			if sessionRunTimedOut(now, hardDeadline, cfg, lastByte) {
				timedOut = true
				_ = proc.Terminate()
				waitErr = <-waitDone
				return finishSessionRun(reader, readDone, &mu, &buf, &capacityHit, timedOut, waitErr, proc, nil)
			}
			timer.Reset(timeUntilTimeout(now, lastByte, hardDeadline, cfg))
		}
	}
}

func startPTYCapture(
	reader io.ReadCloser,
	cfg SessionConfig,
	mu *sync.Mutex,
	buf *[]byte,
	capacityHit *bool,
	lastByteAt *time.Time,
	clock platformclock.Source,
) chan struct{} {
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		scratch := make([]byte, 32*1024)
		for {
			if deadlineReader, ok := reader.(interface{ SetReadDeadline(time.Time) error }); ok {
				_ = deadlineReader.SetReadDeadline(clock.Now().Add(100 * time.Millisecond))
			}
			n, err := reader.Read(scratch)
			if n > 0 {
				mu.Lock()
				*lastByteAt = clock.Now()
				remaining := cfg.MaxCaptureBytes - len(*buf)
				if remaining > 0 {
					if n > remaining {
						*buf = append(*buf, scratch[:remaining]...)
						*capacityHit = true
					} else {
						*buf = append(*buf, scratch[:n]...)
					}
				} else {
					*capacityHit = true
				}
				mu.Unlock()
			}
			if err != nil {
				var timeout interface{ Timeout() bool }
				if errors.As(err, &timeout) && timeout.Timeout() {
					continue
				}
				return
			}
		}
	}()
	return readDone
}

func readLastByteAt(mu *sync.Mutex, lastByteAt *time.Time) time.Time {
	mu.Lock()
	defer mu.Unlock()
	return *lastByteAt
}

func sessionRunTimedOut(now, hardDeadline time.Time, cfg SessionConfig, lastByteAt time.Time) bool {
	if !now.Before(hardDeadline) {
		return true
	}
	return cfg.IdleTimeout > 0 && now.Sub(lastByteAt) >= cfg.IdleTimeout
}

func finishSessionRun(
	reader io.ReadCloser,
	readDone <-chan struct{},
	mu *sync.Mutex,
	buf *[]byte,
	capacityHit *bool,
	timedOut bool,
	waitErr error,
	proc platformpty.Process,
	runErr error,
) (SessionResult, error) {
	drainPTYCapture(reader, readDone)
	mu.Lock()
	resultBuf := append([]byte(nil), (*buf)...)
	hit := *capacityHit
	mu.Unlock()
	return finalizeSessionResult(resultBuf, hit, timedOut, waitErr, proc), runErr
}

// drainPTYCapture lets terminal bytes already buffered by the OS reach the
// capture goroutine after the child exits. The bounded fallback still closes
// readers that do not report EOF on their own.
func drainPTYCapture(reader io.ReadCloser, readDone <-chan struct{}) {
	timer := time.NewTimer(ptyCaptureDrainTimeout)
	defer timer.Stop()
	select {
	case <-readDone:
		return
	case <-timer.C:
		closePTYReader(reader)
		<-readDone
	}
}

func closePTYReader(reader io.ReadCloser) {
	if reader == nil {
		return
	}
	_ = reader.Close()
}

func timeUntilTimeout(now, lastByteAt, hardDeadline time.Time, cfg SessionConfig) time.Duration {
	next := hardDeadline.Sub(now)
	if cfg.IdleTimeout > 0 {
		idleRemaining := cfg.IdleTimeout - now.Sub(lastByteAt)
		if idleRemaining < next {
			next = idleRemaining
		}
	}
	if next < 0 {
		return 0
	}
	if next > 100*time.Millisecond {
		return 100 * time.Millisecond
	}
	return next
}

func finalizeSessionResult(raw []byte, capacityHit, timedOut bool, waitErr error, proc platformpty.Process) SessionResult {
	copied := append([]byte(nil), raw...)
	return SessionResult{
		ExitCode:    exitCodeFromWait(waitErr, proc),
		RawBytes:    copied,
		CleanedText: CleanTerminal(copied),
		TimedOut:    timedOut,
		CapacityHit: capacityHit,
	}
}

func platformProcessLaunch(launch ProcessLaunch) platformpty.ProcessLaunch {
	return platformpty.ProcessLaunch{
		Executable: launch.Executable,
		Argv:       launch.Argv,
		WorkDir:    launch.WorkDir,
		Env:        launch.Env,
	}
}

func platformPTYKind(kind platformpty.Kind) PTYKind {
	switch kind {
	case platformpty.KindPOSIX:
		return PTYKindPOSIX
	case platformpty.KindConPTY:
		return PTYKindConPTY
	default:
		return PTYKindUnknown
	}
}

func closeSessionPTY(session *platformSession) {
	if session == nil {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return
	}
	session.closed = true
	if session.pty != nil {
		_ = session.pty.Close()
		session.pty = nil
	}
}
