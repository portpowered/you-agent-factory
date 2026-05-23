import { describe, expect, test, vi } from "vitest";

import {
  createServerExitPromise,
  ensureStorybookServer,
  main,
} from "./run-storybook-responsive-check.mjs";

describe("ensureStorybookServer", () => {
  test("starts and waits for a dedicated static Storybook server", async () => {
    const assertAvailable = vi.fn().mockResolvedValue(undefined);
    const serverProcess = { once: vi.fn() };
    const spawnProcess = vi.fn().mockReturnValue(serverProcess);
    const waitReady = vi.fn().mockResolvedValue(undefined);

    const server = await ensureStorybookServer({
      assertAvailable,
      host: "127.0.0.1",
      port: "6008",
      spawnProcess,
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
