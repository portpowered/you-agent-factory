import type { FactoryGraphTopology } from "./factory-graph-draft-types";
import {
  createFactoryGraphWorkstationResolver,
  getFactoryGraphConnectionAnchors,
  resolveFactoryGraphConnectionAnchorContext,
  toWorkstationProgressOutcomeRouteContext,
} from "./factory-graph-editor-connections";

const connectableTopology: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
  ],
};

it("includes continue and reject anchors when the assigned worker has a stop token", () => {
  const resolver = createFactoryGraphWorkstationResolver(
    [
      {
        body: "Review the story.",
        inputs: [],
        name: "review",
        outputs: [],
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        worker: "processor",
      },
    ],
    [
      {
        name: "processor",
        stopToken: "<COMPLETE>",
        type: "MODEL_WORKER",
      },
    ],
  );
  const reviewNode = connectableTopology.nodes[0];
  const anchorContext = resolveFactoryGraphConnectionAnchorContext(
    reviewNode,
    resolver,
  );
  const anchorIds = getFactoryGraphConnectionAnchors(
    "workstation",
    anchorContext,
  ).map((anchor) => anchor.id);

  expect(anchorIds).toContain("workstation-on-continue-source");
  expect(anchorIds).toContain("workstation-on-rejection-source");
  expect(
    toWorkstationProgressOutcomeRouteContext(
      {
        body: "Review the story.",
        inputs: [],
        name: "review",
        outputs: [],
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        worker: "processor",
      },
      [
        {
          name: "processor",
          stopToken: "<COMPLETE>",
          type: "MODEL_WORKER",
        },
      ],
    ),
  ).toEqual(
    expect.objectContaining({
      assignedWorkerStopToken: "<COMPLETE>",
    }),
  );
});

it("omits continue and reject anchors for a plain processor resolved without stop markers", () => {
  const resolver = createFactoryGraphWorkstationResolver(
    [
      {
        body: "Review the story.",
        inputs: [],
        name: "review",
        outputs: [],
        type: "MODEL_WORKSTATION",
        behavior: "STANDARD",
        worker: "processor",
      },
    ],
    [
      {
        name: "processor",
        type: "MODEL_WORKER",
      },
    ],
  );
  const reviewNode = connectableTopology.nodes[0];
  const anchorIds = getFactoryGraphConnectionAnchors(
    "workstation",
    resolveFactoryGraphConnectionAnchorContext(reviewNode, resolver),
  ).map((anchor) => anchor.id);

  expect(anchorIds).not.toContain("workstation-on-continue-source");
  expect(anchorIds).not.toContain("workstation-on-rejection-source");
});
