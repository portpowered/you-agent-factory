package main

import (
	"reflect"
	"testing"
)

// findingGolden contains every exported field in the Finding contract. The
// expected values below are authored independently from the evaluator so a
// refactor cannot make the oracle follow the implementation automatically.
type findingGolden struct {
	RuleID       RuleID
	Severity     Severity
	SourcePath   string
	StartLine    int
	StartColumn  int
	EndLine      int
	EndColumn    int
	ContentClass ContentClass
	Excerpt      string
	Guidance     string
	Fingerprint  string
	Identity     string
}

func projectGolden(findings []Finding) []findingGolden {
	result := make([]findingGolden, 0, len(findings))
	for _, finding := range findings {
		result = append(result, findingGolden{
			RuleID:       finding.RuleID,
			Severity:     finding.Severity,
			SourcePath:   finding.SourcePath,
			StartLine:    finding.StartLine,
			StartColumn:  finding.StartColumn,
			EndLine:      finding.EndLine,
			EndColumn:    finding.EndColumn,
			ContentClass: finding.ContentClass,
			Excerpt:      finding.Excerpt,
			Guidance:     finding.Guidance,
			Fingerprint:  finding.Fingerprint,
			Identity:     finding.Identity,
		})
	}
	return result
}

func assertGoldenFindings(t *testing.T, got []Finding, want []findingGolden) {
	t.Helper()
	if projected := projectGolden(got); !reflect.DeepEqual(projected, want) {
		t.Fatalf("findings differ from independent golden:\n got=%#v\nwant=%#v", projected, want)
	}
}

func TestAnalyzeUsesIndependentGoldenForEverySupportedRule(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	spans := []Span{
		{SourcePath: "golden/procedural.md", StartLine: 2, StartColumn: 3, Class: ContentClassProcedural, Text: "Run the command and inspect the result before you continue with the next required operation for this workflow today very carefully."},
		{SourcePath: "golden/descriptive.md", StartLine: 3, StartColumn: 2, Class: ContentClassDescriptive, Text: "The service returns a detailed result that explains how the requested operation changes the current object and reports the observable state after completion for review today."},
		{SourcePath: "golden/paragraph.md", StartLine: 4, StartColumn: 1, Class: ContentClassDescriptive, Text: "First sentence. Second sentence. Third sentence. Fourth sentence. Fifth sentence. Sixth sentence. Seventh sentence."},
		{SourcePath: "golden/semicolon.md", StartLine: 5, StartColumn: 1, Class: ContentClassDescriptive, Text: "One idea; another idea."},
		{SourcePath: "golden/contraction.md", StartLine: 6, StartColumn: 1, Class: ContentClassDescriptive, Text: "The service doesn't stop."},
		{SourcePath: "golden/public-term.md", StartLine: 7, StartColumn: 1, Class: ContentClassDescriptive, Text: "The CPN is internal.", Surfaces: []string{surfaceCustomerDocumentation}},
		{SourcePath: "golden/term-case.md", StartLine: 8, StartColumn: 1, Class: ContentClassDescriptive, Text: "the factory starts.", Surfaces: []string{surfaceCustomerDocumentation}},
		{SourcePath: "golden/suppression.md", StartLine: 9, StartColumn: 1, Class: ContentClassDescriptive, Text: `<!-- prosecheck:ignore B-SEMICOLON owner="Docs" review="2026-12-31" -->`},
		{SourcePath: "golden/parse.md", StartLine: 10, StartColumn: 1, Class: ContentClass("unknown"), Text: "unsafe"},
	}
	want := []findingGolden{
		{RuleID: RuleContraction, Severity: SeverityBlocking, SourcePath: "golden/contraction.md", StartLine: 6, StartColumn: 13, EndLine: 6, EndColumn: 20, ContentClass: ContentClassDescriptive, Excerpt: "doesn't", Guidance: "Use the complete form instead of the contraction.", Fingerprint: "sha256:680e950532c42bc9baa12b73ad0c974a08680e1bc50966d31baa423afcce4fa0"},
		{RuleID: RuleDescriptiveSentenceLength, Severity: SeverityBlocking, SourcePath: "golden/descriptive.md", StartLine: 3, StartColumn: 2, EndLine: 3, EndColumn: 176, ContentClass: ContentClassDescriptive, Excerpt: "The service returns a detailed result that explains how the requested operation changes the current object and reports the observable state after completion fo…", Guidance: "Shorten the descriptive sentence to 25 natural-language words or fewer.", Fingerprint: "sha256:57a2299c4bc33fdea9d90e89876e0253c2f2e954de49a348d7482a20ef4c4a34"},
		{RuleID: RuleDescriptiveParagraphLength, Severity: SeverityBlocking, SourcePath: "golden/paragraph.md", StartLine: 4, StartColumn: 1, EndLine: 4, EndColumn: 116, ContentClass: ContentClassDescriptive, Excerpt: "First sentence. Second sentence. Third sentence. Fourth sentence. Fifth sentence. Sixth sentence. Seventh sentence.", Guidance: "Split the descriptive paragraph into no more than six sentences.", Fingerprint: "sha256:fb4efc51c36a81b1d304a3bf0e1653291cbc9f96b5392d1357694c4969a7162a"},
		{RuleID: RuleParse, Severity: SeverityBlocking, SourcePath: "golden/parse.md", StartLine: 10, StartColumn: 1, EndLine: 10, EndColumn: 2, ContentClass: ContentClass("unknown"), Excerpt: `unknown content class "unknown"`, Guidance: "Fix the source so the analyzer can safely extract every prose span.", Fingerprint: "sha256:6964251ae6d290f4a4b8eaa5c0840bf7bb04fea73b35facaf6ca7b225ab5fcc5"},
		{RuleID: RuleProceduralSentenceLength, Severity: SeverityBlocking, SourcePath: "golden/procedural.md", StartLine: 2, StartColumn: 3, EndLine: 2, EndColumn: 134, ContentClass: ContentClassProcedural, Excerpt: "Run the command and inspect the result before you continue with the next required operation for this workflow today very carefully.", Guidance: "Shorten the procedural sentence to 20 natural-language words or fewer.", Fingerprint: "sha256:92323a28fe678d6e2b14a81b8c7465099c631606bd8a84166383ece436817638"},
		{RuleID: RulePublicTerm, Severity: SeverityBlocking, SourcePath: "golden/public-term.md", StartLine: 7, StartColumn: 5, EndLine: 7, EndColumn: 8, ContentClass: ContentClassDescriptive, Excerpt: "CPN", Guidance: "Use the approved customer term `Factory`.", Fingerprint: "sha256:86ab589f51aa90c3c434f6717872be26a46a3dab416bb59ef9a0d57b87be0933"},
		{RuleID: RuleSemicolon, Severity: SeverityBlocking, SourcePath: "golden/semicolon.md", StartLine: 5, StartColumn: 9, EndLine: 5, EndColumn: 10, ContentClass: ContentClassDescriptive, Excerpt: ";", Guidance: "Replace the semicolon with a full stop or separate the two ideas.", Fingerprint: "sha256:fd84b1d99b6513e13b8d92d902e5f959bbe1ced4396214974202c8bb08bc2fa2"},
		{RuleID: RuleSuppression, Severity: SeverityBlocking, SourcePath: "golden/suppression.md", StartLine: 9, StartColumn: 1, EndLine: 9, EndColumn: 72, ContentClass: ContentClassDescriptive, Excerpt: `<!-- prosecheck:ignore B-SEMICOLON owner="Docs" review="2026-12-31" -->`, Guidance: "Use one known rule, a bounded span, and a reason, owner, and review point.", Fingerprint: "sha256:f5d9b75baa67ea0658d9231b28a950d8bc9bbc710c30be4788de25f64f104c29"},
		{RuleID: RuleTermCase, Severity: SeverityBlocking, SourcePath: "golden/term-case.md", StartLine: 8, StartColumn: 5, EndLine: 8, EndColumn: 12, ContentClass: ContentClassDescriptive, Excerpt: "factory", Guidance: "Use the canonical product-term spelling and capitalization `Factory`.", Fingerprint: "sha256:74d8b144092270e4c5c379c615f12c18546c2e2fe217bbb4892eac88d9601bd5"},
	}
	assertGoldenFindings(t, Analyze(spans, policy), want)
}

