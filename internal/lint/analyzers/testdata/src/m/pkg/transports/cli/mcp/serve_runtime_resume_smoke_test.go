package mcp

import d "m/durabledefs"

func persistence() {
	d.NewLazyProjectStore(d.DirForProjectRoot(""))
	_ = d.DirectoryStore{}
	d.NewExecutionService()
}
