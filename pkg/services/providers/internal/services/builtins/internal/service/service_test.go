package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const validCatalogDocument = `{
  "acp": [{
    "name": " cursor-acp ",
    "aliases": ["cursor-test"],
    "transport": " STDIO ",
    "command": " cursor-agent acp ",
    "arguments": ["acp"],
    "posture": " installed_executable ",
    "implementation": {"kind": "acp_agent", "profile": "cursor-acp"}
  }]
}`

func TestNewBuildsDetachedRuntimeProjection(t *testing.T) {
	service, err := New([]byte(validCatalogDocument))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first := service.ACPIntegrations()
	want := []struct {
		ID                    string
		Name                  string
		Aliases               []string
		Transport             string
		Command               string
		Arguments             []string
		RuntimePosture        string
		ImplementationProfile string
	}{
		{
			ID:                    "cursor-acp",
			Name:                  "cursor-acp",
			Aliases:               []string{"cursor-test"},
			Transport:             "stdio",
			Command:               "cursor-agent acp",
			Arguments:             []string{"acp"},
			RuntimePosture:        "installed_executable",
			ImplementationProfile: "cursor-acp",
		},
	}
	if len(first) != len(want) {
		t.Fatalf("ACPIntegrations() count = %d, want %d", len(first), len(want))
	}
	for index, got := range first {
		if got.ID != want[index].ID || got.Name.String() != want[index].Name || got.Transport != want[index].Transport || got.Command != want[index].Command || got.RuntimePosture != want[index].RuntimePosture || got.ImplementationProfile != want[index].ImplementationProfile || !reflect.DeepEqual(got.Aliases, want[index].Aliases) || !reflect.DeepEqual(got.Arguments, want[index].Arguments) {
			t.Fatalf("ACPIntegrations()[%d] = %#v, want %#v", index, got, want[index])
		}
	}
	first[0].Aliases[0] = "mutated"
	first[0].Arguments[0] = "mutated"
	second := service.ACPIntegrations()
	if second[0].Aliases[0] != "cursor-test" || second[0].Arguments[0] != "acp" {
		t.Fatalf("ACPIntegrations() retained caller mutation: %#v", second[0])
	}
}

func TestNewLoadsLosslessQuotedRuntimeArguments(t *testing.T) {
	wantArguments := []string{"hello world", "semi;colon", "quote's"}
	document, err := json.Marshal(catalogDocument{ACP: []catalogACPIntegration{{
		Name:      "quoted-acp",
		Transport: "stdio",
		Command:   `agent 'hello world' 'semi;colon' 'quote'\''s'`,
		Arguments: wantArguments,
		Posture:   "installed_executable",
		Implementation: runtimeImplementation{
			Kind:    "acp_agent",
			Profile: "cursor-acp",
		},
	}}})
	if err != nil {
		t.Fatalf("marshal catalog document: %v", err)
	}

	service, err := New(document)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	integration := service.ACPIntegrations()[0]
	if integration.Command != `agent 'hello world' 'semi;colon' 'quote'\''s'` {
		t.Fatalf("integration command = %q, want lossless command", integration.Command)
	}
	if !reflect.DeepEqual(integration.Arguments, wantArguments) {
		t.Fatalf("integration arguments = %#v, want %#v", integration.Arguments, wantArguments)
	}
}

