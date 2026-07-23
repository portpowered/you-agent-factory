import { describe, expect, it } from "vitest";

import {
  FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS,
  measureMedianOperationMs,
} from "./factory-graph-layout-performance-budgets";

describe("factory graph layout performance budgets", () => {
  it("documents budgets for every large-editor fixture key", () => {
    expect(FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS.hundred).toEqual({
      initialProjectionMs: 4_000,
      dragSingleNodeMs: 5,
      dragMultiNodeMs: 25,
      saveLayoutRecomputationMs: 25,
      waypointEditMs: 5,
      waypointHistoryMs: 5,
    });
    expect(
      FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS.fiveHundred.initialProjectionMs,
    ).toBe(35_000);
    expect(
      FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS.stressThousand
        .initialProjectionMs,
    ).toBe(90_000);
    expect(
      FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS.fiveHundred
        .saveLayoutRecomputationMs,
    ).toBe(150);
    expect(
      FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS.stressThousand
        .saveLayoutRecomputationMs,
    ).toBe(200);
  });

  it("measures median async operation duration with warmup and iterations", async () => {
    let calls = 0;

    const medianMs = await measureMedianOperationMs(
      async () => {
        calls += 1;
        await Promise.resolve();
      },
      { iterations: 3, warmup: 2 },
    );

    expect(calls).toBe(5);
    expect(medianMs).toBeGreaterThanOrEqual(0);
  });

  it("returns zero when no samples are collected", async () => {
    await expect(
      measureMedianOperationMs(async () => {}, { iterations: 0, warmup: 0 }),
    ).resolves.toBe(0);
  });
});
