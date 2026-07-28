package opencode

import (
	"context"
	"sync"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type modeSelectingEffect interface {
	Effect
	WithMode(Mode) Effect
}

type negotiatingEffect struct {
	runner  workers.CommandRunner
	mu      sync.Mutex
	mode    Mode
	version string
}

func newNegotiatingEffect(runner workers.CommandRunner) *negotiatingEffect {
	return &negotiatingEffect{runner: runner}
}

func (effect *negotiatingEffect) negotiatedMode() Mode {
	effect.mu.Lock()
	defer effect.mu.Unlock()
	if effect.mode == ModeFinalOnly {
		return ModeFinalOnly
	}
	return ModeStructured
}

func (effect *negotiatingEffect) downgrade(version string) {
	effect.mu.Lock()
	defer effect.mu.Unlock()
	effect.mode = ModeFinalOnly
	if safe := safeVersionContext(version); safe != "unknown" {
		effect.version = safe
	}
}

func (effect *negotiatingEffect) versionContext() string {
	effect.mu.Lock()
	defer effect.mu.Unlock()
	if effect.version != "" {
		return effect.version
	}
	return "unknown"
}

func (effect *negotiatingEffect) WithMode(mode Mode) Effect {
	if mode == "" {
		mode = ModeStructured
	}
	return commandEffect{runner: effect.runner, mode: mode}
}

func (effect *negotiatingEffect) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	observe func([]byte) error,
) (EffectResult, error) {
	return effect.WithMode(effect.negotiatedMode()).Execute(ctx, request, observe)
}

func effectForMode(effect Effect, mode Mode) Effect {
	if selector, ok := effect.(modeSelectingEffect); ok {
		return selector.WithMode(mode)
	}
	return effect
}
