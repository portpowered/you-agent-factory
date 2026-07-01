package service

import (
	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

// Config carries explicit runtime collaborators for worker-side scheduling.
type Config struct {
	Logger        *zap.Logger
	Clock         clockwork.Clock
	CommandRunner workers.CommandRunner
}
