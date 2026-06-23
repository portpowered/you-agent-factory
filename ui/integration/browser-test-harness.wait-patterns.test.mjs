// @vitest-environment node

import { describe, expect, it, vi } from "vitest";

import {
  expectNoBrowserErrors,
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
});
