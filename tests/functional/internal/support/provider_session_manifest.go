package support

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ProviderSessionGoldenManifestSchemaVersion is the only supported manifest schema.
	ProviderSessionGoldenManifestSchemaVersion = 1
	// ProviderSessionGoldenSanitizerVersion is the only supported sanitizer contract version.
	ProviderSessionGoldenSanitizerVersion = 1
	// ProviderSessionGoldenManifestFile is the required case manifest filename.
	ProviderSessionGoldenManifestFile = "manifest.json"
)

// ProviderSessionFidelityClass values allowed by the provider-session golden contract.
const (
	ProviderSessionFidelityFullStream    = "full-stream"
	ProviderSessionFidelityPartialStream = "partial-stream"
	ProviderSessionFidelitySnapshotOnly  = "snapshot-only"
	ProviderSessionFidelityFinalOnly     = "final-only"
)

var providerSessionAllowedFidelityClasses = map[string]struct{}{
	ProviderSessionFidelityFullStream:    {},
	ProviderSessionFidelityPartialStream: {},
	ProviderSessionFidelitySnapshotOnly:  {},
	ProviderSessionFidelityFinalOnly:     {},
}

// ProviderSessionGoldenManifest is the schema for a provider-session golden case.
type ProviderSessionGoldenManifest struct {
	SchemaVersion                 int      `json:"schemaVersion"`
	ID                            string   `json:"id"`
	Provider                      string   `json:"provider"`
	ProviderVersion               string   `json:"providerVersion"`
	FidelityClass                 string   `json:"fidelityClass"`
	Case                          string   `json:"case"`
	StdoutFile                    string   `json:"stdoutFile"`
	StderrFile                    string   `json:"stderrFile"`
	RequestFile                   string   `json:"requestFile"`
	ProcessFile                   string   `json:"processFile"`
	ExpectedProviderSessionFile   string   `json:"expectedProviderSessionFile"`
	ExpectedResponseEventsFile    string   `json:"expectedResponseEventsFile"`
	ExpectedInvocationResultFile  string   `json:"expectedInvocationResultFile"`
	NormalizedFields              []string `json:"normalizedFields"`
	SanitizerVersion              int      `json:"sanitizerVersion"`
	Source                        string   `json:"source"`
}

// ProviderSessionManifestPaths holds case-relative and absolute resolved fixture paths.
type ProviderSessionManifestPaths struct {
	Request                  string
	Process                  string
	Stdout                   string
	Stderr                   string
	ExpectedProviderSession  string
	ExpectedResponseEvents   string
	ExpectedInvocationResult string
}

// ProviderSessionManifestError names the case and failing field or rule.
type ProviderSessionManifestError struct {
	CaseID string
	Field  string
	Rule   string
	Detail string
}

func (e *ProviderSessionManifestError) Error() string {
	caseID := e.CaseID
	if caseID == "" {
		caseID = "(unknown)"
	}
	switch {
	case e.Field != "" && e.Rule != "":
		return fmt.Sprintf("provider-session golden manifest case %q field %q failed rule %q: %s", caseID, e.Field, e.Rule, e.Detail)
	case e.Field != "":
		return fmt.Sprintf("provider-session golden manifest case %q field %q: %s", caseID, e.Field, e.Detail)
	case e.Rule != "":
		return fmt.Sprintf("provider-session golden manifest case %q failed rule %q: %s", caseID, e.Rule, e.Detail)
	default:
		return fmt.Sprintf("provider-session golden manifest case %q: %s", caseID, e.Detail)
	}
}

// LoadProviderSessionCaseManifest loads and validates manifest.json under caseDir,
// then resolves declared relative fixture file pointers within that case directory.
func LoadProviderSessionCaseManifest(caseDir string) (ProviderSessionGoldenManifest, ProviderSessionManifestPaths, error) {
	caseDir = filepath.Clean(strings.TrimSpace(caseDir))
	if caseDir == "" || caseDir == "." {
		return ProviderSessionGoldenManifest{}, ProviderSessionManifestPaths{}, &ProviderSessionManifestError{
			Rule:   "case-dir",
			Detail: "case directory is required",
		}
	}

	manifestPath := filepath.Join(caseDir, ProviderSessionGoldenManifestFile)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ProviderSessionGoldenManifest{}, ProviderSessionManifestPaths{}, &ProviderSessionManifestError{
			Rule:   "manifest.json",
			Detail: fmt.Sprintf("read %s: %v", manifestPath, err),
		}
	}

	var manifest ProviderSessionGoldenManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ProviderSessionGoldenManifest{}, ProviderSessionManifestPaths{}, &ProviderSessionManifestError{
			Rule:   "manifest.json",
			Detail: fmt.Sprintf("decode %s: %v", ProviderSessionGoldenManifestFile, err),
		}
	}

	if err := ValidateProviderSessionGoldenManifest(manifest); err != nil {
		return ProviderSessionGoldenManifest{}, ProviderSessionManifestPaths{}, err
	}

	paths, err := ResolveProviderSessionManifestPaths(caseDir, manifest)
	if err != nil {
		return ProviderSessionGoldenManifest{}, ProviderSessionManifestPaths{}, err
	}
	return manifest, paths, nil
}

