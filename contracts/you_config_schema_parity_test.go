package contracts_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operator_settings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type schemaParityKind string

const (
	schemaParityMatchesLoader schemaParityKind = "matches-loader"
	schemaParityNotApplicable schemaParityKind = "not-applicable"
	schemaParityDiverges      schemaParityKind = "diverges"
)

type youConfigSchemaParityRule struct {
	kind         schemaParityKind
	schemaAccept *bool
	reason       string
}

var youConfigSchemaParityOverrides = map[string]youConfigSchemaParityRule{
	"operator_settings:valid-worker-presets-canonicalized": {
		kind:         schemaParityDiverges,
		schemaAccept: boolPtr(false),
		reason:       "operator_settings trims and canonicalizes preset values that Draft 2020-12 cannot express",
	},
	"operator_settings:invalid-preset-duplicate-id": {
		kind:         schemaParityDiverges,
		schemaAccept: boolPtr(true),
		reason:       "duplicate workerPresets[].id after trim is enforced only by operator_settings loaders",
	},
	"operator_settings:invalid-malformed-json": {
		kind:   schemaParityNotApplicable,
		reason: "malformed JSON is rejected before schema instance validation",
	},
	"operator_settings:invalid-trailing-json": {
		kind:   schemaParityNotApplicable,
		reason: "trailing JSON values are rejected by operator_settings decode before schema validation",
	},
	"operator_settings:invalid-load-malformed": {
		kind:   schemaParityNotApplicable,
		reason: "malformed on-disk JSON is rejected before schema instance validation",
	},
	"operator_settings_identity:valid-missing-file": {
		kind:   schemaParityNotApplicable,
		reason: "missing config file has no JSON document for schema validation",
	},
	"operator_settings_identity:invalid-whitespace-config": {
		kind:   schemaParityNotApplicable,
		reason: "whitespace-only config file content is not a JSON document for schema validation",
	},
	"operator_settings_identity:invalid-empty-config-path": {
		kind:   schemaParityNotApplicable,
		reason: "empty config path is rejected before file read and schema validation",
	},
	"operator_settings_identity:invalid-malformed-json": {
		kind:   schemaParityNotApplicable,
		reason: "malformed JSON is rejected before schema instance validation",
	},
	"operator_settings_identity:invalid-persist-empty-scope": {
		kind:   schemaParityNotApplicable,
		reason: "persistBackendScopeID validates scope IDs, not JSON documents",
	},
	"operator_settings_identity:invalid-persist-non-local-scope": {
		kind:   schemaParityNotApplicable,
		reason: "persistBackendScopeID validates scope IDs, not JSON documents",
	},
	"operator_settings_identity:invalid-persist-provider-scope": {
		kind:   schemaParityNotApplicable,
		reason: "persistBackendScopeID validates scope IDs, not JSON documents",
	},
}

func TestYouConfigSchemaParityMatrixCoversAllIndexedCases(t *testing.T) {
	for _, inputCase := range operator_settings.ProjectInputInventory().Cases {
		assertParityRuleRegistered(t, "operator_settings", inputCase.ID)
	}
	for _, inputCase := range committedIdentityInputInventory(t).Cases {
		assertParityRuleRegistered(t, "operator_settings_identity", inputCase.ID)
	}
}

func TestYouConfigSchemaLoaderParityMatrix(t *testing.T) {
	schema := youConfigSchema(t)

	t.Run("operator_settings", func(t *testing.T) {
		for _, inputCase := range operator_settings.ProjectInputInventory().Cases {
			t.Run(inputCase.ID, func(t *testing.T) {
				runOperatorConfigSchemaParityCase(t, schema, inputCase)
			})
		}
	})

	t.Run("operator_settings_identity", func(t *testing.T) {
		for _, inputCase := range committedIdentityInputInventory(t).Cases {
			t.Run(inputCase.ID, func(t *testing.T) {
				runSystemConfigSchemaParityCase(t, schema, inputCase)
			})
		}
	})
}

func TestYouConfigSchemaTopologyMatchesInventoryWithoutUnsupportedFields(t *testing.T) {
	document := readJSON(t, filepath.Join("config", "you-config.schema.json"))
	root := document.(map[string]any)
	contract := root["contract"].(map[string]any)
	fields := contract["fields"].(map[string]any)

	inventory := committedGlobalConfigInventory(t)
	inventoryIDs := make(map[string]struct{}, len(inventory.Fields))
	for _, record := range inventory.Fields {
		inventoryIDs[record.ID] = struct{}{}
		if _, ok := fields[record.ID]; !ok {
			t.Fatalf("schema contract missing inventoried field %q", record.ID)
		}
	}
	for id := range fields {
		if _, ok := inventoryIDs[id]; !ok {
			t.Fatalf("schema contract advertises unsupported field %q", id)
		}
	}
}

