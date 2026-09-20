package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func ReadProbeInputV2(path string) (ProbeInputV2, error) {
	path, err := normalizeProbePath(path, "input path")
	if err != nil {
		return ProbeInputV2{}, err
	}
	if err := validateExistingRegularFile(path, "input"); err != nil {
		return ProbeInputV2{}, err
	}
	data, err := readProbeJSON(path, probeInputMaxBytes)
	if err != nil {
		return ProbeInputV2{}, fmt.Errorf("read corpus runner v2 input: %w", err)
	}
	var input ProbeInputV2
	if err := decodeStrictJSON(data, &input); err != nil {
		return ProbeInputV2{}, strictJSONError("input", err)
	}
	if err := input.Validate(); err != nil {
		return ProbeInputV2{}, err
	}
	return input, nil
}

func WriteProbeInputV2Atomic(path string, input ProbeInputV2) error {
	if err := input.Validate(); err != nil {
		return fmt.Errorf("validate corpus runner v2 input: %w", err)
	}
	body, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return fmt.Errorf("encode corpus runner v2 input: %w", err)
	}
	return writeProbeJSONAtomic(path, append(body, '\n'))
}

func (input ProbeInputV2) Validate() error {
	if input.SchemaVersion != ProbeInputSchemaV2 {
		return validationError(CodeProbeInvalidInput, "schemaVersion", ProbeInputSchemaV2, input.SchemaVersion, nil)
	}
	if err := validateProbeLabel(input.RunID, "runId"); err != nil {
		return err
	}
	if err := validateBuildShape(input.Build); err != nil {
		return err
	}
	for _, item := range []struct {
		name     string
		identity ProbeFileIdentity
	}{
		{name: "dependencies.model", identity: input.Dependencies.Model},
		{name: "dependencies.projector", identity: input.Dependencies.Projector},
		{name: "dependencies.backend", identity: input.Dependencies.Backend},
		{name: "corpusInput", identity: input.CorpusInput},
	} {
		if err := validateFileIdentityShape(item.identity, item.name); err != nil {
			return err
		}
	}
	if err := validateAbsolutePathShape(input.ProbeRoot, "probeRoot"); err != nil {
		return err
	}
	return validateProbeV2Limits(input.Limits)
}

func validateProbeV2Limits(limits ProbeLimitsV2) error {
	if limits.PerCallTimeoutSeconds != ProbeV2PerCallTimeoutSeconds {
		return validationError(CodeProbeInvalidInput, "limits.perCallTimeoutSeconds", fmt.Sprint(ProbeV2PerCallTimeoutSeconds), fmt.Sprint(limits.PerCallTimeoutSeconds), nil)
	}
	if limits.MaxHeavyProcesses != ProbeV2MaxHeavyProcesses || limits.MaxCompilerTestProcesses != ProbeV2MaxCompilerTestProcesses {
		return validationError(CodeProbeInvalidInput, "limits.processes", "maxHeavyProcesses=1 and maxCompilerTestProcesses=4", fmt.Sprintf("%d/%d", limits.MaxHeavyProcesses, limits.MaxCompilerTestProcesses), nil)
	}
	if limits.MaxDiskBytes <= 0 || limits.MaxDiskBytes > ProbeV2MaxDiskBytes {
		return validationError(CodeProbeInvalidInput, "limits.maxDiskBytes", fmt.Sprintf("1..%d", ProbeV2MaxDiskBytes), fmt.Sprint(limits.MaxDiskBytes), nil)
	}
	if limits.MaxDownloadBytes != 0 || limits.MaxPaidUSD != 0 || limits.MaxCalls != ProbeV2MaxCalls || limits.MaxRetries != ProbeV2MaxRetries {
		return validationError(CodeProbeInvalidInput, "limits.effects", "zero downloads, zero paid calls, ten calls, zero retries", fmt.Sprintf("downloads=%d paid=%g calls=%d retries=%d", limits.MaxDownloadBytes, limits.MaxPaidUSD, limits.MaxCalls, limits.MaxRetries), nil)
	}
	if limits.ForbiddenPort != ProbeForbiddenPort || limits.NetworkPolicy != ProbeNetworkPolicy {
		return validationError(CodeProbeInvalidInput, "limits.network", fmt.Sprintf("forbiddenPort=%d networkPolicy=%s", ProbeForbiddenPort, ProbeNetworkPolicy), fmt.Sprintf("forbiddenPort=%d networkPolicy=%s", limits.ForbiddenPort, limits.NetworkPolicy), nil)
	}
	return nil
}

