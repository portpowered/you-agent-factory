package wire

import (
	"testing"
	"time"
)

func TestCLIHTTPProfilesPreserveCommandTimeouts(t *testing.T) {
	t.Parallel()
	standard, err := provideStandardCLIHTTPProtocol()
	if err != nil {
		t.Fatal(err)
	}
	extended, err := provideExtendedCLIHTTPProtocol()
	if err != nil {
		t.Fatal(err)
	}
	if standard.Protocol == nil || standard.timeout != 10*time.Second {
		t.Fatalf("standard profile = %#v", standard)
	}
	if extended.Protocol == nil || extended.timeout != 15*time.Second {
		t.Fatalf("extended profile = %#v", extended)
	}
}
