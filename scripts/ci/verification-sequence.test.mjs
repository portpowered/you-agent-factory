import assert from "node:assert/strict";
import test from "node:test";
import net from "node:net";
import { once } from "node:events";
import { runCommand, runConcurrent, runSequence } from "./verification-sequence.mjs";
import { apiPlan, frontendPlan, workflowPlan } from "./verification-plans.mjs";

function deferred() {
  let resolve;
  const promise = new Promise((done) => { resolve = done; });
  return { promise, resolve };
}

test("split frontend plans preserve all commands and budgets without duplicate suites", () => {
  const component = frontendPlan("component");
  const browser = frontendPlan("browser");
  assert.deepEqual([...component, ...browser], frontendPlan());
  assert.deepEqual(component.map((step) => step.name), ["component"]);
  assert.deepEqual(browser.map((step) => step.name), ["dashboard-browser", "storybook"]);
  assert.throws(() => frontendPlan("typo"), /Unknown frontend suite/);
});

for (const suite of ["component", "browser"]) {
  test(`${suite} job rejects each failed suite and joins its complete peers`, async () => {
    const plan = frontendPlan(suite);
    for (const failed of plan) {
      const attempted = [];
      const messages = [];
      const ok = await runConcurrent(plan, {
        log: (message) => messages.push(message),
        run: async (step) => {
          attempted.push(step.name);
          return step.name !== failed.name;
        },
      });
      assert.equal(ok, false);
      assert.deepEqual(attempted, plan.map((step) => step.name));
      assert.ok(messages.some((message) => message.startsWith(`FAIL ${failed.name}`)));
    }
  });
}

test("concurrent suites all start before completion and success waits for the last peer", async () => {
  const plan = frontendPlan();
  const gates = plan.map(() => deferred());
  const started = [];
  const messages = [];
  let joined = false;
  const running = runConcurrent(plan, {
    log: (message) => messages.push(message),
    run: async (step) => {
      started.push(step.name);
      return gates[plan.indexOf(step)].promise;
    },
  }).then((ok) => { joined = true; return ok; });
  assert.deepEqual(started, plan.map((step) => step.name));
  assert.equal(messages.some((message) => message.startsWith("PASS")), false);
  gates[0].resolve(true);
  gates[1].resolve(true);
  await Promise.all([gates[0].promise, gates[1].promise]);
  assert.equal(joined, false);
  gates[2].resolve(true);
  assert.equal(await running, true);
  assert.equal(messages.filter((message) => message.startsWith("PASS")).length, 3);
});

for (const kind of ["nonzero", "spawn error"]) {
  test(`concurrent ${kind} is attributed and independent peers are joined`, async () => {
    for (const failed of frontendPlan()) {
      const peer = deferred();
      const attempted = [];
      const messages = [];
      let joined = false;
      const running = runConcurrent(frontendPlan(), {
        log: (message) => messages.push(message),
        run: async (step) => {
          attempted.push(step.name);
          if (step.name === failed.name) {
            if (kind === "spawn error") throw new Error("unavailable executable");
            return false;
          }
          return peer.promise;
        },
      }).then((ok) => { joined = true; return ok; });
      assert.equal(attempted.length, 3);
      await Promise.resolve();
      assert.equal(joined, false);
      peer.resolve(true);
      assert.equal(await running, false);
      assert.ok(messages.some((message) => message.startsWith(`FAIL ${failed.name}`)));
      assert.equal(messages.filter((message) => message.startsWith("PASS")).length, 2);
    }
  });
}

test("concurrent cancellation signals every active peer and joins their cleanup", async () => {
  const controller = new AbortController();
  const cleaned = [];
  const running = runConcurrent(frontendPlan(), {
    signal: controller.signal, log() {},
    run: (step, signal) => new Promise((resolve) => {
      signal.addEventListener("abort", () => { cleaned.push(step.name); resolve(false); }, { once: true });
    }),
  });
  controller.abort();
  assert.equal(await running, false);
  assert.deepEqual(cleaned, frontendPlan().map((step) => step.name));
  const attempted = [];
  assert.equal(await runConcurrent(frontendPlan(), {
    signal: controller.signal, log() {}, run: async (step) => { attempted.push(step); return true; },
  }), false);
  assert.deepEqual(attempted, []);
});

