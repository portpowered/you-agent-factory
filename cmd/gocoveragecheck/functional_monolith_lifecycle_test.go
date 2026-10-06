package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFunctionalMonolithCleanupEligibility(t *testing.T) {
	cases := []struct {
		name, before, signature, effect string
		eligible                        bool
	}{
		{name: "cleanup-only", signature: "*testing.T", eligible: true},
		{name: "startup-before-run", before: "initializeFixture();", signature: "*testing.T"},
		{name: "wrong-hook-signature", signature: "*testing.M"},
		{name: "global-state-remains-native", signature: "*testing.T", effect: `t.Setenv("HOME", "other")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			source := `package fixture_test
import "testing"
func TestMain(m *testing.M) { BEFORE code := m.Run(); _ = code }
func FunctionalMonolithCleanup(t SIGNATURE) {}
func TestCustomer(t *testing.T) { EFFECT }
`
			source = strings.NewReplacer("BEFORE", tc.before, "SIGNATURE", tc.signature, "EFFECT", tc.effect).Replace(source)
			if err := os.WriteFile(filepath.Join(dir, "fixture_test.go"), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			registration, _, reason, err := functionalMonolithRegistration(functionalGoListPackage{Dir: dir, XTestGoFiles: []string{"fixture_test.go"}})
			if err != nil {
				t.Fatal(err)
			}
			if (reason == "") != tc.eligible {
				t.Fatalf("eligible=%t, reason=%q", tc.eligible, reason)
			}
			if tc.eligible && !strings.Contains(registration, "var monolithCleanup = _xtest.FunctionalMonolithCleanup") {
				t.Fatalf("cleanup not registered: %s", registration)
			}
		})
	}
}

func TestFunctionalMonolithMainRequiresOneRunBeforeCleanup(t *testing.T) {
	cases := []struct {
		body     string
		accepted bool
	}{
		{body: "code := m.Run(); _ = code", accepted: true},
		{body: "initialize(); code := m.Run(); _ = code"},
		{body: "code := m.Run(); code += m.Run(); _ = code"},
		{body: "code := other.Run(); _ = code"},
		{body: "code := m.Run(1); _ = code"},
		{body: ""},
	}
	for _, tc := range cases {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture;func TestMain(m *testing.M){"+tc.body+"}", 0)
		if err != nil {
			t.Fatal(err)
		}
		fn := file.Decls[0].(*ast.FuncDecl)
		if got := functionalMonolithMainRunsFirst(fn); got != tc.accepted {
			t.Fatalf("body=%q accepted=%t, want %t", tc.body, got, tc.accepted)
		}
	}
}
