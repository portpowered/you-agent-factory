package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type javascriptSymbolInventory struct {
	Symbols []javascriptSymbolRecord `json:"symbols"`
}

type javascriptSymbolRecord struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Kind     string   `json:"kind"`
	Parent   string   `json:"parent,omitempty"`
	Members  []string `json:"members,omitempty"`
	Callable bool     `json:"callable"`
	Async    bool     `json:"async"`
}

type javascriptCallBehaviorInventory struct {
	Records []javascriptCallBehaviorRecord `json:"records"`
}

type javascriptCallBehaviorRecord struct {
	Path           string                    `json:"path"`
	Kind           string                    `json:"kind"`
	Mutability     string                    `json:"mutability,omitempty"`
	Nullability    string                    `json:"nullability,omitempty"`
	Lifecycle      string                    `json:"lifecycle,omitempty"`
	Async          bool                      `json:"async,omitempty"`
	Parameters     []javascriptParameter     `json:"parameters,omitempty"`
	Callback       *javascriptCallbackShape  `json:"callback,omitempty"`
	Return         *javascriptReturnBehavior `json:"return,omitempty"`
	EmittedRecords []string                  `json:"emittedRecords,omitempty"`
	Errors         []javascriptErrorCase     `json:"errors,omitempty"`
	PolicyChecks   []javascriptPolicyCheck   `json:"policyChecks,omitempty"`
	Determinism    string                    `json:"determinism,omitempty"`
	ResumeNotes    string                    `json:"resumeNotes,omitempty"`
}

type javascriptParameter struct {
	Name             string                     `json:"name"`
	Required         bool                       `json:"required"`
	Rest             bool                       `json:"rest,omitempty"`
	Default          string                     `json:"default,omitempty"`
	Type             string                     `json:"type"`
	ObjectProperties []javascriptObjectProperty `json:"objectProperties,omitempty"`
}

type javascriptObjectProperty struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Type     string `json:"type"`
	Default  string `json:"default,omitempty"`
}

type javascriptCallbackShape struct {
	Role       string                `json:"role,omitempty"`
	Parameters []javascriptParameter `json:"parameters"`
	Notes      string                `json:"notes,omitempty"`
}

type javascriptReturnBehavior struct {
	SyncType    string `json:"syncType,omitempty"`
	Async       bool   `json:"async,omitempty"`
	PromiseType string `json:"promiseType,omitempty"`
}

type javascriptErrorCase struct {
	Condition string `json:"condition"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

type javascriptPolicyCheck struct {
	Kind    string `json:"kind"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message,omitempty"`
}

func loadJavaScriptSymbolInventory(t *testing.T) javascriptSymbolInventory {
	t.Helper()
	var inventory javascriptSymbolInventory
	loadJavaScriptRuntimeInventory(
		t,
		filepath.Join(
			"..",
			"pkg",
			"services",
			"factory_runtime",
			"tooling",
			"javascript",
			"javascript-runtime-symbols.json",
		),
		&inventory,
	)
	return inventory
}

func loadJavaScriptCallBehaviorInventory(
	t *testing.T,
) javascriptCallBehaviorInventory {
	t.Helper()
	var inventory javascriptCallBehaviorInventory
	loadJavaScriptRuntimeInventory(
		t,
		filepath.Join(
			"..",
			"pkg",
			"services",
			"factory_runtime",
			"tooling",
			"javascript",
			"javascript-runtime-call-behavior.json",
		),
		&inventory,
	)
	return inventory
}

func loadJavaScriptRuntimeInventory(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JavaScript runtime inventory %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("unmarshal JavaScript runtime inventory %s: %v", path, err)
	}
}
