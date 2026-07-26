package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHostedOwnershipRejectsPollingAssignedToWorkersHostedRunner(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		hostedOwnershipClaimsFile: `{
  "version": 1,
  "claims": [
    {
      "responsibility": "hosted-source-polling",
      "owner": "Workers Hosted Runner"
    },
    {
      "responsibility": "hosted-source-reconciliation",
      "owner": "Workers Hosted Runner"
    }
  ]
}
`,
	})

	findings := scanHostedOwnershipFixture(t, root)
	if len(findings) == 0 {
		t.Fatal("expected hosted-source polling mis-assignment findings")
	}
	for _, item := range findings {
		if item.Rule != ruleHostedOwnershipAssignment {
			t.Fatalf("unexpected rule %q", item.Rule)
		}
		if !strings.Contains(item.Target, ownerAutomationHostedSources) {
			t.Fatalf("diagnostic %#v must name %q as owning responsibility", item, ownerAutomationHostedSources)
		}
	}
	assertHostedFindingNamesOwner(t, findings, responsibilityHostedSourcePolling, ownerAutomationHostedSources)
	assertHostedFindingNamesOwner(t, findings, responsibilityHostedSourceReconciliation, ownerAutomationHostedSources)
}

func TestHostedOwnershipRejectsRunnerExecutionAssignedToAutomationHostedSources(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		hostedOwnershipClaimsFile: `{
  "version": 1,
  "claims": [
    {
      "responsibility": "hosted-runner-execution",
      "owner": "Automation Hosted Sources"
    }
  ]
}
`,
	})

	findings := scanHostedOwnershipFixture(t, root)
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want one hosted-runner mis-assignment", findings)
	}
	assertHostedFindingNamesOwner(t, findings, responsibilityHostedRunnerExecution, ownerWorkersHostedRunner)
}

func TestHostedOwnershipAllowsDistinguishedAutomationAndWorkersClaims(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		hostedOwnershipClaimsFile: `{
  "version": 1,
  "claims": [
    {
      "responsibility": "hosted-source-polling",
      "owner": "Automation Hosted Sources"
    },
    {
      "responsibility": "hosted-source-reconciliation",
      "owner": "Automation Hosted Sources"
    },
    {
      "responsibility": "hosted-runner-execution",
      "owner": "Workers Hosted Runner"
    }
  ]
}
`,
		"pkg/services/workers/services/hosted_logic/linear/poll.go": `package linear
// Transitional package location must not be treated as durable Workers ownership.
func Poll() {}
`,
	})

	findings := scanHostedOwnershipFixture(t, root)
	if len(findings) != 0 {
		t.Fatalf("correctly distinguished claims produced findings: %#v", findings)
	}
}

func TestHostedOwnershipIgnoresTransitionalHostedLogicLocationWithoutClaims(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/workers/services/hosted_logic/linear/poll.go": `package linear
func Poll() {}
`,
		"pkg/services/automations/service/hosted_poller.go": `package service
func Observe() {}
`,
	})

	findings := scanHostedOwnershipFixture(t, root)
	if len(findings) != 0 {
		t.Fatalf("transitional package location alone produced hosted ownership findings: %#v", findings)
	}
}

func TestScanIncludesHostedOwnershipClaims(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		hostedOwnershipClaimsFile: `{
  "version": 1,
  "claims": [
    {
      "responsibility": "hosted-runner-execution",
      "owner": "Automation Hosted Sources"
    }
  ]
}
`,
	})

	findings := scanFixture(t, root)
	assertFinding(t, findings, ruleHostedOwnershipAssignment, ownerWorkersHostedRunner)
}

func scanHostedOwnershipFixture(t *testing.T, root string) []finding {
	t.Helper()
	findings, err := scanHostedOwnershipClaims(root)
	if err != nil {
		t.Fatalf("scan hosted ownership claims: %v", err)
	}
	return findings
}

func assertHostedFindingNamesOwner(t *testing.T, findings []finding, responsibility, wantOwner string) {
	t.Helper()
	for _, item := range findings {
		if item.Rule != ruleHostedOwnershipAssignment {
			continue
		}
		if !strings.Contains(item.Target, responsibility) {
			continue
		}
		if !strings.Contains(item.Target, wantOwner) {
			t.Fatalf("finding %#v for %q does not name owner %q", item, responsibility, wantOwner)
		}
		if filepath.ToSlash(item.FilePath) != hostedOwnershipClaimsFile {
			t.Fatalf("finding file path = %q, want %q", item.FilePath, hostedOwnershipClaimsFile)
		}
		return
	}
	t.Fatalf("findings %#v do not cover responsibility %q", findings, responsibility)
}
