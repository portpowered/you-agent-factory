// @vitest-environment node

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
  waitForFactoryGraphSelectionDeleteButton,
  waitForFactoryGraphSelectionReady,
  waitForStableBoundingBox,
  waitForStableFactoryGraphNodePlacement,
  waitForStableFactoryGraphViewport,
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

  it("waitForDurableCheckpoint accepts a synchronous plain condition", async () => {
    await waitForDurableCheckpoint("synchronous readiness", () => true, 50, 1);
  });

  it("waitForDurableCheckpoint resolves named synchronous and asynchronous conditions", async () => {
    await waitForDurableCheckpoint(
      "named readiness",
      {
        "synchronous condition": () => true,
        "asynchronous condition": async () => true,
      },
      50,
      1,
    );
  });

  it("waitForDurableCheckpoint reports every named condition that returns false", async () => {
    await expect(
      waitForDurableCheckpoint(
        "named readiness",
        {
          "selection projection": () => false,
          "delete control visibility": async () => false,
        },
        10,
        1,
      ),
    ).rejects.toThrow(
      /selection projection \(returned false\).*delete control visibility \(returned false\)/,
    );
  });

  it("waitForDurableCheckpoint reports the last named thrown outcome", async () => {
    await expect(
      waitForDurableCheckpoint(
        "named readiness",
        {
          "delete control enabled": () => {
            throw new Error("detached element");
          },
        },
        10,
        1,
      ),
    ).rejects.toThrow(
      /delete control enabled \(threw: Error: detached element\)/,
    );
  });

  it("waitForDurableCheckpoint recovers after a transient named throw", async () => {
    const condition = vi
      .fn()
      .mockRejectedValueOnce(new Error("detached element"))
      .mockResolvedValue(true);

    await waitForDurableCheckpoint(
      "recovering readiness",
      { "delete control enabled": condition },
      50,
      1,
    );

    expect(condition).toHaveBeenCalledTimes(2);
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

  it("waitForStableBoundingBox observes two matching geometry samples", async () => {
    const stableBox = { height: 40, width: 80, x: 12, y: 24 };
    const locator = {
      boundingBox: vi
        .fn()
        .mockResolvedValueOnce({ ...stableBox, x: 8 })
        .mockResolvedValue(stableBox),
    };

    await expect(waitForStableBoundingBox(locator, 500, 1)).resolves.toEqual(
      stableBox,
    );
    expect(locator.boundingBox).toHaveBeenCalledTimes(3);
  });
});

describe("factory graph wait helpers", () => {
  it("waitForStableFactoryGraphViewport observes two matching transforms", async () => {
    const flowViewport = {
      evaluate: vi
        .fn()
        .mockResolvedValueOnce("matrix(1, 0, 0, 1, 0, 0)")
        .mockResolvedValue("matrix(1, 0, 0, 1, 20, -12)"),
    };
    const page = {
      locator: vi.fn(() => flowViewport),
    };

    await expect(waitForStableFactoryGraphViewport(page, 500, 1)).resolves.toBe(
      "matrix(1, 0, 0, 1, 20, -12)",
    );
    expect(page.locator).toHaveBeenCalledWith(
      "[data-current-activity-flow] .react-flow__viewport",
    );
    expect(flowViewport.evaluate).toHaveBeenCalledTimes(3);
  });

  it("waitForStableFactoryGraphNodePlacement observes settled geometry and transforms", async () => {
    const sample = {
      node: { height: 120, width: 180, x: 420, y: 260 },
      nodeTransform: "translate(420px, 260px)",
      viewport: { height: 720, width: 1280, x: 0, y: 0 },
      viewportTransform: "matrix(1, 0, 0, 1, -80, 40)",
    };
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValueOnce({
          ...sample,
          node: { ...sample.node, x: 1200 },
        })
        .mockResolvedValue(sample),
    };

    await expect(
      waitForStableFactoryGraphNodePlacement(
        page,
        "rf__node-workstation:review",
        500,
        1,
      ),
    ).resolves.toEqual(sample);
    expect(page.evaluate).toHaveBeenCalledWith(
      expect.any(Function),
      "rf__node-workstation:review",
    );
    expect(page.evaluate).toHaveBeenCalledTimes(3);
  });

  it("can settle a saved flow position without requiring camera framing", async () => {
    const sample = {
      node: { height: 80, width: 20, x: 120, y: 400.57 },
      nodeTransform: "translate(120px, 400.57px)",
      viewport: { height: 408.31, width: 500, x: 0, y: 0 },
      viewportTransform: "matrix(1, 0, 0, 1, 0, 0)",
    };
    const observations = [];
    const page = {
      evaluate: vi.fn().mockResolvedValue(sample),
    };

    await expect(
      waitForStableFactoryGraphNodePlacement(
        page,
        "rf__node-resource:extra-gpu",
        500,
        1,
        (observation) => observations.push(observation),
        { requireViewportVisibility: false },
      ),
    ).resolves.toEqual(sample);
    expect(page.evaluate).toHaveBeenCalledTimes(2);
    expect(observations.at(-1)).toMatchObject({
      stable: true,
      stableSampleCount: 2,
      terminalDiagnostic: null,
      withinViewport: false,
    });
  });
});

