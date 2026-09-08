package platform_conformance

import "strings"

// NewReadinessReport turns a successful pre-launch admission into redacted
// readiness evidence. It deliberately records no customer output and makes no
// claim that a model, backend, or platform operation succeeded.
func (admission Admission) NewReadinessReport(ledger LedgerIdentity) Report {
	commands := make([]CommandEvidence, 0, len(admission.Spec.Commands))
	for _, command := range admission.Spec.Commands {
		commands = append(commands, CommandEvidence{
			Name: command.Name, PathIdentity: PathIdentity(command.Path),
			Args:            append([]string(nil), command.Args...),
			EnvironmentKeys: environmentKeys(command.Environment),
			StdoutSHA256:    SHA256Hex(nil), StderrSHA256: SHA256Hex(nil), Redacted: true,
		})
	}
	return Report{
		Schema: ReportSchemaV1, RunID: admission.Spec.RunID, Status: StatusPass,
		Claim: ClaimRunnerReadiness, Target: admission.Spec.Target,
		Identities: ReportIdentities{
			CLI: ReportCLIIdentity{
				PathIdentity: admission.CLIPathIdentity, SHA256: admission.Spec.CLI.SHA256,
				SizeBytes: admission.Spec.CLI.SizeBytes, Version: admission.Spec.CLI.Version,
				Commit: admission.Spec.CLI.Commit,
			},
			Backend: ReportBackendIdentity{
				ID: admission.Spec.Backend.ID, SHA256: admission.Spec.Backend.SHA256,
				SizeBytes: admission.Spec.Backend.SizeBytes, SourceRevision: admission.Spec.Backend.SourceRevision,
			},
			Model: ReportModelIdentity{
				ID: admission.Spec.Model.ID, SHA256: admission.Spec.Model.SHA256,
				SizeBytes: admission.Spec.Model.SizeBytes, Revision: admission.Spec.Model.Revision,
			},
			Fixture: ReportFixtureIdentity{
				ID: admission.Spec.Fixture.ID, SHA256: admission.Spec.Fixture.SHA256,
				SizeBytes: admission.Spec.Fixture.SizeBytes, SemanticAssertion: admission.Spec.Fixture.SemanticAssertion,
			},
		},
		Policy: ReportPolicy{
			RootIdentities: append([]string(nil), admission.RootIdentities...),
			Port:           admission.Spec.Port, TimeoutMillis: admission.Spec.TimeoutMillis,
			NetworkPolicy: admission.Spec.NetworkPolicy, Limits: admission.Spec.Limits,
		},
		Ledger: ledger, Commands: commands,
		SemanticObservations: []SemanticObservation{{
			Assertion: "bounded immutable pre-launch admission",
			Expected:  "declared host, artifacts, roots, policy, and ledger are valid",
			Observed:  "admission completed without an executor effect",
			Passed:    true,
		}},
		Release: ReleaseEvidence{ProcessTreeClosed: true},
	}
}

func environmentKeys(environment []string) []string {
	keys := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			keys = append(keys, key)
		}
	}
	return keys
}
