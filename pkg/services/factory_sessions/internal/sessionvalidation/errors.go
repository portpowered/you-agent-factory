package sessionvalidation

import (
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

const configLoadFailedCode = "FACTORY_SESSION_CONFIG_LOAD_FAILED"

type Error struct {
	reason  string
	field   string
	err     error
	code    string
	targets []factorydefinitions.ValidationTarget
}

func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *Error) ErrorTargets() []factorydefinitions.ValidationTarget {
	if e == nil {
		return nil
	}
	if len(e.targets) > 0 {
		return append([]factorydefinitions.ValidationTarget(nil), e.targets...)
	}
	return []factorydefinitions.ValidationTarget{errorTarget(e.reason, e.field, e.Error())}
}

func (e *Error) ErrorCode() string {
	if e == nil || strings.TrimSpace(e.code) == "" {
		return "BAD_REQUEST"
	}
	return e.code
}

func New(reason, field string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{reason: reason, field: field, err: err}
}

func NewConfigLoadFailed(failures []factorysessions.DiscoveryFailure) error {
	if len(failures) == 0 {
		return nil
	}
	targets := make([]factorydefinitions.ValidationTarget, 0, len(failures))
	for _, failure := range failures {
		targetID := targetDisplayName(failure.Ref)
		message := fmt.Sprintf("Factory target %q at %q could not be loaded: %s", targetID, failure.FactoryDir, failure.Summary)
		targets = append(targets, targetErrorTarget(factorysessions.ValidationReasonConfigLoadFailed, targetID, message))
	}
	return &Error{
		reason:  factorysessions.ValidationReasonConfigLoadFailed,
		field:   "folderPath",
		err:     fmt.Errorf("factory configuration could not be loaded from the selected folder"),
		code:    configLoadFailedCode,
		targets: targets,
	}
}

func ReasonFromError(err error) (reason string, field string, ok bool) {
	var validation *Error
	if !errors.As(err, &validation) || validation == nil {
		return "", "", false
	}
	return validation.reason, validation.field, true
}

func targetDisplayName(ref factorysessions.TargetRef) string {
	if ref.Kind == factorysessions.TargetKindDefault {
		return "default"
	}
	return ref.Name
}

func errorTarget(reason, field, message string) factorydefinitions.ValidationTarget {
	if message == "" {
		message = "factory session validation failed"
	}
	return factorydefinitions.ValidationTarget{
		Code:     factorydefinitions.ValidationCodeFactorySessionField + "." + reason,
		Severity: factorydefinitions.ValidationSeverityError,
		Message:  message,
		Subject: factorydefinitions.ValidationSubject{
			Type:     factorydefinitions.ValidationSubjectTypeFactory,
			ID:       field,
			Location: factorydefinitions.ValidationSubjectLocationReference,
		},
	}
}

func targetErrorTarget(reason, targetID, message string) factorydefinitions.ValidationTarget {
	return factorydefinitions.ValidationTarget{
		Code:     factorydefinitions.ValidationCodeFactorySessionTarget + "." + reason,
		Severity: factorydefinitions.ValidationSeverityError,
		Message:  message,
		Subject: factorydefinitions.ValidationSubject{
			Type:     factorydefinitions.ValidationSubjectTypeFactory,
			ID:       targetID,
			Location: factorydefinitions.ValidationSubjectLocationReference,
		},
	}
}
