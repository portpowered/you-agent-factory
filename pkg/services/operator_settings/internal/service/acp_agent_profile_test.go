package service_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

func TestRootResolveACPAgentProfile_AbsentDocumentReturnsSafeDefault(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")

	resolved, err := root.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() = %v", err)
	}
	want := operatorsettings.DefaultACPAgentProfile()
	if resolved.DefaultTarget != want.DefaultTarget || strings.Join(resolved.AllowedTargets, ",") != strings.Join(want.AllowedTargets, ",") {
		t.Fatalf("ResolveACPAgentProfile() = %#v, want %#v", resolved, want)
	}
}

func TestRootResolveACPAgentProfile_AbsentDocumentIsReadOnly(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")

	if _, err := root.ResolveACPAgentProfile(path); err != nil {
		t.Fatalf("ResolveACPAgentProfile() = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("os.Stat(path) = %v, want ErrNotExist because resolve must not create the document", err)
	}
}

func TestRootResolveACPAgentProfile_MalformedStoredProfileFailsExplicitly(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")
	malformed := `{"workers":{"acp":{"agentProfile":{"defaultTarget":"factory:@you/review","allowedTargets":["factory:@you/factory-builder"]}}}}`
	if err := os.WriteFile(path, []byte(malformed), 0o600); err != nil {
		t.Fatalf("os.WriteFile() = %v", err)
	}

	_, err := root.ResolveACPAgentProfile(path)
	if err == nil {
		t.Fatal("ResolveACPAgentProfile() error = nil, want a typed failure for a malformed stored profile")
	}
	if !strings.Contains(err.Error(), "must be present in allowedTargets") {
		t.Fatalf("ResolveACPAgentProfile() error = %q, want the ACP Agent profile validation fragment", err)
	}
}

func TestRootResolveACPAgentProfile_ValidAuthoredProfileReturnsNormalizedDetachedValue(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")
	authored := operatorsettings.ACPAgentProfile{
		DefaultTarget:  " factory:@you/reviewer ",
		AllowedTargets: []string{" factory:@you/reviewer ", "factory:@you/factory-builder"},
	}

	updated, err := root.UpdateACPAgentProfile(context.Background(), path, authored)
	if err != nil {
		t.Fatalf("UpdateACPAgentProfile() = %v", err)
	}
	if updated.DefaultTarget != "factory:@you/reviewer" {
		t.Fatalf("UpdateACPAgentProfile() default = %q", updated.DefaultTarget)
	}

	resolved, err := root.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() = %v", err)
	}
	if resolved.DefaultTarget != "factory:@you/reviewer" ||
		len(resolved.AllowedTargets) != 2 ||
		resolved.AllowedTargets[0] != "factory:@you/reviewer" ||
		resolved.AllowedTargets[1] != "factory:@you/factory-builder" {
		t.Fatalf("ResolveACPAgentProfile() = %#v", resolved)
	}

	// Mutating the resolved slice must not alias stored state.
	resolved.AllowedTargets[0] = "factory:@you/mutated"
	reresolved, err := root.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() second call = %v", err)
	}
	if reresolved.AllowedTargets[0] != "factory:@you/reviewer" {
		t.Fatalf("ResolveACPAgentProfile() returned an aliased slice: %#v", reresolved)
	}
}

func TestRootResolveACPAgentProfile_LogsStartAndFinishedOutcomeSafely(t *testing.T) {
	t.Parallel()

	spy := &spyLogger{}
	root := newFilesystemRootWithOptions(t, filesystemRootOptions{
		files:      platformfilesystem.Local{},
		createTemp: testCreateTemporaryFile,
		decode:     globalconfigmapping.Decode,
		encode:     globalconfigmapping.Encode,
		logger:     spy,
	})
	path := filepath.Join(t.TempDir(), "config.json")

	if _, err := root.ResolveACPAgentProfile(path); err != nil {
		t.Fatalf("ResolveACPAgentProfile() = %v", err)
	}

	messages := spy.messages()
	if !containsString(messages, "operator_settings.resolve_acp_agent_profile.started") {
		t.Fatalf("log messages = %v, want a start log", messages)
	}
	if !containsString(messages, "operator_settings.resolve_acp_agent_profile.finished") {
		t.Fatalf("log messages = %v, want a finished log", messages)
	}
	assertNoSensitiveValuesLogged(t, spy, path)
}

