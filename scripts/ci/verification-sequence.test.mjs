import assert from "node:assert/strict";
import test from "node:test";
import { runCommand, runSequence } from "./verification-sequence.mjs";
import { apiPlan, frontendPlan, workflowPlan } from "./verification-plans.mjs";

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
