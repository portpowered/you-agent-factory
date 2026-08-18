package submit

import (
	"os"
	"testing"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
)

func submitForTest(t *testing.T, cfg SubmitConfig) error {
	t.Helper()
	read := workdomain.PayloadFileReader(func(path string) ([]byte, error) { return os.ReadFile(path) })
	cfg.HTTP = testHTTPProtocol(t)
	return submit(read, cfg)
}

func Submit(t *testing.T, cfg SubmitConfig) error { return submitForTest(t, cfg) }
