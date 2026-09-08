package platform_conformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

// MarshalCanonical returns the only persisted JSON representation used by the
// readiness schemas. Stable indentation and a terminal newline make the
// ledger/report bytes hashable and make an interrupted write observable.
func MarshalCanonical(value any) ([]byte, error) {
	value = canonicalValue(value)
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal canonical conformance JSON: %w", err)
	}
	return append(body, '\n'), nil
}

func canonicalValue(value any) any {
	switch typed := value.(type) {
	case RunSpec:
		return normalizeRunSpec(typed)
	case *RunSpec:
		if typed == nil {
			return typed
		}
		copy := normalizeRunSpec(*typed)
		return &copy
	case BudgetLedger:
		return normalizeBudgetLedger(typed)
	case *BudgetLedger:
		if typed == nil {
			return typed
		}
		copy := normalizeBudgetLedger(*typed)
		return &copy
	case Report:
		return normalizeReport(typed)
	case *Report:
		if typed == nil {
			return typed
		}
		copy := normalizeReport(*typed)
		return &copy
	default:
		return value
	}
}

func normalizeRunSpec(spec RunSpec) RunSpec {
	if spec.Commands == nil {
		spec.Commands = []CommandSpec{}
	}
	for index := range spec.Commands {
		if spec.Commands[index].Args == nil {
			spec.Commands[index].Args = []string{}
		}
		if spec.Commands[index].Environment == nil {
			spec.Commands[index].Environment = []string{}
		}
	}
	return spec
}

func normalizeBudgetLedger(ledger BudgetLedger) BudgetLedger {
	if ledger.Reservations == nil {
		ledger.Reservations = []BudgetReservation{}
	}
	return ledger
}

func normalizeReport(report Report) Report {
	if report.Commands == nil {
		report.Commands = []CommandEvidence{}
	}
	if report.SemanticObservations == nil {
		report.SemanticObservations = []SemanticObservation{}
	}
	if report.Policy.RootIdentities == nil {
		report.Policy.RootIdentities = []string{}
	}
	for index := range report.Commands {
		if report.Commands[index].Args == nil {
			report.Commands[index].Args = []string{}
		}
		if report.Commands[index].EnvironmentKeys == nil {
			report.Commands[index].EnvironmentKeys = []string{}
		}
	}
	return report
}

func decodeCanonical(body []byte, destination any, maxBytes int) error {
	if len(body) == 0 || len(body) > maxBytes {
		return fmt.Errorf("conformance JSON is empty or exceeds %d bytes", maxBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode conformance JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode conformance JSON: trailing value")
		}
		return fmt.Errorf("decode conformance JSON trailing value: %w", err)
	}
	canonical, err := MarshalCanonical(destination)
	if err != nil {
		return err
	}
	if !bytes.Equal(body, canonical) {
		return errors.New("decode conformance JSON: non-canonical bytes")
	}
	return nil
}

func readBounded(path string, maxBytes int) ([]byte, error) {
	if err := rejectSymlinkComponents(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("read %s: JSON exceeds %d bytes", pathIdentity(path), maxBytes)
	}
	return body, nil
}

// ReadRunSpec reads and validates a canonical run declaration without
// inspecting the referenced files. File and host checks belong to Admit.
func ReadRunSpec(path string) (RunSpec, error) {
	body, err := readBounded(path, MaxJSONBytes)
	if err != nil {
		return RunSpec{}, fmt.Errorf("read run specification: %w", err)
	}
	var spec RunSpec
	if err := decodeCanonical(body, &spec, MaxJSONBytes); err != nil {
		return RunSpec{}, fmt.Errorf("read run specification: %w", err)
	}
	if err := spec.validateShape(); err != nil {
		return RunSpec{}, fmt.Errorf("read run specification: %w", err)
	}
	return spec, nil
}

// WriteRunSpecAtomic persists a validated declaration for a later admission.
// It does not inspect or launch any declared artifact.
func WriteRunSpecAtomic(path string, spec RunSpec) error {
	if err := spec.validateShape(); err != nil {
		return fmt.Errorf("write run specification: %w", err)
	}
	body, err := MarshalCanonical(spec)
	if err != nil {
		return err
	}
	return writeJSONAtomic(path, body, nil)
}

func WriteRunSpec(path string, spec RunSpec) error { return WriteRunSpecAtomic(path, spec) }

