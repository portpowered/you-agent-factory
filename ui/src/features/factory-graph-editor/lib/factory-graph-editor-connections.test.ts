// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: existing factory-graph connection coverage stayed intact during feature-root migration.

import type {
  FactoryGraphDraft,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";
import {
  applyFactoryGraphEdgeAddition,
  applyFactoryGraphEdgeRemoval,
  buildFactoryGraphConnectionNotice,
  buildFactoryGraphEdgeChangeFromConnection,
  createFactoryGraphWorkstationResolver,
  factoryGraphConnectionAnchorContext,
  getLocalizedFactoryGraphConnectionAnchors,
  isValidFactoryGraphConnection,
} from "./factory-graph-editor-connections";

const baseDraft: FactoryGraphDraft = {
  additions: {
    resources: [],
    workers: [],
    workStates: [],
    workTypes: [],
    workstations: [],
  },
  edgeChanges: {
    additions: [],
    removals: [],
  },
  removals: {
    resources: [],
    workers: [],
    workStates: [],
    workTypes: [],
    workstations: [],
  },
};

const connectableTopology: FactoryGraphTopology = {
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
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
  ],
};

const standardProcessorWithoutStopWords = factoryGraphConnectionAnchorContext({
  type: "MODEL_WORKSTATION",
  behavior: "STANDARD",
});

const standardProcessorWithStopWords = factoryGraphConnectionAnchorContext({
  type: "MODEL_WORKSTATION",
  behavior: "STANDARD",
  stopWords: ["DONE"],
});

