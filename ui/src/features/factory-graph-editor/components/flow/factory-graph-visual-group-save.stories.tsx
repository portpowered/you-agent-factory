import { useCallback, useMemo, useState } from "react";
import { expect, userEvent, within } from "storybook/test";

import "../../../../styles.css";
import { baseFactoryDefinition } from "../../lib/draft/factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "../../lib/draft/factory-graph-draft-types";
import { applyFactoryGraphPendingEdits } from "../../lib/operations/factory-graph-operations";
import {
  factoryLayoutGroupById,
  factoryLayoutGroups,
} from "../../lib/layout/factory-graph-layout-groups";
import {
  createDefaultFactoryLayout,
  factoryLayoutFromDefinition,
} from "../../lib/layout/factory-graph-layout-operations";
import { useFactoryGraphLayoutDraftState } from "../../hooks/layout/factory-graph-layout-draft-hook";

type WorkflowPhase =
  | "idle"
  | "created"
  | "renamed"
  | "assigned"
  | "moved"
  | "resized"
  | "deleted"
  | "saved"
  | "reloaded"
  | "undone"
  | "redone";

function VisualGroupSaveWorkflowStory() {
  const [workflowPhase, setWorkflowPhase] = useState<WorkflowPhase>("idle");
  const [savedLayout, setSavedLayout] = useState(createDefaultFactoryLayout());
  const layoutDocument = useMemo(
    () => ({
      ...baseFactoryDefinition,
      layout: createDefaultFactoryLayout(),
    }),
    [],
  );
  const draft = useFactoryGraphLayoutDraftState({
    currentFactoryDocument: layoutDocument,
    factoryDocumentScopeKey: "visual-group-save-workflow",
  });

  const selectedGroupId = factoryLayoutGroups(draft.layout)[0]?.id ?? null;
  const selectedGroup = selectedGroupId
    ? factoryLayoutGroupById(draft.layout, selectedGroupId)
    : undefined;

  const runCreate = useCallback(() => {
    draft.createVisualGroup({ x: 120, y: 80 });
    setWorkflowPhase("created");
  }, [draft]);

  const runRename = useCallback(() => {
    if (!selectedGroupId) {
      return;
    }
    draft.renameVisualGroup(selectedGroupId, "Planning lane");
    setWorkflowPhase("renamed");
  }, [draft, selectedGroupId]);

  const runAssign = useCallback(() => {
    if (!selectedGroupId) {
      return;
    }
    draft.addNodeToVisualGroup(selectedGroupId, "workstation:draft");
    setWorkflowPhase("assigned");
  }, [draft, selectedGroupId]);

  const runMove = useCallback(() => {
    if (!selectedGroupId) {
      return;
    }
    draft.moveVisualGroupByDelta(selectedGroupId, { x: 12, y: 8 });
    setWorkflowPhase("moved");
  }, [draft, selectedGroupId]);

  const runResize = useCallback(() => {
    if (!selectedGroupId) {
      return;
    }
    draft.resizeVisualGroup(selectedGroupId, {
      height: 280,
      width: 420,
      x: 40,
      y: 30,
    });
    setWorkflowPhase("resized");
  }, [draft, selectedGroupId]);

  const runDelete = useCallback(() => {
    if (!selectedGroupId) {
      return;
    }
    draft.deleteVisualGroup(selectedGroupId);
    setWorkflowPhase("deleted");
  }, [draft, selectedGroupId]);

  const runSave = useCallback(() => {
    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition: layoutDocument,
      draft: createEmptyFactoryGraphDraft(),
      pendingLayout: draft.layout,
    });
    if (!saveInput.ok) {
      return;
    }
    const persistedLayout = factoryLayoutFromDefinition(saveInput.value);
    setSavedLayout(persistedLayout);
    draft.adoptSavedLayout(persistedLayout);
    setWorkflowPhase("saved");
  }, [draft, layoutDocument]);

  const runReload = useCallback(() => {
    draft.adoptSavedLayout(savedLayout);
    setWorkflowPhase("reloaded");
  }, [draft, savedLayout]);

  const runUndo = useCallback(() => {
    draft.undoLayout();
    setWorkflowPhase("undone");
  }, [draft]);

  const runRedo = useCallback(() => {
    draft.redoLayout();
    setWorkflowPhase("redone");
  }, [draft]);

  return (
    <div
      className="grid max-w-xl gap-3 rounded-[1.5rem] border border-outline bg-surface-container-high p-4"
      data-visual-group-workflow-phase={workflowPhase}
    >
      <p className="m-0 text-sm text-on-surface">
        Visual group save workflow harness
      </p>
      <div className="flex flex-wrap gap-2">
        <button onClick={runCreate} type="button">
          Create group
        </button>
        <button onClick={runRename} type="button">
          Rename group
        </button>
        <button onClick={runAssign} type="button">
          Assign node
        </button>
        <button onClick={runMove} type="button">
          Move group
        </button>
        <button onClick={runResize} type="button">
          Resize group
        </button>
        <button onClick={runDelete} type="button">
          Delete group
        </button>
        <button onClick={runSave} type="button">
          Save layout
        </button>
        <button onClick={runReload} type="button">
          Reload layout
        </button>
        <button disabled={!draft.canUndoLayout} onClick={runUndo} type="button">
          Undo layout
        </button>
        <button disabled={!draft.canRedoLayout} onClick={runRedo} type="button">
          Redo layout
        </button>
      </div>
      <dl className="m-0 grid gap-1 text-sm text-on-surface">
        <div>
          <dt className="inline font-semibold">Phase:</dt>{" "}
          <dd className="inline" data-visual-group-workflow-phase-label="">
            {workflowPhase}
          </dd>
        </div>
        <div>
          <dt className="inline font-semibold">Layout dirty:</dt>{" "}
          <dd className="inline" data-visual-group-layout-dirty="">
            {draft.layoutDirty ? "true" : "false"}
          </dd>
        </div>
        <div>
          <dt className="inline font-semibold">Group label:</dt>{" "}
          <dd className="inline" data-visual-group-selected-label="">
            {selectedGroup?.label ?? "none"}
          </dd>
        </div>
        <div>
          <dt className="inline font-semibold">Member count:</dt>{" "}
          <dd className="inline" data-visual-group-member-count="">
            {selectedGroup?.nodeIds?.length ?? 0}
          </dd>
        </div>
      </dl>
    </div>
  );
}