func TestLoadPolicyPreservesCanonicalTermSurfaces(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	for _, term := range policy.Terms {
		if term.Canonical != "Factory" {
			continue
		}
		want := []string{"Factory authoring and validation", "CLI help and customer documentation", "Public factory definition API"}
		if !reflect.DeepEqual(term.Surfaces, want) {
			t.Fatalf("Factory surfaces = %#v, want %#v", term.Surfaces, want)
		}
		return
	}
	t.Fatal("Factory term was not loaded")
}

func TestAnalyzeMarkdownUsesIndependentGolden(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	want := []findingGolden{
		{RuleID: RuleContraction, Severity: SeverityBlocking, SourcePath: "docs/guide.md", StartLine: 3, StartColumn: 13, EndLine: 3, EndColumn: 20, ContentClass: ContentClassDescriptive, Excerpt: "doesn't", Guidance: "Use the complete form instead of the contraction.", Fingerprint: "sha256:d0fa3040e1c1715b4b77170d6a44e734feb25d339c25df5c0e4eaa4d7085f7a6"},
		{RuleID: RuleSemicolon, Severity: SeverityBlocking, SourcePath: "docs/guide.md", StartLine: 3, StartColumn: 25, EndLine: 3, EndColumn: 26, ContentClass: ContentClassDescriptive, Excerpt: ";", Guidance: "Replace the semicolon with a full stop or separate the two ideas.", Fingerprint: "sha256:844ec784321746a85031a8744b840756d2756b6a5d37f1b829dd9e613970b6f2"},
	}
	assertGoldenFindings(t, AnalyzeMarkdown("docs/guide.md", []byte("# Guide\n\nThe service doesn't stop; now.\n"), policy), want)
}

