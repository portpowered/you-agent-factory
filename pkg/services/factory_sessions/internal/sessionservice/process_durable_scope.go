package service

import (
	"context"
	"path/filepath"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProcessDurableScope selects the live facts used by process-level persistence
// and resume. It does not retain an opening runtime, gateway or final Root.
type ProcessDurableScope interface {
	CurrentProjectRoot() string
	ResumeRuntimeScope(string) (factorysessionexecution.ResumeRuntimeScope, error)
}

type processDurableScope struct {
	state *sessionruntime.Service
}

func NewProcessDurableScope(state *sessionruntime.Service) ProcessDurableScope {
	return &processDurableScope{state: state}
}

func (s *processDurableScope) selectedSession() *livesession.LiveSession {
	if current := s.state.Resolve(factorysessions.DefaultSessionID); current != nil {
		return current
	}
	if ids := s.state.Registry().IDs(); len(ids) == 1 {
		return s.state.Resolve(ids[0])
	}
	return nil
}

func (s *processDurableScope) CurrentProjectRoot() string {
	if current := s.selectedSession(); current != nil {
		return current.FactoryDir
	}
	return ""
}

func (s *processDurableScope) ResumeRuntimeScope(projectRoot string) (factorysessionexecution.ResumeRuntimeScope, error) {
	current := s.selectedSession()
	if current == nil {
		return factorysessionexecution.ResumeRuntimeScope{}, factorysessions.ErrRuntimeNotAvailable
	}
	if filepath.Clean(current.FactoryDir) != filepath.Clean(projectRoot) {
		return factorysessionexecution.ResumeRuntimeScope{}, factorysessions.ErrSessionNotFound
	}
	// RuntimeRecord/Run capability access is the T15 compatibility boundary.
	instance := runtimebinding.BundleFromSession(current)
	bound := runtimebinding.SessionStateFrom(current)
	if instance == nil || bound == nil {
		return factorysessionexecution.ResumeRuntimeScope{}, factorysessions.ErrRuntimeNotAvailable
	}
	admission, _ := instance.RuntimeService().(factoryruntime.ResourceCapacityLeaseAdmission)
	return factorysessionexecution.ResumeRuntimeScope{
		WorkerSettings: bound.WorkerSettingsSnapshot(), MockWorkers: bound.MockWorkersConfig(),
		WorkerAttemptStarter: s.workerAttemptStarter(instance), WorkerResourceAdmission: admission,
		WorkerProgressPublisher: s.progressPublisher(instance),
	}, nil
}

func (s *processDurableScope) workerAttemptStarter(instance runtimebinding.RuntimeInstance) factorysessions.WorkerAttemptStarter {
	type starter interface {
		BeginWorkerAttempt(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error)
	}
	if provider, ok := instance.(starter); ok {
		return provider.BeginWorkerAttempt
	}
	if provider, ok := instance.RuntimeService().(starter); ok {
		return provider.BeginWorkerAttempt
	}
	return nil
}

func (s *processDurableScope) progressPublisher(instance runtimebinding.RuntimeInstance) workers.ProgressPublisher {
	type publisher interface {
		RuntimeProgressPublisher() workers.ProgressPublisher
	}
	if provider, ok := instance.(publisher); ok {
		return provider.RuntimeProgressPublisher()
	}
	if provider, ok := instance.RuntimeService().(publisher); ok {
		return provider.RuntimeProgressPublisher()
	}
	return nil
}
