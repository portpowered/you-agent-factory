package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestLoadPolicyConsumesCanonicalWritingSources(t *testing.T) {
	policy := loadRepositoryPolicy(t)

	for _, class := range []ContentClass{
		ContentClassLabel,
		ContentClassProcedural,
		ContentClassDescriptive,
		ContentClassTechnical,
		ContentClassTerm,
	} {
		if policy.ContentClasses[class] == "" {
			t.Fatalf("policy content class %q was not loaded", class)
		}
	}
	for _, ruleID := range []RuleID{
		RuleProceduralSentenceLength,
		RuleDescriptiveSentenceLength,
		RuleDescriptiveParagraphLength,
		RuleSemicolon,
		RuleContraction,
		RulePublicTerm,
		RuleTermCase,
		RuleSuppression,
		RuleParse,
	} {
		rule, ok := policy.Rules[ruleID]
		if !ok || !rule.Supported || rule.Severity != SeverityBlocking {
			t.Fatalf("policy rule %q = %#v, want supported blocking rule", ruleID, rule)
		}
	}
	if policy.Rules[RuleBaselineNew].Supported || policy.Rules[RuleBaselineStale].Supported {
		t.Fatal("baseline rules must remain outside the story 001 analyzer")
	}
	if !containsTerm(policy.Terms, "Factory") || !containsTerm(policy.Terms, "CPN") {
		t.Fatal("policy did not load the canonical product and internal terminology records")
	}
}

func TestLoadPolicyRejectsIncompleteCanonicalSources(t *testing.T) {
	standard := []byte("### Labels\n### Procedural prose\n### Descriptive prose\n### Machine and technical text\n### Technical terms\n")
	terms := []byte("version: 1\nterms: []\n")
	if _, err := LoadPolicy(standard, terms); err == nil || !strings.Contains(err.Error(), "no rule IDs") {
		t.Fatalf("LoadPolicy() error = %v, want missing-rule error", err)
	}

	standard = []byte("### Labels\n### Procedural prose\n### Descriptive prose\n### Machine and technical text\n### Technical terms\n- **B-PROC-20:** rule\n- **B-DESC-25:** rule\n- **B-PARA-6:** rule\n- **B-SEMICOLON:** rule\n- **B-CONTRACTION:** rule\n- **B-TERM-PUBLIC:** rule\n- **B-TERM-CASE:** rule\n- **B-LITERAL:** rule\n- **B-SUPPRESSION:** rule\n- **B-BASELINE-STALE:** rule\n- **B-BASELINE-NEW:** rule\n- **B-PARSE:** rule\n")
	if _, err := LoadPolicy(standard, terms); err == nil || !strings.Contains(err.Error(), "no terms") {
		t.Fatalf("LoadPolicy() error = %v, want empty-terms error", err)
	}
}

