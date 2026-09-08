package platform_conformance

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HostIdentity is injectable so unit tests can exercise both supported Unix
// targets without pretending that a foreign operating system was executed.
type HostIdentity struct {
	OS   string
	Arch string
}

func CurrentHost() HostIdentity {
	return HostIdentity{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// ObservedArtifact is the bounded identity returned by the exact filesystem
// inspection edge. It contains no file contents.
type ObservedArtifact struct {
	Regular    bool
	Executable bool
	SizeBytes  int64
	SHA256     string
}

type ArtifactInspector interface {
	Inspect(string) (ObservedArtifact, error)
}

// LocalArtifactInspector hashes declared files by streaming them from the
// local filesystem. Symlinks are rejected before opening the file.
type LocalArtifactInspector struct{}

func (LocalArtifactInspector) Inspect(path string) (ObservedArtifact, error) {
	if err := rejectSymlinkComponents(path); err != nil {
		return ObservedArtifact{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return ObservedArtifact{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ObservedArtifact{}, errors.New("declared artifact is a symlink")
	}
	if !info.Mode().IsRegular() {
		return ObservedArtifact{}, errors.New("declared artifact is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return ObservedArtifact{}, err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return ObservedArtifact{}, err
	}
	return ObservedArtifact{
		Regular: true, Executable: info.Mode().Perm()&0o111 != 0,
		SizeBytes: info.Size(), SHA256: fmt.Sprintf("%x", hasher.Sum(nil)),
	}, nil
}

type Admission struct {
	Spec                RunSpec
	Host                HostIdentity
	SpecIdentity        string
	RootIdentities      []string
	CLIPathIdentity     string
	BackendPathIdentity string
	ModelPathIdentity   string
	FixturePathIdentity string
}

// Admit performs all pre-launch checks. A missing ledger is allowed because
// the first locked reservation creates it; an existing ledger is read and
// checked for foreign, finalized, or already-over-budget state.
func Admit(spec RunSpec) (Admission, error) {
	return AdmitForHost(spec, CurrentHost())
}

func AdmitForHost(spec RunSpec, host HostIdentity) (Admission, error) {
	return AdmitWithInspector(spec, host, LocalArtifactInspector{})
}

func AdmitWithInspector(spec RunSpec, host HostIdentity, inspector ArtifactInspector) (Admission, error) {
	if inspector == nil {
		return Admission{}, admissionFailure("missing_inspector", "inspector", "artifact inspector", "nil", nil)
	}
	if err := spec.validateShape(); err != nil {
		return Admission{}, admissionFromSchema(err)
	}
	if host.OS != spec.Target.OS || host.Arch != spec.Target.Arch {
		return Admission{}, admissionFailure("unsupported_host", "target", spec.Target.OS+"/"+spec.Target.Arch, host.OS+"/"+host.Arch, nil)
	}
	if err := validateIsolation(spec); err != nil {
		return Admission{}, err
	}
	if err := validateDeclaredPaths(spec); err != nil {
		return Admission{}, err
	}
	identities, err := inspectDeclaredArtifacts(spec, inspector)
	if err != nil {
		return Admission{}, err
	}
	if err := validateExistingEvidence(spec); err != nil {
		return Admission{}, err
	}
	canonical, err := MarshalCanonical(spec)
	if err != nil {
		return Admission{}, admissionFailure("identity_unavailable", "run", "canonical run identity", "marshal failed", err)
	}
	return Admission{
		Spec: spec, Host: host, SpecIdentity: "sha256:" + digestBytes(canonical),
		RootIdentities: []string{
			PathIdentity(spec.Roots.Work), PathIdentity(spec.Roots.State),
			PathIdentity(spec.Roots.Cache), PathIdentity(spec.Roots.Temp),
			PathIdentity(spec.Roots.Output),
		},
		CLIPathIdentity: identities.cli.PathIdentity, BackendPathIdentity: identities.backend.PathIdentity,
		ModelPathIdentity: identities.model.PathIdentity, FixturePathIdentity: identities.fixture.PathIdentity,
	}, nil
}

type declaredIdentities struct {
	cli, backend, model, fixture declaredPathIdentity
}

type declaredPathIdentity struct {
	PathIdentity string
	SHA256       string
	SizeBytes    int64
}

func inspectDeclaredArtifacts(spec RunSpec, inspector ArtifactInspector) (declaredIdentities, error) {
	cli, err := inspectOneArtifact("cli", spec.CLI.Path, spec.CLI.SHA256, spec.CLI.SizeBytes, true, inspector)
	if err != nil {
		return declaredIdentities{}, err
	}
	backend, err := inspectOneArtifact("backend", spec.Backend.Path, spec.Backend.SHA256, spec.Backend.SizeBytes, true, inspector)
	if err != nil {
		return declaredIdentities{}, err
	}
	model, err := inspectOneArtifact("model", spec.Model.Path, spec.Model.SHA256, spec.Model.SizeBytes, false, inspector)
	if err != nil {
		return declaredIdentities{}, err
	}
	fixture, err := inspectOneArtifact("fixture", spec.Fixture.Path, spec.Fixture.SHA256, spec.Fixture.SizeBytes, false, inspector)
	if err != nil {
		return declaredIdentities{}, err
	}
	return declaredIdentities{cli: cli, backend: backend, model: model, fixture: fixture}, nil
}

func inspectOneArtifact(label, path, expectedSHA string, expectedSize int64, executable bool, inspector ArtifactInspector) (declaredPathIdentity, error) {
	observed, err := inspector.Inspect(path)
	if err != nil {
		code := "artifact_unavailable"
		if errors.Is(err, os.ErrNotExist) {
			code = "artifact_missing"
		}
		return declaredPathIdentity{}, admissionFailure(code, label+".path", "declared regular artifact", "unavailable", err)
	}
	if !observed.Regular {
		return declaredPathIdentity{}, admissionFailure("artifact_not_regular", label+".path", "regular file", "not regular", nil)
	}
	if executable && !observed.Executable {
		return declaredPathIdentity{}, admissionFailure("artifact_not_executable", label+".path", "executable file", "not executable", nil)
	}
	if observed.SizeBytes != expectedSize {
		return declaredPathIdentity{}, admissionFailure("artifact_size_mismatch", label+".sizeBytes", fmt.Sprint(expectedSize), fmt.Sprint(observed.SizeBytes), nil)
	}
	if strings.ToLower(observed.SHA256) != expectedSHA {
		return declaredPathIdentity{}, admissionFailure("artifact_digest_mismatch", label+".sha256", expectedSHA, observed.SHA256, nil)
	}
	return declaredPathIdentity{PathIdentity: PathIdentity(path), SHA256: observed.SHA256, SizeBytes: observed.SizeBytes}, nil
}

func validateIsolation(spec RunSpec) error {
	rootPaths := []struct {
		name string
		path string
	}{
		{"work", spec.Roots.Work}, {"state", spec.Roots.State}, {"cache", spec.Roots.Cache},
		{"temp", spec.Roots.Temp}, {"output", spec.Roots.Output},
	}
	for first := range rootPaths {
		for second := first + 1; second < len(rootPaths); second++ {
			if pathsOverlap(rootPaths[first].path, rootPaths[second].path) {
				return admissionFailure("path_overlap", "roots", "disjoint isolated roots", rootPaths[first].name+"/"+rootPaths[second].name, nil)
			}
		}
	}
	reservedPaths := []struct {
		name string
		path string
	}{
		{"cli", spec.CLI.Path}, {"backend", spec.Backend.Path}, {"model", spec.Model.Path},
		{"fixture", spec.Fixture.Path}, {"report", spec.ReportPath}, {"ledger", spec.LedgerPath},
	}
	for _, reserved := range reservedPaths {
		for _, root := range rootPaths {
			if pathsOverlap(reserved.path, root.path) {
				return admissionFailure("path_overlap", reserved.name, "outside isolated roots", root.name, nil)
			}
		}
	}
	if pathsOverlap(spec.CLI.Path, spec.Backend.Path) || pathsOverlap(spec.CLI.Path, spec.Model.Path) ||
		pathsOverlap(spec.CLI.Path, spec.Fixture.Path) || pathsOverlap(spec.Backend.Path, spec.Model.Path) ||
		pathsOverlap(spec.Backend.Path, spec.Fixture.Path) || pathsOverlap(spec.Model.Path, spec.Fixture.Path) {
		return admissionFailure("path_collision", "artifacts", "distinct artifacts", "overlap", nil)
	}
	return nil
}

func validateDeclaredPaths(spec RunSpec) error {
	paths := []struct {
		field string
		path  string
	}{
		{"cli.path", spec.CLI.Path}, {"backend.path", spec.Backend.Path}, {"model.path", spec.Model.Path},
		{"fixture.path", spec.Fixture.Path}, {"reportPath", spec.ReportPath}, {"ledgerPath", spec.LedgerPath},
		{"roots.work", spec.Roots.Work}, {"roots.state", spec.Roots.State}, {"roots.cache", spec.Roots.Cache},
		{"roots.temp", spec.Roots.Temp}, {"roots.output", spec.Roots.Output},
	}
	for _, declared := range paths {
		if err := rejectSymlinkComponents(declared.path); err != nil {
			return admissionFailure("symlink_path", declared.field, "no symlink path components", "symlink", err)
		}
	}
	if info, err := os.Lstat(spec.ReportPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return admissionFailure("report_path_reused", "reportPath", "absent regular-file destination", "occupied", nil)
		}
		return admissionFailure("report_path_reused", "reportPath", "absent destination", "already exists", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return admissionFailure("report_path_unavailable", "reportPath", "inspectable destination", "unavailable", err)
	}
	return nil
}

func validateExistingEvidence(spec RunSpec) error {
	info, err := os.Lstat(spec.LedgerPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return admissionFailure("ledger_unavailable", "ledgerPath", "inspectable ledger", "unavailable", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return admissionFailure("ledger_invalid", "ledgerPath", "regular canonical ledger", "not regular", nil)
	}
	ledger, err := ReadBudgetLedger(spec.LedgerPath)
	if err != nil {
		return admissionFailure("ledger_invalid", "ledgerPath", "canonical valid ledger", "invalid", err)
	}
	if ledger.RunID != spec.RunID {
		return admissionFailure("ledger_foreign", "ledger.runId", spec.RunID, "different run", nil)
	}
	if ledger.LedgerID != DefaultLedgerID(spec.RunID) {
		return admissionFailure("ledger_identity_drift", "ledger.ledgerId", DefaultLedgerID(spec.RunID), ledger.LedgerID, nil)
	}
	if !sameLimits(ledger.Limits, spec.Limits) {
		return admissionFailure("ledger_drift", "ledger.limits", "limits match run specification", "different limits", nil)
	}
	if ledger.Finalized {
		return admissionFailure("ledger_finalized", "ledger.finalized", "false", "true", nil)
	}
	return nil
}

func sameLimits(first, second BudgetLimits) bool {
	return first == second
}

func pathsOverlap(first, second string) bool {
	return samePath(first, second) || pathWithin(first, second) || pathWithin(second, first)
}

func pathWithin(path, parent string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(path))
	if err != nil || relative == "." {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func admissionFromSchema(err error) *AdmissionError {
	var schemaErr *SchemaError
	if errors.As(err, &schemaErr) {
		return admissionFailure(schemaErr.Code, schemaErr.Field, schemaErr.Expected, schemaErr.Observed, err)
	}
	return admissionFailure("invalid_specification", "run", "valid run specification", "invalid", err)
}
