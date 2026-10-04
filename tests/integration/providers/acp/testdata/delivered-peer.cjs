// Real external ACP peer for the compiled-deliverable EOF boundary tests.
const readline = require("node:readline");
const mode = process.argv[2];
const fs = require("node:fs");
const attempts = process.argv[3];
const earlyFailure = mode === "initialize-disconnect-once" && !fs.existsSync(attempts);
if (mode === "initialize-disconnect-once") {
  fs.appendFileSync(attempts, earlyFailure ? "initialize-disconnect\n" : "success\n");
  process.on("exit", () => fs.appendFileSync(attempts + ".exits", earlyFailure ? "initialize-disconnect\n" : "success\n"));
}
let disconnect = mode === "prompt-disconnect" || mode === "prompt-secret-disconnect";
if (mode === "disconnect-once") {
  // Observe real launches, including an accidental retry of the failed request.
  disconnect = !fs.existsSync(attempts);
  fs.appendFileSync(attempts, disconnect ? "disconnect\n" : "success\n");
}
const input = readline.createInterface({ input: process.stdin });
const result = (id, value) => JSON.stringify({ jsonrpc: "2.0", id, result: value }) + "\n";
const flushAndExit = (payload) => process.stdout.write(payload, () => process.exit(0));
let control;
let promptID;
// Observe OS process existence from the already-running surviving peer so the
// cancellation witness needs no extra child. Timers retry a live process only;
// the Go control-socket deadline bounds failure, never signals success.
const observeExit = (pid) => {
  try {
    process.kill(pid, 0);
    setTimeout(() => observeExit(pid), 10);
  } catch (error) {
    if (error.code !== "ESRCH") throw error;
    control.write(process.argv[4] + " exited " + pid + "\n");
  }
};
const finish = () => {
  const notification = JSON.stringify({
    jsonrpc: "2.0", method: "session/update", params: {
      sessionId: "eof-fixture-session", update: {
        sessionUpdate: "agent_message_chunk",
        content: { type: "text", text: "delivered EOF primary result " + process.argv[4] },
      },
    },
  }) + "\n";
  flushAndExit(notification + result(promptID, { stopReason: "end_turn" }));
};
input.on("line", (line) => {
  const request = JSON.parse(line);
  if (request.method === "initialize") {
    if (earlyFailure) {
      process.stderr.write("early initialize disconnect\n", () => process.exit(0));
      return;
    }
    const reply = result(request.id, {
      protocolVersion: mode === "initialize-version" ? 999 : 1,
      agentCapabilities: {}, authMethods: [],
    });
    if (mode === "initialize-version") flushAndExit(reply);
    else process.stdout.write(reply);
  } else if (request.method === "session/new") {
    process.stdout.write(result(request.id, { sessionId: "eof-fixture-session", configOptions: [{ id: "model", name: "Model", category: "model", type: "select", currentValue: "fixture", options: [{ value: "fixture", name: "Fixture" }] }] }));
  } else if (request.method === "session/prompt") {
    if (mode === "controlled") {
      promptID = request.id;
      const [host, port] = attempts.split(":");
      control = require("node:net").connect(Number(port), host, () => {
        control.write(process.argv[4] + " started " + process.pid + "\n");
      });
      control.on("data", (data) => {
        if (data.toString().trim() === "release") finish();
        else if (data.toString().startsWith("observe-exit ")) {
          observeExit(Number(data.toString().trim().split(" ")[1]));
        }
      });
      return;
    }
    if (disconnect) {
      // A real pipe EOF before any result must remain a failed attempt.
      const secret = process.env.ACP_TEST_API_TOKEN;
      if (mode === "prompt-secret-disconnect" && !secret) {
        // Fail loudly if the authored workstation did not supply its secret.
        process.stderr.write("missing configured token\n", () => process.exit(1));
      } else if (secret) {
        process.stderr.write("agent diagnostic token=" + secret + "\n", () => process.exit(0));
      } else process.exit(0);
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
  } else if (request.method === "session/cancel" && mode === "controlled") {
    control.write(process.argv[4] + " cancelled\n", () => {
      flushAndExit(result(promptID, { stopReason: "cancelled" }));
    });
  } else if (request.id !== undefined) {
    process.stdout.write(result(request.id, {}));
  }
});
input.on("close", () => process.exit(0));
