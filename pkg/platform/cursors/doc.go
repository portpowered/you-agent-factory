// Package cursors discovers and parses Cursor cursor-agent CLI session storage
// in-process.
//
// v1 required backend: cursor-agent CLI storage under ~/.config/cursor/chats or ~/.cursor/chats
// (Linux, macOS, Windows). Cursor desktop globalStorage is documented as optional/future and is
// not required for v1.
//
// Parsing logic is ported from github.com/iksnae/cursor-session (MIT) at commit
// 340f0f72a760ba8b454eac814f986d1a6a4c2f57. See NOTICE.md in this directory for attribution.
// Runtime code does not shell out to the cursor-session CLI.
package cursors
