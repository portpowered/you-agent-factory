import { describe, expect, it } from "vitest";

import { graphHandleToneFromId } from "./activity-graph-handle-tone";

describe("graphHandleToneFromId", () => {
  it("maps dashboard handle ids to package handle tones", () => {
    expect(graphHandleToneFromId("workstation-on-continue-source")).toBe(
      "continue",
    );
    expect(graphHandleToneFromId("workstation-on-rejection-source")).toBe(
      "rejection",
    );
    expect(graphHandleToneFromId("workstation-output-source")).toBe("output");
    expect(graphHandleToneFromId("resource-input-target")).toBe("resource");
    expect(graphHandleToneFromId("worker-assignment-source")).toBe("worker");
    expect(graphHandleToneFromId("unknown-handle")).toBe("default");
  });
});
