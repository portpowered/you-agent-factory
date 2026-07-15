package modelprovider

import "testing"

func TestSupportedReturnsDetachedCanonicalProviderIdentities(t *testing.T) {
	t.Parallel()
	first := Supported()
	if len(first) != 8 || first[0] != Claude || first[4] != Cursor {
		t.Fatalf("supported providers = %#v", first)
	}
	first[0] = "changed"
	if Supported()[0] != Claude {
		t.Fatal("Supported returned shared mutable state")
	}
}
