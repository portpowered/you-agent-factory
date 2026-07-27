import { describe, expect, it } from "vitest";

import type { FactoryGraphTopology } from "../../draft/factory-graph-draft-types";
import {
  buildFactoryGraphEdgeChangeFromConnection,
  createFactoryGraphWorkstationResolver,
} from "../factory-graph-editor-connections";

const topologyWithDistinctWorkstationId: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "work-state:story:queued",
      key: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:queued",
    },
    {
      id: "workstation:canonical-review-id",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
  ],
};

describe("factory graph editor connections with distinct workstation ids", () => {
  it("resolves the canonical graph id while preserving the authored name", () => {
    expect(
      buildFactoryGraphEdgeChangeFromConnection(
        topologyWithDistinctWorkstationId,
        {
          sourceAnchorId: "workstation-output-source",
          sourceNodeId: "workstation:canonical-review-id",
          targetAnchorId: "work-state-input-target",
          targetNodeId: "work-state:story:queued",
        },
        createFactoryGraphWorkstationResolver([
          {
            body: "Review",
            id: "canonical-review-id",
            inputs: [],
            name: "review",
            outputs: [],
            type: "MODEL_WORKSTATION",
            worker: "writer",
          },
        ]),
      ),
    ).toEqual({
      kind: "workstation-output",
      source: { kind: "workstation", name: "review" },
      target: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
    });
  });
});
