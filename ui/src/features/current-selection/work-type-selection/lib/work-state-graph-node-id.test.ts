import { workStateGraphNodeId } from "./work-state-graph-node-id";

describe("workStateGraphNodeId", () => {
  it("builds the canonical factory-graph work-state node id", () => {
    expect(workStateGraphNodeId("story", "queued")).toBe(
      "work-state:story:queued",
    );
  });
});