describe("settled graph node placement wait helper", () => {
  it("fails fast with geometry when a settled node is outside the viewport", async () => {
    const sample = {
      node: { height: 80, width: 20, x: -80, y: 400.57 },
      nodeTransform: "translate(-80px, 400.57px)",
      viewport: { height: 408.31, width: 500, x: 0, y: 0 },
      viewportTransform: "matrix(1, 0, 0, 1, 0, 0)",
    };
    const observations = [];
    const page = {
      evaluate: vi.fn().mockResolvedValue(sample),
    };
    const startedAt = performance.now();
    let thrownError;

    try {
      await waitForStableFactoryGraphNodePlacement(
        page,
        "rf__node-resource:extra-gpu",
        10_000,
        100,
        (observation) => observations.push(observation),
      );
    } catch (error) {
      thrownError = error;
    }

    expect(thrownError).toBeInstanceOf(Error);
    expect(thrownError.message).toContain(
      "Settled graph node placement cannot satisfy viewport visibility for rf__node-resource:extra-gpu",
    );
    expect(thrownError.message).toContain("node center=(-70.00, 440.57)");
    expect(thrownError.message).toContain(
      "viewport edges={left=0.00, right=500.00, top=0.00, bottom=408.31}",
    );
    expect(thrownError.message).toContain(
      "x left of left edge by 70.00px; y below bottom edge by 32.26px",
    );
    expect(performance.now() - startedAt).toBeLessThan(1_000);
    expect(page.evaluate).toHaveBeenCalledTimes(3);
    expect(observations).toHaveLength(3);
    expect(observations.at(-1)).toMatchObject({
      pollCount: 3,
      stable: true,
      stableSampleCount: 3,
      terminalDiagnostic: thrownError.message,
      viewportViolation: expect.objectContaining({
        violations: expect.arrayContaining([
          expect.objectContaining({ axis: "x", direction: "left of" }),
          expect.objectContaining({ axis: "y", direction: "below" }),
        ]),
      }),
      withinViewport: false,
    });
  });

  it("keeps polling when the node geometry is missing", async () => {
    const page = {
      evaluate: vi.fn().mockResolvedValue(null),
    };

    await expect(
      waitForStableFactoryGraphNodePlacement(
        page,
        "rf__node-resource:missing",
        25,
        1,
      ),
    ).rejects.toThrow(
      "Timed out waiting for durable checkpoint: factory graph node placement: rf__node-resource:missing",
    );
    expect(page.evaluate.mock.calls.length).toBeGreaterThan(1);
  });

  it("keeps polling while changing geometry has not settled", async () => {
    const samples = [
      {
        node: { height: 80, width: 20, x: 100, y: 100 },
        nodeTransform: "translate(100px, 100px)",
        viewport: { height: 400, width: 500, x: 0, y: 0 },
        viewportTransform: "matrix(1, 0, 0, 1, 0, 0)",
      },
      {
        node: { height: 80, width: 20, x: 101, y: 100 },
        nodeTransform: "translate(101px, 100px)",
        viewport: { height: 400, width: 500, x: 0, y: 0 },
        viewportTransform: "matrix(1, 0, 0, 1, 0, 0)",
      },
    ];
    let sampleIndex = 0;
    const page = {
      evaluate: vi.fn().mockImplementation(async () => {
        const sample = samples[sampleIndex % samples.length];
        sampleIndex += 1;
        return sample;
      }),
    };

    await expect(
      waitForStableFactoryGraphNodePlacement(
        page,
        "rf__node-resource:changing",
        25,
        1,
      ),
    ).rejects.toThrow(
      "Timed out waiting for durable checkpoint: factory graph node placement: rf__node-resource:changing",
    );
    expect(page.evaluate.mock.calls.length).toBeGreaterThan(1);
  });
});

describe("browser wait pattern helpers", () => {
  it("waitForFactoryGraphSelectionReady waits for measured graph internals", async () => {
    const readinessMarker = {
      count: vi.fn().mockResolvedValueOnce(0).mockResolvedValue(1),
    };
    const page = {
      locator: vi.fn(() => readinessMarker),
    };

    await waitForFactoryGraphSelectionReady(page, 500);

    expect(page.locator).toHaveBeenCalledWith(
      '[data-factory-graph-selection-ready="true"]',
    );
    expect(readinessMarker.count).toHaveBeenCalledTimes(2);
  });

  it("waitForFactoryGraphSelectionDeleteButton waits for the selected enabled control", async () => {
    const selectedGraphSelection = {
      isVisible: vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true),
    };
    const batchDeleteButton = {
      isEnabled: vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const toolbar = {
      getByRole: vi.fn(() => batchDeleteButton),
      locator: vi.fn(() => selectedGraphSelection),
    };

    await expect(
      waitForFactoryGraphSelectionDeleteButton(toolbar, 500),
    ).resolves.toBe(batchDeleteButton);
    expect(toolbar.locator).toHaveBeenCalledWith(
      '[data-toolbar-graph-selection="single"], [data-toolbar-graph-selection="multi"]',
    );
    expect(toolbar.getByRole).toHaveBeenCalledWith("button", {
      name: /^Delete (?:\d+ )?selected graph items?$/,
    });
  });

  it("waitForFactoryGraphSelectionDeleteButton reports detached graph observations", async () => {
    const selectedGraphSelection = {
      isVisible: vi.fn().mockResolvedValue(false),
    };
    const batchDeleteButton = {
      isEnabled: vi.fn().mockRejectedValue(new Error("detached delete button")),
      isVisible: vi.fn().mockResolvedValue(false),
    };
    const toolbar = {
      getByRole: vi.fn(() => batchDeleteButton),
      locator: vi.fn(() => selectedGraphSelection),
    };

    await expect(
      waitForFactoryGraphSelectionDeleteButton(toolbar, 10),
    ).rejects.toThrow(
      /selection projection \(returned false\).*delete control visibility \(returned false\).*delete control enabled \(threw: Error: detached delete button\)/,
    );
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

describe("browser harness server", () => {
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
