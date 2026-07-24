package sharedsurfaceownership

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const schemaID = "https://schemas.portpowered.com/you/architecture/packaged-service-structure/shared-surface-ownership.schema.json"

var allowedLanes = map[string]struct{}{
	"PSS-I02": {},
	"PSS-I03": {},
	"PSS-I04": {},
}

// ValidateDocument schema-validates and semantically checks one inventory document.
func ValidateDocument(document string, payload []byte) []Diagnostic {
	root, err := decodeObject(payload)
	if err != nil {
		return []Diagnostic{newDiagnostic(
			"inventory.decode",
			"/",
			fmt.Sprintf("document is not a JSON object: %v", err),
			document,
		)}
	}

	schemaDiagnostics := validateAgainstSchema(document, payload)
	if len(schemaDiagnostics) != 0 {
		return append(schemaDiagnostics, semanticDiagnostics(document, root)...)
	}
	return semanticDiagnostics(document, root)
}

func validateAgainstSchema(document string, payload []byte) []Diagnostic {
	schema, err := compileSchema()
	if err != nil {
		return []Diagnostic{newDiagnostic(
			"inventory.schema_compile",
			"/",
			fmt.Sprintf("compile shared-surface ownership schema: %v", err),
			document,
		)}
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return []Diagnostic{newDiagnostic(
			"inventory.decode",
			"/",
			fmt.Sprintf("decode JSON for schema validation: %v", err),
			document,
		)}
	}
	err = schema.Validate(instance)
	if err == nil {
		return nil
	}
	var validationErr *jsonschema.ValidationError
	if !asValidationError(err, &validationErr) {
		return []Diagnostic{newDiagnostic(
			"inventory.schema",
			"/",
			err.Error(),
			document,
		)}
	}
	return schemaDiagnosticsFromError(document, validationErr)
}

func compileSchema() (*jsonschema.Schema, error) {
	payload, err := os.ReadFile(resolveRepoPath(CanonicalSchemaRelPath))
	if err != nil {
		return nil, err
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(schemaID, document); err != nil {
		return nil, err
	}
	return compiler.Compile(schemaID)
}

func semanticDiagnostics(document string, root map[string]any) []Diagnostic {
	var diagnostics []Diagnostic

	if concurrent, ok := root["concurrentWriteLeasesAllowed"]; ok {
		if concurrent != false {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.concurrent_write_leases",
				"/concurrentWriteLeasesAllowed",
				"concurrent write leases on the same shared surface are forbidden",
				document,
			))
		}
	}
	for _, key := range []string{
		"integrationMetadataOnly",
		"authorizesPackageMoves",
		"authorizesPublicContractChanges",
		"authorizesTransportCutovers",
	} {
		value, ok := root[key]
		if !ok {
			continue
		}
		want := key == "integrationMetadataOnly"
		if value != want {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.metadata_only",
				"/"+key,
				fmt.Sprintf("%s must be %v; the model is integration metadata only and does not authorize package moves, public contract changes, or transport cutovers", key, want),
				document,
			))
		}
	}

	surfaces, _ := root["surfaces"].(map[string]any)
	keys := sortedKeys(surfaces)
	for _, key := range keys {
		surface, ok := surfaces[key].(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.surface_type",
				"/surfaces/"+escapeJSONPointerToken(key),
				"surface record must be an object",
				document,
			))
			continue
		}
		diagnostics = append(diagnostics, surfaceDiagnostics(document, key, surface)...)
	}
	return diagnostics
}