func TestAnalyzeReportsEverySupportedBlockingRule(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	tests := []struct {
		name        string
		class       ContentClass
		text        string
		wantRule    RuleID
		wantExcerpt string
	}{
		{
			name:        "procedural sentence length",
			class:       ContentClassProcedural,
			text:        "Run the command and inspect the result before you continue with the next required operation for this workflow today very carefully.",
			wantRule:    RuleProceduralSentenceLength,
			wantExcerpt: "Run the command and inspect the result before you continue with the next required operation for this workflow today very carefully.",
		},
		{
			name:        "descriptive sentence length",
			class:       ContentClassDescriptive,
			text:        "The service returns a detailed result that explains how the requested operation changes the current object and reports the observable state after completion for review today.",
			wantRule:    RuleDescriptiveSentenceLength,
			wantExcerpt: "The service returns a detailed result that explains how the requested operation changes the current object and reports the observable state after completion for review today.",
		},
		{
			name:        "descriptive paragraph length",
			class:       ContentClassDescriptive,
			text:        "First sentence. Second sentence. Third sentence. Fourth sentence. Fifth sentence. Sixth sentence. Seventh sentence.",
			wantRule:    RuleDescriptiveParagraphLength,
			wantExcerpt: "First sentence. Second sentence. Third sentence. Fourth sentence. Fifth sentence. Sixth sentence. Seventh sentence.",
		},
		{
			name:        "semicolon",
			class:       ContentClassDescriptive,
			text:        "One idea; another idea.",
			wantRule:    RuleSemicolon,
			wantExcerpt: ";",
		},
		{
			name:        "contraction",
			class:       ContentClassDescriptive,
			text:        "The service doesn't stop.",
			wantRule:    RuleContraction,
			wantExcerpt: "doesn't",
		},
		{
			name:        "prohibited public term",
			class:       ContentClassDescriptive,
			text:        "The CPN is internal.",
			wantRule:    RulePublicTerm,
			wantExcerpt: "CPN",
		},
		{
			name:        "canonical term case",
			class:       ContentClassDescriptive,
			text:        "the factory starts.",
			wantRule:    RuleTermCase,
			wantExcerpt: "factory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := Analyze([]Span{{
				SourcePath:  "docs/guide.md",
				StartLine:   3,
				StartColumn: 2,
				Class:       tt.class,
				Text:        tt.text,
				Surfaces:    []string{surfaceCustomerDocumentation},
			}}, policy)
			if len(findings) != 1 {
				t.Fatalf("Analyze() returned %d findings: %#v", len(findings), findings)
			}
			finding := findings[0]
			if finding.RuleID != tt.wantRule || finding.Severity != SeverityBlocking || finding.ContentClass != tt.class {
				t.Fatalf("finding identity = %#v, want rule=%s severity=%s class=%s", finding, tt.wantRule, SeverityBlocking, tt.class)
			}
			if finding.SourcePath != "docs/guide.md" || finding.StartLine != 3 {
				t.Fatalf("finding source = %#v, want normalized path on line 3", finding)
			}
			if finding.Excerpt != boundedExcerpt(tt.wantExcerpt) || finding.Guidance == "" {
				t.Fatalf("finding excerpt/guidance = %q / %q, want excerpt %q and guidance", finding.Excerpt, finding.Guidance, boundedExcerpt(tt.wantExcerpt))
			}
			assertStableFingerprint(t, finding)
		})
	}
}

func TestAnalyzeCountsAContractionAsOneNaturalLanguageWord(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	text := "Use this command when you don't need to inspect the result before continuing with the next step in the workflow."
	findings := Analyze([]Span{{
		SourcePath: "docs/procedure.md",
		Class:      ContentClassProcedural,
		Text:       text,
	}}, policy)
	if len(findings) != 1 || findings[0].RuleID != RuleContraction {
		t.Fatalf("findings = %#v, want only B-CONTRACTION at the twenty-word boundary", findings)
	}
}

func TestAnalyzeProtectsTechnicalRangesAndTechnicalSpans(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	text := "The CPN; don't stop."
	findings := Analyze([]Span{{
		SourcePath:  "docs/protected.md",
		StartLine:   1,
		StartColumn: 1,
		Class:       ContentClassDescriptive,
		Text:        text,
		Protected: []TextRange{
			{Start: 4, End: 7},
			{Start: 7, End: 15},
		},
	}}, policy)
	if len(findings) != 0 {
		t.Fatalf("protected ranges produced findings: %#v", findings)
	}

	findings = Analyze([]Span{{
		SourcePath:  "docs/technical.md",
		StartLine:   1,
		StartColumn: 1,
		Class:       ContentClassTechnical,
		Text:        "The CPN; don't stop.",
	}}, policy)
	if len(findings) != 0 {
		t.Fatalf("technical span produced findings: %#v", findings)
	}

	findings = Analyze([]Span{{
		SourcePath: "docs/protected.md",
		Class:      ContentClassDescriptive,
		Text:       text,
		Protected:  []TextRange{{Start: 4, End: 8}, {Start: 7, End: 15}},
	}}, policy)
	if len(findings) != 1 || findings[0].RuleID != RuleParse {
		t.Fatalf("overlapping ranges = %#v, want one B-PARSE finding", findings)
	}
}

func TestAnalyzeReportsLineAndColumnAcrossCRLF(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	findings := Analyze([]Span{{
		SourcePath:  `./docs\guide.md`,
		StartLine:   4,
		StartColumn: 3,
		Class:       ContentClassDescriptive,
		Text:        "A safe sentence.\r\nfactory starts.",
		Surfaces:    []string{surfaceCustomerDocumentation},
	}}, policy)
	if len(findings) != 1 {
		t.Fatalf("Analyze() findings = %#v, want one term-case finding", findings)
	}
	if findings[0].RuleID != RuleTermCase || findings[0].SourcePath != "docs/guide.md" || findings[0].StartLine != 5 || findings[0].StartColumn != 1 {
		t.Fatalf("finding location = %#v, want docs/guide.md:5:1", findings[0])
	}
}

