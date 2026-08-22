import { describe, expect, it } from "vitest";

import {
  buildGridAutoLayoutPositionsByNodeId,
  buildLargeFactoryEditorFixture,
  buildLargeFactoryEditorParityFixture,
  FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS,
  factoryGraphLargeEditorFixtures,
} from "../fixtures/factory-graph-large-editor-fixtures";

describe("factory graph large editor fixtures", () => {
  it("covers the required 100, 500, and stress 1000 node graph sizes", () => {
    expect(
      factoryGraphLargeEditorFixtures.hundred.graphNodeCount,
    ).toBeGreaterThanOrEqual(
      FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS.hundred,
    );
    expect(
      factoryGraphLargeEditorFixtures.fiveHundred.graphNodeCount,
    ).toBeGreaterThanOrEqual(
      FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS.fiveHundred,
    );
    expect(
      factoryGraphLargeEditorFixtures.stressThousand.graphNodeCount,
    ).toBeGreaterThanOrEqual(
      FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS.stressThousand,
    );
  });

  it("builds repeatable fixtures for arbitrary graph-node targets", () => {
    const fixture = buildLargeFactoryEditorFixture(12, "hundred");

    expect(fixture.fixtureKey).toBe("hundred");
    expect(fixture.targetGraphNodeCount).toBe(12);
    expect(fixture.graphNodeCount).toBeGreaterThanOrEqual(12);
    expect(fixture.layout.nodes?.length).toBeGreaterThan(0);
    expect(fixture.topology.nodes.length).toBe(fixture.graphNodeCount);
  });

  it("builds deterministic grid auto-layout positions for browser verification", () => {
    const positions = buildGridAutoLayoutPositionsByNodeId([
      "workstation:ws-0",
      "worker:processor",
      "workstation:ws-1",
    ]);

    expect(positions.get("workstation:ws-0")).toEqual({ x: 0, y: 0 });
    expect(positions.get("worker:processor")).toEqual({ x: 180, y: 0 });
    expect(positions.get("workstation:ws-1")).toEqual({ x: 360, y: 0 });
  });

  it("includes representative shared layout metadata without mutating topology", () => {
    for (const fixture of Object.values(factoryGraphLargeEditorFixtures)) {
      expect(fixture.layout.schemaVersion).toBe(1);
      expect(fixture.layout.nodes?.length ?? 0).toBeGreaterThan(0);
      expect(fixture.layout.viewport).toEqual({
        x: 0,
        y: 0,
        zoom: 0.75,
      });
      expect(fixture.topology.nodes.length).toBe(fixture.graphNodeCount);
      expect(fixture.factoryDefinition.workstations?.length).toBeGreaterThan(0);
    }
  });

  it("builds the combined large visual-parity scenario", () => {
    const parityFixture = buildLargeFactoryEditorParityFixture(
      factoryGraphLargeEditorFixtures.fiveHundred,
    );

    expect(parityFixture.groups.map((group) => group.color)).toEqual([
      "info",
      "warning",
      "success",
    ]);
    expect(parityFixture.authoredSizeByNodeId.size).toBeGreaterThanOrEqual(3);
    expect(parityFixture.workStateNodeIds.length).toBeGreaterThanOrEqual(4);
    expect(parityFixture.layout.groups).toEqual(parityFixture.groups);
    expect(parityFixture.layout.nodes).not.toEqual(
      parityFixture.fixture.layout.nodes,
    );
    expect(parityFixture.fixture.topology.nodes.length).toBeGreaterThanOrEqual(
      FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS.fiveHundred,
    );
  });
});
