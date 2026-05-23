import { act, renderHook, waitFor } from "@testing-library/react";

import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { useFactoryGraphEditorLayoutPositions } from "./react-flow-current-activity-card-editor-layout";

const { mockBuildFactoryGraphEditorLayout } = vi.hoisted(() => ({
  mockBuildFactoryGraphEditorLayout: vi.fn(),
}));

vi.mock("../../factory-graph-editor/lib/factory-graph-editor-layout", () => ({
  buildFactoryGraphEditorLayout: mockBuildFactoryGraphEditorLayout,
}));

const EMPTY_TOPOLOGY: FactoryGraphTopology = {
  edges: [],
  nodes: [],
};

const SINGLE_NODE_TOPOLOGY: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "workstation:review",
      key: {
        kind: "workstation",
        name: "review",
      },
      kind: "workstation",
      label: "review",
    },
  ],
};

describe("factory graph editor layout positions", () => {
  beforeEach(() => {
    mockBuildFactoryGraphEditorLayout.mockReset();
  });

  it("returns the empty positions fallback without running layout work for empty topologies", () => {
    const { result } = renderHook(() =>
      useFactoryGraphEditorLayoutPositions(EMPTY_TOPOLOGY, "workflow:empty"),
    );

    expect(result.current.size).toBe(0);
    expect(mockBuildFactoryGraphEditorLayout).not.toHaveBeenCalled();
  });

  it("falls back to empty positions when editor layout computation fails", async () => {
    mockBuildFactoryGraphEditorLayout.mockRejectedValue(
      new Error("layout failed"),
    );

    const { result } = renderHook(() =>
      useFactoryGraphEditorLayoutPositions(
        SINGLE_NODE_TOPOLOGY,
        "workflow:single-node",
      ),
    );

    await act(async () => {
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(result.current.size).toBe(0);
    });
    expect(mockBuildFactoryGraphEditorLayout).toHaveBeenCalledWith(
      SINGLE_NODE_TOPOLOGY,
    );
  });

  it("reuses cached successful layouts without rerunning layout work on rerender", async () => {
    mockBuildFactoryGraphEditorLayout.mockResolvedValue({
      edges: [],
      nodes: [{ nodeId: "workstation:review", x: 24, y: 48 }],
    });

    const { result, rerender } = renderHook(
      ({ topologyKey }) =>
        useFactoryGraphEditorLayoutPositions(SINGLE_NODE_TOPOLOGY, topologyKey),
      {
        initialProps: {
          topologyKey: "workflow:single-node-success",
        },
      },
    );

    await waitFor(() => {
      expect(result.current.get("workstation:review")).toEqual({
        x: 24,
        y: 48,
      });
    });

    const resolvedPositions = result.current;
    rerender({ topologyKey: "workflow:single-node-success" });

    await waitFor(() => {
      expect(result.current.get("workstation:review")).toEqual({
        x: 24,
        y: 48,
      });
    });

    expect(result.current).toBe(resolvedPositions);
    expect(mockBuildFactoryGraphEditorLayout).toHaveBeenCalledTimes(1);
    expect(mockBuildFactoryGraphEditorLayout).toHaveBeenCalledWith(
      SINGLE_NODE_TOPOLOGY,
    );
  });
});