func TestRootResolveACPAgentProfile_LogsFailureReasonWithoutLeakingValues(t *testing.T) {
	t.Parallel()

	spy := &spyLogger{}
	root := newFilesystemRootWithOptions(t, filesystemRootOptions{
		files:      platformfilesystem.Local{},
		createTemp: testCreateTemporaryFile,
		decode:     globalconfigmapping.Decode,
		encode:     globalconfigmapping.Encode,
		logger:     spy,
	})
	path := filepath.Join(t.TempDir(), "config.json")
	malformed := `{"workers":{"acp":{"agentProfile":{"defaultTarget":"factory:@you/review","allowedTargets":["factory:@you/factory-builder"]}}}}`
	if err := os.WriteFile(path, []byte(malformed), 0o600); err != nil {
		t.Fatalf("os.WriteFile() = %v", err)
	}

	if _, err := root.ResolveACPAgentProfile(path); err == nil {
		t.Fatal("ResolveACPAgentProfile() error = nil, want a malformed-profile failure")
	}

	messages := spy.messages()
	if !containsString(messages, "operator_settings.resolve_acp_agent_profile.failed") {
		t.Fatalf("log messages = %v, want a failed log", messages)
	}
	// The stored document fails Config.Normalize() during decode, so the
	// document-load boundary reports it as a malformed document rather than
	// reaching ResolveACPAgentProfile's own defensive Normalize() call.
	if !containsKeyValue(spy, "reason", "document_malformed") {
		t.Fatalf("log entries = %#v, want reason=document_malformed", spy.entries)
	}
	assertNoSensitiveValuesLogged(t, spy, path, "factory:@you/review", "factory:@you/factory-builder")
}

func TestRootUpdateACPAgentProfile_LogsStartAndFinishedOutcomeSafely(t *testing.T) {
	t.Parallel()

	spy := &spyLogger{}
	root := newFilesystemRootWithOptions(t, filesystemRootOptions{
		files:      platformfilesystem.Local{},
		createTemp: testCreateTemporaryFile,
		decode:     globalconfigmapping.Decode,
		encode:     globalconfigmapping.Encode,
		logger:     spy,
	})
	path := filepath.Join(t.TempDir(), "config.json")
	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}

	if _, err := root.UpdateACPAgentProfile(context.Background(), path, profile); err != nil {
		t.Fatalf("UpdateACPAgentProfile() = %v", err)
	}

	messages := spy.messages()
	if !containsString(messages, "operator_settings.update_acp_agent_profile.started") {
		t.Fatalf("log messages = %v, want a start log", messages)
	}
	if !containsString(messages, "operator_settings.update_acp_agent_profile.finished") {
		t.Fatalf("log messages = %v, want a finished log", messages)
	}
	assertNoSensitiveValuesLogged(t, spy, path, profile.DefaultTarget)
}

func TestRootUpdateACPAgentProfile_LogsFailureReasonForInvalidCandidateWithoutLeakingValues(t *testing.T) {
	t.Parallel()

	spy := &spyLogger{}
	root := newFilesystemRootWithOptions(t, filesystemRootOptions{
		files:      platformfilesystem.Local{},
		createTemp: testCreateTemporaryFile,
		decode:     globalconfigmapping.Decode,
		encode:     globalconfigmapping.Encode,
		logger:     spy,
	})
	path := filepath.Join(t.TempDir(), "config.json")
	invalid := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "not-a-factory-reference",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}

	if _, err := root.UpdateACPAgentProfile(context.Background(), path, invalid); !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("UpdateACPAgentProfile(invalid) = %v, want ErrACPAgentProfileInvalid", err)
	}

	messages := spy.messages()
	if !containsString(messages, "operator_settings.update_acp_agent_profile.failed") {
		t.Fatalf("log messages = %v, want a failed log", messages)
	}
	if !containsKeyValue(spy, "reason", "profile_invalid") {
		t.Fatalf("log entries = %#v, want reason=profile_invalid", spy.entries)
	}
	assertNoSensitiveValuesLogged(t, spy, path, invalid.DefaultTarget, "factory:@you/reviewer")
}

