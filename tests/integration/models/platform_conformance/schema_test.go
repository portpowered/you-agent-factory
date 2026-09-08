package platform_conformance

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	RunSchemaV1    = "localai.platform-conformance.run.v1"
	ReportSchemaV1 = "localai.platform-conformance.report.v1"
	BudgetSchemaV1 = "localai.platform-conformance.budget.v1"

	ClaimRunnerReadiness = "runner-readiness"

	StatusPass         = "PASS"
	StatusFail         = "FAIL"
	StatusInconclusive = "INCONCLUSIVE"

	NetworkPolicyDeny = "deny"

	TargetLinux  = "linux"
	TargetDarwin = "darwin"
	ArchAMD64    = "amd64"
	ArchARM64    = "arm64"

	ReservationStateReserved  = "RESERVED"
	ReservationStateCommitted = "COMMITTED"
	ReservationStateReleased  = "RELEASED"

	BudgetKindDownloadBytes   = "downloadBytes"
	BudgetKindModelCalls      = "modelCalls"
	BudgetKindNetworkRequests = "networkRequests"
	BudgetKindChildProcesses  = "childProcesses"
	BudgetKindTemporaryBytes  = "temporaryBytes"

	MaxChildProcesses     int64 = 4
	MaxTemporaryBytes     int64 = 1 << 30
	MaxTimeoutMillis      int64 = 60 * 60 * 1000
	MaxJSONBytes                = 1 << 20
	MaxCommands                 = 16
	MaxCommandArguments         = 64
	MaxEnvironmentEntries       = 64
)

// RunSpec is the private, immutable input contract for a conformance run.
// Every path is retained in the input because it is needed to inspect the
// declared artifact, but reports use path identities instead of raw paths.
type RunSpec struct {
	Schema        string          `json:"schema"`
	RunID         string          `json:"runId"`
	Target        Target          `json:"target"`
	CLI           CLIIdentity     `json:"cli"`
	Backend       BackendIdentity `json:"backend"`
	Model         ModelIdentity   `json:"model"`
	Fixture       FixtureIdentity `json:"fixture"`
	Roots         Roots           `json:"roots"`
	Port          int             `json:"port"`
	TimeoutMillis int64           `json:"timeoutMillis"`
	NetworkPolicy string          `json:"networkPolicy"`
	Limits        BudgetLimits    `json:"limits"`
	Commands      []CommandSpec   `json:"commands"`
	ReportPath    string          `json:"reportPath"`
	LedgerPath    string          `json:"ledgerPath"`
}

type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type CLIIdentity struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
}

type BackendIdentity struct {
	ID             string `json:"id"`
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	SizeBytes      int64  `json:"sizeBytes"`
	SourceRevision string `json:"sourceRevision"`
}

type ModelIdentity struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
	Revision  string `json:"revision"`
}

type FixtureIdentity struct {
	ID                string `json:"id"`
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	SizeBytes         int64  `json:"sizeBytes"`
	SemanticAssertion string `json:"semanticAssertion"`
}

type Roots struct {
	Work   string `json:"work"`
	State  string `json:"state"`
	Cache  string `json:"cache"`
	Temp   string `json:"temp"`
	Output string `json:"output"`
}

type CommandSpec struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Args        []string `json:"args"`
	Environment []string `json:"environment"`
}

type BudgetLimits struct {
	DownloadBytes     int64 `json:"downloadBytes"`
	ModelCalls        int64 `json:"modelCalls"`
	NetworkRequests   int64 `json:"networkRequests"`
	MaxChildProcesses int64 `json:"maxChildProcesses"`
	TemporaryBytes    int64 `json:"temporaryBytes"`
}

type BudgetConsumed struct {
	DownloadBytes   int64 `json:"downloadBytes"`
	ModelCalls      int64 `json:"modelCalls"`
	NetworkRequests int64 `json:"networkRequests"`
	ChildProcesses  int64 `json:"childProcesses"`
	TemporaryBytes  int64 `json:"temporaryBytes"`
}

type BudgetReservation struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Amount  int64  `json:"amount"`
	State   string `json:"state"`
	Command string `json:"command"`
}

