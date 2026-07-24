package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProviderSessionProcessMetadata records sanitized execution facts needed for
// golden normalization. It must not carry a complete host environment dump.
type ProviderSessionProcessMetadata struct {
	Argv                 []string `json:"argv"`
	Provider             string   `json:"provider"`
	Model                string   `json:"model"`
	ExitCode             *int     `json:"exitCode,omitempty"`
	Signal               string   `json:"signal,omitempty"`
	StdoutStream         bool     `json:"stdoutStream"`
	StderrStream         bool     `json:"stderrStream"`
	WorkingDirectoryRole string   `json:"workingDirectoryRole"`
	TimeoutCancelClass   string   `json:"timeoutCancelClass"`
	TerminalErrorClass   string   `json:"terminalErrorClass"`
}

// ProviderSessionStdoutMediaType describes how stdout was decoded.
type ProviderSessionStdoutMediaType string

const (
	ProviderSessionStdoutJSONL ProviderSessionStdoutMediaType = "jsonl"
	ProviderSessionStdoutJSON  ProviderSessionStdoutMediaType = "json"
	ProviderSessionStdoutText  ProviderSessionStdoutMediaType = "text"
)

// ProviderSessionStdoutArtifact holds raw stdout and its decoded form.
type ProviderSessionStdoutArtifact struct {
	MediaType ProviderSessionStdoutMediaType
	Raw       []byte
	JSON      json.RawMessage
	Records   []json.RawMessage
	Text      string
}

// ProviderSessionExpectedGoldens holds the three expected public metadata artifacts.
type ProviderSessionExpectedGoldens struct {
	ProviderSession   json.RawMessage
	ResponseEvents   []json.RawMessage
	InvocationResult json.RawMessage
}

// ProviderSessionCase is a fully loaded, sanitized provider-session golden case.
type ProviderSessionCase struct {
	CaseDir  string
	Manifest ProviderSessionGoldenManifest
	Paths    ProviderSessionManifestPaths
	Request  json.RawMessage
	Process  ProviderSessionProcessMetadata
	Stdout   ProviderSessionStdoutArtifact
	Stderr   string
	Expected ProviderSessionExpectedGoldens
}

// ProviderSessionLoadError names the case, fixture role, and path for load failures.
type ProviderSessionLoadError struct {
	CaseID string
	Role   string
	Path   string
	Field  string
	Detail string
}

func (e *ProviderSessionLoadError) Error() string {
	caseID := e.CaseID
	if caseID == "" {
		caseID = "(unknown)"
	}
	parts := []string{fmt.Sprintf("provider-session golden load case %q", caseID)}
	if e.Role != "" {
		parts = append(parts, fmt.Sprintf("role %q", e.Role))
	}
	if e.Path != "" {
		parts = append(parts, fmt.Sprintf("path %s", e.Path))
	}
	if e.Field != "" {
		parts = append(parts, fmt.Sprintf("field %q", e.Field))
	}
	return strings.Join(parts, " ") + ": " + e.Detail
}

// LoadProviderSessionCase loads and validates a provider-session golden case:
// manifest → sanitization → request/process/stdout/stderr → expected goldens.
func LoadProviderSessionCase(caseDir string) (ProviderSessionCase, error) {
	manifest, paths, err := LoadProviderSessionCaseManifest(caseDir)
	if err != nil {
		return ProviderSessionCase{}, err
	}
	caseID := strings.TrimSpace(manifest.ID)

	if err := ValidateProviderSessionManifestSanitization(manifest); err != nil {
		return ProviderSessionCase{}, err
	}
	if err := ValidateProviderSessionCaseSanitization(caseID, paths); err != nil {
		return ProviderSessionCase{}, err
	}

	request, err := loadProviderSessionJSONArtifact(caseID, "request", paths.Request)
	if err != nil {
		return ProviderSessionCase{}, err
	}

	process, err := loadProviderSessionProcessMetadata(caseID, paths.Process)
	if err != nil {
		return ProviderSessionCase{}, err
	}

	stdout, err := loadProviderSessionStdoutArtifact(caseID, paths.Stdout)
	if err != nil {
		return ProviderSessionCase{}, err
	}

	stderrRaw, err := os.ReadFile(paths.Stderr)
	if err != nil {
		return ProviderSessionCase{}, &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "stderr",
			Path:   paths.Stderr,
			Detail: fmt.Sprintf("read stderr fixture: %v", err),
		}
	}

	expectedSession, err := loadProviderSessionJSONArtifact(caseID, "expected-provider-session", paths.ExpectedProviderSession)
	if err != nil {
		return ProviderSessionCase{}, err
	}
	expectedEvents, err := loadProviderSessionNDJSONArtifact(caseID, "expected-response-events", paths.ExpectedResponseEvents)
	if err != nil {
		return ProviderSessionCase{}, err
	}
	expectedResult, err := loadProviderSessionJSONArtifact(caseID, "expected-invocation-result", paths.ExpectedInvocationResult)
	if err != nil {
		return ProviderSessionCase{}, err
	}

	return ProviderSessionCase{
		CaseDir:  filepath.Clean(caseDir),
		Manifest: manifest,
		Paths:    paths,
		Request:  request,
		Process:  process,
		Stdout:   stdout,
		Stderr:   string(stderrRaw),
		Expected: ProviderSessionExpectedGoldens{
			ProviderSession:   expectedSession,
			ResponseEvents:   expectedEvents,
			InvocationResult: expectedResult,
		},
	}, nil
}

