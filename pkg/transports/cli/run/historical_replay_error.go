package run

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// replayStructuralCLIError keeps the Recordings-owned first-corrupt-event
// diagnostic intact at the local command boundary. It is intentionally a
// presentation adapter: structural classification and safe event context
// remain owned by Recordings, while the CLI exposes the actionable line and
// standard coded-error contract.
type replayStructuralCLIError struct {
	diagnostic recordings.ReplayArtifactDiagnostic
	cause      error
}

func (err *replayStructuralCLIError) Error() string {
	if err == nil {
		return ""
	}
	diagnostic := err.diagnostic
	code := strings.TrimSpace(string(diagnostic.Code))
	if code == "" {
		code = string(recordings.ReplayArtifactDiagnosticMalformed)
	}
	area := strings.TrimSpace(diagnostic.Area)
	path := strings.TrimSpace(diagnostic.Path)
	if area == "" {
		area = "events"
	}
	if path == "" {
		path = "events"
	}
	eventID := replayStructuralEventID(diagnostic.Message)
	action := strings.TrimSpace(diagnostic.Action)
	if action == "" {
		action = recordings.ReplayArtifactStructuralRepairAction
	}
	message := safeReplayStructuralMessage(diagnostic.Message)
	if message == "" {
		message = "recording event is structurally invalid"
	}
	return fmt.Sprintf(
		"Error: %s area=%s path=%s event=%s action=%s: %s",
		code, area, path, strconv.Quote(eventID), strconv.Quote(action), message,
	)
}

func (err *replayStructuralCLIError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *replayStructuralCLIError) CLIErrorCode() string {
	if err == nil || strings.TrimSpace(string(err.diagnostic.Code)) == "" {
		return string(recordings.ReplayArtifactDiagnosticMalformed)
	}
	return string(err.diagnostic.Code)
}

func (err *replayStructuralCLIError) CLIErrorFamily() factoryapi.ErrorFamily {
	return factoryapi.ErrorFamilyBadRequest
}

func (err *replayStructuralCLIError) CLIErrorMessage() string {
	return err.Error()
}

func newReplayStructuralCLIError(cause error) error {
	if cause == nil {
		return nil
	}
	var diagnostic recordings.ReplayArtifactDiagnostic
	var inputErr *recordings.ReplayInputError
	if errors.As(cause, &inputErr) && inputErr != nil {
		if inputErr.Family != recordings.ReplayInputFamilyLegacy ||
			!isLegacyReplayStructuralDiagnostic(inputErr.Diagnostic) {
			return nil
		}
		diagnostic = inputErr.Diagnostic
	} else {
		var artifactErr *recordings.ReplayArtifactError
		if !errors.As(cause, &artifactErr) || artifactErr == nil {
			return nil
		}
		diagnostic = artifactErr.Diagnostic
		if !isLegacyReplayStructuralDiagnostic(diagnostic) {
			return nil
		}
	}
	if !isStructuralReplayDiagnostic(diagnostic.Code) {
		return nil
	}
	return &replayStructuralCLIError{diagnostic: diagnostic, cause: cause}
}

func isLegacyReplayStructuralDiagnostic(diagnostic recordings.ReplayArtifactDiagnostic) bool {
	return strings.TrimSpace(diagnostic.Area) == "events" &&
		strings.HasPrefix(strings.TrimSpace(diagnostic.Path), "events[")
}

func isStructuralReplayDiagnostic(code recordings.ReplayArtifactDiagnosticCode) bool {
	switch code {
	case recordings.ReplayArtifactDiagnosticMalformed,
		recordings.ReplayArtifactDiagnosticInvalidIdentity,
		recordings.ReplayArtifactDiagnosticInvalidOrder,
		recordings.ReplayArtifactDiagnosticMissingReference,
		recordings.ReplayArtifactDiagnosticForeignReference:
		return true
	default:
		return false
	}
}

func replayStructuralEventID(message string) string {
	const prefix = `event "`
	start := strings.Index(message, prefix)
	if start < 0 {
		return "unknown"
	}
	start += len(prefix)
	end := strings.IndexByte(message[start:], '"')
	if end < 0 {
		return "unknown"
	}
	eventID := strings.TrimSpace(message[start : start+end])
	if eventID == "" {
		return "unknown"
	}
	return eventID
}

func safeReplayStructuralMessage(message string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
}

var _ clidiag.FamilyCodedError = (*replayStructuralCLIError)(nil)