type BudgetLedger struct {
	Schema         string              `json:"schema"`
	LedgerID       string              `json:"ledgerId"`
	RunID          string              `json:"runId"`
	Generation     int64               `json:"generation"`
	PreviousSHA256 string              `json:"previousSha256"`
	Limits         BudgetLimits        `json:"limits"`
	Consumed       BudgetConsumed      `json:"consumed"`
	Reservations   []BudgetReservation `json:"reservations"`
	Finalized      bool                `json:"finalized"`
}

// Report is the durable, redacted readiness evidence contract. It records a
// runner claim only; it is intentionally not a customer or model-semantic
// acceptance result.
type Report struct {
	Schema               string                `json:"schema"`
	RunID                string                `json:"runId"`
	Status               string                `json:"status"`
	Claim                string                `json:"claim"`
	Target               Target                `json:"target"`
	Identities           ReportIdentities      `json:"identities"`
	Policy               ReportPolicy          `json:"policy"`
	Ledger               LedgerIdentity        `json:"ledger"`
	Commands             []CommandEvidence     `json:"commands"`
	SemanticObservations []SemanticObservation `json:"semanticObservations"`
	Release              ReleaseEvidence       `json:"release"`
	Failure              *ReportFailure        `json:"failure"`
}

type ReportIdentities struct {
	CLI     ReportCLIIdentity     `json:"cli"`
	Backend ReportBackendIdentity `json:"backend"`
	Model   ReportModelIdentity   `json:"model"`
	Fixture ReportFixtureIdentity `json:"fixture"`
}

type ReportCLIIdentity struct {
	PathIdentity string `json:"pathIdentity"`
	SHA256       string `json:"sha256"`
	SizeBytes    int64  `json:"sizeBytes"`
	Version      string `json:"version"`
	Commit       string `json:"commit"`
}

type ReportBackendIdentity struct {
	ID             string `json:"id"`
	SHA256         string `json:"sha256"`
	SizeBytes      int64  `json:"sizeBytes"`
	SourceRevision string `json:"sourceRevision"`
}

type ReportModelIdentity struct {
	ID        string `json:"id"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
	Revision  string `json:"revision"`
}

type ReportFixtureIdentity struct {
	ID                string `json:"id"`
	SHA256            string `json:"sha256"`
	SizeBytes         int64  `json:"sizeBytes"`
	SemanticAssertion string `json:"semanticAssertion"`
}

type ReportPolicy struct {
	RootIdentities []string     `json:"rootIdentities"`
	Port           int          `json:"port"`
	TimeoutMillis  int64        `json:"timeoutMillis"`
	NetworkPolicy  string       `json:"networkPolicy"`
	Limits         BudgetLimits `json:"limits"`
}

type LedgerIdentity struct {
	PathIdentity string `json:"pathIdentity"`
	SHA256       string `json:"sha256"`
	Generation   int64  `json:"generation"`
}

type CommandEvidence struct {
	Name            string   `json:"name"`
	PathIdentity    string   `json:"pathIdentity"`
	Args            []string `json:"args"`
	EnvironmentKeys []string `json:"environmentKeys"`
	Started         bool     `json:"started"`
	ExitCode        int      `json:"exitCode"`
	TimedOut        bool     `json:"timedOut"`
	Cancelled       bool     `json:"cancelled"`
	StdoutBytes     int64    `json:"stdoutBytes"`
	StdoutSHA256    string   `json:"stdoutSha256"`
	StderrBytes     int64    `json:"stderrBytes"`
	StderrSHA256    string   `json:"stderrSha256"`
	Redacted        bool     `json:"redacted"`
}

type SemanticObservation struct {
	Assertion string `json:"assertion"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
	Passed    bool   `json:"passed"`
}

type ReleaseEvidence struct {
	ProcessTreeClosed bool  `json:"processTreeClosed"`
	OwnedProcesses    int   `json:"ownedProcesses"`
	OwnedListeners    int   `json:"ownedListeners"`
	PartialArtifacts  int   `json:"partialArtifacts"`
	TemporaryBytes    int64 `json:"temporaryBytes"`
}

