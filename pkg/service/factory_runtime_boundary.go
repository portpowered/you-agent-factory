package service

import (
	"context"
	"fmt"
	"os"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

// SubmitWorkRequest submits a canonical work request batch to the factory.
func (fs *FactoryService) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return interfaces.WorkRequestSubmitResult{}, fmt.Errorf("factory service runtime is not available")
	}
	return activeFactory.SubmitWorkRequest(ctx, request)
}

// SubscribeFactoryEvents returns canonical factory event history followed by
// live events from the current service-owned runtime.
func (fs *FactoryService) SubscribeFactoryEvents(ctx context.Context) (*interfaces.FactoryEventStream, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	stream, err := activeFactory.SubscribeFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

// WaitToComplete returns a channel that is closed when all tokens reach
// terminal or failed places and no dispatches are in flight. Delegates to
// the underlying factory's termination signal.
func (fs *FactoryService) WaitToComplete() <-chan struct{} {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return activeFactory.WaitToComplete()
}

// GetEngineStateSnapshot returns the factory boundary's aggregate
// observability snapshot.
func (fs *FactoryService) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	snap, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snap, nil
}

// Pause pauses the current runtime instance.
func (fs *FactoryService) Pause(ctx context.Context) error {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if err := activeFactory.Pause(ctx); err != nil {
		return fmt.Errorf("pause factory: %w", err)
	}
	return nil
}

// GetFactoryEvents returns the canonical factory event history.
func (fs *FactoryService) GetFactoryEvents(ctx context.Context) ([]factoryapi.FactoryEvent, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	events, err := activeFactory.GetFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("get factory events: %w", err)
	}
	return events, nil
}

func (fs *FactoryService) submitWorkFile(ctx context.Context) error {
	data, err := os.ReadFile(fs.cfg.WorkFile)
	if err != nil {
		return fmt.Errorf("read work file %s: %w", fs.cfg.WorkFile, err)
	}
	workRequest, err := requests.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return fmt.Errorf("parse work file %s: %w", fs.cfg.WorkFile, err)
	}
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if _, err := activeFactory.SubmitWorkRequest(ctx, workRequest); err != nil {
		return fmt.Errorf("submit initial work: %w", err)
	}
	fs.logger.Info("submitted initial work", zap.String("file", fs.cfg.WorkFile))
	return nil
}

func (fs *FactoryService) currentFactory() factory.Factory {
	if fs == nil {
		return nil
	}
	if compatibilitySession := fs.compatibilitySession(); compatibilitySession != nil && compatibilitySession.handle != nil && compatibilitySession.handle.runtime != nil {
		return compatibilitySession.handle.runtime.factory
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.factory
}

func (fs *FactoryService) currentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if fs == nil {
		return nil
	}
	if compatibilitySession := fs.compatibilitySession(); compatibilitySession != nil && compatibilitySession.handle != nil && compatibilitySession.handle.runtime != nil {
		return compatibilitySession.handle.runtime.runtimeCfg
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.runtimeCfg
}

func (fs *FactoryService) compatibilitySession() *liveFactorySession {
	if fs == nil {
		return nil
	}
	return fs.defaultSession()
}

func (fs *FactoryService) workflowID() string {
	if fs == nil || fs.cfg == nil {
		return ""
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.cfg.WorkflowID
}
