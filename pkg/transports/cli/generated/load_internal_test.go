package generated

import (
	"testing"
)

func TestParseFamilyManifestRejectsInvalidPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{name: "invalid json", payload: []byte("{")},
		{name: "missing rootPath", payload: []byte(`{"commands":{"you":{"id":"you"}}}`)},
		{name: "missing commands", payload: []byte(`{"rootPath":"you","commands":{}}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseFamilyManifest(tc.payload, "test-family"); err == nil {
				t.Fatal("parseFamilyManifest() = nil, want error")
			}
		})
	}
}

func TestParseRepresentativeFamilyManifestRejectsInvalidPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{name: "invalid json", payload: []byte("{")},
		{name: "missing rootPath", payload: []byte(`{"commands":{"you":{"id":"you"}}}`)},
		{name: "missing commands", payload: []byte(`{"rootPath":"you","commands":{}}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseRepresentativeFamilyManifest(tc.payload); err == nil {
				t.Fatal("parseRepresentativeFamilyManifest() = nil, want error")
			}
		})
	}
}

func TestParseRepresentativeFamilyManifestAcceptsMinimalFamily(t *testing.T) {
	payload := []byte(`{
		"rootPath":"you",
		"commands":{
			"you":{"id":"you","path":"you"},
			"you.session":{"id":"you.session","path":"you session"},
			"you.session.show":{"id":"you.session.show","path":"you session show"}
		}
	}`)
	manifest, err := parseRepresentativeFamilyManifest(payload)
	if err != nil {
		t.Fatalf("parseRepresentativeFamilyManifest() error = %v", err)
	}
	if manifest.RootPath != "you" || len(manifest.Commands) != 3 {
		t.Fatalf("manifest = %#v, want rooted three-command family", manifest)
	}
	if _, err := manifest.CommandByID("you.session.show"); err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}
}

func TestFactoryConfigInitCommandByIDPropagatesManifestDecodeErrors(t *testing.T) {
	original := factoryConfigInitFamilyJSON
	t.Cleanup(func() { factoryConfigInitFamilyJSON = original })
	factoryConfigInitFamilyJSON = []byte("{")
	if _, err := FactoryConfigInitCommandByID("you.factory"); err == nil {
		t.Fatal("FactoryConfigInitCommandByID() invalid embed = nil, want error")
	}
}

func TestCommandByIDPropagatesManifestDecodeErrors(t *testing.T) {
	original := representativeFamilyJSON
	t.Cleanup(func() { representativeFamilyJSON = original })
	representativeFamilyJSON = []byte("{")
	if _, err := CommandByID("you"); err == nil {
		t.Fatal("CommandByID() invalid embed = nil, want error")
	}
}