func (input CorpusInputV2) Validate(runID string, limits ProbeLimitsV2) error {
	return input.validate(runID, limits, DefaultCorpusV2Authority())
}

func (input CorpusInputV2) validate(runID string, limits ProbeLimitsV2, authority CorpusV2Authority) error {
	if input.SchemaVersion != CorpusV2SchemaVersion || input.RunID != runID {
		return validationError(CodeProbeInvalidInput, "corpusInput", "matching v2 schema and runId", "schema or runId mismatch", nil)
	}
	if err := validateFileIdentityShape(input.Build, "corpusInput.build"); err != nil {
		return err
	}
	for _, item := range []struct {
		name     string
		identity ProbeFileIdentity
	}{
		{name: "corpusInput.dependencies.model", identity: input.Dependencies.Model},
		{name: "corpusInput.dependencies.projector", identity: input.Dependencies.Projector},
		{name: "corpusInput.dependencies.backend", identity: input.Dependencies.Backend},
	} {
		if err := validateFileIdentityShape(item.identity, item.name); err != nil {
			return err
		}
	}
	if err := validateCorpusV2AuthorityFor(input.Corpus, authority); err != nil {
		return err
	}
	if err := validateCorpusV2SamplePolicyFor(input.SamplePolicy, authority); err != nil {
		return err
	}
	return validateCorpusInputV2Limits(input.Limits, limits)
}

func validateCorpusV2Authority(got CorpusAuthorityV2) error {
	return validateCorpusV2AuthorityFor(got, DefaultCorpusV2Authority())
}

func validateCorpusV2AuthorityFor(got CorpusAuthorityV2, authority CorpusV2Authority) error {
	want := CorpusAuthorityV2{Repository: authority.RepositoryRoot, Commit: authority.Commit, IndexPath: authority.IndexPath, IndexSHA256: authority.IndexSHA256, Mode: authority.Mode}
	if got != want {
		return validationError(CodeProbeCorpusAuthority, "corpusInput.corpus", fmt.Sprintf("%+v", want), fmt.Sprintf("%+v", got), nil)
	}
	return nil
}

func validateCorpusV2SamplePolicy(got CorpusSamplePolicyV2) error {
	return validateCorpusV2SamplePolicyFor(got, DefaultCorpusV2Authority())
}

func validateCorpusV2SamplePolicyFor(got CorpusSamplePolicyV2, authority CorpusV2Authority) error {
	if !equalStrings(got.Studies, authority.RequiredStudies) || got.RepresentativesPerStudy != 3 || got.Ordering != CorpusV2Ordering {
		return validationError(CodeProbeCorpusPolicy, "corpusInput.samplePolicy", "pinned studies, three representatives, and pinned ordering", "sample policy mismatch", nil)
	}
	return nil
}