func runOperatorConfigSchemaParityCase(t *testing.T, schema *jsonschema.Schema, inputCase operator_settings.InputCase) {
	t.Helper()

	rule := parityRuleForCase("operator_settings", inputCase.ID, inputCase.Outcome, inputCase.Fixture != "")

	switch inputCase.Entrypoint {
	case "DecodeGlobalConfig":
		runOperatorParseSchemaParityCase(t, schema, inputCase, rule)
	case "LoadFileConfig":
		runOperatorLoadSchemaParityCase(t, schema, inputCase, rule)
	case "Resolve":
		assertResolveCaseSchemaNotApplicable(t, rule, inputCase.ID)
		runOperatorResolveParityCase(t, inputCase)
	default:
		t.Fatalf("unsupported entrypoint %q", inputCase.Entrypoint)
	}
}

func runSystemConfigSchemaParityCase(t *testing.T, schema *jsonschema.Schema, inputCase committedIdentityInputCase) {
	t.Helper()

	rule := parityRuleForCase("operator_settings_identity", inputCase.ID, inputCase.Outcome, inputCase.Fixture != "")

	switch inputCase.Entrypoint {
	case "EnsureLocalBackendScope":
		runSystemEnsureScopeSchemaParityCase(t, schema, inputCase, rule)
	case "persistBackendScopeID":
		assertSchemaParityNotApplicable(t, rule, inputCase.ID)
	default:
		t.Fatalf("unsupported entrypoint %q", inputCase.Entrypoint)
	}
}

