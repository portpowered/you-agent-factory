# Bundled Pi ACP bridge

This directory is a repository-owned fork of [`pi-acp` 0.0.34](https://github.com/svkozak/pi-acp/tree/v0.0.34), by Sergii Kozak, under the MIT license in [LICENSE](LICENSE). The source was imported from upstream commit `b0581c9c1d675e634234674484247008b03d69b4`. The package is private and is not published to npm.

The bundled runtime dependencies and their licenses are recorded in [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt).

The fork retains the final typed assistant `message_end` stop reason until `agent_settled`. A final `error` rejects ACP `session/prompt` with a JSON-RPC internal error; a failed attempt followed by a successful retry returns `end_turn`. It does not inspect text or session files.

Run `npm ci`, `npm test`, `npm run typecheck`, and `npm run build` here after source changes. Commit `dist/index.js` with source changes. `cmd/factory/pi_acp.go` embeds that bundle into the `you` executable. The Pi provider launches `you pi-acp`, which extracts the bundle into a temporary `.mjs` file and runs it with Node.js. Node.js 20+ and Pi 0.81.0+ must be on `PATH`.