func surfaceDiagnostics(document, key string, surface map[string]any) []Diagnostic {
	base := "/surfaces/" + escapeJSONPointerToken(key)
	var diagnostics []Diagnostic

	surfaceID, _ := surface["surfaceId"].(string)
	if surfaceID != "" && surfaceID != key {
		diagnostics = append(diagnostics, newDiagnostic(
			"inventory.surface_id_mismatch",
			base+"/surfaceId",
			fmt.Sprintf("surfaceId %s must match surfaces key %s", strconv.Quote(surfaceID), strconv.Quote(key)),
			document,
		))
	}

	lane, _ := surface["serialIntegratorLaneId"].(string)
	if strings.TrimSpace(lane) == "" {
		diagnostics = append(diagnostics, newDiagnostic(
			"inventory.empty_serial_integrator",
			base+"/serialIntegratorLaneId",
			"serial integrator lane must be non-empty; each surface requires exactly one serial integrator",
			document,
		))
	} else if _, ok := allowedLanes[lane]; !ok {
		diagnostics = append(diagnostics, newDiagnostic(
			"inventory.unknown_serial_integrator",
			base+"/serialIntegratorLaneId",
			fmt.Sprintf("serial integrator lane %s is not one of PSS-I02, PSS-I03, or PSS-I04", strconv.Quote(lane)),
			document,
		))
	}

	integratorKeys := make([]string, 0, 2)
	for field := range surface {
		lower := strings.ToLower(field)
		if strings.Contains(lower, "integrator") && field != "serialIntegratorLaneId" {
			integratorKeys = append(integratorKeys, field)
		}
	}
	sort.Strings(integratorKeys)
	if len(integratorKeys) != 0 {
		diagnostics = append(diagnostics, newDiagnostic(
			"inventory.dual_integrators",
			base+"/"+integratorKeys[0],
			fmt.Sprintf("surface declares additional integrator field %s; each surface requires exactly one serial integrator", strconv.Quote(integratorKeys[0])),
			document,
		))
	}

	queue, _ := surface["ownerRequestQueue"].([]any)
	seenRequestIDs := make(map[string]int, len(queue))
	for index, item := range queue {
		entry, ok := item.(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.owner_request_type",
				fmt.Sprintf("%s/ownerRequestQueue/%d", base, index),
				"owner-request queue entry must be an object",
				document,
			))
			continue
		}
		requestID, _ := entry["requestId"].(string)
		if requestID == "" {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.owner_request_id",
				fmt.Sprintf("%s/ownerRequestQueue/%d/requestId", base, index),
				"owner-request queue entry requires a non-empty requestId",
				document,
			))
		} else if previous, exists := seenRequestIDs[requestID]; exists {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.duplicate_owner_request",
				fmt.Sprintf("%s/ownerRequestQueue/%d/requestId", base, index),
				fmt.Sprintf("duplicate owner-request requestId %s also appears at queue index %d", strconv.Quote(requestID), previous),
				document,
			))
		} else {
			seenRequestIDs[requestID] = index
		}

		position, ok := intValue(entry["queuePosition"])
		if !ok {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.owner_request_position",
				fmt.Sprintf("%s/ownerRequestQueue/%d/queuePosition", base, index),
				"owner-request queue entry requires an integer queuePosition",
				document,
			))
			continue
		}
		if position != index {
			diagnostics = append(diagnostics, newDiagnostic(
				"inventory.unordered_owner_request_queue",
				fmt.Sprintf("%s/ownerRequestQueue/%d/queuePosition", base, index),
				fmt.Sprintf("owner-request queue must be deterministic and contiguous; queuePosition %d at index %d is unordered", position, index),
				document,
			))
		}
	}

	return diagnostics
}

func decodeObject(payload []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	return root, nil
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		if typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escapeJSONPointerToken(token string) string {
	replacer := strings.NewReplacer("~", "~0", "/", "~1")
	return replacer.Replace(token)
}

func newDiagnostic(rule, path, message, document string) Diagnostic {
	return Diagnostic{
		Rule:     rule,
		Path:     path,
		Message:  message,
		Document: document,
	}
}

func resolveRepoPath(rel string) string {
	root, err := findRepoRoot()
	if err != nil {
		return filepath.FromSlash(rel)
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	current := filepath.Clean(cwd)
	for {
		goModPath := filepath.Join(current, "go.mod")
		if info, statErr := os.Stat(goModPath); statErr == nil && !info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found from %s", cwd)
		}
		current = parent
	}
}

func asValidationError(err error, target **jsonschema.ValidationError) bool {
	for err != nil {
		if typed, ok := err.(*jsonschema.ValidationError); ok {
			*target = typed
			return true
		}
		type unwrapper interface{ Unwrap() error }
		wrapped, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = wrapped.Unwrap()
	}
	return false
}

func schemaDiagnosticsFromError(document string, err *jsonschema.ValidationError) []Diagnostic {
	var diagnostics []Diagnostic
	collectSchemaDiagnostics(document, err, &diagnostics)
	return diagnostics
}

func collectSchemaDiagnostics(document string, err *jsonschema.ValidationError, diagnostics *[]Diagnostic) {
	if len(err.Causes) == 0 {
		path := jsonPointer(err.InstanceLocation)
		if path == "" {
			path = "/"
		}
		message := err.Error()
		rule := "inventory.schema"
		lower := strings.ToLower(message + " " + path)
		switch {
		case strings.Contains(path, "serialIntegratorLaneId"):
			rule = "inventory.empty_serial_integrator"
			message = "serial integrator lane must be a non-empty PSS-I02, PSS-I03, or PSS-I04 value"
		case (strings.Contains(lower, "additionalproperties") || strings.Contains(lower, "additional properties")) &&
			strings.Contains(lower, "integrator"):
			rule = "inventory.dual_integrators"
			message = "surface declares an additional integrator field; each surface requires exactly one serial integrator"
		}
		*diagnostics = append(*diagnostics, newDiagnostic(rule, path, message, document))
		return
	}
	for _, cause := range err.Causes {
		collectSchemaDiagnostics(document, cause, diagnostics)
	}
}

func jsonPointer(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = escapeJSONPointerToken(segment)
	}
	return "/" + strings.Join(escaped, "/")
}