func runOperatorParseSchemaParityCase(
	t *testing.T,
	schema *jsonschema.Schema,
	inputCase operator_settings.InputCase,
	rule youConfigSchemaParityRule,
) {
	t.Helper()

	data := readOperatorFixture(t, inputCase.Fixture)
	assertSchemaParityForFixture(t, schema, rule, data)

	cfg, err := globalconfigmapping.Decode(data)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("ParseFileConfig() error = %v, want accept", err)
		}
		assertOperatorFileConfigExpectation(t, cfg, inputCase.ExpectedFileConfig)
		return
	}
	if err == nil {
		t.Fatal("ParseFileConfig() error = nil, want reject")
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func runOperatorLoadSchemaParityCase(
	t *testing.T,
	schema *jsonschema.Schema,
	inputCase operator_settings.InputCase,
	rule youConfigSchemaParityRule,
) {
	t.Helper()

	if inputCase.Fixture == "" {
		assertSchemaParityNotApplicable(t, rule, inputCase.ID)
	} else {
		data := readOperatorFixture(t, inputCase.Fixture)
		assertSchemaParityForFixture(t, schema, rule, data)
	}

	var path string
	if inputCase.ID == "valid-missing-file" {
		path = filepath.Join(t.TempDir(), "missing-config.json")
	} else {
		path = writeOperatorFixtureToTemp(t, inputCase.Fixture)
	}

	cfg, err := operator_settings.LoadFileConfig(platformfilesystem.Local{}, globalconfigmapping.Decode, path)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("LoadFileConfig() error = %v, want accept", err)
		}
		assertOperatorFileConfigExpectation(t, cfg, inputCase.ExpectedFileConfig)
		return
	}
	if err == nil {
		t.Fatal("LoadFileConfig() error = nil, want reject")
	}
	if inputCase.ID == "invalid-load-malformed" && !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %q, want path %q", err.Error(), path)
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func runOperatorResolveParityCase(t *testing.T, inputCase operator_settings.InputCase) {
	t.Helper()

	if inputCase.ResolveLayers == nil {
		t.Fatal("resolve case missing resolveLayers")
	}

	layers := inputCase.ResolveLayers
	fileDefaults := operatorDefaultsFromLayers(t, layers)
	for key, value := range layers.Env {
		t.Setenv(key, value)
	}

	resolved, err := operator_settings.Resolve(operator_settings.ResolveInput{
		File: fileDefaults,
		Env: operator_settings.Defaults{
			WorkerModelProvider: strings.TrimSpace(os.Getenv(operator_settings.EnvDefaultWorkerModelProvider)),
			WorkerModel:         strings.TrimSpace(os.Getenv(operator_settings.EnvDefaultWorkerModel)),
		},
		Flag: operator_settings.Defaults{
			WorkerModelProvider: strings.TrimSpace(layers.Flag.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(layers.Flag.WorkerModel),
		},
	}, "/tmp/operator-config.json")

	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("Resolve() error = %v, want accept", err)
		}
		if inputCase.PrecedenceWinners != nil {
			if string(resolved.WorkerModelProviderSource) != inputCase.PrecedenceWinners.WorkerModelProviderSource {
				t.Fatalf("provider source = %q, want %q", resolved.WorkerModelProviderSource, inputCase.PrecedenceWinners.WorkerModelProviderSource)
			}
			if string(resolved.WorkerModelSource) != inputCase.PrecedenceWinners.WorkerModelSource {
				t.Fatalf("model source = %q, want %q", resolved.WorkerModelSource, inputCase.PrecedenceWinners.WorkerModelSource)
			}
		}
		if inputCase.ExpectedResolved != nil {
			if resolved.WorkerModelProvider != inputCase.ExpectedResolved.WorkerModelProvider {
				t.Fatalf("provider = %q, want %q", resolved.WorkerModelProvider, inputCase.ExpectedResolved.WorkerModelProvider)
			}
			if resolved.WorkerModel != inputCase.ExpectedResolved.WorkerModel {
				t.Fatalf("model = %q, want %q", resolved.WorkerModel, inputCase.ExpectedResolved.WorkerModel)
			}
		}
		return
	}
	if err == nil {
		t.Fatal("Resolve() error = nil, want reject")
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func runSystemEnsureScopeSchemaParityCase(
	t *testing.T,
	schema *jsonschema.Schema,
	inputCase committedIdentityInputCase,
	rule youConfigSchemaParityRule,
) {
	t.Helper()

	if inputCase.Fixture == "" {
		assertSchemaParityNotApplicable(t, rule, inputCase.ID)
	} else {
		data := readSystemFixture(t, inputCase.Fixture)
		assertSchemaParityForFixture(t, schema, rule, data)
	}

	var configPath string
	switch inputCase.ID {
	case "valid-missing-file":
		configPath = filepath.Join(t.TempDir(), "fixture-declared-missing-config.json")
	case "invalid-empty-config-path":
		configPath = "   "
	default:
		configPath = writeSystemFixtureToTemp(t, inputCase.Fixture)
	}

	resolved, err := operator_settings.EnsureLocalBackendScope(
		platformfilesystem.Local{},
		func(dir, pattern string) (operator_settings.TemporaryFile, error) { return os.CreateTemp(dir, pattern) },
		uuid.NewString,
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		configPath,
	)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("EnsureLocalBackendScope() error = %v, want accept", err)
		}
		assertSystemScopeExpectation(t, resolved, inputCase.ExpectedScope)
		return
	}
	if err == nil {
		t.Fatal("EnsureLocalBackendScope() error = nil, want reject")
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func assertSchemaParityForFixture(t *testing.T, schema *jsonschema.Schema, rule youConfigSchemaParityRule, data []byte) {
	t.Helper()

	switch rule.kind {
	case schemaParityNotApplicable:
		return
	case schemaParityDiverges:
		instance, ok := parseJSONDocument(data)
		if !ok {
			t.Fatalf("divergent case expected parseable JSON fixture")
		}
		gotAccept := schema.Validate(instance) == nil
		if rule.schemaAccept == nil {
			t.Fatal("divergent parity rule missing schemaAccept")
		}
		if gotAccept != *rule.schemaAccept {
			t.Fatalf("schema accept = %t, want %t (%s)", gotAccept, *rule.schemaAccept, rule.reason)
		}
		return
	case schemaParityMatchesLoader:
		instance, ok := parseJSONDocument(data)
		if !ok {
			t.Fatal("matches-loader parity rule requires parseable JSON fixture")
		}
		gotAccept := schema.Validate(instance) == nil
		wantAccept := rule.schemaAccept != nil && *rule.schemaAccept
		if gotAccept != wantAccept {
			t.Fatalf("schema accept = %t, want %t", gotAccept, wantAccept)
		}
	default:
		t.Fatalf("unsupported schema parity kind %q", rule.kind)
	}
}

func assertSchemaParityNotApplicable(t *testing.T, rule youConfigSchemaParityRule, caseID string) {
	t.Helper()
	if rule.kind != schemaParityNotApplicable {
		t.Fatalf("case %q parity kind = %q, want %q (%s)", caseID, rule.kind, schemaParityNotApplicable, rule.reason)
	}
}

func assertResolveCaseSchemaNotApplicable(t *testing.T, rule youConfigSchemaParityRule, caseID string) {
	t.Helper()
	if rule.kind != schemaParityNotApplicable {
		t.Fatalf("resolve case %q parity kind = %q, want %q; precedence/env/flag layers are not JSON document properties", caseID, rule.kind, schemaParityNotApplicable)
	}
}

func parityRuleForCase(owner, caseID, outcome string, hasFixture bool) youConfigSchemaParityRule {
	key := owner + ":" + caseID
	if override, ok := youConfigSchemaParityOverrides[key]; ok {
		return override
	}
	if !hasFixture {
		return youConfigSchemaParityRule{
			kind:   schemaParityNotApplicable,
			reason: "indexed case has no JSON fixture document",
		}
	}
	accept := outcome == "accept"
	return youConfigSchemaParityRule{
		kind:         schemaParityMatchesLoader,
		schemaAccept: boolPtr(accept),
	}
}

func assertParityRuleRegistered(t *testing.T, owner, caseID string) {
	t.Helper()
	key := owner + ":" + caseID
	if _, ok := youConfigSchemaParityOverrides[key]; ok {
		return
	}
	if owner == "operator_settings" {
		for _, inputCase := range operator_settings.ProjectInputInventory().Cases {
			if inputCase.ID == caseID {
				if inputCase.Fixture != "" || inputCase.Entrypoint == "Resolve" {
					return
				}
				if inputCase.ID == "valid-missing-file" {
					return
				}
			}
		}
	}
	if owner == "operator_settings_identity" {
		for _, inputCase := range committedIdentityInputInventory(t).Cases {
			if inputCase.ID == caseID {
				if inputCase.Fixture != "" {
					return
				}
				switch inputCase.ID {
				case "valid-missing-file", "invalid-empty-config-path":
					return
				case "invalid-persist-empty-scope", "invalid-persist-non-local-scope", "invalid-persist-provider-scope":
					return
				}
			}
		}
	}
	t.Fatalf("missing parity rule for indexed case %q", key)
}

func parseJSONDocument(data []byte) (any, bool) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	return document, true
}