func TestNewRejectsMalformedPackagedACPEntries(t *testing.T) {
	const base = `{
  "acp": [{
    "name": "cursor-acp",
    "transport": "stdio",
    "command": "cursor-agent acp",
    "arguments": ["acp"],
    "posture": "installed_executable",
    "implementation": {"kind": "acp_agent", "profile": "cursor-acp"}
  }]
}`

	tests := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{
			name: "invalid json",
			mutate: func(string) string {
				return "{"
			},
			want: "decode packaged provider catalog",
		},
		{
			name: "empty canonical identity",
			mutate: func(document string) string {
				return strings.Replace(document, `"name": "cursor-acp"`, `"name": " "`, 1)
			},
			want: "provider id is invalid",
		},
		{
			name: "unsupported transport",
			mutate: func(document string) string {
				return strings.Replace(document, `"transport": "stdio"`, `"transport": "tcp"`, 1)
			},
			want: "unsupported transport",
		},
		{
			name: "empty command",
			mutate: func(document string) string {
				return strings.Replace(document, `"command": "cursor-agent acp"`, `"command": ""`, 1)
			},
			want: "invalid command",
		},
		{
			name: "shell parse failure",
			mutate: func(document string) string {
				return strings.Replace(document, `"command": "cursor-agent acp"`, `"command": "'"`, 1)
			},
			want: "invalid command",
		},
		{
			name: "empty runtime profile",
			mutate: func(document string) string {
				return strings.Replace(document, `"profile": "cursor-acp"`, `"profile": " "`, 1)
			},
			want: "incomplete runtime binding",
		},
		{
			name: "argument length drift",
			mutate: func(document string) string {
				return strings.Replace(document, `"arguments": ["acp"]`, `"arguments": []`, 1)
			},
			want: "command arguments drift",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New([]byte(test.mutate(base)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestNewRejectsIdentityAndAliasCollisions(t *testing.T) {
	const baseEntry = `{
    "name": "%s",
    "aliases": %s,
    "transport": "stdio",
    "command": "cursor-agent acp",
    "arguments": ["acp"],
    "posture": "installed_executable",
    "implementation": {"kind": "acp_agent", "profile": "cursor-acp"}
  }`
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "duplicate canonical identity",
			doc:  `{"acp": [` + fmt.Sprintf(baseEntry, "cursor-acp", "[]") + `,` + fmt.Sprintf(baseEntry, "CURSOR-ACP", "[]") + `]}`,
			want: "is duplicated",
		},
		{
			name: "invalid alias",
			doc:  `{"acp": [` + fmt.Sprintf(baseEntry, "cursor-acp", `[""]`) + `]}`,
			want: "alias: provider id is invalid",
		},
		{
			name: "canonical alias",
			doc:  `{"acp": [` + fmt.Sprintf(baseEntry, "cursor-acp", `["cursor-acp"]`) + `]}`,
			want: "duplicates its canonical identity",
		},
		{
			name: "duplicate aliases",
			doc:  `{"acp": [` + fmt.Sprintf(baseEntry, "cursor-acp", `["cursor-test","CURSOR-TEST"]`) + `]}`,
			want: "is duplicated",
		},
		{
			name: "alias collides with canonical identity",
			doc:  `{"acp": [` + fmt.Sprintf(baseEntry, "cursor-acp", "[]") + `,` + fmt.Sprintf(baseEntry, "second-acp", `["CURSOR-ACP"]`) + `]}`,
			want: "collides with",
		},
		{
			name: "alias collides with prior alias",
			doc:  `{"acp": [` + fmt.Sprintf(baseEntry, "cursor-acp", `["shared-acp"]`) + `,` + fmt.Sprintf(baseEntry, "second-acp", `["SHARED-ACP"]`) + `]}`,
			want: "collides with",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New([]byte(test.doc))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestNewRejectsRuntimeProjectionBindingDrift(t *testing.T) {
	const base = `{
  "acp": [{
    "name": "cursor-acp",
    "transport": "stdio",
    "command": "cursor-agent acp",
    "arguments": ["acp"],
    "posture": "installed_executable",
    "implementation": {"kind": "acp_agent", "profile": "cursor-acp"}
  }]
}`

	tests := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{
			name: "unknown profile",
			mutate: func(document string) string {
				return strings.Replace(document, `"profile": "cursor-acp"`, `"profile": "missing-profile"`, 1)
			},
			want: "unknown runtime profile",
		},
		{
			name: "wrong implementation kind",
			mutate: func(document string) string {
				return strings.Replace(document, `"kind": "acp_agent"`, `"kind": "native_cli"`, 1)
			},
			want: "unsupported implementation kind",
		},
		{
			name: "catalog-only posture",
			mutate: func(document string) string {
				return strings.Replace(document, `"posture": "installed_executable"`, `"posture": "catalog_only"`, 1)
			},
			want: "invalid launch posture",
		},
		{
			name: "argument drift",
			mutate: func(document string) string {
				return strings.Replace(document, `"arguments": ["acp"]`, `"arguments": ["wrong"]`, 1)
			},
			want: "command arguments drift",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New([]byte(test.mutate(base)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want containing %q", err, test.want)
			}
		})
	}
}
