package layouttests

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestCloneFactoryConfigDetachesEntityDescriptions(t *testing.T) {
	description := func(id string) *factorydefinitions.NameValueConfig {
		return &factorydefinitions.NameValueConfig{
			Type:    factorydefinitions.NameValueTypeLocalizableAsset,
			Value:   id,
			Locales: []string{"en-US"},
			Values:  map[string]string{"fr-FR": id + " FR"},
			ID:      id,
		}
	}
	original := &factorydefinitions.FactoryConfig{
		Name:         "described",
		Description:  description("factory"),
		WorkTypes:    []factorydefinitions.WorkTypeConfig{{Name: "task", Description: description("work-type")}},
		Workers:      []factorydefinitions.FactoryWorkerConfig{{Name: "worker", Description: description("worker")}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{Name: "station", Description: description("station")}},
	}

	cloned, err := factorydefinitions.CloneFactoryConfig(original)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	original.Description.Values["fr-FR"] = "changed"
	original.WorkTypes[0].Description.Locales[0] = "de-DE"
	original.Workers[0].Description.Value = "changed"
	original.Workstations[0].Description.ID = "changed"

	if cloned.Description.Values["fr-FR"] != "factory FR" ||
		cloned.WorkTypes[0].Description.Locales[0] != "en-US" ||
		cloned.Workers[0].Description.Value != "worker" ||
		cloned.Workstations[0].Description.ID != "station" {
		t.Fatalf("clone shares description storage: %#v", cloned)
	}
}