test("concurrent entry rejects dependent plans before starting any command", async () => {
  await assert.rejects(runConcurrent(apiPlan({}), { run() { assert.fail("must not start"); } }), /must be independent/);
});

for (const [area, plan] of [["frontend", frontendPlan()], ["API", apiPlan({})], ["workflow", workflowPlan()]]) {
  test(`${area}: each child failure is red and later independent proofs are attempted`, async () => {
    for (const failed of plan) {
      const attempted = [];
      const messages = [];
      const ok = await runSequence(plan, {
        log: (message) => messages.push(message),
        run: async (step) => {
          attempted.push(step.name);
          return step.name !== failed.name;
        },
      });
      assert.equal(ok, false, failed.name);
      assert.ok(messages.includes(`FAIL ${failed.name}`));
      for (const step of plan) {
        assert.equal(attempted.includes(step.name), !step.needs?.includes(failed.name), step.name);
      }
    }
  });
  test(`${area}: successful proofs pass; spawn exceptions fail safely`, async () => {
    assert.equal(await runSequence(plan, { run: async () => true, log() {} }), true);
    const attempted = [];
    assert.equal(await runSequence(plan, { log() {}, run: async (step) => {
      attempted.push(step.name);
      if (step === plan[0]) throw new Error("setup unavailable");
      return true;
    } }), false);
    assert.ok(attempted.includes(plan.at(-1).name));
  });
}

test("cancellation stops the active proof and does not attempt later proofs", async () => {
  const controller = new AbortController();
  const attempted = [];
  assert.equal(await runSequence(frontendPlan(), { signal: controller.signal, log() {},
    run: async (step, signal) => {
      attempted.push(step.name);
      controller.abort();
      assert.equal(signal.aborted, true);
      return false;
    },
  }), false);
  assert.deepEqual(attempted, ["component"]);
});

// Small integration cases consume Node itself; no binary is compiled here.
test("command timeout and cancellation terminate an owned child and report failure", async () => {
  const step = { command: process.execPath, args: ["-e", "setInterval(() => {}, 1000)"], timeoutMs: 100 };
  assert.equal(await runCommand(step), false);
  const controller = new AbortController();
  const running = runCommand({ ...step, timeoutMs: 5000 }, controller.signal);
  controller.abort();
  assert.equal(await running, false);
});

test("command success, failure and spawn errors have distinct results", async () => {
  for (const code of [0, 1]) {
    assert.equal(await runCommand({ command: process.execPath, args: ["-e", `process.exit(${code})`], timeoutMs: 5000 }), code === 0);
  }
  await assert.rejects(runCommand({ command: "missing-ci-command-4f51", args: [], timeoutMs: 5000 }), /ENOENT/);
});

// The descendant holds a real socket open. Socket closure observes process
// cleanup without source inspection, PID reuse assumptions or fixed sleeps.
for (const mode of ["abort", "timeout"]) {
  test(`owned descendant drains after ${mode}`, { timeout: 15000 }, async (t) => {
    const server = net.createServer();
    t.after(() => server.close());
    server.listen(0, "127.0.0.1");
    await once(server, "listening");
    const connection = once(server, "connection");
    const descendant = `require('node:net').connect(${server.address().port}, '127.0.0.1');`;
    const parent = `require('node:child_process').spawn(process.execPath, ['-e', ${JSON.stringify(descendant)}], {stdio: 'inherit'}); setInterval(() => {}, 1000);`;
    const controller = new AbortController();
    t.after(() => controller.abort());
    const running = runCommand({
      command: process.execPath, args: ["-e", parent],
      timeoutMs: mode === "timeout" ? 2000 : 10000,
    }, controller.signal);
    const [socket] = await connection;
    t.after(() => socket.destroy());
    socket.on("error", (error) => assert.equal(error.code, "ECONNRESET"));
    const closed = new Promise((resolve) => socket.once("close", resolve));
    if (mode === "abort") controller.abort();
    assert.equal(await running, false);
    await closed;
    assert.equal(socket.destroyed, true);
  });
}
