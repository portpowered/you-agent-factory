// Package wire constructs the Automations script-poller subservice.
package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollersservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// CursorPersistenceFileSystem is the filesystem effect accepted by the
// script-poller composition boundary.
type CursorPersistenceFileSystem = scriptpollersservice.CursorPersistenceFileSystem

// NewService constructs inert script-poller supervision from direct collaborators.
func NewService(
	logger *zap.Logger,
	scheduler scriptpollers.Scheduler,
	commandRunner platformprocess.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursorRecorder scriptpollers.CursorRecorder,
) scriptpollers.Service {
	return scriptpollersservice.New(logger, scheduler, commandRunner, resolveTemplates, executionPolicy, cursorRecorder)
}

// NewDurableCursorRecorder constructs the Automations-owned durable cursor
// implementation behind the script-poller wire boundary.
func NewDurableCursorRecorder(
	baseDir string,
	files CursorPersistenceFileSystem,
) (scriptpollers.CursorRecorder, error) {
	return scriptpollersservice.NewDurableCursorRecorder(baseDir, files)
}