func TestAnalyzeCLIManifestUsesIndependentGolden(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	want := []findingGolden{
		{RuleID: RuleContraction, Severity: SeverityBlocking, SourcePath: "contracts/cli/commands.json", StartLine: 17, StartColumn: 46, EndLine: 17, EndColumn: 53, ContentClass: ContentClassDescriptive, Excerpt: "doesn't", Guidance: "Use the complete form instead of the contraction.", Fingerprint: "sha256:b16908890b2b74caa40231cbf8d779a37bcc4c06c076574e3bd220fbcb46b6ec", Identity: "command=you.good;input=you.good.description;json=/commands/you.good/documentation/documentation/description/canonicalEnglish"},
		{RuleID: RuleContraction, Severity: SeverityBlocking, SourcePath: "contracts/cli/commands.json", StartLine: 26, StartColumn: 37, EndLine: 26, EndColumn: 44, ContentClass: ContentClassProcedural, Excerpt: "doesn't", Guidance: "Use the complete form instead of the contraction.", Fingerprint: "sha256:44e0bec9761ea197fdf779e24b6b1196361547873ffa15a7cba2c6b86d1bef6d", Identity: "command=you.good;input=you.good.usage.example;json=/commands/you.good/usage/example"},
		{RuleID: RuleSemicolon, Severity: SeverityBlocking, SourcePath: "contracts/cli/commands.json", StartLine: 32, StartColumn: 33, EndLine: 32, EndColumn: 34, ContentClass: ContentClassProcedural, Excerpt: ";", Guidance: "Replace the semicolon with a full stop or separate the two ideas.", Fingerprint: "sha256:396f8e54707d485ce0f451850d2b8f550a31981e573e94a9c1f825c73dfded7f", Identity: "command=you.good;input=you.good.flag.server;json=/commands/you.good/flags/you.good.flag.server/usage"},
		{RuleID: RuleSemicolon, Severity: SeverityBlocking, SourcePath: "contracts/cli/commands.json", StartLine: 66, StartColumn: 51, EndLine: 66, EndColumn: 52, ContentClass: ContentClassProcedural, Excerpt: ";", Guidance: "Replace the semicolon with a full stop or separate the two ideas.", Fingerprint: "sha256:8b4a488a18edb0f85425a31169b62cf4695df8eb7a1e8b9c042abeca684f49e2", Identity: "command=you.hidden;input=you.hidden.successor;json=/commands/you.hidden/lifecycle/successor/canonicalEnglish"},
	}
	assertGoldenFindings(t, AnalyzeCLIManifest("contracts/cli/commands.json", []byte(cliManifestFixture), policy), want)
}

func TestParseFindingsUseIndependentGoldens(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	markdownWant := []findingGolden{{RuleID: RuleParse, Severity: SeverityBlocking, SourcePath: "docs/broken.md", StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2, ContentClass: ContentClassTechnical, Excerpt: "unclosed ``` code fence at line 2", Guidance: "Fix the source so the analyzer can safely extract every prose span.", Fingerprint: "sha256:890cd3c55ea64fc74c0b4369c9890d10de91298f7d96a8ef4c61a94c9769793a"}}
	cliWant := []findingGolden{{RuleID: RuleParse, Severity: SeverityBlocking, SourcePath: "contracts/cli/commands.json", StartLine: 1, StartColumn: 23, EndLine: 1, EndColumn: 24, ContentClass: ContentClassTechnical, Excerpt: "expected a JSON value at byte 23", Guidance: "Fix the source so the analyzer can safely extract every prose span.", Fingerprint: "sha256:780fbdbbdfb043fb8e4e60615baf3f63ca0df2ab0d24d07aac79669074932dd6"}}
	assertGoldenFindings(t, AnalyzeMarkdown("docs/broken.md", []byte("```sh\n# comment\n"), policy), markdownWant)
	assertGoldenFindings(t, AnalyzeCLIManifest("contracts/cli/commands.json", []byte(`{"commands":{"you.bad":`), policy), cliWant)
}

func TestLineEndingFindingsMatchIndependentGolden(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	want := []findingGolden{{RuleID: RuleContraction, Severity: SeverityBlocking, SourcePath: "docs/guide.md", StartLine: 3, StartColumn: 13, EndLine: 3, EndColumn: 20, ContentClass: ContentClassDescriptive, Excerpt: "doesn't", Guidance: "Use the complete form instead of the contraction.", Fingerprint: "sha256:d0fa3040e1c1715b4b77170d6a44e734feb25d339c25df5c0e4eaa4d7085f7a6"}}
	unix := AnalyzeMarkdown("docs/guide.md", []byte("# Guide\n\nThe service doesn't stop.\n"), policy)
	windows := AnalyzeMarkdown("docs/guide.md", []byte("# Guide\r\n\r\nThe service doesn't stop.\r\n"), policy)
	assertGoldenFindings(t, unix, want)
	assertGoldenFindings(t, windows, want)
}
