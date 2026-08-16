package process

import (
	"os/exec"
	"sync"
	"time"
)

const (
	processLifecyclePollInterval = 10 * time.Millisecond
	processExitObservationGrace  = 50 * time.Millisecond
)

// processLifecycleMonitor watches the parent process independently from
// exec.Cmd.Wait. That distinction is the important failure boundary: Wait
// can remain blocked by inherited pipes after the parent process has exited.
type processLifecycleMonitor struct {
	observer ProcessLifecycleObserver
	info     ProcessInfo
	stop     chan struct{}
	done     chan struct{}
	exited   sync.Once
}

func startProcessLifecycleMonitor(
	cmd *exec.Cmd,
	waitDone <-chan struct{},
	observer ProcessLifecycleObserver,
) *processLifecycleMonitor {
	if cmd == nil || cmd.Process == nil || observer == nil {
		return nil
	}
	monitor := &processLifecycleMonitor{
		observer: observer,
		info:     ProcessInfo{PID: cmd.Process.Pid},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	observer.ProcessStarted(monitor.info)
	go monitor.watch(cmd, waitDone)
	return monitor
}

func (m *processLifecycleMonitor) watch(cmd *exec.Cmd, waitDone <-chan struct{}) {
	defer close(m.done)
	ticker := time.NewTicker(processLifecyclePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			if commandProcessLeaderRunning(cmd) {
				continue
			}
			grace := time.NewTimer(processExitObservationGrace)
			select {
			case <-m.stop:
				if !grace.Stop() {
					<-grace.C
				}
				return
			case <-waitDone:
				if !grace.Stop() {
					<-grace.C
				}
				return
			case <-grace.C:
				m.notifyExit()
				return
			}
		}
	}
}

func (m *processLifecycleMonitor) notifyExit() {
	if m == nil || m.observer == nil {
		return
	}
	m.exited.Do(func() {
		m.observer.ProcessExited(m.info)
	})
}

func (m *processLifecycleMonitor) stopAndWait() {
	if m == nil {
		return
	}
	close(m.stop)
	<-m.done
}
