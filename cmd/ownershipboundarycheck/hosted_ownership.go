package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	hostedOwnershipClaimsFile = "hosted-ownership-claims.json"
	hostedOwnershipVersion    = 1

	responsibilityHostedSourcePolling        = "hosted-source-polling"
	responsibilityHostedSourceReconciliation = "hosted-source-reconciliation"
	responsibilityHostedRunnerExecution      = "hosted-runner-execution"

	ownerAutomationHostedSources = "Automation Hosted Sources"
	ownerWorkersHostedRunner     = "Workers Hosted Runner"
)

// Durable hosted ownership is decided by responsibility, not by transitional
// package folder location under workers/services/hosted_logic.
var canonicalHostedResponsibilityOwners = map[string]string{
	responsibilityHostedSourcePolling:        ownerAutomationHostedSources,
	responsibilityHostedSourceReconciliation: ownerAutomationHostedSources,
	responsibilityHostedRunnerExecution:      ownerWorkersHostedRunner,
}

type hostedOwnershipClaimsDocument struct {
	Version int                    `json:"version"`
	Claims  []hostedOwnershipClaim `json:"claims"`
}

type hostedOwnershipClaim struct {
	Responsibility string `json:"responsibility"`
	Owner          string `json:"owner"`
}

func scanHostedOwnershipClaims(repoRoot string) ([]finding, error) {
	path := filepath.Join(repoRoot, hostedOwnershipClaimsFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", hostedOwnershipClaimsFile, err)
	}
	var document hostedOwnershipClaimsDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode %s: %w", hostedOwnershipClaimsFile, err)
	}
	if document.Version != hostedOwnershipVersion {
		return nil, fmt.Errorf(
			"%s version = %d, want %d",
			hostedOwnershipClaimsFile,
			document.Version,
			hostedOwnershipVersion,
		)
	}
	return evaluateHostedOwnershipClaims(document.Claims), nil
}

func evaluateHostedOwnershipClaims(claims []hostedOwnershipClaim) []finding {
	findings := make([]finding, 0, len(claims))
	for _, claim := range claims {
		wantOwner, known := canonicalHostedResponsibilityOwners[claim.Responsibility]
		if !known {
			findings = append(findings, finding{
				Rule:     ruleHostedOwnershipAssignment,
				FilePath: hostedOwnershipClaimsFile,
				Target:   fmt.Sprintf("unknown responsibility %q", claim.Responsibility),
			})
			continue
		}
		if claim.Owner == wantOwner {
			continue
		}
		findings = append(findings, finding{
			Rule:     ruleHostedOwnershipAssignment,
			FilePath: hostedOwnershipClaimsFile,
			Target: fmt.Sprintf(
				"responsibility %q claimed by %q; durable owner is %s",
				claim.Responsibility,
				claim.Owner,
				wantOwner,
			),
		})
	}
	sort.Slice(findings, func(i, j int) bool {
		return findingKey(findings[i]) < findingKey(findings[j])
	})
	return findings
}
