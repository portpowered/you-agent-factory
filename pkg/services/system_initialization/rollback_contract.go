package systeminitialization

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInitializePartialFailure reports that Initialize stopped after completing
// one or more bootstrap steps without finishing the full workflow. Peers branch
// on this sentinel with errors.Is and inspect Bootstrap-owned rollback facts
// with errors.As without importing Operator Settings, Factory Definitions, or
// initializer lifecycle types.
var ErrInitializePartialFailure = errors.New("system bootstrap initialize partial failure")

// InitializeStepID identifies one bootstrap step in partial-failure rollback
// reporting. System Bootstrap owns step ordering, idempotency, and rollback
// fact vocabulary; Operator Settings and Factory Definitions retain their own
// transactional store boundaries.
type InitializeStepID string

const (
	InitializeStepLegacyMigration   InitializeStepID = "legacy_migration"
	InitializeStepSystemConfig      InitializeStepID = "system_config"
	InitializeStepPackagedFactories InitializeStepID = "packaged_factories"
)

// RollbackStepOutcome classifies how one bootstrap step ended when Initialize
// fails partway through the workflow.
type RollbackStepOutcome string

const (
	// RollbackStepCompleted reports the step finished successfully before the
	// partial failure surfaced.
	RollbackStepCompleted RollbackStepOutcome = "completed"
	// RollbackStepRolledBackOrPreserved reports Bootstrap rolled the step back or
	// intentionally preserved customer-owned state after the failure.
	RollbackStepRolledBackOrPreserved RollbackStepOutcome = "rolled_back_or_preserved"
	// RollbackStepUnresolved reports the step remains dirty or otherwise
	// unresolved after the partial failure.
	RollbackStepUnresolved RollbackStepOutcome = "unresolved"
)

// RollbackFact is one Bootstrap-owned rollback outcome for a single initialize
// step. Peers use these plain values to report partial-failure work facts
// without importing Settings or Definitions store types.
type RollbackFact struct {
	Step    InitializeStepID
	Outcome RollbackStepOutcome
}

// Clone returns a detached rollback-fact copy.
func (fact RollbackFact) Clone() RollbackFact {
	return fact
}

// InitializePartialFailure carries Bootstrap-owned rollback facts for one
// partial Initialize failure. Pure validation failures and cancelled-context
// failures do not use this type.
type InitializePartialFailure struct {
	Message string
	Facts   []RollbackFact
	Cause   error
}

func (failure InitializePartialFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	if message == "" {
		return ErrInitializePartialFailure.Error()
	}
	return fmt.Sprintf("%s: %s", ErrInitializePartialFailure.Error(), message)
}

func (failure InitializePartialFailure) Unwrap() error {
	if failure.Cause != nil {
		return failure.Cause
	}
	return ErrInitializePartialFailure
}

func (failure InitializePartialFailure) Is(target error) bool {
	return target == ErrInitializePartialFailure
}

// Clone returns a detached partial-failure copy.
func (failure InitializePartialFailure) Clone() InitializePartialFailure {
	facts := failure.Facts
	failure.Facts = make([]RollbackFact, len(facts))
	for i := range facts {
		failure.Facts[i] = facts[i].Clone()
	}
	return failure
}
