package http

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestOwnedHTTPSurfaceIsDetailsOnly(t *testing.T) {
	want := []string{"getProviderSessionDetails"}
	if !slices.Equal(OwnedHTTPOperationIDs, want) {
		t.Fatalf("OwnedHTTPOperationIDs = %#v, want %#v", OwnedHTTPOperationIDs, want)
	}
}

func TestHandlerExposesOnlyOwnedDetailHTTPMethod(t *testing.T) {
	handlerType := reflect.TypeOf((*Handler)(nil))
	var exportedMethods []string
	for i := 0; i < handlerType.NumMethod(); i++ {
		method := handlerType.Method(i)
		if !method.IsExported() {
			continue
		}
		exportedMethods = append(exportedMethods, method.Name)
	}

	want := []string{"GetProviderSessionDetails"}
	if !slices.Equal(exportedMethods, want) {
		t.Fatalf("Handler exported methods = %#v, want %#v", exportedMethods, want)
	}
}

func TestAdapterDoesNotExposeInspectOrProjectMapping(t *testing.T) {
	adapterType := reflect.TypeOf((*Adapter)(nil))
	for _, forbidden := range []string{"Inspect", "Project"} {
		if _, ok := adapterType.MethodByName(forbidden); ok {
			t.Fatalf("Adapter must not expose %s HTTP mapping in Details-only packet", forbidden)
		}
	}
}

func TestAdapterPackageDoesNotDeclareInspectOrProjectHTTPHandlers(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	packageDir := filepath.Dir(currentFile)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, packageDir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}
	pkg, ok := pkgs["http"]
	if !ok {
		t.Fatal("package http not found")
	}

	forbiddenPrefixes := []string{"Inspect", "Project"}
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil || funcDecl.Name == nil {
				continue
			}
			for _, prefix := range forbiddenPrefixes {
				if strings.HasPrefix(funcDecl.Name.Name, prefix) {
					t.Fatalf("adapter package must not declare %s HTTP handler %s in Details-only packet", prefix, funcDecl.Name.Name)
				}
			}
		}
	}
}

func TestOwnedDetailHTTPMappingUsesDetailsRootSliceOnly(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	text := "owned surface detail"
	fake := &rootServiceFake{
		detail: providersessions.Detail{
			ProviderSession: providersessions.Ref{
				Provider: providersessions.ProviderCodex,
				Kind:     providersessions.SessionIDKind,
				ID:       "sess-owned-surface",
			},
			Source: providersessions.SourceMetadata{
				ModifiedAt:   &modifiedAt,
				RelativePath: "2026/07/27/rollout-sess-owned-surface.jsonl",
				SizeBytes:    64,
			},
			Parse: providersessions.ParseSummary{EventCount: 1, LineCount: 1},
			Transcript: []providersessions.TranscriptEntry{{
				Order: 0,
				Text:  &text,
				Type:  providersessions.TranscriptAssistantMessage,
			}},
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.GetProviderSessionDetails(context.Background(), factoryapi.GetProviderSessionDetailsParams{
		Provider: factoryapi.Codex,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       "sess-owned-surface",
	})
	if err != nil {
		t.Fatalf("GetProviderSessionDetails: %v", err)
	}
	if fake.lastID != "sess-owned-surface" {
		t.Fatalf("fake.lastID = %q, want Details root invocation", fake.lastID)
	}
}
