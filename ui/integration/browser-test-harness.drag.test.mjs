// @vitest-environment node

import { describe, expect, it, vi } from "vitest";

import { dragNodeByOffset } from "./browser-test-harness.mjs";

function createDragPage() {
  return {
    mouse: {
      down: vi.fn().mockResolvedValue(undefined),
      move: vi.fn().mockResolvedValue(undefined),
      up: vi.fn().mockResolvedValue(undefined),
    },
    waitForTimeout: vi.fn().mockResolvedValue(undefined),
  };
}

describe("verified graph node drag helper", () => {
  it("releases and retries a lost pointer grab before returning measurements", async () => {
    const initialPosition = { x: 100, y: 200 };
    const finalPosition = { x: 130, y: 220 };
    const nodeLocator = {
      evaluate: vi
        .fn()
        .mockResolvedValueOnce(initialPosition)
        .mockResolvedValueOnce({ x: 50, y: 60 })
        .mockResolvedValueOnce(initialPosition)
        .mockResolvedValueOnce(initialPosition)
        .mockResolvedValueOnce({ x: 50, y: 60 })
        .mockResolvedValueOnce(finalPosition)
        .mockResolvedValueOnce(finalPosition),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = createDragPage();

    await expect(
      dragNodeByOffset(page, nodeLocator, 28, 20, {
        displacementTolerancePx: 8,
        settleDelayMs: 0,
      }),
    ).resolves.toMatchObject({
      initialFlowPosition: initialPosition,
      midDragDistancePx: 30,
      midDragFlowPosition: finalPosition,
      postMouseUpDistancePx: 30,
      postMouseUpFlowPosition: finalPosition,
    });
    expect(page.mouse.down).toHaveBeenCalledTimes(2);
    expect(page.mouse.up).toHaveBeenCalledTimes(2);
  });

  it("fails quickly with measured positions after bounded no-op attempts", async () => {
    const initialPosition = { x: 100, y: 200 };
    const nodeLocator = {
      evaluate: vi
        .fn()
        .mockResolvedValueOnce(initialPosition)
        .mockResolvedValueOnce({ x: 50, y: 60 })
        .mockResolvedValueOnce(initialPosition)
        .mockResolvedValueOnce(initialPosition)
        .mockResolvedValueOnce({ x: 50, y: 60 })
        .mockResolvedValueOnce(initialPosition)
        .mockResolvedValueOnce(initialPosition),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = createDragPage();

    let thrownError;
    try {
      await dragNodeByOffset(page, nodeLocator, 28, 20, {
        displacementTolerancePx: 8,
        settleDelayMs: 0,
      });
    } catch (error) {
      thrownError = error;
    }

    expect(thrownError).toBeInstanceOf(Error);
    expect(thrownError.message).toContain(
      "Mouse drag did not produce the required flow displacement",
    );
    const diagnostic = JSON.parse(
      thrownError.message.slice(thrownError.message.indexOf("{")),
    );
    expect(diagnostic).toMatchObject({
      attempts: 2,
      initialFlowPosition: initialPosition,
      midDragDistancePx: 0,
      midDragFlowPosition: initialPosition,
      postMouseUpDistancePx: 0,
      postMouseUpFlowPosition: initialPosition,
    });
    expect(page.mouse.up).toHaveBeenCalledTimes(2);
  });
});
