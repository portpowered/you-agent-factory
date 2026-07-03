// @vitest-environment node

import http from "node:http";

import { describe, expect, it, vi } from "vitest";

import {
  defaultFactorySessionID,
  expectNoBrowserErrors,
  previewHost,
  resolvedDefaultFactorySessionID,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
  waitForCapturedDownloadOrDialogError,
  waitForDialogHidden,
  waitForDurableCheckpoint,
  waitForDurableControlEnabled,
  waitForPortAvailable,
} from "./browser-test-harness.mjs";

describe("browser wait pattern helpers", () => {
  it("waitForDurableCheckpoint resolves when the condition becomes true", async () => {
    let ready = false;
    const timer = setTimeout(() => {
      ready = true;
    }, 50);

    await waitForDurableCheckpoint("ready flag", async () => ready, 500, 10);
    clearTimeout(timer);
  });

  it("waitForDurableCheckpoint rejects when the deadline passes", async () => {
    await expect(
      waitForDurableCheckpoint("never-ready", async () => false, 50, 10),
    ).rejects.toThrow(/Timed out waiting for durable checkpoint: never-ready/);
  });

  it("waitForDurableControlEnabled delegates to durable control polling", async () => {
    const locator = {
      isEnabled: vi
        .fn()
        .mockResolvedValueOnce(false)
        .mockResolvedValueOnce(true),
    };

    await waitForDurableControlEnabled(locator, uiInteractionTimeoutMs);

    expect(locator.isEnabled).toHaveBeenCalledTimes(2);
  });

  it("waitForDialogHidden waits for dialog role hidden state", async () => {
    const dialogLocator = {
      waitFor: vi.fn().mockResolvedValue(undefined),
    };

    await waitForDialogHidden(dialogLocator, 1_000);

    expect(dialogLocator.waitFor).toHaveBeenCalledWith({
      state: "hidden",
      timeout: 1_000,
    });
  });

  it("waitForCapturedDownloadOrDialogError throws dialog alert text on error", async () => {
    const page = {
      waitForFunction: vi.fn(() => new Promise(() => {})),
    };
    const dialogLocator = {
      getByRole: vi.fn((role) => {
        if (role === "alert") {
          return {
            waitFor: vi.fn().mockResolvedValue(undefined),
            innerText: vi.fn().mockResolvedValue("Export failed"),
          };
        }
        throw new Error(`unexpected role ${role}`);
      }),
    };

    await expect(
      waitForCapturedDownloadOrDialogError(page, dialogLocator, 25),
    ).rejects.toThrow("Export failed");
  });

  it("ignores browser-generated resource load console errors", () => {
    expectNoBrowserErrors(
      [],
      [
        "Failed to load resource: the server responded with a status of 404 (Not Found)",
      ],
      expect,
    );
  });

  it("waitForPortAvailable resolves when the port is already free", async () => {
    const server = http.createServer();
    await new Promise((resolve, reject) => {
      server.once("error", reject);
      server.listen(0, previewHost, resolve);
    });
    const address = server.address();
    const port = typeof address === "object" && address ? address.port : null;
    expect(port).toBeTypeOf("number");

    await server.close();
    await waitForPortAvailable(previewHost, port, 1_000, 25);
  });

  it("waitForPortAvailable waits until a held port is released", async () => {
    const holder = http.createServer();
    await new Promise((resolve, reject) => {
      holder.once("error", reject);
      holder.listen(0, previewHost, resolve);
    });
    const address = holder.address();
    const port = typeof address === "object" && address ? address.port : null;
    expect(port).toBeTypeOf("number");

    const releaseTimer = setTimeout(() => {
      holder.close();
    }, 100);

    await waitForPortAvailable(previewHost, port, 2_000, 25);
    clearTimeout(releaseTimer);
  });

  it("waitForPortAvailable rejects when the port stays busy", async () => {
    const holder = http.createServer();
    await new Promise((resolve, reject) => {
      holder.once("error", reject);
      holder.listen(0, previewHost, resolve);
    });
    const address = holder.address();
    const port = typeof address === "object" && address ? address.port : null;
    expect(port).toBeTypeOf("number");

    await expect(
      waitForPortAvailable(previewHost, port, 100, 25),
    ).rejects.toThrow(/Timed out waiting for/);

    await new Promise((resolve, reject) => {
      holder.close((error) => {
        if (error) {
          reject(error);
          return;
        }
        resolve();
      });
    });
  });

  it("serves sync preflight responses with the required identity set", async () => {
    const server = await startFactoryApiServer({
      apiPort: 3921,
      currentFactory: { name: "Browser Harness Factory" },
    });

    try {
      const response = await fetch(
        `http://${previewHost}:3921/factory-sessions/${defaultFactorySessionID}/sync-preflight?after_event_id=event-7&after_sequence=7`,
      );
      const body = await response.json();

      expect(response.status).toBe(200);
      expect(body).toMatchObject({
        backendScopeId: "/replay/factory::browser-integration",
        checkpointReusable: true,
        factorySessionId: resolvedDefaultFactorySessionID,
        logicalSessionKeyId: "/replay/factory::default::",
        reasonCode: "ok",
        reconnectCursor: {
          provided: true,
          validForStreamGeneration: true,
        },
        requestedSessionId: defaultFactorySessionID,
      });
      expect(body.streamGenerationId).toBeTypeOf("string");
      expect(body.streamGenerationId).not.toMatch(/^browser-stream-/);
    } finally {
      await server.stop();
    }
  });

  it("reports session_not_found for missing preflight targets", async () => {
    const server = await startFactoryApiServer({
      apiPort: 3922,
      currentFactory: { name: "Browser Harness Factory" },
    });

    try {
      const response = await fetch(
        `http://${previewHost}:3922/factory-sessions/session-missing/sync-preflight`,
      );
      const body = await response.json();

      expect(response.status).toBe(200);
      expect(body).toEqual({
        checkpointReusable: false,
        reasonCode: "session_not_found",
        reconnectCursor: {
          provided: false,
          validForStreamGeneration: false,
        },
        requestedSessionId: "session-missing",
      });
    } finally {
      await server.stop();
    }
  });
});