describe("factory graph editor connections", () => {
  it("exposes separate workstation transition anchors for success, continue, failure, and rejection when progress routes are supported", () => {
    expect(
      getLocalizedFactoryGraphConnectionAnchors(
        "workstation",
        "en",
        standardProcessorWithStopWords,
      ),
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          edgeKind: "workstation-output",
          label: "Success",
        }),
        expect.objectContaining({
          edgeKind: "workstation-on-continue",
          label: "Continue",
        }),
        expect.objectContaining({
          edgeKind: "workstation-on-failure",
          label: "Failure",
        }),
        expect.objectContaining({
          edgeKind: "workstation-on-rejection",
          label: "Reject",
        }),
      ]),
    );
  });

  it("omits continue and reject anchors for a standard processor without stopWords", () => {
    const anchorIds = getLocalizedFactoryGraphConnectionAnchors(
      "workstation",
      "en",
      standardProcessorWithoutStopWords,
    ).map((anchor) => anchor.id);

    expect(anchorIds).toEqual(
      expect.arrayContaining([
        "workstation-input-target",
        "worker-assignment-target",
        "workstation-resource-target",
        "workstation-output-source",
        "workstation-on-failure-source",
      ]),
    );
    expect(anchorIds).not.toContain("workstation-on-continue-source");
    expect(anchorIds).not.toContain("workstation-on-rejection-source");
  });

  it("includes continue and reject anchors for a standard processor with stopWords", () => {
    const anchorIds = getLocalizedFactoryGraphConnectionAnchors(
      "workstation",
      "en",
      standardProcessorWithStopWords,
    ).map((anchor) => anchor.id);

    expect(anchorIds).toContain("workstation-on-continue-source");
    expect(anchorIds).toContain("workstation-on-rejection-source");
  });

  it("rejects new continue and reject connections when those anchors are hidden", () => {
    expect(
      isValidFactoryGraphConnection({
        sourceAnchorId: "workstation-on-continue-source",
        sourceNodeKind: "workstation",
        targetAnchorId: "work-state-input-target",
        targetNodeKind: "work-state",
        sourceWorkstation: standardProcessorWithoutStopWords.workstation,
      }),
    ).toBe(false);

    expect(
      buildFactoryGraphEdgeChangeFromConnection(
        connectableTopology,
        {
          sourceAnchorId: "workstation-on-rejection-source",
          sourceNodeId: "workstation:review",
          targetAnchorId: "work-state-input-target",
          targetNodeId: "work-state:story:queued",
        },
        createFactoryGraphWorkstationResolver([
          {
            body: "Review",
            inputs: [],
            name: "review",
            outputs: [],
            type: "MODEL_WORKSTATION",
            worker: "writer",
          },
        ]),
      ),
    ).toBeNull();
  });

  it("maps compatible anchor selections into a draft edge addition", () => {
    const edgeChange = buildFactoryGraphEdgeChangeFromConnection(
      connectableTopology,
      {
        sourceAnchorId: "workstation-on-failure-source",
        sourceNodeId: "workstation:review",
        targetAnchorId: "work-state-input-target",
        targetNodeId: "work-state:story:queued",
      },
    );

    expect(edgeChange).toEqual({
      kind: "workstation-on-failure",
      source: {
        kind: "workstation",
        name: "review",
      },
      target: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
    });

    expect(
      applyFactoryGraphEdgeAddition(
        baseDraft,
        connectableTopology,
        edgeChange as NonNullable<typeof edgeChange>,
      ).edgeChanges.additions,
    ).toEqual([
      {
        kind: "workstation-on-failure",
        source: {
          kind: "workstation",
          name: "review",
        },
        target: {
          kind: "work-state",
          stateName: "queued",
          workTypeName: "story",
        },
      },
    ]);
  });

  it("rejects incompatible anchors with a user-facing notice", () => {
    expect(
      buildFactoryGraphEdgeChangeFromConnection(connectableTopology, {
        sourceAnchorId: "workstation-on-failure-source",
        sourceNodeId: "workstation:review",
        targetAnchorId: "workstation-input-target",
        targetNodeId: "workstation:review",
      }),
    ).toBeNull();

    expect(
      buildFactoryGraphConnectionNotice({
        sourceAnchorId: "workstation-on-failure-source",
        sourceNode: connectableTopology
          .nodes[1] as FactoryGraphTopology["nodes"][number],
        targetAnchorId: "workstation-input-target",
        targetNode: connectableTopology
          .nodes[1] as FactoryGraphTopology["nodes"][number],
      }),
    ).toBe(
      "Failure connections from review cannot connect to Input on review.",
    );

    expect(
      buildFactoryGraphConnectionNotice({
        locale: "zh-CN",
        sourceAnchorId: "workstation-on-failure-source",
        sourceNode: connectableTopology
          .nodes[1] as FactoryGraphTopology["nodes"][number],
        targetAnchorId: "workstation-input-target",
        targetNode: connectableTopology
          .nodes[1] as FactoryGraphTopology["nodes"][number],
      }),
    ).toBe("review 的失败连接不能连接到 review 上的输入。");
  });

  it("records existing edge removals in the draft", () => {
    const topologyWithEdge: FactoryGraphTopology = {
      ...connectableTopology,
      edges: [
        {
          id: "workstation-on-failure:workstation:review->work-state:story:queued",
          kind: "workstation-on-failure",
          source: { kind: "workstation", name: "review" },
          sourceId: "workstation:review",
          target: {
            kind: "work-state",
            stateName: "queued",
            workTypeName: "story",
          },
          targetId: "work-state:story:queued",
        },
      ],
    };

    expect(
      applyFactoryGraphEdgeRemoval(
        baseDraft,
        topologyWithEdge,
        topologyWithEdge.edges[0] as FactoryGraphTopology["edges"][number],
      ).edgeChanges.removals,
    ).toEqual([
      {
        kind: "workstation-on-failure",
        source: {
          kind: "workstation",
          name: "review",
        },
        target: {
          kind: "work-state",
          stateName: "queued",
          workTypeName: "story",
        },
      },
    ]);
  });

  it("discards pending draft-only edge additions when they are removed", () => {
    const pendingAdditionDraft: FactoryGraphDraft = {
      ...baseDraft,
      edgeChanges: {
        additions: [
          {
            kind: "workstation-on-failure",
            source: {
              kind: "workstation",
              name: "review",
            },
            target: {
              kind: "work-state",
              stateName: "queued",
              workTypeName: "story",
            },
          },
        ],
        removals: [],
      },
    };
    const topologyWithPendingEdge: FactoryGraphTopology = {
      ...connectableTopology,
      edges: [
        {
          id: "workstation-on-failure:workstation:review->work-state:story:queued",
          kind: "workstation-on-failure",
          source: { kind: "workstation", name: "review" },
          sourceId: "workstation:review",
          target: {
            kind: "work-state",
            stateName: "queued",
            workTypeName: "story",
          },
          targetId: "work-state:story:queued",
        },
      ],
    };

    const nextDraft = applyFactoryGraphEdgeRemoval(
      pendingAdditionDraft,
      topologyWithPendingEdge,
      topologyWithPendingEdge.edges[0] as FactoryGraphTopology["edges"][number],
    );

    expect(nextDraft.edgeChanges.additions).toEqual([]);
    expect(nextDraft.edgeChanges.removals).toEqual([]);
  });
});
