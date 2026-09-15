package effects

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	hostProcessExitClassExited     = "EXITED"
	hostProcessExitClassNonzero    = "NONZERO_EXIT"
	hostProcessExitClassWaitFailed = "WAIT_FAILED"
	hostProcessExitClassUnknown    = "UNKNOWN"
)

const (
	hostProcessCauseCancelled        = "CANCELLED"
	hostProcessCauseEndpointBind     = "ENDPOINT_BIND_FAILED"
	hostProcessCauseModelLoad        = "MODEL_LOAD_FAILED"
	hostProcessCauseProcessExited    = "PROCESS_EXITED"
	hostProcessCauseProtocolIncompat = "PROTOCOL_INCOMPATIBLE"
	hostProcessCauseRPCRejected      = "RPC_REJECTED"
	hostProcessCauseTimedOut         = "TIMEOUT"
)

// ProjectHostProcessDiagnostic accepts only bounded terminal facts. The
// optional process effect is outside the Models owner, so every value is
// normalized again before it reaches logs or runtime evidence.
func ProjectHostProcessDiagnostic(
	snapshot HostProcessDiagnosticSnapshot,
) (HostProcessDiagnosticSnapshot, bool) {
	if strings.TrimSpace(snapshot.ExitClass) == "" {
		return HostProcessDiagnosticSnapshot{}, false
	}
	snapshot.ExitClass = normalizeHostProcessExitClass(snapshot.ExitClass)
	if !snapshot.ExitCodeKnown || snapshot.ExitCode < 0 {
		snapshot.ExitCode = 0
		snapshot.ExitCodeKnown = false
	}
	snapshot.Stdout = normalizeHostProcessStream(snapshot.Stdout)
	snapshot.Stderr = normalizeHostProcessStream(snapshot.Stderr)
	snapshot.CauseCode, snapshot.CauseMessage, snapshot.CauseMessageRedacted =
		normalizeHostProcessCause(snapshot.CauseCode, snapshot.CauseMessageRedacted)
	return snapshot, true
}

func normalizeHostProcessExitClass(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case hostProcessExitClassExited:
		return hostProcessExitClassExited
	case hostProcessExitClassNonzero:
		return hostProcessExitClassNonzero
	case hostProcessExitClassWaitFailed:
		return hostProcessExitClassWaitFailed
	default:
		return hostProcessExitClassUnknown
	}
}

func normalizeHostProcessStream(
	stream HostProcessStreamDiagnostic,
) HostProcessStreamDiagnostic {
	digest := strings.ToLower(strings.TrimSpace(stream.SHA256))
	if !validHostProcessSHA256(digest) {
		digest = ""
	}
	stream.SHA256 = digest
	return stream
}

func validHostProcessSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func normalizeHostProcessCause(code string, redacted bool) (string, string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	message, ok := hostProcessCauseMessage(code)
	if !ok {
		return "", "", false
	}
	return code, message, redacted
}

func hostProcessCauseMessage(code string) (string, bool) {
	switch code {
	case hostProcessCauseCancelled:
		return "backend operation cancelled", true
	case hostProcessCauseEndpointBind:
		return "backend endpoint bind failed", true
	case hostProcessCauseModelLoad:
		return "model load failed", true
	case hostProcessCauseProcessExited:
		return "managed backend process exited", true
	case hostProcessCauseProtocolIncompat:
		return "backend protocol incompatible", true
	case hostProcessCauseRPCRejected:
		return "backend RPC request rejected", true
	case hostProcessCauseTimedOut:
		return "backend operation timed out", true
	default:
		return "", false
	}
}

func attachHostProcessDiagnostic(
	record RuntimeEvidenceRecord,
	snapshot HostProcessDiagnosticSnapshot,
) (RuntimeEvidenceRecord, bool) {
	snapshot, ok := ProjectHostProcessDiagnostic(snapshot)
	if !ok {
		return record, false
	}
	record.ExitClass = snapshot.ExitClass
	record.ExitCode = snapshot.ExitCode
	record.ExitCodeKnown = snapshot.ExitCodeKnown
	record.StdoutBytes = snapshot.Stdout.Bytes
	record.StdoutSHA256 = snapshot.Stdout.SHA256
	record.StdoutTruncated = snapshot.Stdout.Truncated
	record.StderrBytes = snapshot.Stderr.Bytes
	record.StderrSHA256 = snapshot.Stderr.SHA256
	record.StderrTruncated = snapshot.Stderr.Truncated
	record.CauseCode = snapshot.CauseCode
	record.CauseMessage = snapshot.CauseMessage
	record.CauseMessageRedacted = snapshot.CauseMessageRedacted
	return record, true
}

func clearHostProcessDiagnostic(record RuntimeEvidenceRecord) RuntimeEvidenceRecord {
	record.ExitClass = ""
	record.ExitCode = 0
	record.ExitCodeKnown = false
	record.StdoutBytes = 0
	record.StdoutSHA256 = ""
	record.StdoutTruncated = false
	record.StderrBytes = 0
	record.StderrSHA256 = ""
	record.StderrTruncated = false
	record.CauseCode = ""
	record.CauseMessage = ""
	record.CauseMessageRedacted = false
	return record
}

func runtimeEvidenceHasHostProcessDiagnostic(record RuntimeEvidenceRecord) bool {
	return record.ExitClass != "" || record.ExitCodeKnown ||
		record.StdoutBytes != 0 || record.StdoutSHA256 != "" ||
		record.StdoutTruncated || record.StderrBytes != 0 ||
		record.StderrSHA256 != "" || record.StderrTruncated ||
		record.CauseCode != "" || record.CauseMessage != "" ||
		record.CauseMessageRedacted
}
