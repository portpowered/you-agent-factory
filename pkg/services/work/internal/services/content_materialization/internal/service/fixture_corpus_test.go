package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	content "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/internal/service"
)

func TestURLMaterializationCorpus_LoadsRepresentativeCases(t *testing.T) {
	corpus, err := service.LoadURLMaterializationCorpus()
	if err != nil {
		t.Fatalf("LoadURLMaterializationCorpus: %v", err)
	}
	if len(corpus.Cases()) < 6 {
		t.Fatalf("case count = %d, want representative coverage", len(corpus.Cases()))
	}
	for _, want := range []string{
		"local_file_ok",
		"local_missing",
		"data_url_png",
		"ssrf_loopback",
		"remote_ok",
		"remote_404",
	} {
		if _, ok := corpus.Case(want); !ok {
			t.Fatalf("missing case %q", want)
		}
	}
}

func TestURLMaterializationCorpus_CasesMatchExpectedOutcomes(t *testing.T) {
	corpus, err := service.LoadURLMaterializationCorpus()
	if err != nil {
		t.Fatalf("LoadURLMaterializationCorpus: %v", err)
	}

	for _, tc := range corpus.Cases() {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			rawURL, opts, localPath := resolveURLMaterializationFixture(t, tc)
			gotPath, cleanup, err := service.MaterializeContentURL(context.Background(), rawURL, opts)
			defer cleanup()

			switch tc.Expect.Outcome {
			case "success":
				assertURLMaterializationSuccess(t, tc, gotPath, localPath, cleanup, err)
			case "error":
				assertURLMaterializationError(t, tc, err)
			default:
				t.Fatalf("unsupported expect.outcome %q", tc.Expect.Outcome)
			}
		})
	}
}

func resolveURLMaterializationFixture(t *testing.T, tc service.URLMaterializationCase) (string, *service.Options, string) {
	t.Helper()

	switch tc.Setup {
	case "":
		return tc.URL, newMaterializeTestOptions(), ""
	case "local_file":
		dir := t.TempDir()
		localPath := filepath.Join(dir, "fixture.png")
		if err := os.WriteFile(localPath, []byte(tc.FileContent), 0o644); err != nil {
			t.Fatalf("write local file: %v", err)
		}
		contentURL, err := content.FilesystemPathToContentURL(localPath)
		if err != nil {
			t.Fatalf("local file URL: %v", err)
		}
		return contentURL, newMaterializeTestOptions(), localPath
	case "httptest_server":
		body := tc.ResponseBody
		statusCode := tc.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.ContentType != "" {
				w.Header().Set("Content-Type", tc.ContentType)
			}
			w.WriteHeader(statusCode)
			if statusCode >= 200 && statusCode < 300 {
				_, _ = w.Write([]byte(body))
			}
		}))
		t.Cleanup(server.Close)

		opts := newMaterializeTestOptions()
		opts.AllowPrivateURLs = true
		opts.HTTPDoer = server.Client()
		if tc.MaxBytes > 0 {
			opts.MaxBytes = tc.MaxBytes
		}
		return server.URL, opts, ""
	default:
		t.Fatalf("unsupported setup %q", tc.Setup)
		return "", nil, ""
	}
}

func assertURLMaterializationSuccess(
	t *testing.T,
	tc service.URLMaterializationCase,
	gotPath, localPath string,
	cleanup func(),
	err error,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if tc.Expect.SameAsLocalPath && gotPath != localPath {
		t.Fatalf("path = %q, want local path %q", gotPath, localPath)
	}
	if tc.Expect.BodyMatch {
		body, readErr := os.ReadFile(gotPath)
		if readErr != nil {
			t.Fatalf("read materialized path: %v", readErr)
		}
		if string(body) != tc.ResponseBody {
			t.Fatalf("body = %q, want %q", body, tc.ResponseBody)
		}
	}
	if prefix := tc.Expect.TempPrefix; prefix != "" && !strings.Contains(filepath.Base(gotPath), strings.TrimSuffix(prefix, "*")) {
		t.Fatalf("path = %q, want temp prefix %q", gotPath, prefix)
	}
	if tc.Expect.TempRemovedOnCleanup {
		cleanup()
		if _, statErr := os.Stat(gotPath); !os.IsNotExist(statErr) {
			t.Fatalf("expected temp removed after cleanup, stat err=%v", statErr)
		}
	}
}

func assertURLMaterializationError(t *testing.T, tc service.URLMaterializationCase, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected materialization error")
	}
	for _, fragment := range tc.Expect.ErrorContains {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %v, want fragment %q", err, fragment)
		}
	}
}
