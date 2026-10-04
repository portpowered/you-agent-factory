// Real external ACP peer for the compiled-deliverable EOF boundary tests.
const readline = require("node:readline");
const mode = process.argv[2];
const input = readline.createInterface({ input: process.stdin });
const result = (id, value) => JSON.stringify({ jsonrpc: "2.0", id, result: value }) + "\n";
const flushAndExit = (payload) => process.stdout.write(payload, () => process.exit(0));
input.on("line", (line) => {
  const request = JSON.parse(line);
  if (request.method === "initialize") {
    const reply = result(request.id, {
      protocolVersion: mode === "initialize-version" ? 999 : 1,
      agentCapabilities: {}, authMethods: [],
    });
    if (mode === "initialize-version") flushAndExit(reply);
    else process.stdout.write(reply);
  } else if (request.method === "session/new") {
    process.stdout.write(result(request.id, { sessionId: "eof-fixture-session", configOptions: [{ id: "model", name: "Model", category: "model", type: "select", currentValue: "fixture", options: [{ value: "fixture", name: "Fixture" }] }] }));
  } else if (request.method === "session/prompt") {
    if (mode === "prompt-disconnect") {
      // A real pipe EOF before any result must remain a failed attempt.
      process.exit(0);
      return;
    }
    const notification = JSON.stringify({
      jsonrpc: "2.0", method: "session/update", params: {
        sessionId: "eof-fixture-session", update: {
          sessionUpdate: "agent_message_chunk",
          content: { type: "text", text: "delivered EOF primary result" },
        },
      },
    }) + "\n";
    flushAndExit(notification + result(request.id, { stopReason: "end_turn" }));
  } else if (request.id !== undefined) {
    process.stdout.write(result(request.id, {}));
  }
});
input.on("close", () => process.exit(0));
