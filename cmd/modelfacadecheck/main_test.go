package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsThinDelegatesDiscoveredAcrossFacadePackageFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFacadeFixture(t, root, "pkg/service/catalog_moved.go", thinDelegateFixture("service", "FactoryService"))
	writeFacadeFixture(t, root, "pkg/runtimehost/compatibility.go", thinDelegateFixture("runtimehost", "Host"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{root: root}, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v, want nil; stderr = %q", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "model facade methods are thin delegates") {
		t.Fatalf("run() stdout = %q, want success message", got)
	}
}

func TestRunRejectsCopiedPullPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFacadeFixture(t, root, "pkg/service/any_name.go", thinDelegateFixture("service", "FactoryService"))
	violatingHost := strings.Replace(
		thinDelegateFixture("runtimehost", "Host"),
		"return facade.requireModelService().PullModel(ctx, modelName)",
		"puller := facade.modelAssetPuller()\n\treturn puller.PullModel(ctx, facade.currentRuntimeConfig(), modelName)",
		1,
	)
	writeFacadeFixture(t, root, "pkg/runtimehost/moved_again.go", violatingHost)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: root}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want copied pull policy rejection")
	}
	if got := stderr.String(); !strings.Contains(got, "Host.PullModel") || !strings.Contains(got, "single return delegation") {
		t.Fatalf("run() stderr = %q, want copied PullModel policy finding", got)
	}
}

func TestRunRejectsRetiredNewFromHostConstruction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFacadeFixture(t, root, "pkg/service/catalog.go", thinDelegateFixture("service", "FactoryService"))
	writeFacadeFixture(t, root, "pkg/runtimehost/catalog.go", thinDelegateFixture("runtimehost", "Host"))
	writeFacadeFixture(t, root, "pkg/service/construction.go", `package service

import modelsservice "github.com/portpowered/infinite-you/pkg/models/service"

func construct(fs *FactoryService) any { return modelsservice.NewFromHost(fs) }
`)

	assertRunRejected(t, root, "NewFromHost")
}

func TestRunRejectsBroadFacadeCarrierConstruction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFacadeFixture(t, root, "pkg/service/catalog.go", thinDelegateFixture("service", "FactoryService"))
	writeFacadeFixture(t, root, "pkg/runtimehost/catalog.go", thinDelegateFixture("runtimehost", "Host"))
	writeFacadeFixture(t, root, "pkg/runtimehost/construction.go", `package runtimehost

import modelsservice "github.com/portpowered/infinite-you/pkg/models/service"

	func construct(host *Host) any { return modelsservice.New(&modelServiceHost{host: host}) }
`)
	writeFacadeFixture(t, root, "pkg/runtimehost/adapter.go", `package runtimehost

type modelServiceHost struct { host *Host }
`)

	assertRunRejected(t, root, "broad Host carrier")
}

func assertRunRejected(t *testing.T, root string, finding string) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{root: root}, stdout, stderr); err == nil {
		t.Fatalf("run() error = nil, want %q rejection", finding)
	}
	if got := stderr.String(); !strings.Contains(got, finding) {
		t.Fatalf("run() stderr = %q, want %q", got, finding)
	}
}

func thinDelegateFixture(packageName string, receiverType string) string {
	return "package " + packageName + `

func (facade *` + receiverType + `) ListModels(ctx context.Context) (ListResponse, error) {
	return facade.requireModelService().ListModels(ctx)
}

func (facade *` + receiverType + `) GetModel(ctx context.Context, modelName string) (ModelDetail, error) {
	return facade.requireModelService().GetModel(ctx, modelName)
}

func (facade *` + receiverType + `) PullModel(ctx context.Context, modelName string) (PullResult, error) {
	return facade.requireModelService().PullModel(ctx, modelName)
}

func (facade *` + receiverType + `) InvokeModel(ctx context.Context, modelName string, request InvocationRequest) (InvocationResult, error) {
	return facade.requireModelService().InvokeModel(ctx, modelName, request)
}
`
}

func writeFacadeFixture(t *testing.T, root string, relativePath string, source string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", relativePath, err)
	}
}