func TestAnalyzeSupportsBoundedSuppressionsAndRejectsMalformedDirectives(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	valid := `<!-- prosecheck:ignore B-SEMICOLON reason="contract wording" owner="Docs" review="2026-12-31" -->
One idea; another idea.`
	if findings := Analyze([]Span{{SourcePath: "docs/guide.md", Class: ContentClassDescriptive, Text: valid}}, policy); len(findings) != 0 {
		t.Fatalf("valid suppression produced findings: %#v", findings)
	}

	for _, text := range []string{
		`<!-- prosecheck:ignore B-SEMICOLON owner="Docs" review="2026-12-31" --> One; two.`,
		`<!-- prosecheck:ignore B-NOT-A-RULE reason="why" owner="Docs" review="2026-12-31" --> One; two.`,
	} {
		findings := Analyze([]Span{{SourcePath: "docs/guide.md", Class: ContentClassDescriptive, Text: text}}, policy)
		if len(findings) != 2 || findings[0].RuleID != RuleSuppression || findings[1].RuleID != RuleSemicolon || findings[0].SourcePath != "docs/guide.md" {
			t.Fatalf("malformed suppression findings = %#v, want B-SUPPRESSION plus the unsuppressed semicolon", findings)
		}
	}

	if _, err := ParseSuppressionDirective(`<!-- prosecheck:ignore B-SEMICOLON reason="why" owner="Docs" -->`); err == nil {
		t.Fatal("ParseSuppressionDirective accepted a directive without review")
	}
}

func TestAnalyzeOrderingRenderingAndFingerprintsAreDeterministic(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	spans := []Span{
		{SourcePath: `b\two.md`, StartLine: 2, StartColumn: 1, Class: ContentClassDescriptive, Text: "One; two."},
		{SourcePath: `a\one.md`, StartLine: 3, StartColumn: 1, Class: ContentClassDescriptive, Text: "The service doesn't stop."},
	}
	first := Analyze(spans, policy)
	second := Analyze([]Span{spans[1], spans[0]}, policy)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reordered spans changed findings:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first) != 2 || first[0].SourcePath != "a/one.md" || first[1].SourcePath != "b/two.md" {
		t.Fatalf("findings = %#v, want a/one.md before b/two.md", first)
	}
	var output bytes.Buffer
	if err := RenderFindings(&output, first); err != nil {
		t.Fatalf("RenderFindings() error = %v", err)
	}
	if !strings.Contains(output.String(), "[B-CONTRACTION]") || !strings.Contains(output.String(), "[B-SEMICOLON]") {
		t.Fatalf("rendered output = %q, want both stable rule IDs", output.String())
	}
	for _, finding := range first {
		assertStableFingerprint(t, finding)
	}

	unix := Analyze([]Span{{SourcePath: "docs/guide.md", Class: ContentClassDescriptive, Text: "One; two.\n"}}, policy)
	windows := Analyze([]Span{{SourcePath: "docs/guide.md", Class: ContentClassDescriptive, Text: "One; two.\r\n"}}, policy)
	if len(unix) != 1 || len(windows) != 1 || unix[0].Fingerprint != windows[0].Fingerprint {
		t.Fatalf("line-ending fingerprints differ: unix=%#v windows=%#v", unix, windows)
	}
}

func TestLexicalProtectedRangesProtectInlineCodeAndURLs(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	text := "Use `CPN; don't` or https://example.com/path; now."
	findings := Analyze([]Span{{
		SourcePath: "docs/guide.md",
		Class:      ContentClassDescriptive,
		Text:       text,
		Protected:  LexicalProtectedRanges(text),
	}}, policy)
	if len(findings) != 1 || findings[0].RuleID != RuleSemicolon || findings[0].Excerpt != ";" {
		t.Fatalf("lexical protection findings = %#v, want only trailing semicolon", findings)
	}
}

