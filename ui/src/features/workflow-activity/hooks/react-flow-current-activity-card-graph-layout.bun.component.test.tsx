import { beforeEach, describe, expect, it, mock } from "bun:test";
import { renderHook, waitFor } from "@testing-library/react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { GraphLayout } from "../../flowchart/lib/layout";
import {
  type CurrentActivityGraphLayoutBuilder,
  resetCurrentActivityGraphLayoutCacheForTests,
  useCurrentActivityGraphLayoutForFactory,
} from "./react-flow-current-activity-card-graph-layout";

const factory = {
  name: "injected-layout-factory",
  workTypes: [],
  workstations: [],
};

const snapshot = {
  factory,
} as DashboardSnapshot;

const explicitLayout: GraphLayout = {
  edges: [
    {
      edgeId: "workstation-output:review->done",
      fromNodeId: "workstation:review",
      label: "done",
      labelX: 0,
      labelY: 0,
      outcomeKind: "success",
      path: "M0 0",
      sourcePlaceKind: undefined,
      stateCategory: "TERMINAL",
      targetPlaceKind: "work_state",
      toNodeId: "work-state:story:done",
    },
  ],
  height: 240,
  nodes: [
    {
      column: 0,
      height: 120,
      nodeId: "workstation:review",
      nodeKind: "workstation",
      row: 0,
      width: 220,
      workstationNodeId: "review",
      x: 40,
      y: 60,
    },
    {
      column: 1,
      height: 86,
      nodeId: "work-state:story:done",
      nodeKind: "state_position",
      place: {
        kind: "work_state",
        place_id: "story:done",
        state_category: "TERMINAL",
        state_value: "done",
        type_id: "story",
      },
      row: 0,
      width: 168,
      x: 340,
      y: 80,
    },
  ],
  width: 508,
};

describe("Current activity graph layout builder contract", () => {
  beforeEach(() => {
    resetCurrentActivityGraphLayoutCacheForTests();
  });

  it("uses the injected builder and preserves its canonical positions and edges", async () => {
    const buildLayout = mock<CurrentActivityGraphLayoutBuilder>(
      async (
        _factory,
        _hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>,
        _visibilityPreset,
      ) => explicitLayout,
    );

    const { result } = renderHook(() =>
      useCurrentActivityGraphLayoutForFactory(
        snapshot,
        snapshot.factory,
        new Set(),
        "all",
        buildLayout,
      ),
    );

    await waitFor(() => {
      expect(result.current.nodes).toHaveLength(2);
    });

    expect(buildLayout).toHaveBeenCalledTimes(1);
    expect(buildLayout.mock.calls[0]?.[0]).toEqual(factory);
    expect(result.current).toEqual(explicitLayout);
  });

  it("does not construct a layout when the feature has no canonical factory", async () => {
    const buildLayout = mock<CurrentActivityGraphLayoutBuilder>(
      async () => explicitLayout,
    );

    const { result } = renderHook(() =>
      useCurrentActivityGraphLayoutForFactory(
        { factory: undefined } as DashboardSnapshot,
        undefined,
        new Set(),
        "all",
        buildLayout,
      ),
    );

    await waitFor(() => {
      expect(result.current.nodes).toHaveLength(0);
      expect(result.current.edges).toHaveLength(0);
    });

    expect(buildLayout).not.toHaveBeenCalled();
  });

  it("deduplicates concurrent requests through the injected builder", async () => {
    const buildLayout = mock<CurrentActivityGraphLayoutBuilder>(
      async () => explicitLayout,
    );
    const replacementLayout: GraphLayout = {
      ...explicitLayout,
      nodes: explicitLayout.nodes.map((node) => ({
        ...node,
        nodeId: `${node.nodeId}:replacement`,
      })),
    };
    const replacementBuilder = mock<CurrentActivityGraphLayoutBuilder>(
      async () => replacementLayout,
    );

    const first = renderHook(
      ({ builder }) =>
        useCurrentActivityGraphLayoutForFactory(
          snapshot,
          snapshot.factory,
          new Set(),
          "all",
          builder,
        ),
      { initialProps: { builder: buildLayout } },
    );
    const second = renderHook(() =>
      useCurrentActivityGraphLayoutForFactory(
        snapshot,
        snapshot.factory,
        new Set(),
        "all",
        buildLayout,
      ),
    );

    await waitFor(() => {
      expect(first.result.current.nodes).toHaveLength(2);
      expect(second.result.current.nodes).toHaveLength(2);
    });

    expect(buildLayout).toHaveBeenCalledTimes(1);

    first.rerender({ builder: replacementBuilder });

    await waitFor(() => {
      expect(first.result.current.nodes[0]?.nodeId).toBe(
        "workstation:review:replacement",
      );
    });

    expect(replacementBuilder).toHaveBeenCalledTimes(1);
    expect(second.result.current).toEqual(explicitLayout);
  });
});