func TestRootUpdateACPAgentProfile_RejectsInvalidCandidateWithoutPersisting(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")
	valid := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}
	if _, err := root.UpdateACPAgentProfile(context.Background(), path, valid); err != nil {
		t.Fatalf("UpdateACPAgentProfile(valid) = %v", err)
	}

	invalid := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "not-a-factory-reference",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}
	if _, err := root.UpdateACPAgentProfile(context.Background(), path, invalid); !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("UpdateACPAgentProfile(invalid) = %v, want ErrACPAgentProfileInvalid", err)
	}

	resolved, err := root.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() after rejected update = %v", err)
	}
	if resolved.DefaultTarget != "factory:@you/reviewer" {
		t.Fatalf("ResolveACPAgentProfile() after rejected update = %#v, want the prior valid profile intact", resolved)
	}
}

func TestRootUpdateACPAgentProfile_RejectsCanceledContextWithoutPersisting(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}
	if _, err := root.UpdateACPAgentProfile(ctx, path, profile); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateACPAgentProfile(canceled ctx) = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("config path stat error = %v, want destination to remain absent", statErr)
	}
}

func TestRootUpdateACPAgentProfile_RejectsShortWriteWithoutReplacement(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		return shortWriteTemporaryFile{name: filepath.Join(dir, pattern)}, nil
	})
	path := filepath.Join(t.TempDir(), "config.json")
	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}

	_, err := root.UpdateACPAgentProfile(context.Background(), path, profile)
	if err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("UpdateACPAgentProfile() = %v, want short-write failure", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("config path stat error = %v, want destination to remain absent", statErr)
	}
}

func TestRootUpdateACPAgentProfile_RejectsNilServiceWithoutPanicking(t *testing.T) {
	t.Parallel()

	var nilService *operatorservice.Service
	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}

	_, err := nilService.UpdateACPAgentProfile(context.Background(), filepath.Join(t.TempDir(), "config.json"), profile)
	if err == nil || !strings.Contains(err.Error(), "operator settings document service is required") {
		t.Fatalf("UpdateACPAgentProfile() on a nil service = %v, want the actionable service-required error", err)
	}
}

func TestRootUpdateACPAgentProfile_RejectsCodecEncodeFailureWithoutReplacement(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	original := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}
	seedRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	if _, err := seedRoot.UpdateACPAgentProfile(context.Background(), path, original); err != nil {
		t.Fatalf("seed UpdateACPAgentProfile() = %v", err)
	}
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(seed) = %v", err)
	}

	failingRoot := newFilesystemRootWithOptions(t, filesystemRootOptions{
		files:      platformfilesystem.Local{},
		createTemp: testCreateTemporaryFile,
		decode:     globalconfigmapping.Decode,
		encode: func(operatorsettings.Config) ([]byte, error) {
			return nil, errors.New("injected encode failure")
		},
	})
	candidate := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/rejected",
		AllowedTargets: []string{"factory:@you/rejected"},
	}
	if _, err := failingRoot.UpdateACPAgentProfile(context.Background(), path, candidate); err == nil ||
		!strings.Contains(err.Error(), "injected encode failure") {
		t.Fatalf("UpdateACPAgentProfile() with failing encoder = %v, want injected encode failure", err)
	}

	assertDocumentBytesUnchanged(t, path, originalBytes)
	verifyRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	resolved, err := verifyRoot.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() after rejected encode = %v", err)
	}
	if resolved.DefaultTarget != original.DefaultTarget {
		t.Fatalf("ResolveACPAgentProfile() after rejected encode = %#v, want the prior profile intact", resolved)
	}
}