type ReportFailure struct {
	Owner     string `json:"owner"`
	Assertion string `json:"assertion"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
}

// Validate checks only the declarative shape. Admit adds host, filesystem,
// identity, isolation, and existing-ledger checks.
func (spec RunSpec) Validate() error { return spec.validateShape() }

func (ledger BudgetLedger) Validate() error { return ledger.validate() }

func (report Report) Validate() error { return report.validate() }

func (spec RunSpec) validateShape() error {
	if spec.Schema != RunSchemaV1 {
		return schemaFailure("invalid_schema", "schema", RunSchemaV1, spec.Schema)
	}
	if !validIdentifier(spec.RunID) {
		return schemaFailure("invalid_identifier", "runId", "safe non-empty identifier", spec.RunID)
	}
	if err := validateTarget(spec.Target); err != nil {
		return err
	}
	if err := validateCLIIdentity(spec.CLI); err != nil {
		return err
	}
	if err := validateBackendIdentity(spec.Backend); err != nil {
		return err
	}
	if err := validateModelIdentity(spec.Model); err != nil {
		return err
	}
	if err := validateFixtureIdentity(spec.Fixture); err != nil {
		return err
	}
	if err := validateRoots(spec.Roots); err != nil {
		return err
	}
	if spec.Port <= 0 || spec.Port > 65535 || spec.Port == 7437 {
		return schemaFailure("invalid_port", "port", "a non-7437 TCP port", fmt.Sprint(spec.Port))
	}
	if spec.TimeoutMillis <= 0 || spec.TimeoutMillis > MaxTimeoutMillis {
		return schemaFailure("invalid_timeout", "timeoutMillis", fmt.Sprintf("1..%d", MaxTimeoutMillis), fmt.Sprint(spec.TimeoutMillis))
	}
	if spec.NetworkPolicy != NetworkPolicyDeny {
		return schemaFailure("invalid_network_policy", "networkPolicy", NetworkPolicyDeny, spec.NetworkPolicy)
	}
	if err := validateRunLimits(spec.Limits); err != nil {
		return err
	}
	if len(spec.Commands) == 0 || len(spec.Commands) > MaxCommands {
		return schemaFailure("invalid_commands", "commands", fmt.Sprintf("1..%d commands", MaxCommands), fmt.Sprint(len(spec.Commands)))
	}
	seen := make(map[string]struct{}, len(spec.Commands))
	for index, command := range spec.Commands {
		if err := validateCommand(command, index); err != nil {
			return err
		}
		if _, exists := seen[command.Name]; exists {
			return schemaFailure("duplicate_command", fmt.Sprintf("commands[%d].name", index), "unique command name", command.Name)
		}
		seen[command.Name] = struct{}{}
		if command.Path != spec.CLI.Path {
			return schemaFailure("command_path_drift", fmt.Sprintf("commands[%d].path", index), "the declared CLI path", "different path")
		}
	}
	if !validAbsolutePath(spec.ReportPath) {
		return schemaFailure("unsafe_path", "reportPath", "absolute clean path", "invalid path")
	}
	if !validAbsolutePath(spec.LedgerPath) {
		return schemaFailure("unsafe_path", "ledgerPath", "absolute clean path", "invalid path")
	}
	if samePath(spec.ReportPath, spec.LedgerPath) {
		return schemaFailure("path_collision", "reportPath/ledgerPath", "distinct paths", "same path")
	}
	return nil
}

func validateTarget(target Target) error {
	if target.OS != TargetLinux && target.OS != TargetDarwin {
		return schemaFailure("unsupported_target", "target.os", "linux or darwin", target.OS)
	}
	if (target.OS == TargetLinux && target.Arch != ArchAMD64) ||
		(target.OS == TargetDarwin && target.Arch != ArchARM64) {
		return schemaFailure("unsupported_target", "target.arch", "linux/amd64 or darwin/arm64", target.Arch)
	}
	return nil
}

func validateCLIIdentity(identity CLIIdentity) error {
	if err := validateArtifactIdentity("cli", identity.Path, identity.SHA256, identity.SizeBytes); err != nil {
		return err
	}
	if identity.Version == "" {
		return schemaFailure("missing_identity", "cli.version", "non-empty version", "empty")
	}
	if !validRevision(identity.Commit) {
		return schemaFailure("invalid_identity", "cli.commit", "non-empty revision", identity.Commit)
	}
	return nil
}

func validateBackendIdentity(identity BackendIdentity) error {
	if !validIdentifier(identity.ID) {
		return schemaFailure("invalid_identity", "backend.id", "safe non-empty identifier", identity.ID)
	}
	if err := validateArtifactIdentity("backend", identity.Path, identity.SHA256, identity.SizeBytes); err != nil {
		return err
	}
	if !validRevision(identity.SourceRevision) {
		return schemaFailure("invalid_identity", "backend.sourceRevision", "non-empty revision", identity.SourceRevision)
	}
	return nil
}

func validateModelIdentity(identity ModelIdentity) error {
	if !validIdentifier(identity.ID) {
		return schemaFailure("invalid_identity", "model.id", "safe non-empty identifier", identity.ID)
	}
	if err := validateArtifactIdentity("model", identity.Path, identity.SHA256, identity.SizeBytes); err != nil {
		return err
	}
	if !validRevision(identity.Revision) {
		return schemaFailure("invalid_identity", "model.revision", "non-empty revision", identity.Revision)
	}
	return nil
}

func validateFixtureIdentity(identity FixtureIdentity) error {
	if !validIdentifier(identity.ID) {
		return schemaFailure("invalid_identity", "fixture.id", "safe non-empty identifier", identity.ID)
	}
	if err := validateArtifactIdentity("fixture", identity.Path, identity.SHA256, identity.SizeBytes); err != nil {
		return err
	}
	if strings.TrimSpace(identity.SemanticAssertion) == "" {
		return schemaFailure("missing_identity", "fixture.semanticAssertion", "non-empty semantic assertion", "empty")
	}
	return nil
}

func validateArtifactIdentity(label, path, digest string, size int64) error {
	if !validAbsolutePath(path) {
		return schemaFailure("unsafe_path", label+".path", "absolute clean path", "invalid path")
	}
	if !validDigest(digest) {
		return schemaFailure("invalid_digest", label+".sha256", "lowercase SHA-256", digest)
	}
	if size <= 0 {
		return schemaFailure("invalid_size", label+".sizeBytes", "positive byte size", fmt.Sprint(size))
	}
	return nil
}

func validateRoots(roots Roots) error {
	paths := map[string]string{
		"work": roots.Work, "state": roots.State, "cache": roots.Cache,
		"temp": roots.Temp, "output": roots.Output,
	}
	for name, path := range paths {
		if !validAbsolutePath(path) {
			return schemaFailure("unsafe_path", "roots."+name, "absolute clean non-root path", "invalid path")
		}
		if isFilesystemRoot(path) {
			return schemaFailure("unsafe_path", "roots."+name, "isolated non-root path", "filesystem root")
		}
	}
	return nil
}

func validateCommand(command CommandSpec, index int) error {
	prefix := fmt.Sprintf("commands[%d]", index)
	if !validIdentifier(command.Name) {
		return schemaFailure("invalid_command", prefix+".name", "safe non-empty identifier", command.Name)
	}
	if !validAbsolutePath(command.Path) {
		return schemaFailure("unsafe_path", prefix+".path", "absolute clean path", "invalid path")
	}
	if len(command.Args) == 0 || len(command.Args) > MaxCommandArguments {
		return schemaFailure("invalid_command", prefix+".args", fmt.Sprintf("1..%d arguments", MaxCommandArguments), fmt.Sprint(len(command.Args)))
	}
	for argIndex, arg := range command.Args {
		if strings.ContainsRune(arg, 0) {
			return schemaFailure("invalid_command", fmt.Sprintf("%s.args[%d]", prefix, argIndex), "argument without NUL", "NUL")
		}
	}
	if len(command.Environment) > MaxEnvironmentEntries {
		return schemaFailure("invalid_command", prefix+".environment", fmt.Sprintf("at most %d entries", MaxEnvironmentEntries), fmt.Sprint(len(command.Environment)))
	}
	seen := make(map[string]struct{}, len(command.Environment))
	for envIndex, entry := range command.Environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !validEnvironmentKey(key) || strings.ContainsRune(value, 0) {
			return schemaFailure("invalid_environment", fmt.Sprintf("%s.environment[%d]", prefix, envIndex), "KEY=value with a valid key", "invalid entry")
		}
		if _, exists := seen[key]; exists {
			return schemaFailure("duplicate_environment", fmt.Sprintf("%s.environment[%d]", prefix, envIndex), "unique environment key", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateRunLimits(limits BudgetLimits) error {
	if limits.DownloadBytes != 0 || limits.ModelCalls != 0 || limits.NetworkRequests != 0 {
		return schemaFailure("nonzero_external_budget", "limits", "download/model/network limits equal zero", "nonzero external budget")
	}
	if limits.MaxChildProcesses <= 0 || limits.MaxChildProcesses > MaxChildProcesses {
		return schemaFailure("invalid_limit", "limits.maxChildProcesses", fmt.Sprintf("1..%d", MaxChildProcesses), fmt.Sprint(limits.MaxChildProcesses))
	}
	if limits.TemporaryBytes < 0 || limits.TemporaryBytes > MaxTemporaryBytes {
		return schemaFailure("invalid_limit", "limits.temporaryBytes", fmt.Sprintf("0..%d", MaxTemporaryBytes), fmt.Sprint(limits.TemporaryBytes))
	}
	return nil
}

func (ledger BudgetLedger) validate() error {
	if ledger.Schema != BudgetSchemaV1 {
		return schemaFailure("invalid_schema", "schema", BudgetSchemaV1, ledger.Schema)
	}
	if !validIdentifier(ledger.LedgerID) {
		return schemaFailure("invalid_identifier", "ledgerId", "safe non-empty identifier", ledger.LedgerID)
	}
	if !validIdentifier(ledger.RunID) {
		return schemaFailure("invalid_identifier", "runId", "safe non-empty identifier", ledger.RunID)
	}
	if ledger.Generation <= 0 {
		return schemaFailure("invalid_generation", "generation", "positive generation", fmt.Sprint(ledger.Generation))
	}
	if ledger.Generation == 1 && ledger.PreviousSHA256 != "" {
		return schemaFailure("invalid_chain", "previousSha256", "empty hash for generation one", ledger.PreviousSHA256)
	}
	if ledger.Generation > 1 && !validDigest(ledger.PreviousSHA256) {
		return schemaFailure("invalid_chain", "previousSha256", "SHA-256 for later generation", ledger.PreviousSHA256)
	}
	if err := validateLedgerLimits(ledger.Limits); err != nil {
		return err
	}
	if err := validateConsumed(ledger.Consumed, ledger.Limits); err != nil {
		return err
	}
	if ledger.Reservations == nil {
		return schemaFailure("missing_field", "reservations", "an array, including an empty array", "null")
	}
	seen := make(map[string]struct{}, len(ledger.Reservations))
	reserved := BudgetConsumed{}
	for index, reservation := range ledger.Reservations {
		if !validIdentifier(reservation.ID) {
			return schemaFailure("invalid_reservation", fmt.Sprintf("reservations[%d].id", index), "safe non-empty identifier", reservation.ID)
		}
		if _, exists := seen[reservation.ID]; exists {
			return schemaFailure("duplicate_reservation", fmt.Sprintf("reservations[%d].id", index), "unique reservation id", reservation.ID)
		}
		seen[reservation.ID] = struct{}{}
		if !validBudgetKind(reservation.Kind) {
			return schemaFailure("invalid_reservation", fmt.Sprintf("reservations[%d].kind", index), "known budget kind", reservation.Kind)
		}
		if reservation.Amount <= 0 {
			return schemaFailure("invalid_reservation", fmt.Sprintf("reservations[%d].amount", index), "positive amount", fmt.Sprint(reservation.Amount))
		}
		if reservation.State != ReservationStateReserved && reservation.State != ReservationStateCommitted && reservation.State != ReservationStateReleased {
			return schemaFailure("invalid_reservation", fmt.Sprintf("reservations[%d].state", index), "RESERVED, COMMITTED, or RELEASED", reservation.State)
		}
		if strings.TrimSpace(reservation.Command) == "" {
			return schemaFailure("invalid_reservation", fmt.Sprintf("reservations[%d].command", index), "non-empty command", "empty")
		}
		if reservation.State == ReservationStateReserved {
			if !addConsumedWithinLimit(&reserved, reservation.Kind, reservation.Amount, ledger.Limits) {
				return schemaFailure("budget_exhausted", fmt.Sprintf("reservations[%d]", index), "active reservations within limits", "over limit")
			}
		}
	}
	combined, ok := combineConsumedWithinLimit(ledger.Consumed, reserved, ledger.Limits)
	if !ok {
		return schemaFailure("budget_exhausted", "reservations", "consumed plus active reservations within limits", "over limit")
	}
	if err := validateConsumed(combined, ledger.Limits); err != nil {
		return schemaFailure("budget_exhausted", "reservations", "consumed plus active reservations within limits", "over limit")
	}
	return nil
}

func validateLedgerLimits(limits BudgetLimits) error {
	if limits.DownloadBytes < 0 || limits.ModelCalls < 0 || limits.NetworkRequests < 0 ||
		limits.MaxChildProcesses <= 0 || limits.MaxChildProcesses > MaxChildProcesses ||
		limits.TemporaryBytes < 0 || limits.TemporaryBytes > MaxTemporaryBytes {
		return schemaFailure("invalid_limit", "limits", "non-negative bounded limits", "invalid limit")
	}
	return nil
}

func validateConsumed(consumed BudgetConsumed, limits BudgetLimits) error {
	if consumed.DownloadBytes < 0 || consumed.ModelCalls < 0 || consumed.NetworkRequests < 0 ||
		consumed.ChildProcesses < 0 || consumed.TemporaryBytes < 0 {
		return schemaFailure("invalid_consumed", "consumed", "non-negative counters", "negative counter")
	}
	if consumed.DownloadBytes > limits.DownloadBytes || consumed.ModelCalls > limits.ModelCalls ||
		consumed.NetworkRequests > limits.NetworkRequests || consumed.ChildProcesses > limits.MaxChildProcesses ||
		consumed.TemporaryBytes > limits.TemporaryBytes {
		return schemaFailure("budget_exhausted", "consumed", "counters within declared limits", "over limit")
	}
	return nil
}

func validBudgetKind(kind string) bool {
	switch kind {
	case BudgetKindDownloadBytes, BudgetKindModelCalls, BudgetKindNetworkRequests,
		BudgetKindChildProcesses, BudgetKindTemporaryBytes:
		return true
	default:
		return false
	}
}

func validRevision(value string) bool {
	return strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\x00\r\n")
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			if index == 0 && char == '.' {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validAbsolutePath(path string) bool {
	return path != "" && strings.TrimSpace(path) == path &&
		!strings.ContainsRune(path, 0) && filepath.IsAbs(path) && filepath.Clean(path) == path
}

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	root := volume + string(filepath.Separator)
	return clean == root || clean == volume+string(filepath.Separator)+string(filepath.Separator)
}

func samePath(first, second string) bool {
	return filepath.Clean(first) == filepath.Clean(second)
}

func validEnvironmentKey(key string) bool {
	if key == "" {
		return false
	}
	for index, char := range key {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_' ||
			(index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func validPathIdentity(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validDigest(strings.TrimPrefix(value, "sha256:"))
}

func (report Report) validate() error {
	if report.Schema != ReportSchemaV1 {
		return schemaFailure("invalid_schema", "schema", ReportSchemaV1, report.Schema)
	}
	if !validIdentifier(report.RunID) {
		return schemaFailure("invalid_identifier", "runId", "safe non-empty identifier", report.RunID)
	}
	if report.Status != StatusPass && report.Status != StatusFail && report.Status != StatusInconclusive {
		return schemaFailure("invalid_status", "status", "PASS, FAIL, or INCONCLUSIVE", report.Status)
	}
	if report.Claim != ClaimRunnerReadiness {
		return schemaFailure("invalid_claim", "claim", ClaimRunnerReadiness, report.Claim)
	}
	if err := validateTarget(report.Target); err != nil {
		return err
	}
	if err := validateReportIdentities(report.Identities); err != nil {
		return err
	}
	if len(report.Policy.RootIdentities) == 0 {
		return schemaFailure("missing_field", "policy.rootIdentities", "one or more path identities", "empty")
	}
	for index, identity := range report.Policy.RootIdentities {
		if !validPathIdentity(identity) {
			return schemaFailure("invalid_identity", fmt.Sprintf("policy.rootIdentities[%d]", index), "sha256 path identity", identity)
		}
	}
	if report.Policy.Port <= 0 || report.Policy.Port > 65535 || report.Policy.Port == 7437 {
		return schemaFailure("invalid_port", "policy.port", "a non-7437 TCP port", fmt.Sprint(report.Policy.Port))
	}
	if report.Policy.TimeoutMillis <= 0 || report.Policy.TimeoutMillis > MaxTimeoutMillis {
		return schemaFailure("invalid_timeout", "policy.timeoutMillis", fmt.Sprintf("1..%d", MaxTimeoutMillis), fmt.Sprint(report.Policy.TimeoutMillis))
	}
	if report.Policy.NetworkPolicy != NetworkPolicyDeny {
		return schemaFailure("invalid_network_policy", "policy.networkPolicy", NetworkPolicyDeny, report.Policy.NetworkPolicy)
	}
	if err := validateLedgerLimits(report.Policy.Limits); err != nil {
		return err
	}
	if !validPathIdentity(report.Ledger.PathIdentity) || !validDigest(report.Ledger.SHA256) || report.Ledger.Generation <= 0 {
		return schemaFailure("invalid_identity", "ledger", "path/hash identity and positive generation", "invalid ledger identity")
	}
	if report.Commands == nil || len(report.Commands) == 0 {
		return schemaFailure("missing_field", "commands", "one or more command observations", "empty")
	}
	for index, command := range report.Commands {
		if err := validateCommandEvidence(command, index); err != nil {
			return err
		}
	}
	if report.SemanticObservations == nil {
		return schemaFailure("missing_field", "semanticObservations", "an array, including an empty array", "null")
	}
	for index, observation := range report.SemanticObservations {
		if strings.TrimSpace(observation.Assertion) == "" || strings.TrimSpace(observation.Expected) == "" || strings.TrimSpace(observation.Observed) == "" {
			return schemaFailure("invalid_observation", fmt.Sprintf("semanticObservations[%d]", index), "non-empty assertion, expected, and observed values", "empty observation")
		}
	}
	if report.Release.OwnedProcesses < 0 || report.Release.OwnedListeners < 0 || report.Release.PartialArtifacts < 0 || report.Release.TemporaryBytes < 0 {
		return schemaFailure("invalid_release", "release", "non-negative release counters", "negative counter")
	}
	if report.Status == StatusPass {
		if report.Failure != nil {
			return schemaFailure("pass_with_failure", "failure", "null for PASS", "failure present")
		}
		for index, observation := range report.SemanticObservations {
			if !observation.Passed {
				return schemaFailure("pass_with_failed_observation", fmt.Sprintf("semanticObservations[%d].passed", index), "true for PASS", "false")
			}
		}
	} else if report.Failure == nil {
		return schemaFailure("failure_missing", "failure", "failure object for non-PASS", "null")
	}
	if report.Failure != nil {
		if strings.TrimSpace(report.Failure.Owner) == "" || strings.TrimSpace(report.Failure.Assertion) == "" ||
			strings.TrimSpace(report.Failure.Expected) == "" || strings.TrimSpace(report.Failure.Observed) == "" {
			return schemaFailure("invalid_failure", "failure", "non-empty failure details", "incomplete failure")
		}
	}
	return nil
}

func validateReportIdentities(identities ReportIdentities) error {
	if !validPathIdentity(identities.CLI.PathIdentity) || !validDigest(identities.CLI.SHA256) || identities.CLI.SizeBytes <= 0 ||
		identities.CLI.Version == "" || !validRevision(identities.CLI.Commit) {
		return schemaFailure("invalid_identity", "identities.cli", "complete CLI identity", "invalid CLI identity")
	}
	if !validIdentifier(identities.Backend.ID) || !validDigest(identities.Backend.SHA256) || identities.Backend.SizeBytes <= 0 || !validRevision(identities.Backend.SourceRevision) {
		return schemaFailure("invalid_identity", "identities.backend", "complete backend identity", "invalid backend identity")
	}
	if !validIdentifier(identities.Model.ID) || !validDigest(identities.Model.SHA256) || identities.Model.SizeBytes <= 0 || !validRevision(identities.Model.Revision) {
		return schemaFailure("invalid_identity", "identities.model", "complete model identity", "invalid model identity")
	}
	if !validIdentifier(identities.Fixture.ID) || !validDigest(identities.Fixture.SHA256) || identities.Fixture.SizeBytes <= 0 || strings.TrimSpace(identities.Fixture.SemanticAssertion) == "" {
		return schemaFailure("invalid_identity", "identities.fixture", "complete fixture identity", "invalid fixture identity")
	}
	return nil
}

func validateCommandEvidence(command CommandEvidence, index int) error {
	prefix := fmt.Sprintf("commands[%d]", index)
	if !validIdentifier(command.Name) || !validPathIdentity(command.PathIdentity) {
		return schemaFailure("invalid_command", prefix, "name and path identity", "invalid command identity")
	}
	if command.Args == nil || command.EnvironmentKeys == nil {
		return schemaFailure("missing_field", prefix, "args and environmentKeys arrays", "null array")
	}
	if command.StdoutBytes < 0 || command.StderrBytes < 0 || !validDigest(command.StdoutSHA256) || !validDigest(command.StderrSHA256) {
		return schemaFailure("invalid_command", prefix, "bounded stream byte counts and SHA-256 values", "invalid stream evidence")
	}
	if !command.Redacted {
		return schemaFailure("unredacted_evidence", prefix+".redacted", "true", "false")
	}
	for envIndex, key := range command.EnvironmentKeys {
		if !validEnvironmentKey(key) {
			return schemaFailure("invalid_environment", fmt.Sprintf("%s.environmentKeys[%d]", prefix, envIndex), "environment key", key)
		}
	}
	return nil
}

func addConsumed(target *BudgetConsumed, kind string, amount int64) {
	switch kind {
	case BudgetKindDownloadBytes:
		target.DownloadBytes += amount
	case BudgetKindModelCalls:
		target.ModelCalls += amount
	case BudgetKindNetworkRequests:
		target.NetworkRequests += amount
	case BudgetKindChildProcesses:
		target.ChildProcesses += amount
	case BudgetKindTemporaryBytes:
		target.TemporaryBytes += amount
	}
}

func addConsumedCopy(first, second BudgetConsumed) BudgetConsumed {
	return BudgetConsumed{
		DownloadBytes:   first.DownloadBytes + second.DownloadBytes,
		ModelCalls:      first.ModelCalls + second.ModelCalls,
		NetworkRequests: first.NetworkRequests + second.NetworkRequests,
		ChildProcesses:  first.ChildProcesses + second.ChildProcesses,
		TemporaryBytes:  first.TemporaryBytes + second.TemporaryBytes,
	}
}

func combineConsumedWithinLimit(first, second BudgetConsumed, limits BudgetLimits) (BudgetConsumed, bool) {
	combined := first
	for _, value := range []struct {
		kind   string
		amount int64
	}{
		{BudgetKindDownloadBytes, second.DownloadBytes},
		{BudgetKindModelCalls, second.ModelCalls},
		{BudgetKindNetworkRequests, second.NetworkRequests},
		{BudgetKindChildProcesses, second.ChildProcesses},
		{BudgetKindTemporaryBytes, second.TemporaryBytes},
	} {
		if value.amount == 0 {
			continue
		}
		if !addConsumedWithinLimit(&combined, value.kind, value.amount, limits) {
			return BudgetConsumed{}, false
		}
	}
	return combined, true
}