// ValidateProviderSessionGoldenManifest checks schema version 1 identity, fidelity,
// file-pointer, sanitizer, source, and normalizedFields contract fields.
func ValidateProviderSessionGoldenManifest(manifest ProviderSessionGoldenManifest) error {
	caseID := strings.TrimSpace(manifest.ID)

	if manifest.SchemaVersion != ProviderSessionGoldenManifestSchemaVersion {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  "schemaVersion",
			Rule:   "schema-version",
			Detail: fmt.Sprintf("got %d, want %d", manifest.SchemaVersion, ProviderSessionGoldenManifestSchemaVersion),
		}
	}
	if caseID == "" {
		return &ProviderSessionManifestError{
			Field:  "id",
			Rule:   "required",
			Detail: "must be a non-empty string",
		}
	}
	if strings.TrimSpace(manifest.Provider) == "" {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  "provider",
			Rule:   "required",
			Detail: "must be a non-empty string",
		}
	}
	if strings.TrimSpace(manifest.ProviderVersion) == "" {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  "providerVersion",
			Rule:   "required",
			Detail: "must be a non-empty string",
		}
	}
	if strings.TrimSpace(manifest.Case) == "" {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  "case",
			Rule:   "required",
			Detail: "must be a non-empty string",
		}
	}
	if _, ok := providerSessionAllowedFidelityClasses[manifest.FidelityClass]; !ok {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  "fidelityClass",
			Rule:   "allowed-values",
			Detail: fmt.Sprintf(
				"got %q, want one of %s, %s, %s, %s",
				manifest.FidelityClass,
				ProviderSessionFidelityFullStream,
				ProviderSessionFidelityPartialStream,
				ProviderSessionFidelitySnapshotOnly,
				ProviderSessionFidelityFinalOnly,
			),
		}
	}
	if manifest.SanitizerVersion != ProviderSessionGoldenSanitizerVersion {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  "sanitizerVersion",
			Rule:   "sanitizer-version",
			Detail: fmt.Sprintf("got %d, want %d", manifest.SanitizerVersion, ProviderSessionGoldenSanitizerVersion),
		}
	}
	if strings.TrimSpace(manifest.Source) == "" {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  "source",
			Rule:   "required",
			Detail: "must be a non-empty string",
		}
	}
	if manifest.NormalizedFields == nil {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  "normalizedFields",
			Rule:   "required",
			Detail: "must be present (use an empty array when no fields are normalized)",
		}
	}
	for i, field := range manifest.NormalizedFields {
		if strings.TrimSpace(field) == "" {
			return &ProviderSessionManifestError{
				CaseID: caseID,
				Field:  "normalizedFields",
				Rule:   "non-empty-entries",
				Detail: fmt.Sprintf("entry %d must be a non-empty string", i),
			}
		}
	}

	for _, pointer := range providerSessionFilePointers(manifest) {
		if err := validateRelativeFixturePointer(caseID, pointer.field, pointer.value); err != nil {
			return err
		}
	}
	return nil
}

