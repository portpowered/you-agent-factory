import { render, screen } from "@testing-library/react";

import * as flowchartPublic from "./index";

describe("flowchart public barrel", () => {
  it("keeps current activity nodes and graph semantic icons available", () => {
    expect(flowchartPublic.CURRENT_ACTIVITY_NODE_TYPES).toMatchObject({
      constraint: expect.any(Function),
      resource: expect.any(Function),
      statePosition: expect.any(Function),
      worker: expect.any(Function),
      workType: expect.any(Function),
      workstation: expect.any(Function),
    });
    expect(flowchartPublic.GRAPH_SEMANTIC_ICON_KINDS).toContain("queue");

    render(<flowchartPublic.GraphSemanticIcon kind="queue" />);

    expect(
      screen
        .getByRole("img", { name: "Queue state" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
  });

  it("does not expose workstation icon metadata helpers through the public barrel", () => {
    expect(flowchartPublic).not.toHaveProperty("workstationIconMetadata");
    expect(flowchartPublic).not.toHaveProperty(
      "SUPPORTED_WORKSTATION_ICON_METADATA",
    );
    expect(flowchartPublic).not.toHaveProperty(
      "EXHAUSTION_WORKSTATION_ICON_METADATA",
    );
  });
});
