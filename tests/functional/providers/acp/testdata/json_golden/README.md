# ACP SDK JSON goldens

The files under `upstream/` are verbatim JSON parameter/result fixtures from
`github.com/coder/acp-go-sdk@v0.13.5/testdata/json_golden` at commit
`0845a3bb9eddda5bfc22a94dd3598c90cb842451`.

They are used only by the functional test's raw JSON-RPC edge peer. Production
ACP traffic is encoded and decoded by `acp-go-sdk`; the repository does not
maintain a second production codec.

The allowlist and source checksums live in `manifest.json`. Refreshing a fixture
is an explicit dependency-upgrade operation: update the module version, copy
the allowlisted upstream file, update its checksum, and review the resulting
functional response-stream changes.
