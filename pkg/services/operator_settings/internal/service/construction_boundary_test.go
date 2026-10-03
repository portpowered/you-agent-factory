package service_test

import (
	"testing"

	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

// These inert leaves deliberately have no operations: only construction's
// existing nil classification is under test, not either owner's behavior.
type constructionDocument struct{ settingsdocument.Service }
type constructionResolution struct{ resolution.Service }

func TestConstructionBoundaryNilAndTypedNilClassification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		document   settingsdocument.Service
		resolution resolution.Service
		wantError  string
	}{
		{"nil document", nil, &constructionResolution{}, "construct Operator Settings: document is required"},
		{"nil resolution", &constructionDocument{}, nil, "construct Operator Settings: resolution is required"},
		{"typed nil document", (*constructionDocument)(nil), &constructionResolution{}, ""},
		{"typed nil resolution", &constructionDocument{}, (*constructionResolution)(nil), ""},
		{"optional effects absent", &constructionDocument{}, &constructionResolution{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, err := operatorservice.New(tc.document, tc.resolution, nil, nil, nil, nil, nil, nil)
			if tc.wantError != "" {
				if root != nil || err == nil || err.Error() != tc.wantError {
					t.Fatalf("New = (%v, %v), want %q", root, err, tc.wantError)
				}
			} else if root == nil || err != nil {
				t.Fatalf("New = (%v, %v), want inert accepted construction", root, err)
			}
		})
	}
}
