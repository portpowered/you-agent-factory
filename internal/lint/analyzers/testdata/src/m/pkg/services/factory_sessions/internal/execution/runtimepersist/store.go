package runtimepersist

import d "m/durabledefs"

func persistence(root string) {
	d.NewLazyProjectStore(d.DirForProjectRoot(root))
	_ = d.DirectoryStore{}
	d.MapCanonicalRuntimeSessionEvents()
}
