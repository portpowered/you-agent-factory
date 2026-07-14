package agypty

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

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
	return runPlatformSession(ctx, s)
}

func executeSessionRun(
	ctx context.Context,
	cfg SessionConfig,
	reader io.ReadCloser,
	proc *sessionProcess,
) (SessionResult, error) {
	if reader == nil {
		return SessionResult{}, errors.New("agypty: PTY reader is required")
	}
	if proc == nil {
		return SessionResult{}, errors.New("agypty: supervised process is required")
	}

	defer closePTYReader(reader)
	defer proc.Close()

	var (
		mu          sync.Mutex
		buf         []byte
		capacityHit bool
		lastByteAt  = time.Now()
	)

	hardDeadline := time.Now().Add(cfg.HardTimeout)

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- proc.Wait()
	}()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		scratch := make([]byte, 32*1024)
		for {
			if file, ok := reader.(*os.File); ok {
				_ = file.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			}
			n, err := reader.Read(scratch)
			if n > 0 {
				mu.Lock()
				lastByteAt = time.Now()
				remaining := cfg.MaxCaptureBytes - len(buf)
				if remaining > 0 {
					if n > remaining {
						buf = append(buf, scratch[:remaining]...)
						capacityHit = true
					} else {
						buf = append(buf, scratch[:n]...)
					}
				} else {
					capacityHit = true
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	timer := time.NewTimer(timeUntilTimeout(lastByteAt, hardDeadline, cfg))
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
			closePTYReader(reader)
			<-readDone
			mu.Lock()
			resultBuf := append([]byte(nil), buf...)
			hit := capacityHit
			mu.Unlock()
			return finalizeSessionResult(resultBuf, hit, timedOut, waitErr, proc), runErr
		case <-ctx.Done():
			timer.Stop()
			_ = proc.Terminate()
			waitErr = <-waitDone
			closePTYReader(reader)
			<-readDone
			mu.Lock()
			resultBuf := append([]byte(nil), buf...)
			hit := capacityHit
			mu.Unlock()
			return finalizeSessionResult(resultBuf, hit, timedOut, waitErr, proc), ctx.Err()
		case <-timer.C:
			now := time.Now()
			if !now.Before(hardDeadline) || (cfg.IdleTimeout > 0 && now.Sub(lastByteAt) >= cfg.IdleTimeout) {
				timedOut = true
				_ = proc.Terminate()
				waitErr = <-waitDone
				closePTYReader(reader)
				<-readDone
				mu.Lock()
				resultBuf := append([]byte(nil), buf...)
				hit := capacityHit
				mu.Unlock()
				return finalizeSessionResult(resultBuf, hit, timedOut, waitErr, proc), nil
			}
			timer.Reset(timeUntilTimeout(lastByteAt, hardDeadline, cfg))
		}
	}
}

func closePTYReader(reader io.ReadCloser) {
	if reader == nil {
		return
	}
	_ = reader.Close()
}

func timeUntilTimeout(lastByteAt, hardDeadline time.Time, cfg SessionConfig) time.Duration {
	next := time.Until(hardDeadline)
	if cfg.IdleTimeout > 0 {
		idleRemaining := cfg.IdleTimeout - time.Since(lastByteAt)
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

func finalizeSessionResult(raw []byte, capacityHit, timedOut bool, waitErr error, proc *sessionProcess) SessionResult {
	copied := append([]byte(nil), raw...)
	return SessionResult{
		ExitCode:    exitCodeFromWait(waitErr, proc),
		RawBytes:    copied,
		CleanedText: CleanTerminal(copied),
		TimedOut:    timedOut,
		CapacityHit: capacityHit,
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
