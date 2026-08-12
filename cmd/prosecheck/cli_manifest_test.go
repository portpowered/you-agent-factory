package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const cliManifestFixture = `{
  "formatVersion": "1.0.0",
  "rootPath": "you",
  "commands": {
    "you.good": {
      "id": "you.good",
      "visibility": "visible",
      "documentation": {
        "documentation": {
          "visibility": "public",
          "title": {
            "id": "you.good.title",
            "canonicalEnglish": "Start Factory"
          },
          "description": {
            "id": "you.good.description",
            "canonicalEnglish": "The Factory doesn't stop."
          }
        },
        "examples": [
          "you good --flag <value>"
        ]
      },
      "usage": {
        "line": "you good",
        "example": "  # The Factory doesn't stop.\n  you good --flag <value>\n"
      },
      "flags": {
        "you.good.flag.server": {
          "id": "you.good.flag.server",
          "visibility": "visible",
          "usage": "Use --server; continue."
        }
      },
      "arguments": {
        "you.good.arg.0": {
          "id": "you.good.arg.0",
          "visibility": "visible",
          "name": "factory-path",
          "usage": "Use the path."
        }
      },
      "lifecycle": {
        "state": "active"
      }
    },
    "you.hidden": {
      "id": "you.hidden",
      "visibility": "hidden",
      "documentation": {
        "documentation": {
          "title": {
            "id": "you.hidden.title",
            "canonicalEnglish": "The Factory doesn't stop."
          },
          "description": {
            "id": "you.hidden.description",
            "canonicalEnglish": "The hidden command doesn't stop."
          }
        }
      },
      "lifecycle": {
        "state": "deprecated",
        "successor": {
          "targetItemId": "you.good",
          "canonicalEnglish": "Use the old command; replace it."
        }
      }
    }
  }
}`

const ambiguousCLIManifestFixture = `{
  "commands": {
    "you.status": {
      "id": "you.status",
      "visibility": "visible",
      "documentation": {
        "documentation": {
          "title": {"id": "you.status.title", "canonicalEnglish": "Status"},
          "description": {"id": "you.status.description", "canonicalEnglish": "The active service is running before you place the file."}
        }
      }
    }
  }
}`

func TestAnalyzeCLIManifestExtractsVisibleFieldsAndLifecycleGuidance(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	findings := AnalyzeCLIManifest(`.\contracts\cli\commands.json`, []byte(cliManifestFixture), policy)
	if len(findings) != 4 {
		t.Fatalf("AnalyzeCLIManifest() returned %d findings: %#v", len(findings), findings)
	}

	want := []struct {
		rule     RuleID
		class    ContentClass
		excerpt  string
		inputID  string
		jsonPath string
	}{
		{RuleContraction, ContentClassDescriptive, "doesn't", "you.good.description", "/commands/you.good/documentation/documentation/description/canonicalEnglish"},
		{RuleContraction, ContentClassProcedural, "doesn't", "you.good.usage.example", "/commands/you.good/usage/example"},
		{RuleSemicolon, ContentClassProcedural, ";", "you.good.flag.server", "/commands/you.good/flags/you.good.flag.server/usage"},
		{RuleSemicolon, ContentClassProcedural, ";", "you.hidden.successor", "/commands/you.hidden/lifecycle/successor/canonicalEnglish"},
	}
	for index, expected := range want {
		finding := findings[index]
		if finding.RuleID != expected.rule || finding.ContentClass != expected.class || finding.Excerpt != expected.excerpt {
			t.Fatalf("finding %d = %#v, want rule=%s class=%s excerpt=%q", index, finding, expected.rule, expected.class, expected.excerpt)
		}
		if !strings.Contains(finding.Identity, "command=") || !strings.Contains(finding.Identity, "input="+expected.inputID) || !strings.Contains(finding.Identity, "json="+expected.jsonPath) {
			t.Fatalf("finding %d identity = %q, want command, input, and JSON location %q", index, finding.Identity, expected.jsonPath)
		}
		if finding.SourcePath != "contracts/cli/commands.json" || finding.StartLine < 1 || finding.StartColumn < 1 || finding.EndLine < finding.StartLine || finding.EndColumn < 1 {
			t.Fatalf("finding %d source location = %#v, want normalized manifest location", index, finding)
		}
		assertStableFingerprint(t, finding)
	}
	if strings.Contains(findings[0].Identity, "you.hidden") || strings.Contains(findings[1].Identity, "you.hidden") {
		t.Fatalf("hidden command documentation was analyzed: %#v", findings)
	}
}

