package migrationledgercheck

import "testing"

func TestPackagedFactoryInvocationMatrixExpandsJavaScriptFamilyConsumers(t *testing.T) {
	t.Parallel()

	got := packagedFactoryInvocationMatrixSlugsFromChecklist(map[string]struct{}{
		"tests/functional/factory/packaged/javascript_families/invocation_test.go": {},
	})
	for _, slug := range []string{"spawn", "tournament"} {
		if _, ok := got[slug]; !ok {
			t.Fatalf("matrix slugs = %#v, missing %q", got, slug)
		}
	}
	if len(got) != 2 {
		t.Fatalf("matrix slugs = %#v, want exactly the two JavaScript family factories", got)
	}
}
