import {
  applyFactoryGraphEdgeRemoval,
  applyFactoryGraphEdgeAddition,
  buildFactoryGraphConnectionNotice,
  buildFactoryGraphEdgeChangeFromConnection,
  getFactoryGraphConnectionAnchors,
} from "./factory-graph-editor-connections";
import type { FactoryGraphDraft, FactoryGraphTopology } from "./factory-graph-draft-types";

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

describe("factory graph editor connections", () => {
  it("exposes separate workstation transition anchors for success, continue, failure, and rejection", () => {
    expect(getFactoryGraphConnectionAnchors("workstation")).toEqual(
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

  it("maps compatible anchor selections into a draft edge addition", () => {
    const edgeChange = buildFactoryGraphEdgeChangeFromConnection(
      connectableTopology,
      {
        sourceAnchorId: "workstation-on-failure-source",
        sourceNodeId: "workstation:review",
        targetAnchorId: "workstation-on-failure-target",
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
        targetAnchorId: "workstation-on-continue-target",
        targetNodeId: "work-state:story:queued",
      }),
    ).toBeNull();

    expect(
      buildFactoryGraphConnectionNotice({
        sourceAnchorId: "workstation-on-failure-source",
        sourceNode: connectableTopology.nodes[1] as FactoryGraphTopology["nodes"][number],
        targetAnchorId: "workstation-on-continue-target",
        targetNode: connectableTopology.nodes[0] as FactoryGraphTopology["nodes"][number],
      }),
    ).toBe(
      "Failure connections from review cannot connect to Continue on story:queued.",
    );
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
