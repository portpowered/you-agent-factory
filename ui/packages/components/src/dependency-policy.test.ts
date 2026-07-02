import { describe, expect, it } from "vitest";

describe("@you-agent-factory/components dependency policy", () => {
  it("resolves recharts as an installable package dependency", async () => {
    const recharts = await import("recharts");
    expect(recharts.ResponsiveContainer).toBeDefined();
  });

  it("resolves @xyflow/react as an installable package dependency", async () => {
    const xyflow = await import("@xyflow/react");
    expect(xyflow.ReactFlow).toBeDefined();
  });
});
