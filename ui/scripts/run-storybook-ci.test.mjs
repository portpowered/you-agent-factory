import { describe, expect, test, vi } from "vitest";
import { EventEmitter } from "node:events";

import {
  createStorybookIndexTimeoutError,
  runStorybookCI,
  verifyStorybookIframe,
  waitForStorybookReady,
  waitForStableStorybookIframe,
} from "./run-storybook-ci.mjs";

describe("verifyStorybookIframe", () => {
  test("accepts a ready iframe shell that contains the storybook root", async () => {
    const fetchFn = vi.fn().mockResolvedValue({
      ok: true,
      text: vi.fn().mockResolvedValue(
        '<!doctype html><html><body><div id="storybook-root"></div></body></html>',
      ),
    });

    await expect(verifyStorybookIframe({ fetchFn })).resolves.toBeUndefined();
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  test("retries until the iframe shell contains the storybook root", async () => {
    const delayFn = vi.fn().mockResolvedValue(undefined);
    const fetchFn = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        text: vi.fn().mockResolvedValue("<html><body>booting</body></html>"),
      })
      .mockResolvedValueOnce({
        ok: true,
        text: vi.fn().mockResolvedValue(
          '<!doctype html><html><body><div id="storybook-root"></div></body></html>',
        ),
      });

    await expect(verifyStorybookIframe({ delayFn, fetchFn })).resolves.toBeUndefined();
    expect(fetchFn).toHaveBeenCalledTimes(2);
    expect(delayFn).toHaveBeenCalledWith(250);
  });

  test("fails after the last retry when the iframe shell never becomes ready", async () => {
    const delayFn = vi.fn().mockResolvedValue(undefined);
    const fetchFn = vi.fn().mockResolvedValue({
      ok: true,
      text: vi.fn().mockResolvedValue("<html><body>still booting</body></html>"),
    });

    await expect(
      verifyStorybookIframe({
        delayFn,
        fetchFn,
        maxAttempts: 2,
      }),
    ).rejects.toThrow("did not contain #storybook-root");
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });
});

describe("waitForStableStorybookIframe", () => {
  test("keeps verifying the iframe shell throughout the settle window", async () => {
    const delayFn = vi.fn().mockResolvedValue(undefined);
    const verifyIframe = vi.fn().mockResolvedValue(undefined);
    const nowValues = [0, 0, 250, 500, 750, 1000];
    const nowFn = vi.fn(() => nowValues.shift() ?? 1000);

    await waitForStableStorybookIframe({
      delayFn,
      nowFn,
      settleMs: 1000,
      verifyIframe,
    });

    expect(verifyIframe).toHaveBeenCalledTimes(2);
    expect(delayFn).toHaveBeenNthCalledWith(1, 250);
    expect(delayFn).toHaveBeenNthCalledWith(2, 250);
  });
});

describe("waitForStorybookReady", () => {
  test("waits for both the index and the iframe shell to stabilize", async () => {
    const runWaitOn = vi.fn().mockResolvedValue(undefined);
    const waitForStableIframe = vi.fn().mockResolvedValue(undefined);
    const waitForStableIndex = vi.fn().mockResolvedValue(undefined);

    await waitForStorybookReady({
      runWaitOn,
      serverExit: new Promise(() => {}),
      waitForStableIframe,
      waitForStableIndex,
    });

    expect(runWaitOn).toHaveBeenCalledTimes(1);
    expect(waitForStableIndex).toHaveBeenCalledTimes(1);
    expect(waitForStableIframe).toHaveBeenCalledTimes(1);
  });

  test("maps wait-on failures to the iframe timeout error contract", async () => {
    await expect(
      waitForStorybookReady({
        runWaitOn: vi.fn().mockRejectedValue(new Error("not ready")),
        serverExit: new Promise(() => {}),
      }),
    ).rejects.toThrow(createStorybookIndexTimeoutError().message);
  });
});

describe("runStorybookCI", () => {
  test("runs the interaction lane before the responsive lane and stops the server", async () => {
    const server = new EventEmitter();
    server.pid = 1234;
    server.exitCode = null;
    const assertAvailable = vi.fn().mockResolvedValue(undefined);
    const runCommand = vi.fn().mockResolvedValue(undefined);
    const settle = vi.fn().mockResolvedValue(undefined);
    const spawnServer = vi.fn(() => server);
    const stop = vi.fn(async () => {
      server.exitCode = 0;
    });
    const waitForReady = vi.fn().mockResolvedValue(undefined);
    const waitForStableIndex = vi.fn().mockResolvedValue(undefined);

    await runStorybookCI({
      assertAvailable,
      runCommand,
      settle,
      spawnServer,
      stop,
      waitForReady,
      waitForStableIndex,
    });

    expect(assertAvailable).toHaveBeenCalledTimes(1);
    expect(spawnServer).toHaveBeenCalledTimes(1);
    expect(waitForReady).toHaveBeenCalledTimes(1);
    expect(waitForReady.mock.calls[0]?.[0]?.serverExit).toBeInstanceOf(Promise);
    expect(runCommand).toHaveBeenNthCalledWith(1, ["run", "storybook:test-runner:ci"]);
    expect(settle).toHaveBeenCalledWith(1000);
    expect(waitForStableIndex).toHaveBeenCalledTimes(1);
    expect(runCommand).toHaveBeenNthCalledWith(2, ["run", "storybook:responsive-check"]);
    expect(stop).toHaveBeenCalledWith(server);
  });

  test("fails when the static server exits before readiness completes", async () => {
    const server = new EventEmitter();
    server.pid = 1234;
    server.exitCode = null;
    const stop = vi.fn(async () => {
      server.exitCode = 1;
    });

    await expect(
      runStorybookCI({
        assertAvailable: vi.fn().mockResolvedValue(undefined),
        runCommand: vi.fn().mockResolvedValue(undefined),
        settle: vi.fn().mockResolvedValue(undefined),
        spawnServer: vi.fn(() => server),
        stop,
        waitForReady: ({ serverExit }) => {
          server.emit("exit", 1, null);
          return serverExit;
        },
        waitForStableIndex: vi.fn().mockResolvedValue(undefined),
      }),
    ).rejects.toThrow(
      "Storybook static server exited before readiness or interaction tests completed (code 1).",
    );
    expect(stop).toHaveBeenCalledWith(server);
  });
});
