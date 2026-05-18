import { describe, expect, test, vi } from "vitest";

import {
  createServerExitPromise,
  ensureStorybookServer,
  main,
} from "./run-storybook-responsive-check.mjs";

describe("ensureStorybookServer", () => {
  test("reuses an already-running Storybook server", async () => {
    const verifyIndex = vi.fn().mockResolvedValue(undefined);
    const assertAvailable = vi.fn();
    const spawnProcess = vi.fn();
    const waitReady = vi.fn();

    const server = await ensureStorybookServer({
      assertAvailable,
      spawnProcess,
      verifyIndex,
      waitReady,
    });

    expect(verifyIndex).toHaveBeenCalledTimes(1);
    expect(assertAvailable).not.toHaveBeenCalled();
    expect(spawnProcess).not.toHaveBeenCalled();
    expect(waitReady).not.toHaveBeenCalled();
    expect(server.startedServer).toBe(false);
  });

  test("starts and waits for a static Storybook server when none is running", async () => {
    const verifyIndex = vi.fn().mockRejectedValue(new Error("down"));
    const assertAvailable = vi.fn().mockResolvedValue(undefined);
    const serverProcess = { once: vi.fn() };
    const spawnProcess = vi.fn().mockReturnValue(serverProcess);
    const waitReady = vi.fn().mockResolvedValue(undefined);

    const server = await ensureStorybookServer({
      assertAvailable,
      host: "127.0.0.1",
      port: "6008",
      spawnProcess,
      verifyIndex,
      waitReady,
    });

    expect(assertAvailable).toHaveBeenCalledWith("127.0.0.1", "6008");
    expect(spawnProcess).toHaveBeenCalledWith([
      "x",
      "--no-install",
      "http-server",
      "storybook-static",
      "-p",
      "6008",
      "-a",
      "127.0.0.1",
      "-s",
    ]);
    expect(waitReady).toHaveBeenCalledTimes(1);
    expect(server.startedServer).toBe(true);
  });
});

describe("createServerExitPromise", () => {
  test("rejects when the server exits early", async () => {
    const handlers = new Map();
    const server = {
      once: vi.fn((event, handler) => {
        handlers.set(event, handler);
      }),
    };

    const exitPromise = createServerExitPromise(server);
    handlers.get("exit")?.(1, null);

    await expect(exitPromise).rejects.toThrow(
      "Storybook static server exited before the responsive checks completed (code 1).",
    );
  });
});

describe("main", () => {
  test("runs the responsive checks and stops a started server", async () => {
    const stop = vi.fn().mockResolvedValue(undefined);
    const ensureServer = vi.fn().mockResolvedValue({
      startedServer: true,
      stop,
    });
    const stopProcess = vi.fn().mockResolvedValue(undefined);
    const verifyResponsive = vi.fn().mockResolvedValue(undefined);

    await main({
      ensureServer,
      stopProcess,
      verifyResponsive,
    });

    expect(ensureServer).toHaveBeenCalledTimes(1);
    expect(verifyResponsive).toHaveBeenCalledTimes(1);
    expect(stop).toHaveBeenCalledWith(stopProcess);
  });

  test("stops the server when the responsive checks fail", async () => {
    const stop = vi.fn().mockResolvedValue(undefined);
    const ensureServer = vi.fn().mockResolvedValue({
      startedServer: true,
      stop,
    });
    const failure = new Error("responsive check failed");

    await expect(
      main({
        ensureServer,
        verifyResponsive: vi.fn().mockRejectedValue(failure),
      }),
    ).rejects.toThrow("responsive check failed");

    expect(stop).toHaveBeenCalledTimes(1);
  });
});