func TestExtractCLIManifestSpansPreservesAuthoredInputsAndLocations(t *testing.T) {
	spans, err := ExtractCLIManifestSpans("contracts/cli/commands.json", []byte(cliManifestFixture))
	if err != nil {
		t.Fatalf("ExtractCLIManifestSpans() error = %v", err)
	}
	want := map[string]struct {
		class ContentClass
		line  int
	}{
		"json=/commands/you.good/documentation/documentation/title/canonicalEnglish":       {ContentClassLabel, 13},
		"json=/commands/you.good/documentation/documentation/description/canonicalEnglish": {ContentClassDescriptive, 17},
		"json=/commands/you.good/usage/example":                                            {ContentClassProcedural, 26},
		"json=/commands/you.good/flags/you.good.flag.server/usage":                         {ContentClassProcedural, 32},
		"json=/commands/you.good/arguments/you.good.arg.0/usage":                           {ContentClassProcedural, 40},
		"json=/commands/you.hidden/lifecycle/successor/canonicalEnglish":                   {ContentClassProcedural, 66},
	}
	seen := make(map[string]bool, len(spans))
	for _, span := range spans {
		for suffix, expected := range want {
			if !strings.HasSuffix(span.Identity, suffix) {
				continue
			}
			seen[suffix] = true
			if span.Class != expected.class || span.StartLine != expected.line || span.StartColumn < 1 || len(span.Protected) == 0 && strings.Contains(span.Text, "--") {
				t.Fatalf("span %q = %#v, want class=%s line=%d and known column", suffix, span, expected.class, expected.line)
			}
		}
	}
	for suffix := range want {
		if !seen[suffix] {
			t.Fatalf("missing extracted span ending in %q: %#v", suffix, spans)
		}
	}
}

func TestAnalyzeCLIManifestProtectsAuthoredTechnicalLiterals(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	fixture := strings.ReplaceAll(cliManifestFixture, "The Factory doesn't stop.", "Use you good --flag <value> at https://example.com/api and POST /factory-sessions with FACTORY_REQUEST_BATCH, codex, -d \\\"error: doesn't stop\\\".")
	fixture = strings.Replace(fixture, "Use --server; continue.", "Use --server at ./factory/factory.json with error INVOCATION_INPUT_SOURCE_CONFLICT.", 1)
	fixture = strings.Replace(fixture, "Use the old command; replace it.", "Use you good --flag <value> at https://example.com/api.", 1)
	findings := AnalyzeCLIManifest("contracts/cli/commands.json", []byte(fixture), policy)
	if len(findings) != 0 {
		t.Fatalf("technical CLI literals produced findings: %#v", findings)
	}
}

func TestAnalyzeCLIManifestDeclinesAmbiguousRegisterTermsOutsideTheirSurfaces(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	findings := AnalyzeCLIManifest("contracts/cli/commands.json", []byte(ambiguousCLIManifestFixture), policy)
	if len(findings) != 0 {
		t.Fatalf("ambiguous ordinary words produced terminology findings: %#v", findings)
	}
}

