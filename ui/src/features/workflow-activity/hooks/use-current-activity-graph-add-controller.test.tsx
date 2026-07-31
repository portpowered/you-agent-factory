import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { useFactoryGraphAddEntityController } from "./use-current-activity-graph-add-controller";

function buildEditableGraph(): EditableFactoryGraphViewModel {
  return {
    actions: {
      addNode: vi.fn(() => ({ ok: true, value: {} })),
      connectNodes: vi.fn(),
      discard: vi.fn(),
      disconnectEdge: vi.fn(),
      moveLayoutNode: vi.fn(),
      removeNode: vi.fn(),
      save: vi.fn(async () => true),
      updateNodeField: vi.fn(),
    },
  } as unknown as EditableFactoryGraphViewModel;
}

describe("useFactoryGraphAddEntityController doc add flow", () => {
  it("opens a doc add draft from the add menu", () => {
    const setActiveTool = vi.fn();
    const { result } = renderHook(() =>
      useFactoryGraphAddEntityController({
        currentFactoryDefinition: { name: "factory", workTypes: [] },
        editableGraph: buildEditableGraph(),
        setActiveTool,
      }),
    );

    act(() => {
      result.current.handleAddEntityAction("doc");
    });

    expect(result.current.addEntityDraft).toMatchObject({
      fileName: "new-doc.md",
      inlineContent: "",
      kind: "doc",
    });
    expect(setActiveTool).toHaveBeenCalledWith("add");
    expect(result.current.addMenuOpen).toBe(false);
  });
});

describe("useFactoryGraphAddEntityController add submit", () => {
  it("applies resolved layout placement after a successful add submit", () => {
    const editableGraph = buildEditableGraph();
    const setActiveTool = vi.fn();

    const { result } = renderHook(() =>
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
    );

    act(() => {
      result.current.setAddEntityDraft({
        kind: "worker",
        model: "gpt",
        modelProvider: "CODEX",
        name: "reviewer",
      });
    });

    act(() => {
      result.current.handleAddEntitySubmit({
        nodeId: "worker:reviewer",
        position: { x: 120, y: 240 },
      });
    });

    expect(editableGraph.actions.addNode).toHaveBeenCalledTimes(1);
    expect(editableGraph.actions.moveLayoutNode).toHaveBeenCalledWith(
      "worker:reviewer",
      { x: 120, y: 240 },
    );
    expect(setActiveTool).toHaveBeenCalledWith(null);
  });

  it("routes successful doc adds through onDocAdded with the canonical target path", () => {
    const editableGraph = buildEditableGraph();
    const onDocAdded = vi.fn();
    const setActiveTool = vi.fn();

    const { result } = renderHook(() =>
      useFactoryGraphAddEntityController({
        currentFactoryDefinition: {
          name: "factory",
          workTypes: [],
        },
        editableGraph,
        onDocAdded,
        setActiveTool,
      }),
    );

    act(() => {
      result.current.setAddEntityDraft({
        fileName: "playbook.md",
        inlineContent: "# Playbook\n",
        kind: "doc",
      });
    });

    act(() => {
      result.current.handleAddEntitySubmit();
    });

    expect(onDocAdded).toHaveBeenCalledWith("factory/docs/playbook.md");
    expect(editableGraph.actions.moveLayoutNode).not.toHaveBeenCalled();
  });

  it("ignores submit when no add draft is active", () => {
    const editableGraph = buildEditableGraph();

    const { result } = renderHook(() =>
      useFactoryGraphAddEntityController({
        currentFactoryDefinition: { name: "factory", workTypes: [] },
        editableGraph,
        setActiveTool: vi.fn(),
      }),
    );

    act(() => {
      result.current.handleAddEntitySubmit();
    });

    expect(editableGraph.actions.addNode).not.toHaveBeenCalled();
  });
});

describe("useFactoryGraphAddEntityController doc validation", () => {
  it("surfaces validation errors before attempting to add a doc", () => {
    const editableGraph = buildEditableGraph();
    const setActiveTool = vi.fn();

    const { result } = renderHook(() =>
      useFactoryGraphAddEntityController({
        currentFactoryDefinition: {
          name: "factory",
          supportingFiles: {
            bundledFiles: [
              {
                content: { encoding: "utf-8", inline: "# Guide\n" },
                targetPath: "factory/docs/guide.md",
                type: "DOC",
              },
            ],
          },
          workTypes: [],
        },
        editableGraph,
        setActiveTool,
      }),
    );

    act(() => {
      result.current.setAddEntityDraft({
        fileName: "guide.md",
        inlineContent: "# Duplicate\n",
        kind: "doc",
      });
    });

    act(() => {
      result.current.handleAddEntitySubmit();
    });

    expect(editableGraph.actions.addNode).not.toHaveBeenCalled();
    expect(result.current.addEntityErrors).toEqual({
      fileName: 'A doc at "factory/docs/guide.md" already exists in the draft.',
    });
  });

  it("surfaces add failures as field errors", () => {
    const editableGraph = buildEditableGraph();
    editableGraph.actions.addNode = vi.fn(() => ({
      fieldErrors: { fileName: "Duplicate doc path." },
      message: "Duplicate doc path.",
      ok: false,
    }));

    const { result } = renderHook(() =>
      useFactoryGraphAddEntityController({
        currentFactoryDefinition: { name: "factory", workTypes: [] },
        editableGraph,
        setActiveTool: vi.fn(),
      }),
    );

    act(() => {
      result.current.setAddEntityDraft({
        fileName: "guide.md",
        inlineContent: "# Guide\n",
        kind: "doc",
      });
    });

    act(() => {
      result.current.handleAddEntitySubmit();
    });

    expect(result.current.addEntityErrors).toEqual({
      fileName: "Duplicate doc path.",
    });
  });
});
