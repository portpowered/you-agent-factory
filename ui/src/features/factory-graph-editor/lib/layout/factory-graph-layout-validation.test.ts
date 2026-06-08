// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: layout validation scenarios stay grouped around shared topology fixtures.
import { describe, expect, it } from "vitest";

import { buildFactoryGraphTopologyFromDefinition } from "../draft/factory-graph-draft-graph";
import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import { setFactoryLayoutEdgeWaypoints } from "./factory-graph-layout-edge-waypoints";
import { createDefaultFactoryLayout } from "./factory-graph-layout-operations";
import {
  collectFactoryLayoutEdgeValidationTargets,
  FACTORY_LAYOUT_VALIDATION_CODE,
  factoryLayoutTopologyEdgeIds,
  preparePendingFactoryLayoutForSave,
  projectFactoryLayoutValidationTargets,
  pruneFactoryLayoutEdgesForTopology,
  resolveFactoryLayoutEdgeWaypointsForRendering,
} from "./factory-graph-layout-validation";

const VALID_EDGE_ID =
  "workstation-output:workstation:draft->work-state:story:done";
const STALE_EDGE_ID =
  "workstation-output:workstation:missing->work-state:story:done";

describe("factory-graph-layout-validation", () => {
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
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
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

    expect(projectFactoryLayoutValidationTargets(layout, validEdgeIds)).toEqual([
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

  it("prepares pending layout for save by pruning stale edge waypoints and reporting outcomes", () => {
    const topology = buildFactoryGraphTopologyFromDefinition(
      baseFactoryDefinition,
    );
    const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
    const layout: ReturnType<typeof createDefaultFactoryLayout> = {
      schemaVersion: 1,
      edges: [
        {
          id: STALE_EDGE_ID,
          waypoints: [{ x: 10, y: 20 }],
        },
      ],
    };

    const prepared = preparePendingFactoryLayoutForSave(layout, validEdgeIds);

    expect(prepared.layout.edges).toBeUndefined();
    expect(prepared.layoutOutcomes).toEqual([
      expect.objectContaining({
        code: FACTORY_LAYOUT_VALIDATION_CODE.unknownEdgeReference,
        subject: {
          id: STALE_EDGE_ID,
          location: "REFERENCE",
          type: "FACTORY",
        },
      }),
    ]);
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
});