// ResolveProviderSessionManifestPaths joins and verifies each declared relative
// fixture pointer under caseDir without leaving the case directory.
func ResolveProviderSessionManifestPaths(caseDir string, manifest ProviderSessionGoldenManifest) (ProviderSessionManifestPaths, error) {
	caseID := strings.TrimSpace(manifest.ID)
	caseDir = filepath.Clean(caseDir)

	resolve := func(field, relative string) (string, error) {
		role := providerSessionRoleForManifestField(field)
		abs, err := resolveRelativeFixturePath(caseDir, relative)
		if err != nil {
			return "", &ProviderSessionManifestError{
				CaseID: caseID,
				Field:  field,
				Rule:   "relative-path",
				Detail: err.Error(),
			}
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return "", &ProviderSessionLoadError{
					CaseID: caseID,
					Role:   role,
					Path:   abs,
					Detail: fmt.Sprintf("required %s fixture is missing", role),
				}
			}
			return "", &ProviderSessionManifestError{
				CaseID: caseID,
				Field:  field,
				Rule:   "file-resolve",
				Detail: fmt.Sprintf("resolve %q under %s: %v", relative, caseDir, err),
			}
		}
		if info.IsDir() {
			return "", &ProviderSessionManifestError{
				CaseID: caseID,
				Field:  field,
				Rule:   "file-resolve",
				Detail: fmt.Sprintf("%q resolves to a directory, want a fixture file", relative),
			}
		}
		return abs, nil
	}

	var (
		paths ProviderSessionManifestPaths
		err   error
	)
	if paths.Request, err = resolve("requestFile", manifest.RequestFile); err != nil {
		return ProviderSessionManifestPaths{}, err
	}
	if paths.Process, err = resolve("processFile", manifest.ProcessFile); err != nil {
		return ProviderSessionManifestPaths{}, err
	}
	if paths.Stdout, err = resolve("stdoutFile", manifest.StdoutFile); err != nil {
		return ProviderSessionManifestPaths{}, err
	}
	if paths.Stderr, err = resolve("stderrFile", manifest.StderrFile); err != nil {
		return ProviderSessionManifestPaths{}, err
	}
	if paths.ExpectedProviderSession, err = resolve("expectedProviderSessionFile", manifest.ExpectedProviderSessionFile); err != nil {
		return ProviderSessionManifestPaths{}, err
	}
	if paths.ExpectedResponseEvents, err = resolve("expectedResponseEventsFile", manifest.ExpectedResponseEventsFile); err != nil {
		return ProviderSessionManifestPaths{}, err
	}
	if paths.ExpectedInvocationResult, err = resolve("expectedInvocationResultFile", manifest.ExpectedInvocationResultFile); err != nil {
		return ProviderSessionManifestPaths{}, err
	}
	return paths, nil
}

type providerSessionFilePointer struct {
	field string
	value string
}

func providerSessionFilePointers(manifest ProviderSessionGoldenManifest) []providerSessionFilePointer {
	return []providerSessionFilePointer{
		{field: "requestFile", value: manifest.RequestFile},
		{field: "processFile", value: manifest.ProcessFile},
		{field: "stdoutFile", value: manifest.StdoutFile},
		{field: "stderrFile", value: manifest.StderrFile},
		{field: "expectedProviderSessionFile", value: manifest.ExpectedProviderSessionFile},
		{field: "expectedResponseEventsFile", value: manifest.ExpectedResponseEventsFile},
		{field: "expectedInvocationResultFile", value: manifest.ExpectedInvocationResultFile},
	}
}

func validateRelativeFixturePointer(caseID, field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  field,
			Rule:   "required",
			Detail: "must be a non-empty relative fixture path",
		}
	}
	if trimmed != value {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  field,
			Rule:   "relative-path",
			Detail: "must not include leading or trailing whitespace",
		}
	}
	if filepath.IsAbs(filepath.FromSlash(value)) || filepath.IsAbs(value) {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  field,
			Rule:   "relative-path",
			Detail: fmt.Sprintf("%q must be a relative path", value),
		}
	}
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return &ProviderSessionManifestError{
			CaseID: caseID,
			Field:  field,
			Rule:   "relative-path",
			Detail: fmt.Sprintf("%q must stay within the case directory", value),
		}
	}
	return nil
}

func resolveRelativeFixturePath(caseDir, relative string) (string, error) {
	if err := validateRelativeFixturePointer("", "path", relative); err != nil {
		if manifestErr, ok := err.(*ProviderSessionManifestError); ok {
			return "", fmt.Errorf("%s", manifestErr.Detail)
		}
		return "", err
	}
	joined := filepath.Clean(filepath.Join(caseDir, filepath.FromSlash(relative)))
	casePrefix := caseDir + string(filepath.Separator)
	if joined != caseDir && !strings.HasPrefix(joined, casePrefix) {
		return "", fmt.Errorf("%q escapes case directory %s", relative, caseDir)
	}
	return joined, nil
}

// providerSessionRoleForManifestField maps manifest file-pointer fields to the
// fixture roles used in missing-file and load diagnostics.
func providerSessionRoleForManifestField(field string) string {
	switch field {
	case "requestFile":
		return "request"
	case "processFile":
		return "process"
	case "stdoutFile":
		return "stdout"
	case "stderrFile":
		return "stderr"
	case "expectedProviderSessionFile":
		return "expected-provider-session"
	case "expectedResponseEventsFile":
		return "expected-response-events"
	case "expectedInvocationResultFile":
		return "expected-invocation-result"
	default:
		return field
	}
}
