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

// Only independent complete suites opt in. Their internal prerequisites stay
// ordered by their canonical entrypoints; API/workflow callers stay sequential.
export async function runConcurrent(steps, options = {}) {
  if (steps.some((step) => step.needs?.length)) {
    throw new Error("Concurrent suites must be independent");
  }
  const results = await Promise.all(steps.map((step) => runSequence([step], options)));
  return results.every(Boolean);
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
    let termination;
    const stop = () => {
      if (stopped) return;
      stopped = true;
      if (!child.pid) return;
      // Kill the owned group, including make/Bun/browser grandchildren.
      try {
        if (grouped) process.kill(-child.pid, "SIGKILL");
        else {
          // Killing only the shell leaves Bun/browser descendants alive.
          termination = new Promise((done) => {
            const killer = spawn("taskkill", ["/pid", String(child.pid), "/T", "/F"], {
              stdio: "ignore", windowsHide: true,
            });
            killer.once("error", (error) => { reject(error); done(); });
            killer.once("close", (code) => {
              if (code !== 0) reject(new Error(`taskkill failed with exit status ${code}`));
              done();
            });
          });
        }
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
    child.once("close", async (code) => {
      cleanup();
      await termination;
      resolve(!stopped && code === 0);
    });
  });
}

export async function runCLI(steps, execute = runSequence) {
  const controller = new AbortController();
  const cancel = () => controller.abort();
  process.once("SIGTERM", cancel);
  process.once("SIGINT", cancel);
  try {
    process.exitCode = await execute(steps, { signal: controller.signal }) ? 0 : 1;
  } finally {
    process.removeListener("SIGTERM", cancel);
    process.removeListener("SIGINT", cancel);
  }
}

export async function runConcurrentCLI(steps) {
  await runCLI(steps, runConcurrent);
}
