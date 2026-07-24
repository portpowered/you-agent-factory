package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsConstructedServiceEdgesImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_sessions/internal/applicationopening/service.go", `package applicationopening

import serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"

var _ = serviceedges.Edges{}
`)

	var stdout, stderr bytes.Buffer
	err := run(config{root: repoRoot, packageRoot: "pkg"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want constructed-service Edges import rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited constructed-service Edges import:",
		"pkg/services/factory_sessions/internal/applicationopening/service.go",
		"inject exact external-effect ports",
		"not a service locator",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want %q", got, want)
		}
	}
}

func TestRunRejectsConstructedServiceEdgesFieldParameter(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_sessions/internal/runtimeopening/factory.go", `package runtimeopening

import serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"

type Factory struct {
	effects serviceedges.Edges
}

func New(effects serviceedges.Edges) *Factory {
	return &Factory{effects: effects}
}
`)

	var stderr bytes.Buffer
	err := run(config{root: repoRoot, packageRoot: "pkg"}, &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want constructed-service Edges dependency rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited constructed-service Edges dependency: edges.Edges effects",
		"exact external-effect ports projected at pkg/wire",
		"not a service locator",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want %q", got, want)
		}
	}
}

func TestRunAllowsProcessEdgeExceptionConsumers(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/edges/definition.go", `package edges

type Edges struct{}
`)
	writeGoSourceFile(t, repoRoot, "pkg/root/process.go", `package root

import serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"

func BuildProcess(edges serviceedges.Edges) {}
`)
	writeGoSourceFile(t, repoRoot, "pkg/wire/wire.go", `package wire

import serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"

func InjectBundle(edges serviceedges.Edges) {}
`)
	writeGoSourceFile(t, repoRoot, "tests/functional/smoke/override_test.go", `package smoke

import serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"

var overrides = serviceedges.Edges{}
`)

	var stdout, stderr bytes.Buffer
	if err := run(config{root: repoRoot, packageRoot: "pkg"}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, stderr = %q, want allowed process-edge consumers", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "package boundary passed") {
		t.Fatalf("run() stdout = %q, want package-boundary success", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunRejectsBlankConstructedServiceEdgesImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/workers/service/open.go", `package service

import _ "github.com/portpowered/infinite-you/pkg/services/edges"
`)

	var stderr bytes.Buffer
	err := run(config{root: repoRoot, packageRoot: "pkg"}, &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want blank constructed-service Edges import rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited constructed-service Edges import:",
		"pkg/services/workers/service/open.go",
		"not a service locator",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want %q", got, want)
		}
	}
}