func TestRootUpdateACPAgentProfile_RejectsTempFileCreationFailureWithExistingDocumentIntact(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	original := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}
	seedRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	if _, err := seedRoot.UpdateACPAgentProfile(context.Background(), path, original); err != nil {
		t.Fatalf("seed UpdateACPAgentProfile() = %v", err)
	}
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(seed) = %v", err)
	}

	failingRoot := newFilesystemRoot(t, func(string, string) (operatorsettings.TemporaryFile, error) {
		return nil, errors.New("injected create failure")
	})
	candidate := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/rejected",
		AllowedTargets: []string{"factory:@you/rejected"},
	}
	if _, err := failingRoot.UpdateACPAgentProfile(context.Background(), path, candidate); err == nil ||
		!strings.Contains(err.Error(), "injected create failure") {
		t.Fatalf("UpdateACPAgentProfile() with failing temp-file creation = %v, want injected create failure", err)
	}

	assertDocumentBytesUnchanged(t, path, originalBytes)
	verifyRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	resolved, err := verifyRoot.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() after rejected create = %v", err)
	}
	if resolved.DefaultTarget != original.DefaultTarget {
		t.Fatalf("ResolveACPAgentProfile() after rejected create = %#v, want the prior profile intact", resolved)
	}
}

func TestRootUpdateACPAgentProfile_RejectsPublishFailureWithExistingDocumentIntact(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	original := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}
	seedRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	if _, err := seedRoot.UpdateACPAgentProfile(context.Background(), path, original); err != nil {
		t.Fatalf("seed UpdateACPAgentProfile() = %v", err)
	}
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(seed) = %v", err)
	}

	failingRoot := newFilesystemRootWithOptions(t, filesystemRootOptions{
		files:      &renameFailingFileSystem{FileSystem: platformfilesystem.Local{}},
		createTemp: testCreateTemporaryFile,
		decode:     globalconfigmapping.Decode,
		encode:     globalconfigmapping.Encode,
	})
	candidate := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/rejected",
		AllowedTargets: []string{"factory:@you/rejected"},
	}
	if _, err := failingRoot.UpdateACPAgentProfile(context.Background(), path, candidate); err == nil ||
		!strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("UpdateACPAgentProfile() with failing rename = %v, want injected rename failure", err)
	}

	assertDocumentBytesUnchanged(t, path, originalBytes)
	verifyRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	resolved, err := verifyRoot.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() after rejected publish = %v", err)
	}
	if resolved.DefaultTarget != original.DefaultTarget {
		t.Fatalf("ResolveACPAgentProfile() after rejected publish = %#v, want the prior profile intact", resolved)
	}
}

func TestRootUpdateACPAgentProfile_ConcurrentUpdatesNeverPersistMixedProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	root := newFilesystemRoot(t, testCreateTemporaryFile)

	const attempts = 8
	candidates := make([]operatorsettings.ACPAgentProfile, attempts)
	for index := range candidates {
		target := fmt.Sprintf("factory:@you/candidate-%d", index)
		candidates[index] = operatorsettings.ACPAgentProfile{
			DefaultTarget:  target,
			AllowedTargets: []string{target, "factory:@you/factory-builder"},
		}
	}

	var start, done sync.WaitGroup
	start.Add(1)
	errs := make([]error, attempts)
	for index := range candidates {
		done.Add(1)
		go func(index int) {
			defer done.Done()
			start.Wait()
			_, errs[index] = root.UpdateACPAgentProfile(context.Background(), path, candidates[index])
		}(index)
	}
	start.Done()
	done.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf("UpdateACPAgentProfile() attempt %d = %v", index, err)
		}
	}

	resolved, err := root.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() after concurrent updates = %v", err)
	}
	matchedIndex := -1
	for index, candidate := range candidates {
		if resolved.DefaultTarget == candidate.DefaultTarget {
			matchedIndex = index
			break
		}
	}
	if matchedIndex == -1 {
		t.Fatalf("ResolveACPAgentProfile() = %#v, want an exact match to one attempted candidate", resolved)
	}
	want := candidates[matchedIndex]
	if len(resolved.AllowedTargets) != len(want.AllowedTargets) ||
		resolved.AllowedTargets[0] != want.AllowedTargets[0] ||
		resolved.AllowedTargets[1] != want.AllowedTargets[1] {
		t.Fatalf(
			"ResolveACPAgentProfile() = %#v, mixed with another attempt instead of matching candidate %#v",
			resolved, want,
		)
	}
}

