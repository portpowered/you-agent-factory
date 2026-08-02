package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	documentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

func newACPAgentProfileTestRoot(t *testing.T) operatorsettings.Service {
	t.Helper()

	providersRoot := internaltestproviders.StandardCatalog()
	documentService := documentwire.NewService(
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		rootTestProviderCatalog,
	)
	resolutionService, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		t.Fatalf("resolutionwire.NewService() = %v", err)
	}
	root, err := operatorservice.New(
		documentService,
		resolutionService,
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		nil,
	)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return root
}

func TestResolveACPAgentProfileWithNoAuthoredProfileReturnsBuiltIn(t *testing.T) {
	t.Parallel()

	root := newACPAgentProfileTestRoot(t)

	result, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	if !reflect.DeepEqual(result.Profile, operatorsettings.BuiltInACPAgentProfile()) {
		t.Fatalf("ResolveACPAgentProfile() = %#v, want built-in profile", result.Profile)
	}
}

func TestResolveACPAgentProfileWithAuthoredProfileReturnsNormalizedValue(t *testing.T) {
	t.Parallel()

	root := newACPAgentProfileTestRoot(t)

	authored := &operatorsettings.DocumentACPAgentProfile{
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/factory-builder", "@you/custom"},
	}
	result, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{AuthoredProfile: authored})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	want := operatorsettings.ACPAgentProfile{
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/factory-builder", "@you/custom"},
	}
	if !reflect.DeepEqual(result.Profile, want) {
		t.Fatalf("ResolveACPAgentProfile() = %#v, want %#v", result.Profile, want)
	}

	// Mutating the request input after resolution must not affect the
	// already-returned detached result.
	authored.Allowlist[0] = "mutated"
	if result.Profile.Allowlist[0] == "mutated" {
		t.Fatalf("resolved profile was not detached from request input: %#v", result.Profile.Allowlist)
	}
}

func TestResolveACPAgentProfileWithInvalidAuthoredProfileFails(t *testing.T) {
	t.Parallel()

	root := newACPAgentProfileTestRoot(t)

	authored := &operatorsettings.DocumentACPAgentProfile{
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/factory-builder"},
	}
	_, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{AuthoredProfile: authored})
	if !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("ResolveACPAgentProfile() error = %v, want ErrACPAgentProfileInvalid", err)
	}
}

func TestUpdateACPAgentProfilePersistsAndReloadsAcrossNewService(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	root := newFilesystemRoot(t, testCreateTemporaryFile)

	updated, err := root.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/factory-builder", "@you/custom"},
	})
	if err != nil {
		t.Fatalf("UpdateACPAgentProfile() error = %v", err)
	}
	if !updated.Persisted {
		t.Fatalf("UpdateACPAgentProfile() Persisted = false, want true")
	}
	want := operatorsettings.ACPAgentProfile{
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/factory-builder", "@you/custom"},
	}
	if !reflect.DeepEqual(updated.Profile, want) {
		t.Fatalf("UpdateACPAgentProfile() profile = %#v, want %#v", updated.Profile, want)
	}

	// A newly constructed service (a fresh root over the same on-disk state)
	// must resolve the same effective profile after reconstruction/reload.
	reconstructed := newFilesystemRoot(t, testCreateTemporaryFile)
	resolved, err := reconstructed.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{Path: path})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	if !reflect.DeepEqual(resolved.Profile, want) {
		t.Fatalf("ResolveACPAgentProfile() after reload = %#v, want %#v", resolved.Profile, want)
	}
}

func TestUpdateACPAgentProfileRejectsInvalidAndPreservesPriorProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	root := newFilesystemRoot(t, testCreateTemporaryFile)

	first, err := root.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: "@you/first",
		Allowlist:               []string{"@you/first"},
	})
	if err != nil {
		t.Fatalf("UpdateACPAgentProfile() first error = %v", err)
	}

	_, err = root.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: "@you/missing",
		Allowlist:               []string{"@you/first"},
	})
	if !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("UpdateACPAgentProfile() invalid update error = %v, want ErrACPAgentProfileInvalid", err)
	}

	resolved, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{Path: path})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	if !reflect.DeepEqual(resolved.Profile, first.Profile) {
		t.Fatalf("ResolveACPAgentProfile() after rejected update = %#v, want prior profile %#v", resolved.Profile, first.Profile)
	}
}

func TestUpdateACPAgentProfileRejectsCanceledContext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	root := newFilesystemRoot(t, testCreateTemporaryFile)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := root.UpdateACPAgentProfile(ctx, operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/custom"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateACPAgentProfile() with canceled context error = %v, want context.Canceled", err)
	}

	resolved, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{Path: path})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	if !reflect.DeepEqual(resolved.Profile, operatorsettings.BuiltInACPAgentProfile()) {
		t.Fatalf("ResolveACPAgentProfile() after canceled update = %#v, want built-in profile", resolved.Profile)
	}
}