func validateCorpusInputV2Limits(got CorpusInputLimitsV2, outer ProbeLimitsV2) error {
	if got.PerInvocationTimeoutSeconds != ProbeV2PerCallTimeoutSeconds || got.Retries != ProbeV2MaxRetries || got.MaxHeavyProcesses != ProbeV2MaxHeavyProcesses || got.MaxDiskBytes <= 0 || got.MaxDiskBytes > ProbeV2CorpusInputMaxDiskBytes || got.MaxDownloadBytes != 0 || got.MaxPaidUSD != 0 || got.ForbiddenPort != ProbeForbiddenPort || got.NetworkPolicy != ProbeNetworkPolicy {
		return validationError(CodeProbeInvalidInput, "corpusInput.limits", "bounded read-only corpus limits", "corpus limit mismatch", nil)
	}
	if got.PerInvocationTimeoutSeconds != outer.PerCallTimeoutSeconds || got.Retries != outer.MaxRetries || got.MaxHeavyProcesses != outer.MaxHeavyProcesses || got.MaxDownloadBytes != outer.MaxDownloadBytes || got.MaxPaidUSD != outer.MaxPaidUSD || got.ForbiddenPort != outer.ForbiddenPort || got.NetworkPolicy != outer.NetworkPolicy {
		return validationError(CodeProbeInvalidInput, "limits", "matching outer and corpus execution policies", "policy mismatch", nil)
	}
	return nil
}

