import { renderHook, act } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { GraphEditorPlacementProvider } from "./graph-editor-placement-context";
import { useFactoryGraphAddEntityController } from "./react-flow-current-activity-card-editor-chrome";

const placeAddedNode = vi.fn();

vi.mock("./graph-editor-placement-context", async () => {
  const actual = await vi.importActual("./graph-editor-placement-context");
  return {
    ...actual,
    useGraphEditorPlaceAddedNode: () => placeAddedNode,
  };
});

function buildEditableGraph(): EditableFactoryGraphViewModel {
  return {
    actions: {
      addNode: vi.fn(() => ({ ok: true, value: {} })),
      connectNodes: vi.fn(),
      discard: vi.fn(),
      disconnectEdge: vi.fn(),
      removeNode: vi.fn(),
      save: vi.fn(async () => true),
      updateNodeField: vi.fn(),
    },
  } as unknown as EditableFactoryGraphViewModel;
}

describe("useFactoryGraphAddEntityController placement", () => {
  it("places a newly added node after a successful add submit", () => {
    placeAddedNode.mockReset();
    const editableGraph = buildEditableGraph();
    const setActiveTool = vi.fn();

    const { result } = renderHook(
      () =>
        useFactoryGraphAddEntityController({
          currentFactoryDefinition: {
            name: "factory",
            resources: [],
            workers: [],
            workTypes: [],
            workstations: [],
          },
          editableGraph,
          setActiveTool,
        }),
      { wrapper: GraphEditorPlacementProvider },
    );

    act(() => {
      result.current.setAddEntityDraft({
        kind: "worker",
        model: "gpt",
        name: "reviewer",
      });
    });

    act(() => {
      result.current.handleAddEntitySubmit();
    });

    expect(editableGraph.actions.addNode).toHaveBeenCalledTimes(1);
    expect(placeAddedNode).toHaveBeenCalledWith({
      kind: "worker",
      model: "gpt",
      name: "reviewer",
    });
  });
});
