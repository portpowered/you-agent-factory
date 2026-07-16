package namedfactorypath

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type platformRootFixture struct {
	name      string
	root      string
	separator byte
}

var namedFactoryPlatformRoots = []platformRootFixture{
	{
		name:      "unix",
		root:      "/home/alice/.you-agent-factory/factories",
		separator: '/',
	},
	{
		name:      "windows",
		root:      `C:\Users\alice\.you-agent-factory\factories`,
		separator: '\\',
	},
}

type namedFactoryResolveCase struct {
	name         string
	wantSegments []string
}

var namedFactoryResolveCases = []namedFactoryResolveCase{
	{name: "alpha", wantSegments: []string{"alpha"}},
	{name: "@you/goal", wantSegments: []string{"@you", "goal"}},
	{name: "@you/tts", wantSegments: []string{"@you", "tts"}},
}

type namedFactoryRejectCase struct {
	name       string
	wantSubstr string
}

var namedFactoryRejectCases = []namedFactoryRejectCase{
	{name: "", wantSubstr: "factory name is required"},
	{name: "   ", wantSubstr: "factory name is required"},
	{name: "/absolute", wantSubstr: "cannot contain path separators"},
	{name: `C:\absolute`, wantSubstr: "cannot contain path separators"},
	{name: "../alpha", wantSubstr: "cannot contain path separators"},
	{name: "alpha/beta", wantSubstr: "cannot contain path separators"},
	{name: `alpha\beta`, wantSubstr: "cannot contain path separators"},
	{name: ".", wantSubstr: "not a valid directory name"},
	{name: "..", wantSubstr: "not a valid directory name"},
	{name: "@you", wantSubstr: "must be scoped as @scope/name"},
	{name: "@you/", wantSubstr: "must be scoped as @scope/name"},
	{name: "@you/tts/extra", wantSubstr: "must be scoped as @scope/name"},
	{name: "@you/../goal", wantSubstr: "must be scoped as @scope/name"},
	{name: "@you/./goal", wantSubstr: "must be scoped as @scope/name"},
	{name: "@you/..", wantSubstr: "not a valid directory name"},
	{name: "@you/.", wantSubstr: "not a valid directory name"},
	{name: "@you/foo/bar", wantSubstr: "must be scoped as @scope/name"},
	{name: `@you/foo\bar`, wantSubstr: "cannot contain path separators"},
}

func (fixture platformRootFixture) matchesCurrentOS() bool {
	switch fixture.name {
	case "unix":
		return runtime.GOOS != "windows"
	case "windows":
		return runtime.GOOS == "windows"
	default:
		return true
	}
}

func joinPlatformPath(separator byte, parts ...string) string {
	return strings.Join(parts, string(separator))
}

func TestCrossPlatformPathTable_ResolveCases(t *testing.T) {
	for _, fixture := range namedFactoryPlatformRoots {
		for _, tc := range namedFactoryResolveCases {
			t.Run(fixture.name+"/"+tc.name, func(t *testing.T) {
				segments, err := PathSegments(tc.name)
				if err != nil {
					t.Fatalf("PathSegments(%q): %v", tc.name, err)
				}
				if len(segments) != len(tc.wantSegments) {
					t.Fatalf("PathSegments(%q) = %#v, want %#v", tc.name, segments, tc.wantSegments)
				}
				for i := range tc.wantSegments {
					if segments[i] != tc.wantSegments[i] {
						t.Fatalf("segment[%d] = %q, want %q", i, segments[i], tc.wantSegments[i])
					}
				}

				got, err := MapDir(fixture.root, tc.name)
				if err != nil {
					t.Fatalf("MapDir(%q, %q): %v", fixture.root, tc.name, err)
				}
				want := joinPlatformPath(fixture.separator, append([]string{fixture.root}, tc.wantSegments...)...)
				if fixture.matchesCurrentOS() {
					if got != want {
						t.Fatalf("MapDir = %q, want %q", got, want)
					}
					return
				}
				for _, segment := range tc.wantSegments {
					if !strings.Contains(got, segment) {
						t.Fatalf("MapDir = %q missing segment %q", got, segment)
					}
				}
				if strings.Contains(got, "%2F") {
					t.Fatalf("MapDir = %q must not percent-encode scoped names", got)
				}
			})
		}
	}
}

func TestCrossPlatformPathTable_RejectCases(t *testing.T) {
	for _, fixture := range namedFactoryPlatformRoots {
		for _, tc := range namedFactoryRejectCases {
			t.Run(fixture.name+"/"+tc.name, func(t *testing.T) {
				_, err := PathSegments(tc.name)
				if err == nil {
					t.Fatalf("PathSegments(%q) expected error", tc.name)
				}
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Fatalf("PathSegments(%q) error = %v, want substring %q", tc.name, err, tc.wantSubstr)
				}

				_, err = MapDir(fixture.root, tc.name)
				if err == nil {
					t.Fatalf("MapDir(%q, %q) expected error", fixture.root, tc.name)
				}
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Fatalf("MapDir(%q, %q) error = %v, want substring %q", fixture.root, tc.name, err, tc.wantSubstr)
				}
			})
		}
	}
}

func TestCrossPlatformPathTable_PortableRelativeRoot(t *testing.T) {
	root := filepath.Join("home", ".you-agent-factory", "factories")
	for _, tc := range namedFactoryResolveCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MapDir(root, tc.name)
			if err != nil {
				t.Fatalf("MapDir(%q): %v", tc.name, err)
			}
			want := filepath.Join(append([]string{root}, tc.wantSegments...)...)
			if got != want {
				t.Fatalf("MapDir = %q, want %q", got, want)
			}
		})
	}
}
