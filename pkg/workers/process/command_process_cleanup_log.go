package process

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/logging"
)

var errCommandProcessCleanupPartialFailure = errors.New("command process cleanup partial failure")

const (
	workLogEventCommandProcessCleanupStarted    = "command_process.cleanup_started"
	workLogEventCommandProcessCleanupGraceful   = "command_process.cleanup_graceful"
	workLogEventCommandProcessCleanupForceKill  = "command_process.cleanup_force_kill"
	workLogEventCommandProcessCleanupCompleted  = "command_process.cleanup_completed"
)

type commandProcessCleanupReason string

const (
	commandProcessCleanupReasonCancel  commandProcessCleanupReason = "cancel"
	commandProcessCleanupReasonPostRun commandProcessCleanupReason = "post_run"
)

type commandProcessCleanupOutcome string

const (
	commandProcessCleanupOutcomeNoOp             commandProcessCleanupOutcome = "no_op"
	commandProcessCleanupOutcomeGracefulSuccess  commandProcessCleanupOutcome = "graceful_success"
	commandProcessCleanupOutcomeForceKillSuccess commandProcessCleanupOutcome = "force_kill_success"
	commandProcessCleanupOutcomePartialFailure   commandProcessCleanupOutcome = "partial_failure"
	commandProcessCleanupOutcomeFailure          commandProcessCleanupOutcome = "failure"
)

type commandProcessCleanupContext struct {
	logger logging.Logger
	req    CommandRequest
	reason commandProcessCleanupReason
}

func newCommandProcessCleanupContext(logger logging.Logger, req CommandRequest, reason commandProcessCleanupReason) commandProcessCleanupContext {
	return commandProcessCleanupContext{
		logger: logging.EnsureLogger(logger),
		req:    req,
		reason: reason,
	}
}

func (c commandProcessCleanupContext) baseFields(eventName string, supervisorID int, extra ...any) []any {
	fields := []any{
		"event_name", eventName,
		"cleanup_reason", string(c.reason),
		"command", c.req.Command,
		"dispatch_id", c.req.DispatchID,
	}
	if supervisorID > 0 {
		fields = append(fields, "process_group_id", supervisorID)
	}
	fields = append(fields, extra...)
	return workLogFields(c.req.Execution, fields...)
}

func (c commandProcessCleanupContext) logStarted(supervisorID int) {
	c.logger.Info("command process cleanup: started",
		c.baseFields(workLogEventCommandProcessCleanupStarted, supervisorID, "status", "started")...)
}

func (c commandProcessCleanupContext) logGraceful(supervisorID int) {
	c.logger.Verbose("command process cleanup: graceful termination",
		c.baseFields(workLogEventCommandProcessCleanupGraceful, supervisorID, "status", "graceful")...)
}

func (c commandProcessCleanupContext) logForceKill(supervisorID int) {
	c.logger.Info("command process cleanup: force kill",
		c.baseFields(workLogEventCommandProcessCleanupForceKill, supervisorID, "status", "force_kill")...)
}

func (c commandProcessCleanupContext) logCompleted(outcome commandProcessCleanupOutcome, supervisorID int, err error, detail string) {
	fields := c.baseFields(
		workLogEventCommandProcessCleanupCompleted,
		supervisorID,
		"status", string(outcome),
		"outcome", string(outcome),
	)
	if detail != "" {
		fields = append(fields, "detail", detail)
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}

	msg := "command process cleanup: completed"
	switch outcome {
	case commandProcessCleanupOutcomeNoOp,
		commandProcessCleanupOutcomeGracefulSuccess,
		commandProcessCleanupOutcomeForceKillSuccess:
		if outcome == commandProcessCleanupOutcomeNoOp {
			c.logger.Verbose(msg, fields...)
			return
		}
		c.logger.Info(msg, fields...)
	case commandProcessCleanupOutcomePartialFailure:
		c.logger.Warn(msg, fields...)
	case commandProcessCleanupOutcomeFailure:
		c.logger.Warn(msg, fields...)
	default:
		c.logger.Info(msg, fields...)
	}
}