func TestExtractMarkdownSpansCoversSupportedStructures(t *testing.T) {
	content := strings.Join([]string{
		"# Guide",
		"",
		"The description explains the current state.",
		"",
		"1. Run the command and inspect the result.",
		"- A descriptive list item.",
		"",
		"> [!NOTE]",
		"> The note explains the current state.",
		"",
		"!!! warning",
		"",
		"   The warning explains the recovery path.",
		"",
		"| Name | Description |",
		"| --- | --- |",
		"| Factory | A saved definition. |",
		"",
		"<!-- The comment explains the current state. -->",
		"",
		"```sh",
		"# Start the local server before sending the request.",
		"you server --listen 127.0.0.1:7437",
		"```",
	}, "\n")

	spans, err := ExtractMarkdownSpans("docs/guide.md", []byte(content))
	if err != nil {
		t.Fatalf("ExtractMarkdownSpans() error = %v", err)
	}
	if len(spans) != 12 {
		t.Fatalf("ExtractMarkdownSpans() returned %d spans: %#v", len(spans), spans)
	}
	want := []struct {
		text  string
		class ContentClass
		line  int
		col   int
	}{
		{"Guide", ContentClassLabel, 1, 3},
		{"The description explains the current state.", ContentClassDescriptive, 3, 1},
		{"Run the command and inspect the result.", ContentClassProcedural, 5, 4},
		{"A descriptive list item.", ContentClassDescriptive, 6, 3},
		{"The note explains the current state.", ContentClassDescriptive, 9, 3},
		{"The warning explains the recovery path.", ContentClassDescriptive, 13, 4},
		{"Name", ContentClassLabel, 15, 3},
		{"Description", ContentClassLabel, 15, 10},
		{"Factory", ContentClassDescriptive, 17, 3},
		{"A saved definition.", ContentClassDescriptive, 17, 13},
		{"The comment explains the current state.", ContentClassDescriptive, 19, 6},
		{"Start the local server before sending the request.", ContentClassProcedural, 22, 3},
	}
	for index, expected := range want {
		if spans[index].Text != expected.text || spans[index].Class != expected.class || spans[index].StartLine != expected.line || spans[index].StartColumn != expected.col {
			t.Fatalf("span %d = %#v, want text=%q class=%s at %d:%d", index, spans[index], expected.text, expected.class, expected.line, expected.col)
		}
	}
}

func TestMarkdownAdmonitionVariantsExtractNaturalContent(t *testing.T) {
	content := strings.Join([]string{
		"> [!NOTE] A note explains the current state.",
		"",
		":::warning",
		"A warning explains the recovery path.",
		":::",
	}, "\n")
	spans, err := ExtractMarkdownSpans("docs/admonitions.md", []byte(content))
	if err != nil {
		t.Fatalf("ExtractMarkdownSpans() error = %v", err)
	}
	if len(spans) != 2 || spans[0].Text != "A note explains the current state." || spans[1].Text != "A warning explains the recovery path." {
		t.Fatalf("admonition spans = %#v, want both natural-language bodies", spans)
	}
}

func TestAnalyzeMarkdownProtectsTechnicalTextAndReportsNaturalProse(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	content := strings.Join([]string{
		"Keep `CPN; don't` and `POST /factory-sessions/sync` exact.",
		"The service doesn't stop; now.",
		"",
		"```sh",
		"# Start the local server before sending the request.",
		"you server --listen 127.0.0.1:7437",
		"```",
	}, "\n")
	findings := AnalyzeMarkdown("docs/guide.md", []byte(content), policy)
	if len(findings) != 2 {
		t.Fatalf("AnalyzeMarkdown() returned %d findings: %#v", len(findings), findings)
	}
	if findings[0].RuleID != RuleContraction || findings[0].StartLine != 2 || findings[0].Excerpt != "doesn't" {
		t.Fatalf("first Markdown finding = %#v, want contraction on line 2", findings[0])
	}
	if findings[1].RuleID != RuleSemicolon || findings[1].StartLine != 2 || findings[1].Excerpt != ";" {
		t.Fatalf("second Markdown finding = %#v, want semicolon on line 2", findings[1])
	}
	for _, finding := range findings {
		assertStableFingerprint(t, finding)
	}
}

func TestMarkdownLinkDestinationProtectionKeepsLabelNatural(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	findings := AnalyzeMarkdown("docs/links.md", []byte("the [factory](./factory.json) starts."), policy)
	if len(findings) != 1 || findings[0].RuleID != RuleTermCase || findings[0].Excerpt != "factory" || findings[0].StartColumn != 6 {
		t.Fatalf("link findings = %#v, want visible label term-case at column 6", findings)
	}
}

