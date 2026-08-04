package service

import (
	"fmt"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// genericFailureDetail is the fixed, per-Kind placeholder attached to a
// FailureCause when no Workers-documented structured classification is
// available. FailureCause.Kind already identifies the failure category
// (including EXECUTOR_PANIC), so a fixed generic Detail never hides the
// classification itself.
var genericFailureDetail = map[workersessions.FailureCauseKind]string{
	workersessions.FailureCauseStartFailure:            "the attempt could not be handed off to Workers",
	workersessions.FailureCauseWorkersExecutionFailure: "the Workers execution result was not successful",
	workersessions.FailureCauseAdapterFailure:          "the Workers adapter reported a failure",
	workersessions.FailureCauseExecutorPanic:           "the Workers executor reported a panic",
	workersessions.FailureCauseEventPublicationFailure: "the Worker Session opening record could not be published",
}

// knownFailureFamilies whitelists the exact WorkFailureFamily constants
// Workers documents (execution_contracts.go). WorkFailureFamily is an
// exported Go string type, not a runtime-validated enum, so a
// WorkFailureMetadata value can be constructed with any string, including
// attacker-controlled prompt/command/credential text; only a value present in
// this set is ever echoed into the public FailureCause.Detail.
var knownFailureFamilies = map[workers.WorkFailureFamily]bool{
	workers.WorkFailureFamilyTerminal:  true,
	workers.WorkFailureFamilyRetryable: true,
	workers.WorkFailureFamilyThrottle:  true,
}

// knownFailureTypes whitelists the exact WorkFailureType constants Workers
// documents. See knownFailureFamilies for why this whitelist exists.
var knownFailureTypes = map[workers.WorkFailureType]bool{
	workers.WorkFailureTypeAuthFailure:         true,
	workers.WorkFailureTypePermanentBadRequest: true,
	workers.WorkFailureTypeThrottled:           true,
	workers.WorkFailureTypeInternalServerError: true,
	workers.WorkFailureTypeTimeout:             true,
	workers.WorkFailureTypeUnknown:             true,
	workers.WorkFailureTypeMisconfigured:       true,
	workers.WorkFailureTypeCommandLineTooLong:  true,
	workers.WorkFailureTypeMissingExecutable:   true,
}

// safeDetail derives the public FailureCause.Detail for kind exclusively
// from Workers-documented, closed-vocabulary structured fields
// (WorkResult.FailureMetadata's Family/Type, whitelisted against the exact
// constants Workers defines as its stable customer-facing normalized failure
// vocabulary) or a fixed generic placeholder. safeDetail never reads
// WorkResult.Error or an adapter error's message: neither Workers nor the
// adapter boundary establishes that free-form text as free of payloads,
// credentials, environment values, prompts, or raw provider commands, so it
// is never attached to Detail in any form, redacted or otherwise.
// classifyTerminal still inspects that raw text for executor-panic evidence,
// but only to choose kind, never to build Detail. A Family or Type value
// that is not blank and not one of the whitelisted constants falls back to
// the fixed generic placeholder for kind rather than being echoed, so an
// unrecognized (potentially attacker-controlled) string can never reach
// Detail.
func safeDetail(kind workersessions.FailureCauseKind, metadata *workers.WorkFailureMetadata) string {
	if metadata == nil {
		return genericFailureDetail[kind]
	}
	family, familyKnown := safeFamily(metadata.Family)
	typ, typeKnown := safeType(metadata.Type)
	if !familyKnown || !typeKnown {
		return genericFailureDetail[kind]
	}
	if family == "" && typ == "" {
		return genericFailureDetail[kind]
	}
	return fmt.Sprintf("family=%s type=%s", orUnknown(family), orUnknown(typ))
}

// safeFamily returns (value, true) when family is blank or a whitelisted
// WorkFailureFamily constant. Any other value returns ("", false), which
// tells safeDetail to fall back to the fixed generic placeholder instead of
// echoing an unrecognized value.
func safeFamily(family workers.WorkFailureFamily) (string, bool) {
	if family == "" {
		return "", true
	}
	if knownFailureFamilies[family] {
		return string(family), true
	}
	return "", false
}

// safeType returns (value, true) when typ is blank or a whitelisted
// WorkFailureType constant. Any other value returns ("", false); see
// safeFamily.
func safeType(typ workers.WorkFailureType) (string, bool) {
	if typ == "" {
		return "", true
	}
	if knownFailureTypes[typ] {
		return string(typ), true
	}
	return "", false
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
