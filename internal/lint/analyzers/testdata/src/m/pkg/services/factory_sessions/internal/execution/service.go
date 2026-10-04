package execution

import d "m/durabledefs"

func compose(root string) {
	d.NewJavaScriptRuntimeService(d.Config{})
	d.NewLazyProjectStore(d.DirForProjectRoot(root))
	_ = d.DirectoryStore{}
	d.NewExecutionService()
	d.BuildCanonicalRuntimeSessionEvents()
}