func TestAnalyzeMarkdownBlocksOnUnsafeDocumentsAndNormalizesLineEndings(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	broken := AnalyzeMarkdown("docs/broken.md", []byte("```sh\n# comment\n"), policy)
	if len(broken) != 1 || broken[0].RuleID != RuleParse || broken[0].SourcePath != "docs/broken.md" || !strings.Contains(broken[0].Excerpt, "unclosed") {
		t.Fatalf("unclosed Markdown fence findings = %#v, want one file-scoped B-PARSE finding", broken)
	}
	invalid := AnalyzeMarkdown("docs/invalid.md", []byte{0xff, '\n'}, policy)
	if len(invalid) != 1 || invalid[0].RuleID != RuleParse || invalid[0].SourcePath != "docs/invalid.md" {
		t.Fatalf("invalid UTF-8 findings = %#v, want one file-scoped B-PARSE finding", invalid)
	}
	for name, source := range map[string]string{
		"inline code": "Keep `an unclosed literal in prose.",
		"table":       "| Name | Description |\n| --- |",
	} {
		findings := AnalyzeMarkdown("docs/"+name+".md", []byte(source), policy)
		if len(findings) != 1 || findings[0].RuleID != RuleParse {
			t.Fatalf("%s parse findings = %#v, want one B-PARSE finding", name, findings)
		}
	}

	unix := AnalyzeMarkdown("docs/guide.md", []byte("# Guide\n\nThe service doesn't stop.\n"), policy)
	windows := AnalyzeMarkdown("docs/guide.md", []byte("# Guide\r\n\r\nThe service doesn't stop.\r\n"), policy)
	if !reflect.DeepEqual(unix, windows) {
		t.Fatalf("line endings changed Markdown findings:\nunix=%#v\nwindows=%#v", unix, windows)
	}
}

func TestMarkdownMetadataProvidesExplicitClassificationAndBoundedSuppression(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	content := strings.Join([]string{
		"<!-- prosecheck:class procedural -->",
		"Run the command and inspect the result before you continue with the next required operation for this workflow today very carefully now.",
		"",
		"<!-- prosecheck:ignore B-SEMICOLON reason=\"contract wording\" owner=\"Docs\" review=\"2026-12-31\" -->",
		"One; two.",
	}, "\n")
	findings := AnalyzeMarkdown("docs/metadata.md", []byte(content), policy)
	if len(findings) != 1 || findings[0].RuleID != RuleProceduralSentenceLength || findings[0].ContentClass != ContentClassProcedural || findings[0].StartLine != 2 {
		t.Fatalf("metadata findings = %#v, want one procedural-length finding on line 2", findings)
	}
}

func TestAnalyzeMarkdownDeclinesAmbiguousRegisterTermsOutsideTheirSurfaces(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	findings := AnalyzeMarkdown("docs/ambiguous.md", []byte("The active service is running before you place the file.\n"), policy)
	if len(findings) != 0 {
		t.Fatalf("ambiguous ordinary words produced terminology findings: %#v", findings)
	}
}

func TestMarkdownSurfaceContextKeepsProductTermsBounded(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	internal := AnalyzeMarkdown("docs/architecture/ambiguous.md", []byte("Workers are things that can do work.\n"), policy)
	for _, finding := range internal {
		if finding.RuleID == RuleTermCase || finding.RuleID == RulePublicTerm {
			t.Fatalf("internal architecture prose produced product terminology finding: %#v", finding)
		}
	}
	reference := AnalyzeMarkdown("docs/reference/ambiguous.md", []byte("The model worker is described here.\n"), policy)
	for _, finding := range reference {
		if finding.RuleID == RuleTermCase || finding.RuleID == RulePublicTerm {
			t.Fatalf("reference prose produced ambiguous product terminology finding: %#v", finding)
		}
	}
	customer := AnalyzeMarkdown("docs/guide.md", []byte("the factory starts.\n"), policy)
	if len(customer) != 1 || customer[0].RuleID != RuleTermCase || customer[0].Excerpt != "factory" {
		t.Fatalf("customer prose lost positive product-term finding: %#v", customer)
	}
}

func TestAnalyzeMarkdownProtectsRequiredTechnicalFixtureMatrix(t *testing.T) {
	policy := loadRepositoryPolicy(t)
	cases := []struct {
		name      string
		technical string
	}{
		{name: "shell operators", technical: "$ go test ./cmd/prosecheck && go vet ./cmd/prosecheck || exit 1"},
		{name: "file and package paths", technical: "Read ./pkg/services/factory_runtime/internal/runner.go and github.com/portpowered/infinite-you/pkg/services."},
		{name: "identifiers", technical: "Keep FactoryEvent and FACTORY_REQUEST_BATCH unchanged."},
		{name: "schema and event literals", technical: "The FactoryResponseEvent schema carries the event payload."},
		{name: "JSON contract example", technical: `{"event":"FactoryEvent","state":"active; doesn't stop"}`},
		{name: "YAML contract example", technical: "state: active; message: doesn't stop"},
		{name: "model and provider names", technical: "Use gpt-5 with OpenAI and anthropic."},
		{name: "quoted external output", technical: `error: doesn't stop; external output`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			content := testCase.technical + "\n\nThe service doesn't stop.\n"
			findings := AnalyzeMarkdown("docs/technical-matrix.md", []byte(content), policy)
			if len(findings) != 1 || findings[0].RuleID != RuleContraction || findings[0].Excerpt != "doesn't" || findings[0].StartLine != 3 {
				t.Fatalf("technical fixture findings = %#v, want only adjacent prose contraction", findings)
			}
		})
	}
}