func readOperatorFixture(t *testing.T, rel string) []byte {
	t.Helper()
	path := testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join("pkg", "services", "operator_settings", "testdata", "fixtures", rel)))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func readSystemFixture(t *testing.T, rel string) []byte {
	t.Helper()
	path := testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join(identityFixtureDirectory, rel)))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func writeOperatorFixtureToTemp(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, readOperatorFixture(t, rel), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func writeSystemFixtureToTemp(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, readSystemFixture(t, rel), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func operatorDefaultsFromLayers(t *testing.T, layers *operator_settings.ResolveLayers) operator_settings.Defaults {
	t.Helper()
	if layers.FileFixture != "" {
		cfg, err := globalconfigmapping.Decode(readOperatorFixture(t, layers.FileFixture))
		if err != nil {
			t.Fatalf("ParseFileConfig(file fixture) error = %v", err)
		}
		return cfg.Defaults
	}
	return operator_settings.Defaults{
		WorkerModelProvider: layers.FileDefaults.WorkerModelProvider,
		WorkerModel:         layers.FileDefaults.WorkerModel,
	}
}

func assertOperatorFileConfigExpectation(t *testing.T, cfg operator_settings.Config, want *operator_settings.FileConfigExpectation) {
	t.Helper()
	if want == nil {
		t.Fatal("accept case missing expectedFileConfig")
	}
	gotDefaults := operator_settings.DefaultsSnapshot{
		WorkerModelProvider: cfg.Defaults.WorkerModelProvider,
		WorkerModel:         cfg.Defaults.WorkerModel,
	}
	if gotDefaults != want.Defaults {
		t.Fatalf("defaults = %#v, want %#v", gotDefaults, want.Defaults)
	}
	if len(cfg.WorkerPresets) != len(want.WorkerPresets) {
		t.Fatalf("worker presets len = %d, want %d", len(cfg.WorkerPresets), len(want.WorkerPresets))
	}
	for i := range want.WorkerPresets {
		if cfg.WorkerPresets[i] != want.WorkerPresets[i] {
			t.Fatalf("workerPresets[%d] = %#v, want %#v", i, cfg.WorkerPresets[i], want.WorkerPresets[i])
		}
	}
}

func assertSystemScopeExpectation(t *testing.T, resolved operator_settings.ResolvedBackendScope, want *committedIdentityScopeExpectation) {
	t.Helper()
	if want == nil {
		t.Fatal("accept case missing expectedScope")
	}
	if want.BackendScopeID != "" && resolved.BackendScopeID != want.BackendScopeID {
		t.Fatalf("backendScopeID = %q, want %q", resolved.BackendScopeID, want.BackendScopeID)
	}
	if want.Outcome != "" && string(resolved.Outcome) != want.Outcome {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, want.Outcome)
	}
	if want.RequireLocalUUID && !strings.HasPrefix(resolved.BackendScopeID, "local-") {
		t.Fatalf("backendScopeID = %q, want local-<uuid> prefix", resolved.BackendScopeID)
	}
}

func assertErrorFragments(t *testing.T, err error, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %q, want fragment %q", err.Error(), fragment)
		}
	}
}

func boolPtr(value bool) *bool {
	return &value
}