func TestAnalyzeCLIManifestReportsAllSafeParseFailuresAndContinues(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	fixture := `{
  "commands": {
    "bad": {
      "id": "bad",
      "visibility": "visible",
      "documentation": 7,
      "flags": {
        "bad.flag": "not an object"
      }
    },
    "good": {
      "id": "good",
      "visibility": "visible",
      "documentation": {
        "documentation": {
          "title": {"id": "good.title", "canonicalEnglish": "Good"},
          "description": {"id": "good.description", "canonicalEnglish": "The Factory doesn't stop."}
        }
      }
    }
  }
}`
	findings := AnalyzeCLIManifest("contracts/cli/commands.json", []byte(fixture), policy)
	var parseCount, contractionCount int
	for _, finding := range findings {
		if finding.RuleID == RuleParse {
			parseCount++
		}
		if finding.RuleID == RuleContraction {
			contractionCount++
		}
	}
	if parseCount != 2 || contractionCount != 1 {
		t.Fatalf("parse/valid findings = %#v, want two B-PARSE findings and one valid-command contraction", findings)
	}
	for _, finding := range findings {
		if finding.RuleID == RuleParse && !strings.Contains(finding.Identity, "command=bad") {
			t.Fatalf("parse finding identity = %q, want bad command identity", finding.Identity)
		}
	}

	malformed := []byte(`{"commands":{"you.bad":`)
	findings = AnalyzeCLIManifest("contracts/cli/commands.json", malformed, policy)
	if len(findings) != 1 || findings[0].RuleID != RuleParse || !strings.Contains(findings[0].SourcePath, "contracts/cli/commands.json") {
		t.Fatalf("malformed JSON findings = %#v, want one manifest-scoped B-PARSE finding", findings)
	}
}

func TestAnalyzeCLIManifestIsReadOnlyAndStableAcrossLineEndingsAndRepeatedRuns(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	data := []byte(cliManifestFixture)
	before := append([]byte(nil), data...)
	first := AnalyzeCLIManifest("contracts/cli/commands.json", data, policy)
	second := AnalyzeCLIManifest("contracts/cli/commands.json", data, policy)
	if !reflect.DeepEqual(first, second) || !bytes.Equal(data, before) {
		t.Fatalf("repeated analysis changed output or input: first=%#v second=%#v inputChanged=%t", first, second, !bytes.Equal(data, before))
	}

	windows := strings.ReplaceAll(cliManifestFixture, "\n", "\r\n")
	windowFindings := AnalyzeCLIManifest("contracts/cli/commands.json", []byte(windows), policy)
	if !reflect.DeepEqual(first, windowFindings) {
		t.Fatalf("line endings changed findings:\nLF=%#v\r\nCRLF=%#v", first, windowFindings)
	}
}

func TestRunDispatchesAuthoredCLIManifestAndRejectsGeneratedProjection(t *testing.T) {
	root := t.TempDir()
	authored := filepath.Join(root, "contracts", "cli", "commands.json")
	if err := os.MkdirAll(filepath.Dir(authored), 0o700); err != nil {
		t.Fatalf("mkdir manifest fixture: %v", err)
	}
	if err := os.WriteFile(authored, []byte(cliManifestFixture), 0o600); err != nil {
		t.Fatalf("write manifest fixture: %v", err)
	}
	standardPath, termsPath := repositoryPolicyPaths(t)
	var stdout, stderr bytes.Buffer
	err := run([]string{"-standard", standardPath, "-terms", termsPath, authored}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "found 4 blocking finding") || stdout.Len() != 0 || !strings.Contains(stderr.String(), "[B-CONTRACTION]") {
		t.Fatalf("run(authored manifest) = err %v stdout %q stderr %q", err, stdout.String(), stderr.String())
	}

	generated := filepath.Join(root, "packages", "api", "generated", "cli", "commands.json")
	err = run([]string{"-standard", standardPath, "-terms", termsPath, generated}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "generated CLI projection") {
		t.Fatalf("run(generated projection) error = %v, want explicit authoring-input rejection", err)
	}

	stdout.Reset()
	stderr.Reset()
	t.Chdir(root)
	err = run([]string{"-standard", standardPath, "-terms", termsPath, "packages/api/generated/cli/commands.json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "generated CLI projection") {
		t.Fatalf("run(repo-relative generated projection) error = %v, want rejection before read", err)
	}
}
