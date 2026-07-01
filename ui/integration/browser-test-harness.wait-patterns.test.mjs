// @vitest-environment node

import { describe, expect, it, vi } from "vitest";

import {
  defaultFactorySessionID,
  expectNoBrowserErrors,
  previewHost,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
  waitForCapturedDownloadOrDialogError,
  waitForDialogHidden,
  waitForDurableCheckpoint,
  waitForDurableControlEnabled,
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
        factorySessionId: defaultFactorySessionID,
        logicalSessionKeyId: defaultFactorySessionID,
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