export default {
  title: "Factory Graph Editor/Visual Groups",
  tags: ["test"],
};

export const SaveReloadWorkflow = {
  render: () => <VisualGroupSaveWorkflowStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();

    const root = canvasElement.querySelector(
      "[data-visual-group-workflow-phase]",
    );
    if (!root) {
      throw new Error("Expected visual group workflow harness.");
    }

    await user.click(canvas.getByRole("button", { name: "Create group" }));
    await expect(root).toHaveAttribute("data-visual-group-workflow-phase", "created");

    await user.click(canvas.getByRole("button", { name: "Rename group" }));
    await expect(
      canvasElement.querySelector("[data-visual-group-selected-label]"),
    ).toHaveTextContent("Planning lane");

    await user.click(canvas.getByRole("button", { name: "Assign node" }));
    await expect(
      canvasElement.querySelector("[data-visual-group-member-count]"),
    ).toHaveTextContent("1");

    await user.click(canvas.getByRole("button", { name: "Move group" }));
    await user.click(canvas.getByRole("button", { name: "Resize group" }));
    await user.click(canvas.getByRole("button", { name: "Delete group" }));
    await expect(root).toHaveAttribute("data-visual-group-workflow-phase", "deleted");

    await user.click(canvas.getByRole("button", { name: "Create group" }));
    await user.click(canvas.getByRole("button", { name: "Rename group" }));
    await user.click(canvas.getByRole("button", { name: "Assign node" }));
    await user.click(canvas.getByRole("button", { name: "Move group" }));
    await user.click(canvas.getByRole("button", { name: "Resize group" }));
    await user.click(canvas.getByRole("button", { name: "Save layout" }));
    await expect(
      canvasElement.querySelector("[data-visual-group-layout-dirty]"),
    ).toHaveTextContent("false");

    await user.click(canvas.getByRole("button", { name: "Reload layout" }));
    await expect(root).toHaveAttribute("data-visual-group-workflow-phase", "reloaded");
    await expect(
      canvasElement.querySelector("[data-visual-group-selected-label]"),
    ).toHaveTextContent("Planning lane");

    await user.click(canvas.getByRole("button", { name: "Move group" }));
    await user.click(canvas.getByRole("button", { name: "Undo layout" }));
    await user.click(canvas.getByRole("button", { name: "Redo layout" }));
    await expect(root).toHaveAttribute("data-visual-group-workflow-phase", "redone");
  },
};