func TestRootUpdateACPAgentProfile_PersistsAtomicallyAndSurvivesReloadThroughNewService(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer", "factory:@you/factory-builder"},
	}

	firstRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	if _, err := firstRoot.UpdateACPAgentProfile(context.Background(), path, profile); err != nil {
		t.Fatalf("UpdateACPAgentProfile() = %v", err)
	}

	secondRoot := newFilesystemRoot(t, testCreateTemporaryFile)
	resolved, err := secondRoot.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() through newly constructed service = %v", err)
	}
	if resolved.DefaultTarget != profile.DefaultTarget ||
		len(resolved.AllowedTargets) != len(profile.AllowedTargets) ||
		resolved.AllowedTargets[0] != profile.AllowedTargets[0] ||
		resolved.AllowedTargets[1] != profile.AllowedTargets[1] {
		t.Fatalf("ResolveACPAgentProfile() through new service = %#v, want %#v", resolved, profile)
	}
}

func TestRootACPIntegrationAndProviderModelUpdates_PreserveAuthoredACPAgentProfile(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")
	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}
	if _, err := root.UpdateACPAgentProfile(context.Background(), path, profile); err != nil {
		t.Fatalf("UpdateACPAgentProfile() = %v", err)
	}

	integration := operatorsettings.ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}
	afterIntegrationAdd, err := root.ConfigureACPIntegrationAdd(context.Background(), path, integration)
	if err != nil {
		t.Fatalf("ConfigureACPIntegrationAdd() = %v", err)
	}
	if afterIntegrationAdd.Workers.ACP.AgentProfile == nil ||
		afterIntegrationAdd.Workers.ACP.AgentProfile.DefaultTarget != profile.DefaultTarget {
		t.Fatalf("ConfigureACPIntegrationAdd() dropped the authored ACP Agent profile: %#v", afterIntegrationAdd.Workers.ACP.AgentProfile)
	}

	nextModel := "gpt-5.2"
	afterProviderModelUpdate, err := root.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: path,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Model: &nextModel,
		},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() = %v", err)
	}
	if afterProviderModelUpdate.Document.Workers.ACP.AgentProfile == nil ||
		afterProviderModelUpdate.Document.Workers.ACP.AgentProfile.DefaultTarget != profile.DefaultTarget {
		t.Fatalf(
			"ApplyDocumentUpdate() dropped the authored ACP Agent profile: %#v",
			afterProviderModelUpdate.Document.Workers.ACP.AgentProfile,
		)
	}

	resolved, err := root.ResolveACPAgentProfile(path)
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() = %v", err)
	}
	if resolved.DefaultTarget != profile.DefaultTarget {
		t.Fatalf("ResolveACPAgentProfile() after unrelated updates = %#v, want %#v", resolved, profile)
	}
}

