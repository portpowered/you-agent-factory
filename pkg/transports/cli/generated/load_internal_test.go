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
		{name: "missing rootPath", payload: []byte(`{"commands":{"you.work":{"id":"you.work"}}}`)},
		{name: "missing commands", payload: []byte(`{"rootPath":"you","commands":{}}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseFamilyManifest(tc.payload, "work"); err == nil {
				t.Fatal("parseFamilyManifest(work) = nil, want error")
			}
		})
	}
}

func TestParseWorkFamilyManifestAcceptsMinimalFamily(t *testing.T) {
	payload := []byte(`{
		"rootPath":"you",
		"commands":{
			"you.work":{"id":"you.work","path":"you work"},
			"you.work.list":{"id":"you.work.list","path":"you work list"}
		}
	}`)
	manifest, err := parseFamilyManifest(payload, "work")
	if err != nil {
		t.Fatalf("parseFamilyManifest(work) error = %v", err)
	}
	if manifest.RootPath != "you" || len(manifest.Commands) != 2 {
		t.Fatalf("manifest = %#v, want rooted two-command family", manifest)
	}
	if _, err := manifest.CommandByID("you.work.list"); err != nil {
		t.Fatalf("CommandByID(you.work.list) error = %v", err)
	}
}

func TestWorkCommandByIDPropagatesManifestDecodeErrors(t *testing.T) {
	original := workFamilyJSON
	t.Cleanup(func() { workFamilyJSON = original })
	workFamilyJSON = []byte("{")
	if _, err := WorkCommandByID("you.work"); err == nil {
		t.Fatal("WorkCommandByID() invalid embed = nil, want error")
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
			if _, err := parseFamilyManifest(tc.payload, "representative"); err == nil {
				t.Fatal("parseFamilyManifest(representative) = nil, want error")
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
	manifest, err := parseFamilyManifest(payload, "representative")
	if err != nil {
		t.Fatalf("parseFamilyManifest(representative) error = %v", err)
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