func TestUpdateACPAgentProfilePersistFailureLeavesPriorProfileObservable(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")

	workingRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	first, err := workingRoot.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: "@you/first",
		Allowlist:               []string{"@you/first"},
	})
	if err != nil {
		t.Fatalf("UpdateACPAgentProfile() first error = %v", err)
	}

	failingRoot := newFilesystemRoot(t, func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		return shortWriteTemporaryFile{name: filepath.Join(dir, pattern)}, nil
	})
	_, err = failingRoot.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: "@you/second",
		Allowlist:               []string{"@you/second"},
	})
	if !errors.Is(err, operatorsettings.ErrACPAgentProfilePersistFailed) {
		t.Fatalf("UpdateACPAgentProfile() persist-failure error = %v, want ErrACPAgentProfilePersistFailed", err)
	}

	resolved, err := workingRoot.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{Path: path})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	if !reflect.DeepEqual(resolved.Profile, first.Profile) {
		t.Fatalf("ResolveACPAgentProfile() after persist failure = %#v, want prior profile %#v", resolved.Profile, first.Profile)
	}
}

func TestUpdateACPAgentProfileIsIsolatedFromUnrelatedDocumentUpdates(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	root := newFilesystemRoot(t, testCreateTemporaryFile)

	profile, err := root.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/custom"},
	})
	if err != nil {
		t.Fatalf("UpdateACPAgentProfile() error = %v", err)
	}

	integration := operatorsettings.ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}
	document, err := root.ConfigureACPIntegrationAdd(context.Background(), path, integration)
	if err != nil {
		t.Fatalf("ConfigureACPIntegrationAdd() error = %v", err)
	}
	if len(document.Workers.ACP.Integrations) != 1 || document.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("ConfigureACPIntegrationAdd() integrations = %#v, want %#v", document.Workers.ACP.Integrations, []operatorsettings.ACPIntegration{integration})
	}

	resolved, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{Path: path})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	if !reflect.DeepEqual(resolved.Profile, profile.Profile) {
		t.Fatalf("ResolveACPAgentProfile() after unrelated document update = %#v, want unchanged profile %#v", resolved.Profile, profile.Profile)
	}

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if len(loaded.Document.Workers.ACP.Integrations) != 1 || loaded.Document.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("LoadDocument() integrations = %#v, want %#v", loaded.Document.Workers.ACP.Integrations, []operatorsettings.ACPIntegration{integration})
	}
}

func newACPAgentProfileTestRootWithLogger(t *testing.T, logger operatorsettings.ACPAgentProfileLogger) operatorsettings.Service {
	t.Helper()

	providersRoot := internaltestproviders.StandardCatalog()
	documentService := documentwire.NewService(
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		rootTestProviderCatalog,
	)
	resolutionService, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		t.Fatalf("resolutionwire.NewService() = %v", err)
	}
	root, err := operatorservice.New(
		documentService,
		resolutionService,
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logger,
	)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return root
}

func newACPAgentProfileFilesystemRootWithLogger(t *testing.T, logger operatorsettings.ACPAgentProfileLogger) operatorsettings.Service {
	t.Helper()

	files := platformfilesystem.Local{}
	documentService := documentwire.NewService(
		files,
		testCreateTemporaryFile,
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		rootTestProviderCatalog,
	)
	resolutionService, err := resolutionwire.NewService(internaltestproviders.StandardCatalog())
	if err != nil {
		t.Fatalf("resolutionwire.NewService() = %v", err)
	}
	root, err := operatorservice.New(
		documentService,
		resolutionService,
		files,
		testCreateTemporaryFile,
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logger,
	)
	if err != nil {
		t.Fatalf("operatorservice.New() = %v", err)
	}
	return root
}

func TestResolveACPAgentProfileEmitsAcceptedIntentAndSuccessLogs(t *testing.T) {
	t.Parallel()

	var records []operatorsettings.ACPAgentProfileLogRecord
	root := newACPAgentProfileTestRootWithLogger(t, func(record operatorsettings.ACPAgentProfileLogRecord) {
		records = append(records, record)
	})

	if _, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{}); err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("log records = %#v, want exactly 2 (accepted-intent, success)", records)
	}
	if records[0].Operation != operatorsettings.ACPAgentProfileOperationResolve ||
		records[0].Stage != operatorsettings.ACPAgentProfileLogStageAcceptedIntent {
		t.Fatalf("first record = %#v, want accepted-intent for resolve operation", records[0])
	}
	if records[1].Operation != operatorsettings.ACPAgentProfileOperationResolve ||
		records[1].Stage != operatorsettings.ACPAgentProfileLogStageSuccess ||
		records[1].FailureKind != "" {
		t.Fatalf("second record = %#v, want success terminal log with no failure kind", records[1])
	}
	if records[1].Fields["source"] != "built_in" {
		t.Fatalf("success record fields = %#v, want source=built_in", records[1].Fields)
	}
}

