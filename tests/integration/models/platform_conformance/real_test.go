package platform_conformance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	conformanceRealEnv   = "LOCALAI_CONFORMANCE_REAL"
	conformanceSpecEnv   = "LOCALAI_CONFORMANCE_SPEC"
	conformanceRealOptIn = "1"
)

// realEntryEnvironment is kept separate from process-global environment reads
// so the admission gate can be checked without launching a child or mutating
// the test process environment.
type realEntryEnvironment struct {
	OptIn    string
	SpecPath string
}

type realEntryResult struct {
	Skipped   bool
	Admission Admission
}

func currentRealEntryEnvironment() realEntryEnvironment {
	return realEntryEnvironment{
		OptIn:    os.Getenv(conformanceRealEnv),
		SpecPath: os.Getenv(conformanceSpecEnv),
	}
}

// prepareRealEntry is the only gate before a future native runner may launch.
// This story deliberately stops after admission: a successful result proves
// immutable runner readiness, not a customer, model, backend, or platform
// operation.
func prepareRealEntry(environment realEntryEnvironment, host HostIdentity, inspector ArtifactInspector) (realEntryResult, error) {
	if environment.OptIn != conformanceRealOptIn {
		return realEntryResult{Skipped: true}, nil
	}
	if strings.TrimSpace(environment.SpecPath) == "" {
		return realEntryResult{}, admissionFailure(
			"missing_specification", conformanceSpecEnv, "canonical run specification path", "empty", nil,
		)
	}
	spec, err := ReadRunSpec(environment.SpecPath)
	if err != nil {
		var schemaErr *SchemaError
		if errors.As(err, &schemaErr) {
			return realEntryResult{}, admissionFromSchema(schemaErr)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return realEntryResult{}, admissionFailure(
				"invalid_specification", conformanceSpecEnv, "canonical run specification", "malformed", err,
			)
		}
		return realEntryResult{}, admissionFailure(
			"spec_unavailable", conformanceSpecEnv, "readable canonical run specification", "unavailable", err,
		)
	}
	admission, err := AdmitWithInspector(spec, host, inspector)
	if err != nil {
		return realEntryResult{}, err
	}
	return realEntryResult{Admission: admission}, nil
}

// TestPortableRealConformance is dark unless explicitly opted in. Its
// successful result is intentionally a runner-readiness claim only; native
// execution and semantic customer evidence belong to the separately budgeted
// macOS/Linux C1 gates.
func TestPortableRealConformance(t *testing.T) {
	result, err := prepareRealEntry(currentRealEntryEnvironment(), CurrentHost(), LocalArtifactInspector{})
	if result.Skipped {
		t.Skipf("real conformance is dark by default; set %s=%s and %s to opt in", conformanceRealEnv, conformanceRealOptIn, conformanceSpecEnv)
	}
	if err != nil {
		t.Fatalf("real conformance admission failed before launch: %v", err)
	}
	t.Logf(
		"runner readiness PASS only: run=%s target=%s/%s; no child process, network request, download, model call, or customer semantic PASS was attempted; later gates: VAL-C1-MACOS-REAL, VAL-C1-LINUX-REAL, named macOS/Linux blind gates, G-REVIEW-CI-MERGE",
		result.Admission.Spec.RunID, result.Admission.Spec.Target.OS, result.Admission.Spec.Target.Arch,
	)
}

func TestPortableRealEntryDarkByDefault(t *testing.T) {
	fixture := newReadinessFixture(t)
	result, err := prepareRealEntry(realEntryEnvironment{}, fixture.host, fixture.inspector)
	if err != nil {
		t.Fatalf("dark real entry returned an error: %v", err)
	}
	if !result.Skipped || result.Admission.Spec.RunID != "" {
		t.Fatalf("dark real entry result = %+v, want skipped without admission", result)
	}
	if len(fixture.inspector.calls) != 0 {
		t.Fatalf("dark real entry inspected artifacts: %v", fixture.inspector.calls)
	}
	requireNoRealReport(t, fixture.spec.ReportPath)
}