func TestRootUpdateACPAgentProfile_PreservesUnrelatedSettings(t *testing.T) {
	t.Parallel()

	root := newFilesystemRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")
	integration := operatorsettings.ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}
	if _, err := root.ConfigureACPIntegrationAdd(context.Background(), path, integration); err != nil {
		t.Fatalf("ConfigureACPIntegrationAdd() = %v", err)
	}
	scope, err := root.EnsureLocalBackendScope(path)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() = %v", err)
	}

	profile := operatorsettings.ACPAgentProfile{
		DefaultTarget:  "factory:@you/reviewer",
		AllowedTargets: []string{"factory:@you/reviewer"},
	}
	if _, err := root.UpdateACPAgentProfile(context.Background(), path, profile); err != nil {
		t.Fatalf("UpdateACPAgentProfile() = %v", err)
	}

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	if loaded.Document.BackendScopeID != scope.BackendScopeID {
		t.Fatalf("LoadDocument().BackendScopeID = %q, want %q", loaded.Document.BackendScopeID, scope.BackendScopeID)
	}
	if len(loaded.Document.Workers.ACP.Integrations) != 1 || loaded.Document.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("LoadDocument() integrations = %#v, want %#v", loaded.Document.Workers.ACP.Integrations, []operatorsettings.ACPIntegration{integration})
	}
}

// renameFailingFileSystem wraps a real FileSystem and injects a failure only
// on the final atomic-publish Rename step, so PersistDocument's earlier
// steps (mkdir, temp-file write) still exercise the real filesystem while
// proving the destination survives a rename/publish failure untouched.
type renameFailingFileSystem struct {
	operatorsettings.FileSystem
}

func (files *renameFailingFileSystem) Rename(string, string) error {
	return errors.New("injected rename failure")
}

// assertDocumentBytesUnchanged fails the test unless the file at path still
// holds exactly want, proving a rejected mutation never replaced the
// previously persisted document.
func assertDocumentBytesUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("document at %q changed after a rejected update: got %q, want %q", path, got, want)
	}
}

// spyLoggedEntry captures one structured log call for assertion.
type spyLoggedEntry struct {
	level string
	msg   string
	kv    []any
}

// spyLogger is a logging.Logger fake that records every call so tests can
// assert on operation-log shape (message names, safe fields) without a real
// logging backend.
type spyLogger struct {
	mu      sync.Mutex
	entries []spyLoggedEntry
}

func (s *spyLogger) Debug(msg string, kv ...any)   { s.record("debug", msg, kv) }
func (s *spyLogger) Info(msg string, kv ...any)    { s.record("info", msg, kv) }
func (s *spyLogger) Warn(msg string, kv ...any)    { s.record("warn", msg, kv) }
func (s *spyLogger) Error(msg string, kv ...any)   { s.record("error", msg, kv) }
func (s *spyLogger) Verbose(msg string, kv ...any) { s.record("verbose", msg, kv) }

func (s *spyLogger) record(level, msg string, kv []any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, spyLoggedEntry{level: level, msg: msg, kv: append([]any(nil), kv...)})
}

func (s *spyLogger) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.entries))
	for index, entry := range s.entries {
		out[index] = entry.msg
	}
	return out
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

// containsKeyValue reports whether any recorded entry has key immediately
// followed by want in its key/value pairs.
func containsKeyValue(spy *spyLogger, key string, want any) bool {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	for _, entry := range spy.entries {
		for index := 0; index+1 < len(entry.kv); index += 2 {
			if entry.kv[index] == key && entry.kv[index+1] == want {
				return true
			}
		}
	}
	return false
}

// assertNoSensitiveValuesLogged fails the test if any recorded log message or
// key/value field contains the config path or any other forbidden value
// (profile target references, raw configuration contents).
func assertNoSensitiveValuesLogged(t *testing.T, spy *spyLogger, forbidden ...string) {
	t.Helper()
	spy.mu.Lock()
	defer spy.mu.Unlock()
	for _, entry := range spy.entries {
		if containsForbiddenSubstring(entry.msg, forbidden) {
			t.Fatalf("log message %q leaked a forbidden value from %v", entry.msg, forbidden)
		}
		for _, value := range entry.kv {
			text, ok := value.(string)
			if !ok {
				continue
			}
			if containsForbiddenSubstring(text, forbidden) {
				t.Fatalf("log field %q leaked a forbidden value from %v", text, forbidden)
			}
		}
	}
}

func containsForbiddenSubstring(value string, forbidden []string) bool {
	for _, needle := range forbidden {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
