package local

import (
	"errors"
	"testing"

	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestManagedRuntimeReadinessProjection_BlocksMissingRuntime(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	inspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, MissingAssets: []string{"omnivoice-base-Q4_K_M.gguf"}},
	}}

	managed, err := ManagedRuntimeReadinessForFactory(
		loaded, "OMNIVOICE_Q4_K_M", inspector, DefaultManagedRuntimeSourceResolver(),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = managed.InvocationError()
	if err == nil {
		t.Fatalf("error = %v, want managed runtime invocation block", err)
	}
	if !errors.Is(err, apisurface.ErrMissing) {
		t.Fatalf("error = %v, want missing readiness", err)
	}
}

func TestManagedRuntimeReadinessProjection_BlocksLoadingAndFailedRuntimes(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	loadingInspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, InstalledFileCount: 1},
	}}
	loading, loadingErr := ManagedRuntimeReadinessForFactory(
		loaded, "OMNIVOICE_Q4_K_M", loadingInspector, nil,
	)
	if loadingErr != nil {
		t.Fatal(loadingErr)
	}
	loadingErr = loading.InvocationError()
	if loadingErr == nil {
		t.Fatalf("loading error = %v, want blocked invocation", loadingErr)
	}
	if !errors.Is(loadingErr, apisurface.ErrLoading) {
		t.Fatalf("loading error = %v, want ErrManagedRuntimeLoading", loadingErr)
	}

	failedInspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, PartialArtifacts: true},
	}}
	failed, failedErr := ManagedRuntimeReadinessForFactory(
		loaded, "OMNIVOICE_Q4_K_M", failedInspector, nil,
	)
	if failedErr != nil {
		t.Fatal(failedErr)
	}
	failedErr = failed.InvocationError()
	if failedErr == nil {
		t.Fatalf("failed error = %v, want blocked invocation", failedErr)
	}
	if !errors.Is(failedErr, apisurface.ErrFailed) {
		t.Fatalf("failed error = %v, want ErrManagedRuntimeFailed", failedErr)
	}
}

func TestManagedRuntimeReadinessProjection_AllowsReadyRuntime(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	inspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, InstalledFileCount: 2},
	}}

	managed, err := ManagedRuntimeReadinessForFactory(
		loaded, "OMNIVOICE_Q4_K_M", inspector, DefaultManagedRuntimeSourceResolver(),
	)
	if err != nil {
		t.Fatalf("ManagedRuntimeReadinessForFactory: %v", err)
	}
	if err := managed.InvocationError(); err != nil {
		t.Fatalf("ready projection refuses invocation: %v", err)
	}
	if managed.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
}

func TestManagedRuntimeReadinessProjection_EquivalentConfigurationsMatch(t *testing.T) {
	t.Parallel()
	authored := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	equivalent := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	inspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, InstalledFileCount: 2},
	}}
	resolver := DefaultManagedRuntimeSourceResolver()

	authoredManaged, err := ManagedRuntimeReadinessForFactory(authored, "OMNIVOICE_Q4_K_M", inspector, resolver)
	if err != nil {
		t.Fatalf("authored readiness: %v", err)
	}
	equivalentManaged, err := ManagedRuntimeReadinessForFactory(equivalent, "OMNIVOICE_Q4_K_M", inspector, resolver)
	if err != nil {
		t.Fatalf("equivalent readiness: %v", err)
	}
	if authoredManaged.Identity != equivalentManaged.Identity ||
		authoredManaged.ReadinessState != equivalentManaged.ReadinessState ||
		authoredManaged.LifecycleState != equivalentManaged.LifecycleState {
		t.Fatalf("authored = %#v, equivalent = %#v, want identical readiness", authoredManaged, equivalentManaged)
	}
}
