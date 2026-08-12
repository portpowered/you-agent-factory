package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultStandardPath = "docs/internal/standards/writing/customer-technical-writing-standard.md"
	defaultTermsPath    = "docs/internal/standards/writing/customer-technical-terms.yaml"
)

type config struct {
	standardPath string
	termsPath    string
	class        ContentClass
	inputs       []string
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run is the executable boundary. Markdown files are structurally extracted;
// other files retain the explicit content-class behavior used by the core.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	standard, err := os.ReadFile(cfg.standardPath)
	if err != nil {
		return fmt.Errorf("read writing standard %s: %w", cfg.standardPath, err)
	}
	terms, err := os.ReadFile(cfg.termsPath)
	if err != nil {
		return fmt.Errorf("read terminology register %s: %w", cfg.termsPath, err)
	}
	policy, err := LoadPolicy(standard, terms)
	if err != nil {
		return fmt.Errorf("load prose policy: %w", err)
	}

	spans := make([]Span, 0, len(cfg.inputs))
	var adapterFindings []Finding
	for _, input := range cfg.inputs {
		if isGeneratedCLIProjection(input) {
			return fmt.Errorf("generated CLI projection %s is not an authored prosecheck input", input)
		}
		data, readErr := os.ReadFile(input)
		if readErr != nil {
			return fmt.Errorf("read prose input %s: %w", input, readErr)
		}
		if isCLIManifestInput(input) {
			adapterFindings = append(adapterFindings, AnalyzeCLIManifest(input, data, policy)...)
			continue
		}
		if isMarkdownInput(input) {
			markdownSpans, parseErr := ExtractMarkdownSpans(input, data)
			if parseErr != nil {
				adapterFindings = append(adapterFindings, markdownParseFinding(input, data, parseErr.Error()))
				continue
			}
			spans = append(spans, markdownSpans...)
			continue
		}
		spans = append(spans, Span{
			SourcePath:  input,
			StartLine:   1,
			StartColumn: 1,
			Class:       cfg.class,
			Text:        string(data),
			Surfaces:    []string{surfaceCustomerDocumentation},
			Protected:   LexicalProtectedRanges(string(data)),
		})
	}

	findings := append(adapterFindings, Analyze(spans, policy)...)
	findings = SortFindings(findings)
	if len(findings) == 0 {
		_, err = fmt.Fprintf(stdout, "prosecheck passed (%d input(s))\n", len(cfg.inputs))
		return err
	}
	if err = RenderFindings(stderr, findings); err != nil {
		return fmt.Errorf("render findings: %w", err)
	}
	return fmt.Errorf("prosecheck found %d blocking finding(s)", len(findings))
}

func isMarkdownInput(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".md" || extension == ".markdown"
}

func isCLIManifestInput(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(filepath.Clean(path), "\\", "/"))
	return normalized == strings.TrimPrefix(authoredCLIManifestSuffix, "/") || strings.HasSuffix(normalized, authoredCLIManifestSuffix)
}

func isGeneratedCLIProjection(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(filepath.Clean(path), "\\", "/"))
	const generatedPath = "packages/api/generated/cli/commands.json"
	return normalized == generatedPath || strings.HasSuffix(normalized, "/"+generatedPath)
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("prosecheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	standardPath := flags.String("standard", defaultStandardPath, "path to the canonical writing standard")
	termsPath := flags.String("terms", defaultTermsPath, "path to the canonical terminology register")
	class := flags.String("class", string(ContentClassDescriptive), "content class: label, procedural, or descriptive")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() == 0 {
		return config{}, errors.New("usage: prosecheck [flags] <text-file> [...]")
	}
	parsedClass := ContentClass(strings.TrimSpace(*class))
	switch parsedClass {
	case ContentClassLabel, ContentClassProcedural, ContentClassDescriptive:
	default:
		return config{}, fmt.Errorf("unsupported prose content class %q", *class)
	}
	return config{
		standardPath: *standardPath,
		termsPath:    *termsPath,
		class:        parsedClass,
		inputs:       append([]string(nil), flags.Args()...),
	}, nil
}
