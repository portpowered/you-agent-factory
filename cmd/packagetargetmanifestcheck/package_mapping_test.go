package main

import "testing"

type committedOwnerPackageMappingCase struct {
	path        string
	want        PackageMapping
	wantRetain  bool
	retainOwner string
}

func assertCommittedOwnerPackageMapping(t *testing.T, tc committedOwnerPackageMappingCase) {
	t.Helper()

	got, ok := mapCommittedOwnerPackage(tc.path)
	if !ok {
		t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", tc.path)
	}
	if tc.wantRetain {
		if got.Disposition != DispositionRetain || got.Destination != tc.retainOwner {
			t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want retain→%s", tc.path, got, tc.retainOwner)
		}
		return
	}
	if got != tc.want {
		t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want %#v", tc.path, got, tc.want)
	}
}
