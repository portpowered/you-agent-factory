package factorysessions

import (
	"errors"
	"fmt"
	"strings"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const validationTargetKind = "factory-session-validation"

const (
	validationReasonRequired         = "required"
	validationReasonMissing          = "missing"
	validationReasonNotDirectory     = "not_directory"
	validationReasonNotRunnable      = "not_runnable"
	validationReasonConfigLoadFailed = "config_load_failed"
	validationReasonTargetNotFound   = "target_not_found"
	validationReasonUnreadable       = "unreadable"
	validationReasonConflict         = "conflict"
)

const validationErrorCodeConfigLoadFailed = "FACTORY_SESSION_CONFIG_LOAD_FAILED"

// ValidationTargetKind is the API error-target kind for factory-session validation failures.
const ValidationTargetKind = validationTargetKind

// ValidationReasonRequired reports that a required session field was empty.
const ValidationReasonRequired = validationReasonRequired

// ValidationReasonMissing reports that the session folder path does not exist.
const ValidationReasonMissing = validationReasonMissing

// ValidationReasonNotDirectory reports that the session folder path is not a directory.
const ValidationReasonNotDirectory = validationReasonNotDirectory

// ValidationReasonNotRunnable reports that the folder exposes no runnable factory targets.
const ValidationReasonNotRunnable = validationReasonNotRunnable

// ValidationReasonConfigLoadFailed reports that discovery found a factory target but loading its config failed.
const ValidationReasonConfigLoadFailed = validationReasonConfigLoadFailed

// ValidationReasonTargetNotFound reports that the requested target was not discovered.
const ValidationReasonTargetNotFound = validationReasonTargetNotFound

// ValidationReasonUnreadable reports that the session folder could not be read.
const ValidationReasonUnreadable = validationReasonUnreadable

// ValidationReasonConflict reports that init-new-factory cannot safely materialize a scaffold.
const ValidationReasonConflict = validationReasonConflict

type validationError struct {
	reason  string
	field   string
	err     error
	code    string
	targets []factoryvalidation.ValidationTarget
}

func (e *validationError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *validationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *validationError) ErrorTargets() []factoryvalidation.ValidationTarget {
	if e == nil {
		return nil
	}
	if len(e.targets) > 0 {
		return append([]factoryvalidation.ValidationTarget(nil), e.targets...)
	}
	return []factoryvalidation.ValidationTarget{validationErrorTarget(e.reason, e.field, e.Error())}
}

func (e *validationError) ErrorCode() string {
	if e == nil || strings.TrimSpace(e.code) == "" {
		return "BAD_REQUEST"
	}
	return e.code
}

// NewValidationError builds a structured factory-session validation error.
func NewValidationError(reason string, field string, err error) error {
	if err == nil {
		return nil
	}
	return &validationError{
		reason: reason,
		field:  field,
		err:    err,
	}
}

// NewConfigLoadFailedError builds a structured discovery-time config load failure.
func NewConfigLoadFailedError(failures []DiscoveryFailure) error {
	if len(failures) == 0 {
		return nil
	}
	targets := make([]factoryvalidation.ValidationTarget, 0, len(failures))
	for _, failure := range failures {
		targetID := TargetDisplayName(failure.Ref)
		message := fmt.Sprintf("Factory target %q at %q could not be loaded: %s", targetID, failure.FactoryDir, failure.Summary)
		targets = append(targets, validationTargetErrorTarget(validationReasonConfigLoadFailed, targetID, message))
	}
	return &validationError{
		reason:  validationReasonConfigLoadFailed,
		field:   "folderPath",
		err:     fmt.Errorf("factory configuration could not be loaded from the selected folder"),
		code:    validationErrorCodeConfigLoadFailed,
		targets: targets,
	}
}

// ValidationReasonFromError returns the validation reason and field when err is a factory-session validation error.
func ValidationReasonFromError(err error) (reason string, field string, ok bool) {
	var ve *validationError
	if !errors.As(err, &ve) || ve == nil {
		return "", "", false
	}
	return ve.reason, ve.field, true
}

func validationErrorTarget(reason string, field string, message string) factoryvalidation.ValidationTarget {
	if message == "" {
		message = "factory session validation failed"
	}
	return factoryvalidation.ValidationTarget{
		Code:     factoryvalidation.ValidationCodeFactorySessionField + "." + reason,
		Severity: factoryvalidation.ValidationSeverityError,
		Message:  message,
		Subject: factoryvalidation.ValidationSubject{
			Type:     factoryvalidation.ValidationSubjectTypeFactory,
			ID:       field,
			Location: factoryvalidation.ValidationSubjectLocationReference,
		},
	}
}

func validationTargetErrorTarget(reason string, targetID string, message string) factoryvalidation.ValidationTarget {
	return factoryvalidation.ValidationTarget{
		Code:     factoryvalidation.ValidationCodeFactorySessionTarget + "." + reason,
		Severity: factoryvalidation.ValidationSeverityError,
		Message:  message,
		Subject: factoryvalidation.ValidationSubject{
			Type:     factoryvalidation.ValidationSubjectTypeFactory,
			ID:       targetID,
			Location: factoryvalidation.ValidationSubjectLocationReference,
		},
	}
}
