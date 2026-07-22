package metrics

import (
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

func TestRuntimeMetricsOpenerRejectsMissingExplicitSelections(t *testing.T) {
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatal(err)
	}
	opener, err := NewRuntimeMetricsOpener(paths)
	if err != nil {
		t.Fatal(err)
	}
	valid := RuntimeMetricsOpeningRequest{RuntimeInstanceID: "runtime", RootDirectory: t.TempDir(), StartTimeUTC: time.Now(), CollisionID: "collision"}
	tests := []struct {
		name string
		edit func(*RuntimeMetricsOpeningRequest)
		want string
	}{
		{name: "root", edit: func(r *RuntimeMetricsOpeningRequest) { r.RootDirectory = "" }, want: "runtime metrics root is required"},
		{name: "clock", edit: func(r *RuntimeMetricsOpeningRequest) { r.StartTimeUTC = time.Time{} }, want: "runtime metrics start time is required"},
		{name: "id", edit: func(r *RuntimeMetricsOpeningRequest) { r.CollisionID = "" }, want: "runtime metrics collision ID is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.edit(&request)
			_, err := opener.Open(request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Open() error = %v, want %q", err, test.want)
			}
		})
	}
}