func ReadBudgetLedger(path string) (BudgetLedger, error) {
	body, err := readBounded(path, MaxJSONBytes)
	if err != nil {
		return BudgetLedger{}, fmt.Errorf("read budget ledger: %w", err)
	}
	var ledger BudgetLedger
	if err := decodeCanonical(body, &ledger, MaxJSONBytes); err != nil {
		return BudgetLedger{}, fmt.Errorf("read budget ledger: %w", err)
	}
	if err := ledger.validate(); err != nil {
		return BudgetLedger{}, fmt.Errorf("read budget ledger: %w", err)
	}
	return ledger, nil
}

func WriteBudgetLedgerAtomic(path string, ledger BudgetLedger) error {
	ledger = normalizeBudgetLedger(ledger)
	if err := ledger.validate(); err != nil {
		return fmt.Errorf("write budget ledger: %w", err)
	}
	body, err := MarshalCanonical(ledger)
	if err != nil {
		return err
	}
	return writeJSONAtomic(path, body, nil)
}

func WriteBudgetLedger(path string, ledger BudgetLedger) error {
	return WriteBudgetLedgerAtomic(path, ledger)
}

func ReadReport(path string) (Report, error) {
	body, err := readBounded(path, MaxJSONBytes)
	if err != nil {
		return Report{}, fmt.Errorf("read readiness report: %w", err)
	}
	var report Report
	if err := decodeCanonical(body, &report, MaxJSONBytes); err != nil {
		return Report{}, fmt.Errorf("read readiness report: %w", err)
	}
	if err := report.validate(); err != nil {
		return Report{}, fmt.Errorf("read readiness report: %w", err)
	}
	return report, nil
}

func WriteReportAtomic(path string, report Report) error {
	return writeReportAtomicWithHook(path, report, nil)
}

func writeReportAtomicWithHook(path string, report Report, beforeRename func() error) error {
	report = normalizeReport(report)
	if err := report.validate(); err != nil {
		return fmt.Errorf("write readiness report: %w", err)
	}
	body, err := MarshalCanonical(report)
	if err != nil {
		return err
	}
	return writeJSONAtomic(path, body, beforeRename)
}

func WriteReport(path string, report Report) error { return WriteReportAtomic(path, report) }

// writeJSONAtomic is kept as the one persistence primitive so tests can inject
// an interruption immediately before publication and prove that the prior
// canonical destination remains unchanged.
func writeJSONAtomic(path string, body []byte, beforeRename func() error) error {
	if !validAbsolutePath(path) {
		return errors.New("atomic JSON path must be an absolute clean path")
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return fmt.Errorf("prepare atomic JSON path: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("atomic JSON destination is a symlink")
		}
		if !info.Mode().IsRegular() {
			return errors.New("atomic JSON destination is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect atomic JSON destination: %w", err)
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create atomic JSON parent: %w", err)
	}
	temporary, err := os.CreateTemp(parent, ".platform-conformance-*.tmp")
	if err != nil {
		return fmt.Errorf("create atomic JSON temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect atomic JSON temporary file: %w", err)
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write atomic JSON temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync atomic JSON temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close atomic JSON temporary file: %w", err)
	}
	if beforeRename != nil {
		if err := beforeRename(); err != nil {
			return err
		}
	}
	if err := (filesystem.Local{AllowRenameReplacement: true}).RenameReplacing(temporaryPath, path); err != nil {
		return fmt.Errorf("publish atomic JSON file: %w", err)
	}
	removeTemporary = false
	if err := syncAtomicDirectory(parent); err != nil {
		return fmt.Errorf("sync atomic JSON directory: %w", err)
	}
	return nil
}

func rejectSymlinkComponents(path string) error {
	if !validAbsolutePath(path) {
		return errors.New("path must be absolute and clean")
	}
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path component has a symlink (%s)", pathIdentity(current))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func canonicalBytes(path string) ([]byte, error) {
	body, err := readBounded(path, MaxJSONBytes)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func pathIdentity(path string) string {
	return "sha256:" + digestBytes([]byte(filepath.ToSlash(filepath.Clean(path))))
}

func PathIdentity(path string) string { return pathIdentity(path) }

func SHA256Hex(body []byte) string { return digestBytes(body) }

func digestBytes(body []byte) string {
	return fmt.Sprintf("%x", sha256Sum(body))
}

func sha256Sum(body []byte) [32]byte {
	// Kept in this small helper so all persisted identity code uses the same
	// byte-level operation without exposing raw paths in reports or errors.
	return sha256.Sum256(body)
}
