//go:build linux

package process

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestOwnedLinuxGroupObservation(t *testing.T) {
	t.Parallel()
	unreadable := errors.New("procfs unavailable")
	for _, test := range []struct {
		name    string
		procFS  fs.FS
		live    bool
		wantErr bool
	}{
		{name: "live owned descendant", procFS: fstest.MapFS{
			"46/stat": {Data: []byte("46 (grandchild) S 45 42 42")},
		}, live: true},
		{name: "joined group and unrelated survivor", procFS: fstest.MapFS{
			"42/stat":   {Data: []byte("42 (root) Z 1 42 42")},
			"46/stat":   {Data: []byte("46 (grandchild) X 1 42 42")},
			"52/stat":   {Data: []byte("52 (sibling) R 1 52 52")},
			"self/stat": {Data: []byte("ignored nonnumeric entry")},
		}},
		{name: "disappeared process", procFS: fstest.MapFS{
			"46": {Mode: fs.ModeDir},
		}},
		{name: "malformed process fact", procFS: fstest.MapFS{
			"46/stat": {Data: []byte("not a process stat")},
		}, wantErr: true},
		{name: "unreadable directory", procFS: failingProcFS{path: ".", err: unreadable}, wantErr: true},
		{name: "unreadable stat", procFS: failingProcFS{
			FS:   fstest.MapFS{"46/stat": {Data: []byte("46 (child) R 1 42 42")}},
			path: "46/stat", err: unreadable,
		}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			live, err := ownedLinuxGroupRunning(test.procFS, 42)
			if live != test.live || (err != nil) != test.wantErr {
				t.Fatalf("owned group running=%t error=%v; want running=%t error=%t", live, err, test.live, test.wantErr)
			}
		})
	}
}

type failingProcFS struct {
	fs.FS
	path string
	err  error
}

func (proc failingProcFS) Open(path string) (fs.File, error) {
	if path == proc.path {
		return nil, proc.err
	}
	return proc.FS.Open(path)
}

func TestOwnedLinuxControlRequiresProcessFacts(t *testing.T) {
	t.Parallel()
	tree := &commandProcessTree{pgid: 42}
	if control := tree.ownedControl(make(chan struct{}), nil, nil); control != nil {
		t.Fatal("missing process observation must not publish a force capability")
	}
}

func TestOwnedLinuxGroupStat(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		stat string
		live bool
		bad  bool
	}{
		{name: "owned root", stat: "42 (provider) S 1 42 42", live: true},
		{name: "owned grandchild", stat: "46 (child (with spaces)) R 45 42 42", live: true},
		{name: "unrelated live group", stat: "52 (sibling) S 1 52 52"},
		{name: "exited zombie", stat: "46 (child) Z 1 42 42"},
		{name: "exited dead", stat: "46 (child) X 1 42 42"},
		{name: "missing command", stat: "42 S 1 42 42", bad: true},
		{name: "missing group", stat: "42 (provider) S 1", bad: true},
		{name: "invalid group", stat: "42 (provider) S 1 invalid", bad: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			live, err := linuxStatGroupRunning(test.stat, 42)
			if live != test.live || (err != nil) != test.bad {
				t.Fatalf("group liveness = %t, error = %v; want live=%t bad=%t", live, err, test.live, test.bad)
			}
		})
	}
}
