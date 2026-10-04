package construct_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

func TestCompletedSettingsBoundaryRejectsMissingOwners(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		document   settingswire.DocumentService
		resolution settingswire.ResolutionService
		want       string
	}{
		{name: "document", want: "construct Operator Settings: document is required"},
		{name: "resolution", document: settingswire.NewDocumentService(nil, nil, nil, nil, nil, nil, nil),
			want: "construct Operator Settings: resolution is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, err := settingswire.NewService(test.document, test.resolution, nil, nil, nil,
				nil, nil, logging.NoopLogger{}, nil)
			if err == nil || err.Error() != test.want || root != nil {
				t.Fatalf("NewService() = %v, %v, want nil root and %q", root, err, test.want)
			}
		})
	}
}
