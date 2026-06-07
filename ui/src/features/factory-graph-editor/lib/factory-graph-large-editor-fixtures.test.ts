import { describe, expect, it } from "vitest";

import {
  FACTORY_GRAPH_LARGE_EDITOR_FIXTURE_TARGETS,
  factoryGraphLargeEditorFixtures,
} from "./factory-graph-large-editor-fixtures";

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
});