func TestRunReadsExplicitInputsAndKeepsOutputStable(t *testing.T) {
	root := t.TempDir()
	standardPath, termsPath := repositoryPolicyPaths(t)
	validPath := filepath.Join(root, "valid.txt")
	invalidPath := filepath.Join(root, "invalid.txt")
	if err := os.WriteFile(validPath, []byte("The service is ready."), 0o600); err != nil {
		t.Fatalf("write valid input: %v", err)
	}
	if err := os.WriteFile(invalidPath, []byte("One; two."), 0o600); err != nil {
		t.Fatalf("write invalid input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := run([]string{"-standard", standardPath, "-terms", termsPath, validPath}, &stdout, &stderr); err != nil {
		t.Fatalf("run(valid) error = %v", err)
	}
	if stdout.String() != "prosecheck passed (1 input(s))\n" || stderr.Len() != 0 {
		t.Fatalf("run(valid) output = stdout %q stderr %q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	err := run([]string{"-standard", standardPath, "-terms", termsPath, invalidPath}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "found 1 blocking finding") {
		t.Fatalf("run(invalid) error = %v, want blocking finding error", err)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "[B-SEMICOLON]") {
		t.Fatalf("run(invalid) output = stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

func TestRunDispatchesMarkdownInputs(t *testing.T) {
	root := t.TempDir()
	standardPath, termsPath := repositoryPolicyPaths(t)
	path := filepath.Join(root, "guide.md")
	if err := os.WriteFile(path, []byte("# Guide\r\n\r\nThe service doesn't stop.\r\n"), 0o600); err != nil {
		t.Fatalf("write Markdown input: %v", err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"-standard", standardPath, "-terms", termsPath, path}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "found 1 blocking finding") {
		t.Fatalf("run(Markdown) error = %v, want blocking finding error", err)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), ":3:13 [B-CONTRACTION]") {
		t.Fatalf("run(Markdown) output = stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

func loadRepositoryPolicy(t *testing.T) Policy {
	t.Helper()
	standardPath, termsPath := repositoryPolicyPaths(t)
	standard, err := os.ReadFile(standardPath)
	if err != nil {
		t.Fatalf("read writing standard: %v", err)
	}
	terms, err := os.ReadFile(termsPath)
	if err != nil {
		t.Fatalf("read terminology register: %v", err)
	}
	policy, err := LoadPolicy(standard, terms)
	if err != nil {
		t.Fatalf("LoadPolicy(): %v", err)
	}
	return policy
}

func repositoryPolicyPaths(t *testing.T) (string, string) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	return filepath.Join(root, "docs", "internal", "standards", "writing", "customer-technical-writing-standard.md"), filepath.Join(root, "docs", "internal", "standards", "writing", "customer-technical-terms.yaml")
}

func containsTerm(terms []Term, canonical string) bool {
	for _, term := range terms {
		if term.Canonical == canonical {
			return true
		}
	}
	return false
}

func assertStableFingerprint(t *testing.T, finding Finding) {
	t.Helper()
	if !strings.HasPrefix(finding.Fingerprint, "sha256:") || len(finding.Fingerprint) != len("sha256:")+64 {
		t.Fatalf("fingerprint = %q, want sha256 hex", finding.Fingerprint)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(finding.Fingerprint, "sha256:")); err != nil {
		t.Fatalf("fingerprint %q is not hex: %v", finding.Fingerprint, err)
	}
	recomputed := sha256.Sum256([]byte(strings.Join([]string{
		string(finding.RuleID), string(finding.Severity), finding.SourcePath,
		finding.Identity, string(finding.ContentClass),
		strconv.Itoa(finding.StartLine), strconv.Itoa(finding.StartColumn), finding.Excerpt,
	}, "\x00")))
	if finding.Fingerprint != "sha256:"+hex.EncodeToString(recomputed[:]) {
		t.Fatalf("fingerprint = %q is not derived from normalized finding fields", finding.Fingerprint)
	}
}
