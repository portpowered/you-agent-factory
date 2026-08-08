package acpbaseline

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// CapabilityMatrix is what one agent was observed to do, in a form that can be
// compared against any other agent -- including our own.
type CapabilityMatrix struct {
	SchemaVersion         int            `json:"schemaVersion"`
	Agent                 string         `json:"agent"`
	CapturedAtUTC         string         `json:"capturedAtUtc"`
	Scenarios             []string       `json:"scenarios"`
	AgentMethodsAccepted  []string       `json:"agentMethodsAccepted"`
	AgentMethodsRejected  map[string]int `json:"agentMethodsRejected"`
	ClientMethodsInvoked  []string       `json:"clientMethodsInvoked"`
	SessionUpdateVariants map[string]int `json:"sessionUpdateVariants"`
	AgentCapabilities     []string       `json:"agentCapabilities"`
	ConfigOptionCategory  string         `json:"configOptionCategory,omitempty"`
	ConfigOptionCount     int            `json:"configOptionCount"`
	PermissionChoices     []string       `json:"permissionChoices,omitempty"`
	UnknownClientMethods  []string       `json:"unknownClientMethods,omitempty"`
	// Caveats record conditions that make a row unsafe to read as a
	// capability difference. A capture environment without a model provider
	// produces no assistant text, which would otherwise look identical to an
	// agent that cannot produce assistant text at all.
	Caveats []string `json:"caveats,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

// BuildMatrix derives a matrix from one capture's manifest.
func BuildMatrix(manifest *Manifest) *CapabilityMatrix {
	matrix := &CapabilityMatrix{
		SchemaVersion:         1,
		Agent:                 manifest.Agent,
		CapturedAtUTC:         manifest.CapturedAtUTC,
		Scenarios:             manifest.Scenarios,
		AgentMethodsRejected:  manifest.Observation.AgentMethodsRejected,
		SessionUpdateVariants: manifest.Observation.SessionUpdateVariants,
		PermissionChoices:     manifest.Permissions,
		UnknownClientMethods:  dedupe(manifest.UnknownMethods),
	}
	matrix.AgentMethodsAccepted = sortedKeys(manifest.Observation.AgentMethodsAccepted)
	matrix.ClientMethodsInvoked = sortedKeys(manifest.Observation.ClientMethodsInvoked)
	matrix.AgentCapabilities = advertisedCapabilities(manifest.Observation.Results["initialize"])
	matrix.ConfigOptionCategory, matrix.ConfigOptionCount = configOptionShape(
		manifest.Observation.Results["session/new"])
	for _, notes := range manifest.StepNotes {
		matrix.Notes = append(matrix.Notes, notes...)
	}
	matrix.Notes = dedupe(matrix.Notes)
	matrix.Caveats = deriveCaveats(manifest)
	return matrix
}

// deriveCaveats flags capture conditions that would otherwise be misread as
// capability differences.
func deriveCaveats(manifest *Manifest) []string {
	var caveats []string
	prompts := manifest.Observation.AgentMethodsAccepted["session/prompt"]
	messages := manifest.Observation.SessionUpdateVariants["agent_message_chunk"]
	if prompts > 0 && messages == 0 {
		caveats = append(caveats, "prompts completed but produced no agent_message_chunk: "+
			"the capture environment most likely had no model provider configured, so "+
			"message-bearing rows are not evidence of a missing capability")
	}
	if len(manifest.Errors) > 0 {
		caveats = append(caveats, fmt.Sprintf("%d scenario(s) reported an error; see the manifest",
			len(manifest.Errors)))
	}
	return caveats
}

// advertisedCapabilities flattens the agent capability object into dotted
// paths, so two agents can be compared on presence rather than on shape.
func advertisedCapabilities(result json.RawMessage) []string {
	if len(result) == 0 {
		return nil
	}
	var response struct {
		AgentCapabilities map[string]any `json:"agentCapabilities"`
		AuthMethods       []any          `json:"authMethods"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return nil
	}
	paths := flatten("", response.AgentCapabilities)
	if len(response.AuthMethods) > 0 {
		paths = append(paths, fmt.Sprintf("authMethods[%d]", len(response.AuthMethods)))
	}
	sort.Strings(paths)
	return paths
}

func flatten(prefix string, value map[string]any) []string {
	var paths []string
	for key, nested := range value {
		if strings.HasPrefix(key, "_") {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		switch typed := nested.(type) {
		case map[string]any:
			if len(typed) == 0 {
				paths = append(paths, path)
				continue
			}
			paths = append(paths, flatten(path, typed)...)
		case bool:
			if typed {
				paths = append(paths, path)
			}
		default:
			paths = append(paths, path)
		}
	}
	return paths
}

// configOptionShape reports the category and option count of the first select
// configuration option. The verdict keys on existence and category, never on
// the exact ids: a model list is account-entitlement-scoped, so two operators
// legitimately see different ids.
func configOptionShape(result json.RawMessage) (string, int) {
	if len(result) == 0 {
		return "", 0
	}
	var response struct {
		ConfigOptions []struct {
			Category string `json:"category"`
			Options  []any  `json:"options"`
			Select   *struct {
				Category string `json:"category"`
				Options  []any  `json:"options"`
			} `json:"select"`
		} `json:"configOptions"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return "", 0
	}
	for _, option := range response.ConfigOptions {
		category, values := option.Category, option.Options
		if option.Select != nil {
			category, values = option.Select.Category, option.Select.Options
		}
		if len(values) > 0 {
			return category, len(values)
		}
	}
	return "", 0
}

func sortedKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

// LoadMatrix reads a matrix from disk.
func LoadMatrix(path string) (*CapabilityMatrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var matrix CapabilityMatrix
	if err := json.Unmarshal(data, &matrix); err != nil {
		return nil, err
	}
	return &matrix, nil
}
