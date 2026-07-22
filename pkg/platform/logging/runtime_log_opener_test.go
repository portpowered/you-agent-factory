package logging

import (
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"go.uber.org/zap"
)

func TestRuntimeLogOpenerRejectsMissingExplicitSelections(t *testing.T) {
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatal(err)
	}
	opener, err := NewRuntimeLogOpener(paths)
	if err != nil {
		t.Fatal(err)
	}
	valid := RuntimeLogOpeningRequest{BaseLogger: zap.NewNop(), RuntimeInstanceID: "runtime", RootDirectory: t.TempDir(), StartTimeUTC: time.Now(), CollisionID: "collision"}
	tests := []struct {
		name string
		edit func(*RuntimeLogOpeningRequest)
		want string
	}{
		{name: "logger", edit: func(r *RuntimeLogOpeningRequest) { r.BaseLogger = nil }, want: "base logger is required"},
		{name: "root", edit: func(r *RuntimeLogOpeningRequest) { r.RootDirectory = "" }, want: "runtime log root is required"},
		{name: "clock", edit: func(r *RuntimeLogOpeningRequest) { r.StartTimeUTC = time.Time{} }, want: "runtime log start time is required"},
		{name: "id", edit: func(r *RuntimeLogOpeningRequest) { r.CollisionID = "" }, want: "runtime log collision ID is required"},
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