func TestPortableRealEntryRejectsMissingSpecification(t *testing.T) {
	fixture := newReadinessFixture(t)
	result, err := prepareRealEntry(realEntryEnvironment{
		OptIn: conformanceRealOptIn, SpecPath: filepath.Join(fixture.root, "missing-run.json"),
	}, fixture.host, fixture.inspector)
	if result.Admission.Spec.RunID != "" {
		t.Fatalf("unavailable real entry returned admission: %+v", result.Admission)
	}
	requireAdmissionCode(t, err, "spec_unavailable")
	if len(fixture.inspector.calls) != 0 {
		t.Fatalf("unavailable specification inspected artifacts: %v", fixture.inspector.calls)
	}
	requireNoRealReport(t, fixture.spec.ReportPath)
}

func TestPortableRealEntryRejectsMalformedSpecification(t *testing.T) {
	fixture := newReadinessFixture(t)
	specPath := filepath.Join(fixture.root, "malformed-run.json")
	if err := os.WriteFile(specPath, []byte("{\n"), 0o600); err != nil {
		t.Fatalf("write malformed real entry specification: %v", err)
	}
	result, err := prepareRealEntry(realEntryEnvironment{
		OptIn: conformanceRealOptIn, SpecPath: specPath,
	}, fixture.host, fixture.inspector)
	if result.Admission.Spec.RunID != "" {
		t.Fatalf("malformed real entry returned admission: %+v", result.Admission)
	}
	requireAdmissionCode(t, err, "invalid_specification")
	if len(fixture.inspector.calls) != 0 {
		t.Fatalf("malformed specification inspected artifacts: %v", fixture.inspector.calls)
	}
	requireNoRealReport(t, fixture.spec.ReportPath)
}

func TestPortableRealEntryRejectsArtifactDrift(t *testing.T) {
	fixture := newReadinessFixture(t)
	specPath := filepath.Join(fixture.root, "run.json")
	if err := WriteRunSpecAtomic(specPath, fixture.spec); err != nil {
		t.Fatalf("write real entry specification: %v", err)
	}
	fixture.inspector.artifacts[fixture.spec.Model.Path] = ObservedArtifact{
		Regular: true, SizeBytes: fixture.spec.Model.SizeBytes, SHA256: strings.Repeat("0", 64),
	}
	result, err := prepareRealEntry(realEntryEnvironment{
		OptIn: conformanceRealOptIn, SpecPath: specPath,
	}, fixture.host, fixture.inspector)
	if result.Admission.Spec.RunID != "" {
		t.Fatalf("drifted real entry returned admission: %+v", result.Admission)
	}
	requireAdmissionCode(t, err, "artifact_digest_mismatch")
	requireNoRealReport(t, fixture.spec.ReportPath)
}

func TestPortableRealEntryAdmitsReadinessOnly(t *testing.T) {
	fixture := newReadinessFixture(t)
	specPath := filepath.Join(fixture.root, "run.json")
	if err := WriteRunSpecAtomic(specPath, fixture.spec); err != nil {
		t.Fatalf("write real entry specification: %v", err)
	}
	result, err := prepareRealEntry(realEntryEnvironment{
		OptIn: conformanceRealOptIn, SpecPath: specPath,
	}, fixture.host, fixture.inspector)
	if err != nil {
		t.Fatalf("valid real entry rejected: %v", err)
	}
	if result.Skipped || result.Admission.SpecIdentity == "" {
		t.Fatalf("valid real entry result = %+v, want admitted readiness", result)
	}
	requireNoRealReport(t, fixture.spec.ReportPath)
}

func TestPortableRealEntryRejectsForeignHost(t *testing.T) {
	fixture := newReadinessFixture(t)
	specPath := filepath.Join(fixture.root, "run.json")
	if err := WriteRunSpecAtomic(specPath, fixture.spec); err != nil {
		t.Fatalf("write real entry specification: %v", err)
	}
	result, err := prepareRealEntry(realEntryEnvironment{
		OptIn: conformanceRealOptIn, SpecPath: specPath,
	}, HostIdentity{OS: TargetDarwin, Arch: ArchARM64}, fixture.inspector)
	if result.Admission.Spec.RunID != "" {
		t.Fatalf("foreign-host real entry returned admission: %+v", result.Admission)
	}
	requireAdmissionCode(t, err, "unsupported_host")
	if len(fixture.inspector.calls) != 0 {
		t.Fatalf("foreign-host admission inspected artifacts: %v", fixture.inspector.calls)
	}
}

func requireNoRealReport(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("real entry changed report destination: %v", err)
	}
}
