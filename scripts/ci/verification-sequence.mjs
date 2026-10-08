import { spawn } from "node:child_process";

// Independent proofs continue after ordinary failures. Failed prerequisites
// suppress only their consumers; cancellation stops the entire sequence.
export async function runSequence(steps, { run = runCommand, signal, log = console.log } = {}) {
  const results = new Map();
  for (const step of steps) {
    if (signal?.aborted) return false;
    if (step.needs?.some((name) => results.get(name) !== true)) {
      log(`SKIP ${step.name}: prerequisite failed`);
      results.set(step.name, false);
      continue;
    }
    log(`RUN ${step.name}`);
    try {
      const ok = await run(step, signal);
      results.set(step.name, ok);
      log(`${ok ? "PASS" : "FAIL"} ${step.name}`);
    } catch (error) {
      log(`FAIL ${step.name}: ${error.message}`);
      results.set(step.name, false);
    }
  }
  return !signal?.aborted && [...results.values()].every(Boolean);
}

export function runCommand(step, signal) {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) return resolve(false);
    const grouped = process.platform !== "win32";
    const child = spawn(step.command, step.args, {
      cwd: step.cwd, env: { ...process.env, ...step.env },
      stdio: "inherit", detached: grouped, windowsHide: true,
    });
    let stopped = false;
    const stop = () => {
      stopped = true;
      if (!child.pid) return;
      // Kill the owned group, including make/Bun/browser grandchildren.
      try {
        if (grouped) process.kill(-child.pid, "SIGKILL");
        else child.kill("SIGKILL");
      } catch (error) {
        if (error.code !== "ESRCH") reject(error);
      }
    };
    const timer = setTimeout(stop, step.timeoutMs);
    signal?.addEventListener("abort", stop, { once: true });
    const cleanup = () => {
      clearTimeout(timer);
      signal?.removeEventListener("abort", stop);
    };
    child.once("error", (error) => { cleanup(); reject(error); });
    child.once("close", (code) => { cleanup(); resolve(!stopped && code === 0); });
  });
}

export async function runCLI(steps) {
  const controller = new AbortController();
  const cancel = () => controller.abort();
  process.once("SIGTERM", cancel);
  process.once("SIGINT", cancel);
  try {
    process.exitCode = await runSequence(steps, { signal: controller.signal }) ? 0 : 1;
  } finally {
    process.removeListener("SIGTERM", cancel);
    process.removeListener("SIGINT", cancel);
  }
}