func TestResolveACPAgentProfileFailureLogHasTypedKindAndNoRawReferenceValues(t *testing.T) {
	t.Parallel()

	var records []operatorsettings.ACPAgentProfileLogRecord
	root := newACPAgentProfileTestRootWithLogger(t, func(record operatorsettings.ACPAgentProfileLogRecord) {
		records = append(records, record)
	})

	const sensitiveReference = "@you/should-not-appear-in-logs"
	authored := &operatorsettings.DocumentACPAgentProfile{
		DefaultFactoryReference: sensitiveReference,
		Allowlist:               []string{"@you/factory-builder"},
	}
	_, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{AuthoredProfile: authored})
	if !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("ResolveACPAgentProfile() error = %v, want ErrACPAgentProfileInvalid", err)
	}

	if len(records) != 2 {
		t.Fatalf("log records = %#v, want exactly 2 (accepted-intent, failure)", records)
	}
	failure := records[1]
	if failure.Stage != operatorsettings.ACPAgentProfileLogStageFailure || failure.FailureKind != string(operatorsettings.ACPAgentProfileFailureKindInvalid) {
		t.Fatalf("failure record = %#v, want failure stage with typed invalid kind", failure)
	}
	if strings.Contains(recordText(failure), sensitiveReference) {
		t.Fatalf("failure record leaked the candidate Factory reference: %#v", failure)
	}
}

func TestUpdateACPAgentProfileEmitsAcceptedIntentAndSuccessLogs(t *testing.T) {
	t.Parallel()

	var records []operatorsettings.ACPAgentProfileLogRecord
	root := newACPAgentProfileFilesystemRootWithLogger(t, func(record operatorsettings.ACPAgentProfileLogRecord) {
		records = append(records, record)
	})
	path := filepath.Join(t.TempDir(), "config.json")

	_, err := root.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/custom"},
	})
	if err != nil {
		t.Fatalf("UpdateACPAgentProfile() error = %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("log records = %#v, want exactly 2 (accepted-intent, success)", records)
	}
	if records[0].Operation != operatorsettings.ACPAgentProfileOperationUpdate ||
		records[0].Stage != operatorsettings.ACPAgentProfileLogStageAcceptedIntent {
		t.Fatalf("first record = %#v, want accepted-intent for update operation", records[0])
	}
	if records[1].Stage != operatorsettings.ACPAgentProfileLogStageSuccess || records[1].FailureKind != "" {
		t.Fatalf("second record = %#v, want success terminal log with no failure kind", records[1])
	}
	if records[1].Fields["persisted"] != true {
		t.Fatalf("success record fields = %#v, want persisted=true", records[1].Fields)
	}
	if _, hasPath := records[1].Fields["path"]; hasPath {
		t.Fatalf("success record fields = %#v, must not include the raw filesystem path", records[1].Fields)
	}
}

func TestUpdateACPAgentProfileFailureLogHasTypedKindAndNoRawReferenceValues(t *testing.T) {
	t.Parallel()

	var records []operatorsettings.ACPAgentProfileLogRecord
	root := newACPAgentProfileFilesystemRootWithLogger(t, func(record operatorsettings.ACPAgentProfileLogRecord) {
		records = append(records, record)
	})
	path := filepath.Join(t.TempDir(), "config.json")

	const sensitiveReference = "@you/should-not-appear-in-logs"
	_, err := root.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    path,
		DefaultFactoryReference: sensitiveReference,
		Allowlist:               []string{"@you/other"},
	})
	if !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("UpdateACPAgentProfile() error = %v, want ErrACPAgentProfileInvalid", err)
	}

	if len(records) != 2 {
		t.Fatalf("log records = %#v, want exactly 2 (accepted-intent, failure)", records)
	}
	failure := records[1]
	if failure.Stage != operatorsettings.ACPAgentProfileLogStageFailure || failure.FailureKind != string(operatorsettings.ACPAgentProfileFailureKindInvalid) {
		t.Fatalf("failure record = %#v, want failure stage with typed invalid kind", failure)
	}
	if strings.Contains(recordText(failure), sensitiveReference) {
		t.Fatalf("failure record leaked the candidate Factory reference: %#v", failure)
	}
}

// recordText renders a log record's textual surface (everything other than
// numeric/boolean counters) so tests can assert sensitive values never cross
// the logging boundary.
func recordText(record operatorsettings.ACPAgentProfileLogRecord) string {
	var builder strings.Builder
	builder.WriteString(record.Operation)
	builder.WriteString(record.Stage)
	builder.WriteString(record.FailureKind)
	for key, value := range record.Fields {
		builder.WriteString(key)
		if text, ok := value.(string); ok {
			builder.WriteString(text)
		}
	}
	return builder.String()
}
