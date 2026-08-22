package process

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

var errCommandProcessCleanupPartialFailure = errors.New("command process cleanup partial failure")

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
	logger             logging.Logger
	req                CommandRequest
	reason             commandProcessCleanupReason
	cancellationReason CancellationReason
}

func newCommandProcessCleanupContext(
	logger logging.Logger,
	req CommandRequest,
	reason commandProcessCleanupReason,
	cancellationReasons ...CancellationReason,
) commandProcessCleanupContext {
	cleanup := commandProcessCleanupContext{logger: logging.EnsureLogger(logger), req: req, reason: reason}
	if len(cancellationReasons) > 0 {
		cleanup.cancellationReason = cancellationReasons[0]
	}
	return cleanup
}

func (c commandProcessCleanupContext) logStarted(supervisorID int) {
	c.logger.Info("command runner: cleanup started",
		commandRunnerCleanupLogFields(c.req, "command_runner.cleanup_started", c.reason, supervisorID, c.cancellationReason, "status", "started")...)
}

func (c commandProcessCleanupContext) logGraceful(supervisorID int) {
	c.logger.Verbose("command runner: cleanup graceful termination",
		commandRunnerCleanupLogFields(c.req, "command_runner.cleanup_graceful", c.reason, supervisorID, c.cancellationReason, "status", "graceful")...)
}

func (c commandProcessCleanupContext) logForceKill(supervisorID int) {
	c.logger.Info("command runner: cleanup force kill",
		commandRunnerCleanupLogFields(c.req, "command_runner.cleanup_force_kill", c.reason, supervisorID, c.cancellationReason, "status", "force_kill")...)
}

func (c commandProcessCleanupContext) logCompleted(outcome commandProcessCleanupOutcome, supervisorID int, err error, detail string) {
	fields := commandRunnerCleanupLogFields(
		c.req,
		"command_runner.cleanup_completed",
		c.reason,
		supervisorID,
		c.cancellationReason,
		"status", string(outcome),
		"outcome", string(outcome),
	)
	if detail != "" {
		fields = append(fields, "detail", detail)
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}

	msg := "command runner: cleanup completed"
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

func commandRunnerCleanupLogFields(req CommandRequest, eventName string, reason commandProcessCleanupReason, supervisorID int, cancellationReason CancellationReason, extra ...any) []any {
	fields := []any{
		"event_name", eventName,
		"cleanup_reason", string(reason),
		"command", req.Command,
		"args_count", len(req.Args),
	}
	if reason == commandProcessCleanupReasonCancel {
		// The caller supplied reason is intentionally bounded and contains no
		// command payload or process output.
		fields = append(fields, "cancellation_reason", string(firstCancellationReason(cancellationReason, CancellationReasonCanceled)))
	}
	if req.WorkDir != "" {
		fields = append(fields, "working_dir", req.WorkDir)
	}
	if supervisorID > 0 {
		fields = append(fields, "process_group_id", supervisorID)
	}
	return append(fields, extra...)
}
