package modelhost

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type execProcessLauncher struct{}

func (execProcessLauncher) Start(ctx context.Context, spec ProcessStartSpec) (ManagedProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := strings.TrimSpace(spec.Command)
	if command == "" {
		return nil, fmt.Errorf("supervised process command is required")
	}
	endpoint := strings.TrimSpace(spec.HealthEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("supervised process health endpoint is required")
	}

	cmd := exec.Command(command, spec.Args...)
	if len(spec.Env) > 0 {
		cmd.Env = spec.Env
	}
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	return &execManagedProcess{
		cmd:            cmd,
		healthEndpoint: endpoint,
		done:           done,
	}, nil
}

type execManagedProcess struct {
	mu             sync.Mutex
	cmd            *exec.Cmd
	healthEndpoint string
	done           chan error
	stopped        bool
}

func (p *execManagedProcess) HealthEndpoint() string {
	return p.healthEndpoint
}

func (p *execManagedProcess) Wait() error {
	if p == nil || p.done == nil {
		return nil
	}
	return <-p.done
}

func (p *execManagedProcess) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	p.stopped = true
	if err := p.cmd.Process.Kill(); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
