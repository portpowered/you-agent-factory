// Package root_composition proves root.BuildProcess wires the production ACP
// stdio server to the one canonical Chat Sessions authority: Process.ACPServer()
// serves real session/new and session/set_config_option JSON-RPC calls backed
// by real installed packaged Factories and a real persisted ACP Agent profile.
// The package is owned by the sessions functional domain.
package root_composition
