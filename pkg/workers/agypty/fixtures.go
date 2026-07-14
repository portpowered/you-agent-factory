package agypty

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed testdata/argv_fixtures.json
var argvFixturesJSON []byte

//go:embed testdata/workspace_fixtures.json
var workspaceFixturesJSON []byte

type argvFixtureFile struct {
	Fixtures []argvFixture `json:"fixtures"`
}

type argvFixture struct {
	Name     string   `json:"name"`
	Spec     *ArgvSpec `json:"spec,omitempty"`
	Argv     []string `json:"argv,omitempty"`
	WantArgv []string `json:"want_argv,omitempty"`
	WantError string  `json:"want_error,omitempty"`
}

type workspaceFixtureFile struct {
	Fixtures []workspaceFixture `json:"fixtures"`
}

type workspaceFixture struct {
	Name        string   `json:"name"`
	FactoryRoot string   `json:"factory_root"`
	RawPath     string   `json:"raw_path"`
	WantSuffix  []string `json:"want_suffix,omitempty"`
	WantError   string   `json:"want_error,omitempty"`
}

// LoadArgvFixtures returns hermetic argv corpus entries for unit tests.
func LoadArgvFixtures() ([]argvFixture, error) {
	var file argvFixtureFile
	if err := json.Unmarshal(argvFixturesJSON, &file); err != nil {
		return nil, fmt.Errorf("agypty: decode argv fixtures: %w", err)
	}
	return file.Fixtures, nil
}

// LoadWorkspaceFixtures returns hermetic workspace path corpus entries for unit tests.
func LoadWorkspaceFixtures() ([]workspaceFixture, error) {
	var file workspaceFixtureFile
	if err := json.Unmarshal(workspaceFixturesJSON, &file); err != nil {
		return nil, fmt.Errorf("agypty: decode workspace fixtures: %w", err)
	}
	return file.Fixtures, nil
}
