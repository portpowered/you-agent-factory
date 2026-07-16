// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: layout validation scenarios stay grouped around shared topology fixtures.
// biome-ignore-all lint/style/noExcessiveLinesPerFile: layout validation scenarios stay grouped around shared topology fixtures.
import { describe, expect, it } from "vitest";

import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import { setFactoryLayoutEdgeWaypoints } from "./factory-graph-layout-edge-waypoints";
import { createDefaultFactoryLayout } from "./factory-graph-layout-operations";
import {
  collectFactoryLayoutEdgeValidationTargets,
  collectFactoryLayoutGroupValidationTargets,
  FACTORY_LAYOUT_VALIDATION_CODE,
  factoryLayoutTopologyEdgeIds,
  factoryLayoutTopologyNodeIds,
  preparePendingFactoryLayoutForSave,
  projectFactoryLayoutValidationTargets,
  pruneFactoryLayoutEdgesForTopology,
  pruneFactoryLayoutGroupsForTopology,
  resolveFactoryLayoutEdgeWaypointsForRendering,
} from "./factory-graph-layout-validation";

const VALID_EDGE_ID =
  "workstation-output:workstation:draft->work-state:story:done";
const STALE_EDGE_ID =
  "workstation-output:workstation:missing->work-state:story:done";

describe("factory-graph-layout-validation", () => {
  it.each([
    {
      name: "reports only stale-edge references when stale edges also carry poisoned geometry",
      layout: {
        schemaVersion: 1,
        edges: [
          {
            id: STALE_EDGE_ID,
            waypoints: [
              { x: Number.NaN, y: 20 },
              { x: Number.POSITIVE_INFINITY, y: 30 },
            ],
            labelPosition: { x: Number.NEGATIVE_INFINITY, y: 0 },
          },
        ],
      } satisfies ReturnType<typeof createDefaultFactoryLayout>,
      expected: [
        {
          code: FACTORY_LAYOUT_VALIDATION_CODE.unknownEdgeReference,
          path: "factory.layout.edges[0].id",
        },
      ],
    },
    {
      name: "reports every non-finite waypoint on one valid edge in authored order",
      layout: {
        schemaVersion: 1,
        edges: [
          {
            id: VALID_EDGE_ID,
            waypoints: [
              { x: Number.NaN, y: 20 },
              { x: 40, y: 50 },
              { x: 60, y: Number.NEGATIVE_INFINITY },
            ],
          },
        ],
      } satisfies ReturnType<typeof createDefaultFactoryLayout>,
      expected: [
        {
          code: FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry,
          path: "factory.layout.edges[0].waypoints[0]",
        },
        {
          code: FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry,
          path: "factory.layout.edges[0].waypoints[2]",
        },
      ],
    },
    {
      name: "ignores geometry on edge entries that never resolved to an id",
      layout: {
        schemaVersion: 1,
        edges: [
          {
            waypoints: [
              { x: Number.NaN, y: 20 },
              { x: 40, y: Number.POSITIVE_INFINITY },
            ],
          } as { id?: string; waypoints: { x: number; y: number }[] },
        ],
      } satisfies ReturnType<typeof createDefaultFactoryLayout>,
      expected: [],
    },
  ])("$name", ({ layout, expected }) => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);

    expect(collectFactoryLayoutEdgeValidationTargets(layout, validEdgeIds)).toEqual(
      expected,
    );
  });

  it("reports stale edge layout references against pending topology", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
    const layout = setFactoryLayoutEdgeWaypoints(
      setFactoryLayoutEdgeWaypoints(createDefaultFactoryLayout(), VALID_EDGE_ID, [
        { x: 120, y: 80 },
      ]),
      STALE_EDGE_ID,
      [{ x: 40, y: 60 }],
    );

    expect(collectFactoryLayoutEdgeValidationTargets(layout, validEdgeIds)).toEqual([
      {
        code: FACTORY_LAYOUT_VALIDATION_CODE.unknownEdgeReference,
        path: "factory.layout.edges[1].id",
      },
    ]);
  });

  it("reports invalid waypoint geometry as recoverable layout validation targets", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      edges: [
        {
          id: VALID_EDGE_ID,
          waypoints: [
            { x: 100, y: 50 },
            { x: Number.NaN, y: 20 },
          ],
        },
      ],
    };

    expect(collectFactoryLayoutEdgeValidationTargets(layout, validEdgeIds)).toEqual([
      {
        code: FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry,
        path: "factory.layout.edges[0].waypoints[1]",
      },
    ]);
  });

  it("prunes stale edge layout and invalid waypoint geometry before persistence", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      edges: [
        {
          id: VALID_EDGE_ID,
          waypoints: [{ x: Number.POSITIVE_INFINITY, y: 10 }],
        },
        {
          id: STALE_EDGE_ID,
          waypoints: [{ x: 10, y: 20 }],
        },
      ],
    };

    const { layout: prunedLayout, prunedEdgeIds } =
      pruneFactoryLayoutEdgesForTopology(layout, validEdgeIds);

    expect(prunedEdgeIds).toEqual([VALID_EDGE_ID, STALE_EDGE_ID]);
    expect(prunedLayout.edges).toBeUndefined();
  });

  it("keeps valid authored waypoints for unchanged topology edges", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
    const layout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      VALID_EDGE_ID,
      [
        { x: 180, y: 220 },
        { x: 240, y: 260 },
      ],
    );

    const { layout: prunedLayout, prunedEdgeIds } =
      pruneFactoryLayoutEdgesForTopology(layout, validEdgeIds);

    expect(prunedEdgeIds).toEqual([]);
    expect(prunedLayout.edges).toEqual([
      {
        id: VALID_EDGE_ID,
        waypoints: [
          { x: 180, y: 220 },
          { x: 240, y: 260 },
        ],
      },
    ]);
  });

  it("projects recoverable layout validation targets for editor and save flows", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      edges: [
        {
          id: VALID_EDGE_ID,
          waypoints: [{ x: Number.NaN, y: 10 }],
        },
        {
          id: STALE_EDGE_ID,
          waypoints: [{ x: 10, y: 20 }],
        },
      ],
    };

    expect(projectFactoryLayoutValidationTargets(layout, topology)).toEqual([
      expect.objectContaining({
        code: FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry,
        severity: "warning",
        subject: {
          id: VALID_EDGE_ID,
          location: "REFERENCE",
          type: "FACTORY",
        },
      }),
      expect.objectContaining({
        code: FACTORY_LAYOUT_VALIDATION_CODE.unknownEdgeReference,
        severity: "warning",
        subject: {
          id: STALE_EDGE_ID,
          location: "REFERENCE",
          type: "FACTORY",
        },
      }),
    ]);
  });

  it("prepares pending layout for save by pruning stale edge waypoints", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      edges: [
        {
          id: STALE_EDGE_ID,
          waypoints: [{ x: 10, y: 20 }],
        },
      ],
    };

    const prepared = preparePendingFactoryLayoutForSave(layout, topology);

    expect(prepared.layout.edges).toBeUndefined();
  });

  it("falls back to generated routing when only invalid waypoint geometry remains", () => {
    const layout = setFactoryLayoutEdgeWaypoints(
      createDefaultFactoryLayout(),
      VALID_EDGE_ID,
      [
        { x: Number.NaN, y: 10 },
        { x: 20, y: Number.NaN },
      ],
    );

    expect(
      resolveFactoryLayoutEdgeWaypointsForRendering(layout, VALID_EDGE_ID),
    ).toBeUndefined();
  });

  it("ignores layout edge entries without ids when collecting validation targets", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      edges: [{ waypoints: [{ x: 1, y: 2 }] } as { id?: string; waypoints: { x: number; y: number }[] }],
    };

    expect(collectFactoryLayoutEdgeValidationTargets(layout, validEdgeIds)).toEqual(
      [],
    );
  });

  it("returns the original layout when there are no edge layout entries to prune", () => {
    const layout = createDefaultFactoryLayout();

    expect(pruneFactoryLayoutEdgesForTopology(layout, new Set())).toEqual({
      layout,
      prunedEdgeIds: [],
    });
  });

  it("preserves valid label positions without authored waypoints", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      edges: [
        {
          id: VALID_EDGE_ID,
          labelPosition: { x: 140, y: 160 },
        },
      ],
    };

    const { layout: prunedLayout, prunedEdgeIds } =
      pruneFactoryLayoutEdgesForTopology(layout, validEdgeIds);

    expect(prunedEdgeIds).toEqual([]);
    expect(prunedLayout.edges).toEqual([
      {
        id: VALID_EDGE_ID,
        labelPosition: { x: 140, y: 160 },
      },
    ]);
  });

  it("prunes topology edges that only retain empty layout metadata", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      edges: [{ id: VALID_EDGE_ID }],
    };

    const { layout: prunedLayout, prunedEdgeIds } =
      pruneFactoryLayoutEdgesForTopology(layout, validEdgeIds);

    expect(prunedEdgeIds).toEqual([VALID_EDGE_ID]);
    expect(prunedLayout.edges).toBeUndefined();
  });

  it.each([
    {
      name: "drops non-finite label position while preserving finite authored waypoints",
      layout: {
        schemaVersion: 1,
        edges: [
          {
            id: VALID_EDGE_ID,
            waypoints: [{ x: 100, y: 120 }],
            labelPosition: { x: Number.NaN, y: 160 },
          },
        ],
      } satisfies ReturnType<typeof createDefaultFactoryLayout>,
      expectedPrunedEdgeIds: [],
      expectedEdges: [
        {
          id: VALID_EDGE_ID,
          waypoints: [{ x: 100, y: 120 }],
        },
      ],
    },
    {
      name: "preserves duplicate finite waypoints on valid edges while pruning a stale sibling",
      layout: {
        edges: [
          {
            id: VALID_EDGE_ID,
            waypoints: [
              { x: 180, y: 220 },
              { x: 180, y: 220 },
              { x: 260, y: 280 },
            ],
          },
          {
            id: STALE_EDGE_ID,
            waypoints: [{ x: 10, y: 20 }],
          },
        ],
      } satisfies ReturnType<typeof createDefaultFactoryLayout>,
      expectedPrunedEdgeIds: [STALE_EDGE_ID],
      expectedEdges: [
        {
          id: VALID_EDGE_ID,
          waypoints: [
            { x: 180, y: 220 },
            { x: 180, y: 220 },
            { x: 260, y: 280 },
          ],
        },
      ],
    },
  ])("$name", ({ layout, expectedPrunedEdgeIds, expectedEdges }) => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);

    const { layout: prunedLayout, prunedEdgeIds } =
      pruneFactoryLayoutEdgesForTopology(layout, validEdgeIds);

    expect(prunedEdgeIds).toEqual(expectedPrunedEdgeIds);
    expect(prunedLayout.edges).toEqual(expectedEdges);
  });

  it("projects group layout validation targets with recoverable warning messages", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      groups: [
        {
          bounds: { x: 10, y: Number.NaN, width: 320, height: 180 },
          id: "broken-lane",
          nodeIds: ["workstation:draft", "workstation:missing"],
        },
      ],
    };

    expect(projectFactoryLayoutValidationTargets(layout, topology)).toEqual([
      expect.objectContaining({
        code: FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry,
        message:
          'Layout group "broken-lane" bounds contain non-finite geometry.',
      }),
      expect.objectContaining({
        code: FACTORY_LAYOUT_VALIDATION_CODE.unknownGroupMemberReference,
        message:
          'Layout group "broken-lane" references unknown graph node "workstation:missing".',
      }),
    ]);
  });

  it("ignores groups without ids and blank group member ids when collecting targets", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validNodeIds = factoryLayoutTopologyNodeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      groups: [
        {
          bounds: { x: 0, y: 0, width: 100, height: 100 },
          id: "",
          nodeIds: ["workstation:missing"],
        },
        {
          bounds: { x: 40, y: 60, width: 120, height: 80 },
          id: "valid-lane",
          nodeIds: ["", "workstation:missing"],
        },
      ],
    };

    expect(
      collectFactoryLayoutGroupValidationTargets(layout, validNodeIds),
    ).toEqual([
      {
        code: FACTORY_LAYOUT_VALIDATION_CODE.unknownGroupMemberReference,
        path: "factory.layout.groups[1].nodeIds[1]",
      },
    ]);
  });

  it("reports stale group member references against pending topology", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validNodeIds = factoryLayoutTopologyNodeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      groups: [
        {
          bounds: { x: 10, y: 20, width: 320, height: 180 },
          id: "review-lane",
          label: "Review",
          nodeIds: ["workstation:draft", "workstation:missing"],
        },
      ],
    };

    expect(
      collectFactoryLayoutGroupValidationTargets(layout, validNodeIds),
    ).toEqual([
      {
        code: FACTORY_LAYOUT_VALIDATION_CODE.unknownGroupMemberReference,
        path: "factory.layout.groups[0].nodeIds[1]",
      },
    ]);
  });

  it("reports non-finite group bounds as recoverable layout validation targets", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validNodeIds = factoryLayoutTopologyNodeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      groups: [
        {
          bounds: {
            x: 10,
            y: Number.NaN,
            width: 320,
            height: 180,
          },
          id: "broken-lane",
          nodeIds: ["workstation:draft"],
        },
      ],
    };

    expect(
      collectFactoryLayoutGroupValidationTargets(layout, validNodeIds),
    ).toEqual([
      {
        code: FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry,
        path: "factory.layout.groups[0].bounds",
      },
    ]);
  });

  it("prunes stale group members while preserving empty groups and group metadata", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validNodeIds = factoryLayoutTopologyNodeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      groups: [
        {
          bounds: { x: 360, y: 120, width: 520, height: 360 },
          color: "blue",
          id: "review-lane",
          label: "Review",
          locked: false,
          nodeIds: ["workstation:draft", "workstation:missing"],
          parentGroupId: null,
        },
        {
          bounds: { x: 0, y: 0, width: 10, height: 10 },
          id: "empty-lane",
          nodeIds: ["workstation:missing"],
        },
      ],
    };

    const { layout: prunedLayout, prunedGroupMemberNodeIds, rejectedGroupIds } =
      pruneFactoryLayoutGroupsForTopology(layout, validNodeIds);

    expect(prunedGroupMemberNodeIds).toEqual([
      "workstation:missing",
      "workstation:missing",
    ]);
    expect(rejectedGroupIds).toEqual([]);
    expect(prunedLayout.groups).toEqual([
      {
        bounds: { x: 360, y: 120, width: 520, height: 360 },
        color: "blue",
        id: "review-lane",
        label: "Review",
        locked: false,
        nodeIds: ["workstation:draft"],
        parentGroupId: null,
      },
      {
        bounds: { x: 0, y: 0, width: 10, height: 10 },
        id: "empty-lane",
        nodeIds: [],
      },
    ]);
  });

  it("drops groups without ids and blank member ids during topology pruning", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validNodeIds = factoryLayoutTopologyNodeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      groups: [
        {
          bounds: { x: 0, y: 0, width: 120, height: 80 },
          id: "",
          nodeIds: ["workstation:missing"],
        },
        {
          bounds: { x: 40, y: 60, width: 120, height: 80 },
          id: "valid-lane",
          nodeIds: ["", "workstation:missing", "workstation:draft"],
        },
      ],
    };

    const { layout: prunedLayout, prunedGroupMemberNodeIds } =
      pruneFactoryLayoutGroupsForTopology(layout, validNodeIds);

    expect(prunedGroupMemberNodeIds).toEqual(["workstation:missing"]);
    expect(prunedLayout.groups).toEqual([
      {
        bounds: { x: 40, y: 60, width: 120, height: 80 },
        id: "valid-lane",
        nodeIds: ["workstation:draft"],
      },
    ]);
  });

  it("rejects groups with non-finite bounds during save preparation", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      groups: [
        {
          bounds: {
            x: 10,
            y: 20,
            width: Number.POSITIVE_INFINITY,
            height: 180,
          },
          id: "broken-lane",
          nodeIds: ["workstation:draft"],
        },
        {
          bounds: { x: 40, y: 60, width: 120, height: 80 },
          id: "valid-lane",
          nodeIds: ["workstation:draft"],
        },
      ],
    };

    const prepared = preparePendingFactoryLayoutForSave(layout, topology);

    expect(prepared.layout.groups).toEqual([
      {
        bounds: { x: 40, y: 60, width: 120, height: 80 },
        id: "valid-lane",
        nodeIds: ["workstation:draft"],
      },
    ]);
  });
});