func equalStrings(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func validateExistingRegularFile(path, field string) error {
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return validationError(CodeProbeInvalidIdentity, field+".path", "regular file without symlink components", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return validationError(CodeProbeInvalidIdentity, field+".path", "existing regular file", path, err)
	}
	return nil
}

func strictJSONError(field string, err error) error {
	var typed *ValidationError
	if errors.As(err, &typed) {
		return err
	}
	if strings.Contains(err.Error(), "json: unknown field") {
		return validationError(CodeUnknownField, "$", "known "+field+" fields", "unknown field", err)
	}
	return validationError(CodeProbeInvalidJSON, "$", "one strict "+field+" JSON value", "invalid", err)
}

type admittedProbeInputV2 struct {
	input        ProbeInputV2
	reportPath   string
	build        RecordedIdentity
	dependencies ProbeReportDependencies
	manifest     CorpusV2Manifest
}

func (runner RunnerV2) Preflight(ctx context.Context, inputPath, reportPath string) (ProbeReportV2, error) {
	input, err := ReadProbeInputV2(inputPath)
	if err != nil {
		return ProbeReportV2{}, err
	}
	return runner.PreflightInput(ctx, input, inputPath, reportPath)
}

func (runner RunnerV2) PreflightInput(ctx context.Context, input ProbeInputV2, inputPath, reportPath string) (ProbeReportV2, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	prepared, err := runner.admit(ctx, input, inputPath, reportPath)
	if err != nil {
		return ProbeReportV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProbeReportV2{}, probeContextError(err)
	}
	listener, port, err := reserveProbePort(input.Limits.ForbiddenPort)
	if err != nil {
		return ProbeReportV2{}, fmt.Errorf("reserve corpus runner v2 port: %w", err)
	}
	if err := listener.Close(); err != nil {
		return ProbeReportV2{}, wrapProbeCleanupError("close corpus runner v2 preflight port", err)
	}
	report := newProbeReportV2(prepared, port)
	if err := report.validate(runner.corpusAuthority()); err != nil {
		return ProbeReportV2{}, fmt.Errorf("validate corpus runner v2 report: %w", err)
	}
	if err := writeProbeReportV2Atomic(prepared.reportPath, report, runner.corpusAuthority()); err != nil {
		return ProbeReportV2{}, fmt.Errorf("persist corpus runner v2 report: %w", err)
	}
	return report, nil
}

func (runner RunnerV2) admit(ctx context.Context, input ProbeInputV2, inputPath, reportPath string) (admittedProbeInputV2, error) {
	if err := input.Validate(); err != nil {
		return admittedProbeInputV2{}, err
	}
	inputPath, err := normalizeProbePath(inputPath, "input path")
	if err != nil {
		return admittedProbeInputV2{}, err
	}
	if err := validateExistingRegularFile(inputPath, "input"); err != nil {
		return admittedProbeInputV2{}, err
	}
	reportPath, err = normalizeProbePath(reportPath, "report path")
	if err != nil {
		return admittedProbeInputV2{}, validationError(CodeProbeInvalidReport, "reportPath", "absolute clean report path", reportPath, err)
	}
	if err := validateProbeRoot(input.ProbeRoot); err != nil {
		return admittedProbeInputV2{}, err
	}
	if err := validateFreshReportDestination(reportPath, input.ProbeRoot); err != nil {
		return admittedProbeInputV2{}, err
	}
	if err := validateProbeV2Paths(input, inputPath, reportPath); err != nil {
		return admittedProbeInputV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return admittedProbeInputV2{}, probeContextError(err)
	}
	return runner.admitIdentities(ctx, input, reportPath)
}

func (runner RunnerV2) admitIdentities(ctx context.Context, input ProbeInputV2, reportPath string) (admittedProbeInputV2, error) {
	authority := runner.corpusAuthority()
	build, err := admitBuildIdentity(input.Build)
	if err != nil {
		return admittedProbeInputV2{}, err
	}
	dependencies, err := admitProbeV2Dependencies(input.Dependencies)
	if err != nil {
		return admittedProbeInputV2{}, err
	}
	if _, err := admitFileIdentity(input.CorpusInput, "corpusInput"); err != nil {
		return admittedProbeInputV2{}, err
	}
	corpusInput, err := readCorpusInputV2(input.CorpusInput.Path)
	if err != nil {
		return admittedProbeInputV2{}, err
	}
	if err := corpusInput.validate(input.RunID, input.Limits, authority); err != nil {
		return admittedProbeInputV2{}, err
	}
	if err := validateMirroredBuildAndDependencies(input, corpusInput, build, dependencies); err != nil {
		return admittedProbeInputV2{}, err
	}
	manifest, err := runner.readCorpus(ctx)
	if err != nil {
		return admittedProbeInputV2{}, err
	}
	if err := ValidateCorpusV2Manifest(manifest, authority); err != nil {
		return admittedProbeInputV2{}, err
	}
	return admittedProbeInputV2{input: input, reportPath: reportPath, build: build, dependencies: dependencies, manifest: manifest}, nil
}

func (runner RunnerV2) readCorpus(ctx context.Context) (CorpusV2Manifest, error) {
	reader := runner.corpusReader
	if reader == nil {
		reader = ReadCorpusV2Manifest
	}
	return reader(ctx, runner.corpusAuthority())
}

func (runner RunnerV2) corpusAuthority() CorpusV2Authority {
	if runner.authority.RepositoryRoot == "" {
		return DefaultCorpusV2Authority()
	}
	return runner.authority
}

func admitProbeV2Dependencies(input ProbeDependencies) (ProbeReportDependencies, error) {
	identities := []struct {
		name     string
		declared ProbeFileIdentity
	}{
		{name: "model", declared: input.Model},
		{name: "projector", declared: input.Projector},
		{name: "backend", declared: input.Backend},
	}
	observed := make([]RecordedIdentity, len(identities))
	for index, identity := range identities {
		record, err := admitFileIdentity(identity.declared, "dependencies."+identity.name)
		if err != nil {
			return ProbeReportDependencies{}, err
		}
		observed[index] = record
	}
	return ProbeReportDependencies{Model: observed[0], Projector: observed[1], Backend: observed[2]}, nil
}

func readCorpusInputV2(path string) (CorpusInputV2, error) {
	data, err := readProbeJSON(path, probeInputMaxBytes)
	if err != nil {
		return CorpusInputV2{}, fmt.Errorf("read corpus input v2: %w", err)
	}
	var input CorpusInputV2
	if err := decodeStrictJSON(data, &input); err != nil {
		return CorpusInputV2{}, strictJSONError("corpus input", err)
	}
	return input, nil
}

func validateMirroredBuildAndDependencies(outer ProbeInputV2, inner CorpusInputV2, build RecordedIdentity, dependencies ProbeReportDependencies) error {
	if !mirrorBuildIdentity(outer.Build, inner.Build, build) {
		return validationError(CodeProbeIdentityMismatch, "build", "matching outer, corpus input, and observed build identities", "identity mismatch", nil)
	}
	for _, item := range []struct {
		name     string
		outer    ProbeFileIdentity
		inner    ProbeFileIdentity
		observed RecordedIdentity
	}{
		{name: "model", outer: outer.Dependencies.Model, inner: inner.Dependencies.Model, observed: dependencies.Model},
		{name: "projector", outer: outer.Dependencies.Projector, inner: inner.Dependencies.Projector, observed: dependencies.Projector},
		{name: "backend", outer: outer.Dependencies.Backend, inner: inner.Dependencies.Backend, observed: dependencies.Backend},
	} {
		if !mirrorFileIdentity(item.outer, item.inner, item.observed) {
			return validationError(CodeProbeIdentityMismatch, "dependencies."+item.name, "matching outer, corpus input, and observed file identities", "identity mismatch", nil)
		}
	}
	return nil
}

func mirrorBuildIdentity(outer ProbeBuildIdentity, inner ProbeFileIdentity, observed RecordedIdentity) bool {
	return sameProbePath(outer.Path, inner.Path) && outer.Identity == inner.Identity && outer.SHA256 == inner.SHA256 && inner.Identity == observed.Identity && inner.Bytes == observed.Bytes && inner.SHA256 == observed.SHA256 && pathIdentity(inner.Path) == observed.PathIdentity
}

func mirrorFileIdentity(outer, inner ProbeFileIdentity, observed RecordedIdentity) bool {
	return sameProbePath(outer.Path, inner.Path) && outer.Identity == inner.Identity && outer.Bytes == inner.Bytes && outer.SHA256 == inner.SHA256 && inner.Identity == observed.Identity && inner.Bytes == observed.Bytes && inner.SHA256 == observed.SHA256 && pathIdentity(inner.Path) == observed.PathIdentity
}

func validateProbeV2Paths(input ProbeInputV2, inputPath, reportPath string) error {
	paths := []struct{ name, path string }{
		{name: "input", path: inputPath},
		{name: "report", path: reportPath},
		{name: "build", path: input.Build.Path},
		{name: "dependencies.model", path: input.Dependencies.Model.Path},
		{name: "dependencies.projector", path: input.Dependencies.Projector.Path},
		{name: "dependencies.backend", path: input.Dependencies.Backend.Path},
		{name: "corpusInput", path: input.CorpusInput.Path},
	}
	seen := make(map[string]string, len(paths))
	for _, item := range paths {
		key := probePathKey(item.path)
		if previous, exists := seen[key]; exists {
			return validationError(CodeProbeInvalidIdentity, item.name+".path", "unique declared paths", previous, nil)
		}
		seen[key] = item.name
		if pathWithin(input.ProbeRoot, item.path) || pathWithin(item.path, input.ProbeRoot) {
			return validationError(CodeProbeInvalidRoot, "probeRoot", "isolated from input, report, and declared assets", item.name, nil)
		}
	}
	return nil
}

func probePathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func sameProbePath(first, second string) bool {
	return probePathKey(first) == probePathKey(second)
}

func validateFreshReportDestination(path, probeRoot string) error {
	if err := validateReportDestination(path, probeRoot); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return validationError(CodeProbeInvalidReport, "reportPath", "fresh absent report destination", "already exists", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return validationError(CodeProbeInvalidReport, "reportPath", "inspectable absent report destination", "unavailable", err)
	}
	return nil
}

func PreflightV2(ctx context.Context, inputPath, reportPath string, executor Executor) (ProbeReportV2, error) {
	return NewRunnerV2(executor).Preflight(ctx, inputPath, reportPath)
}

func WriteProbeInputV2(path string, input ProbeInputV2) error {
	return WriteProbeInputV2Atomic(path, input)
}