func loadProviderSessionProcessMetadata(caseID, path string) (ProviderSessionProcessMetadata, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ProviderSessionProcessMetadata{}, &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Detail: fmt.Sprintf("read process fixture: %v", err),
		}
	}

	var process ProviderSessionProcessMetadata
	if err := json.Unmarshal(raw, &process); err != nil {
		return ProviderSessionProcessMetadata{}, &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Detail: fmt.Sprintf("decode process.json: %v", err),
		}
	}
	if err := validateProviderSessionProcessMetadata(caseID, path, process); err != nil {
		return ProviderSessionProcessMetadata{}, err
	}
	return process, nil
}

func validateProviderSessionProcessMetadata(caseID, path string, process ProviderSessionProcessMetadata) error {
	if process.Argv == nil {
		return &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Field:  "argv",
			Detail: "must be present (use an empty array when argv is intentionally empty)",
		}
	}
	if strings.TrimSpace(process.Provider) == "" {
		return &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Field:  "provider",
			Detail: "must be a non-empty string",
		}
	}
	if strings.TrimSpace(process.Model) == "" {
		return &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Field:  "model",
			Detail: "must be a non-empty string",
		}
	}
	if strings.TrimSpace(process.WorkingDirectoryRole) == "" {
		return &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Field:  "workingDirectoryRole",
			Detail: "must be a non-empty role (never an absolute host path)",
		}
	}
	if strings.TrimSpace(process.TimeoutCancelClass) == "" {
		return &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Field:  "timeoutCancelClass",
			Detail: "must be a non-empty classification (use \"none\" when not timed out or canceled)",
		}
	}
	if strings.TrimSpace(process.TerminalErrorClass) == "" {
		return &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Field:  "terminalErrorClass",
			Detail: "must be a non-empty classification (use \"none\" for successful terminals)",
		}
	}
	if process.ExitCode == nil && strings.TrimSpace(process.Signal) == "" {
		return &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "process",
			Path:   path,
			Field:  "exitCode",
			Detail: "must set exitCode and/or signal",
		}
	}
	return nil
}

func loadProviderSessionStdoutArtifact(caseID, path string) (ProviderSessionStdoutArtifact, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ProviderSessionStdoutArtifact{}, &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   "stdout",
			Path:   path,
			Detail: fmt.Sprintf("read stdout fixture: %v", err),
		}
	}

	mediaType := providerSessionStdoutMediaType(path)
	artifact := ProviderSessionStdoutArtifact{
		MediaType: mediaType,
		Raw:       append([]byte(nil), raw...),
	}

	switch mediaType {
	case ProviderSessionStdoutJSONL:
		records, err := decodeProviderSessionNDJSON(caseID, "stdout", path, raw)
		if err != nil {
			return ProviderSessionStdoutArtifact{}, err
		}
		artifact.Records = records
	case ProviderSessionStdoutJSON:
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 {
			return ProviderSessionStdoutArtifact{}, &ProviderSessionLoadError{
				CaseID: caseID,
				Role:   "stdout",
				Path:   path,
				Detail: "stdout.json must contain a JSON document",
			}
		}
		if !json.Valid(trimmed) {
			return ProviderSessionStdoutArtifact{}, &ProviderSessionLoadError{
				CaseID: caseID,
				Role:   "stdout",
				Path:   path,
				Detail: "stdout.json is not valid JSON",
			}
		}
		artifact.JSON = append(json.RawMessage(nil), trimmed...)
	default:
		artifact.Text = string(raw)
	}
	return artifact, nil
}

func providerSessionStdoutMediaType(path string) ProviderSessionStdoutMediaType {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case strings.HasSuffix(base, ".jsonl"), strings.HasSuffix(base, ".ndjson"):
		return ProviderSessionStdoutJSONL
	case strings.HasSuffix(base, ".json"):
		return ProviderSessionStdoutJSON
	default:
		return ProviderSessionStdoutText
	}
}

func loadProviderSessionJSONArtifact(caseID, role, path string) (json.RawMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   role,
			Path:   path,
			Detail: fmt.Sprintf("read %s fixture: %v", role, err),
		}
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   role,
			Path:   path,
			Detail: fmt.Sprintf("%s fixture must contain a JSON document", role),
		}
	}
	if !json.Valid(trimmed) {
		return nil, &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   role,
			Path:   path,
			Detail: fmt.Sprintf("%s fixture is not valid JSON", role),
		}
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func loadProviderSessionNDJSONArtifact(caseID, role, path string) ([]json.RawMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &ProviderSessionLoadError{
			CaseID: caseID,
			Role:   role,
			Path:   path,
			Detail: fmt.Sprintf("read %s fixture: %v", role, err),
		}
	}
	return decodeProviderSessionNDJSON(caseID, role, path, raw)
}

func decodeProviderSessionNDJSON(caseID, role, path string, raw []byte) ([]json.RawMessage, error) {
	text := string(raw)
	if strings.TrimSpace(text) == "" {
		return []json.RawMessage{}, nil
	}

	records := make([]json.RawMessage, 0)
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !json.Valid([]byte(trimmed)) {
			return nil, &ProviderSessionLoadError{
				CaseID: caseID,
				Role:   role,
				Path:   path,
				Field:  fmt.Sprintf("line[%d]", i+1),
				Detail: "NDJSON line is not valid JSON",
			}
		}
		records = append(records, append(json.RawMessage(nil), trimmed...))
	}
	return records, nil
}
