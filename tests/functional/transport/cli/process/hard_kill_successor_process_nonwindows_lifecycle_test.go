//go:build !windows

package process_test

import (
	"errors"
	"os"
	"sync"
	"syscall"
	"testing"
)

func TestNewHardKillProcessControlReleasesAfterInitialStopFailure(t *testing.T) {
	stopErr := errors.New("stop failed")
	releaseErr := errors.New("release failed")
	process := &fakeHardKillProcess{
		signalErrors: map[os.Signal]error{syscall.SIGSTOP: stopErr},
		releaseErr:   releaseErr,
	}

	control, err := newHardKillProcessControl(process)
	if control.resume != nil || control.terminate != nil {
		t.Fatalf("initial stop failure returned a terminal control: %#v", control)
	}
	if !errors.Is(err, stopErr) {
		t.Fatalf("initial stop error = %v, want %v", err, stopErr)
	}
	if !errors.Is(err, releaseErr) {
		t.Fatalf("initial stop error = %v, want release error %v", err, releaseErr)
	}
	if got := process.releaseCount(); got != 1 {
		t.Fatalf("release count = %d, want 1", got)
	}
	if got := process.signalCount(syscall.SIGSTOP); got != 1 {
		t.Fatalf("SIGSTOP count = %d, want 1", got)
	}
}

func TestNewHardKillProcessControlReleasesAfterFirstTerminalOperation(t *testing.T) {
	signalErr := errors.New("signal failed")
	releaseErr := errors.New("release failed")
	tests := []struct {
		name          string
		terminal      func(hardKillProcessControl) error
		signalErr     error
		killErr       error
		releaseErr    error
		wantSignal    os.Signal
		wantKillCount int
		wantErrs      []error
	}{
		{
			name:       "resume succeeds",
			terminal:   func(control hardKillProcessControl) error { return control.resume() },
			wantSignal: syscall.SIGCONT,
		},
		{
			name:       "resume joins signal and release failures",
			terminal:   func(control hardKillProcessControl) error { return control.resume() },
			signalErr:  signalErr,
			releaseErr: releaseErr,
			wantSignal: syscall.SIGCONT,
			wantErrs:   []error{signalErr, releaseErr},
		},
		{
			name:       "resume returns release-only failure",
			terminal:   func(control hardKillProcessControl) error { return control.resume() },
			releaseErr: releaseErr,
			wantSignal: syscall.SIGCONT,
			wantErrs:   []error{releaseErr},
		},
		{
			name:          "terminate succeeds",
			terminal:      func(control hardKillProcessControl) error { return control.terminate() },
			wantKillCount: 1,
		},
		{
			name:          "terminate joins kill and release failures",
			terminal:      func(control hardKillProcessControl) error { return control.terminate() },
			killErr:       signalErr,
			releaseErr:    releaseErr,
			wantErrs:      []error{signalErr, releaseErr},
			wantKillCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			process := &fakeHardKillProcess{
				signalErrors: map[os.Signal]error{syscall.SIGCONT: test.signalErr},
				killErr:      test.killErr,
				releaseErr:   test.releaseErr,
			}
			control, err := newHardKillProcessControl(process)
			if err != nil {
				t.Fatalf("create control: %v", err)
			}

			if got := test.terminal(control); !sameErrorSet(got, test.wantErrs...) {
				t.Fatalf("first terminal operation error = %v, want errors %v", got, test.wantErrs)
			}
			if got := test.terminal(control); got != nil {
				t.Fatalf("repeated terminal operation error = %v, want nil", got)
			}
			if got := process.releaseCount(); got != 1 {
				t.Fatalf("release count = %d, want 1", got)
			}
			if got := process.signalCount(syscall.SIGSTOP); got != 1 {
				t.Fatalf("SIGSTOP count = %d, want 1", got)
			}
			if test.wantSignal != nil {
				if got := process.signalCount(test.wantSignal); got != 1 {
					t.Fatalf("terminal signal count = %d, want 1", got)
				}
			}
			if got := process.killCount(); got != test.wantKillCount {
				t.Fatalf("kill count = %d, want %d", got, test.wantKillCount)
			}
		})
	}
}

func TestNewHardKillProcessControlCoordinatesCompetingTerminalOperations(t *testing.T) {
	process := &fakeHardKillProcess{}
	control, err := newHardKillProcessControl(process)
	if err != nil {
		t.Fatalf("create control: %v", err)
	}

	const callers = 32
	var waitGroup sync.WaitGroup
	errorsCh := make(chan error, callers)
	waitGroup.Add(callers)
	for index := 0; index < callers; index++ {
		go func(index int) {
			defer waitGroup.Done()
			if index%2 == 0 {
				errorsCh <- control.resume()
				return
			}
			errorsCh <- control.terminate()
		}(index)
	}
	waitGroup.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("competing terminal operation error = %v, want nil", err)
		}
	}

	terminalSignals := process.signalCount(syscall.SIGCONT)
	if got := process.killCount(); terminalSignals+got != 1 {
		t.Fatalf("terminal operation count = %d, want 1", terminalSignals+got)
	}
	if got := process.releaseCount(); got != 1 {
		t.Fatalf("release count = %d, want 1", got)
	}
}

func sameErrorSet(got error, want ...error) bool {
	if len(want) == 0 {
		return got == nil
	}
	if got == nil {
		return false
	}
	for _, wantErr := range want {
		if !errors.Is(got, wantErr) {
			return false
		}
	}
	return true
}

type fakeHardKillProcess struct {
	mu           sync.Mutex
	signalErrors map[os.Signal]error
	signalCalls  []os.Signal
	killErr      error
	killCalls    int
	releaseErr   error
	releaseCalls int
}

func (process *fakeHardKillProcess) Signal(signal os.Signal) error {
	process.mu.Lock()
	defer process.mu.Unlock()
	process.signalCalls = append(process.signalCalls, signal)
	return process.signalErrors[signal]
}

func (process *fakeHardKillProcess) Kill() error {
	process.mu.Lock()
	defer process.mu.Unlock()
	process.killCalls++
	return process.killErr
}

func (process *fakeHardKillProcess) Release() error {
	process.mu.Lock()
	defer process.mu.Unlock()
	process.releaseCalls++
	return process.releaseErr
}

func (process *fakeHardKillProcess) signalCount(want os.Signal) int {
	process.mu.Lock()
	defer process.mu.Unlock()
	count := 0
	for _, got := range process.signalCalls {
		if got == want {
			count++
		}
	}
	return count
}

func (process *fakeHardKillProcess) killCount() int {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.killCalls
}

func (process *fakeHardKillProcess) releaseCount() int {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.releaseCalls
}
