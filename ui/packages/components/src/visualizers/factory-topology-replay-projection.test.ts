import { describe, expect, it } from "vitest";

import {
  factoryTopologyReplayMessages,
  factoryTopologyReplayProjection,
} from "./factory-topology-replay-fixtures";
import { projectFactoryTopologyToFlow } from "./factory-topology-replay-projection";

describe("projectFactoryTopologyToFlow failure classification", () => {
  it("classifies unexpected layout failures without exposing thrown details", () => {
    const projection = structuredClone(factoryTopologyReplayProjection);
    const nodes = projection.topology.nodes;
    projection.topology.nodes = {
      [Symbol.iterator]: () => nodes[Symbol.iterator](),
      filter: () => {
        throw new Error("secret layout payload");
      },
    } as unknown as typeof nodes;

    expect(
      projectFactoryTopologyToFlow({
        formatNumber: String,
        messages: factoryTopologyReplayMessages,
        projection,
      }),
    ).toEqual({
      cause: { name: "Error" },
      kind: "layout",
      message: "Factory topology layout could not be prepared.",
      recoverable: true,
    });
  });
});
