import { useState } from "react";

import { expect, userEvent, within } from "storybook/test";

import "../../../styles.css";
import { Button } from "../../../components/ui";
import {
  FactoryGraphEditorActionPopover,
  FactoryGraphEditorConfirmationDialog,
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorNotice,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
  FactoryGraphEditorToolbar,
} from "./factory-graph-editor-controls";

const ADD_MENU_ACTIONS = [
  {
    description: "Add a new workstation node to the factory draft.",
    id: "workstation",
    label: "Workstation",
  },
  {
    description: "Add a reusable worker to the factory draft.",
    id: "worker",
    label: "Worker",
  },
] as const;

function ObserveModeStory() {
  const [editorMode, setEditorMode] = useState(false);

  return (
    <div className="grid gap-4 p-6">
      <div className="flex items-center justify-between gap-4">
        <FactoryGraphEditorStatus
          editorMode={editorMode}
          hasChanges={false}
          isDefinitionLoading={false}
        />
        <FactoryGraphEditorModeToggle
          editorMode={editorMode}
          onClick={() => setEditorMode((current) => !current)}
        />
      </div>
      <FactoryGraphEditorToolbar
        activeTool={null}
        canInteract={false}
        canDiscard={false}
        canSave={false}
        hasPendingChanges={false}
        onDiscard={() => {}}
        onSelectTool={() => {}}
        onSave={() => {}}
        visible={editorMode}
      />
    </div>
  );
}

function EditorModeStory() {
  const [activeTool, setActiveTool] =
    useState<FactoryGraphEditorTool>("connect");
  const [addMenuOpen, setAddMenuOpen] = useState(false);

  return (
    <div className="grid gap-4 p-6">
      <FactoryGraphEditorStatus
        editorMode={true}
        hasChanges={true}
        isDefinitionLoading={false}
      />
      <div className="relative min-h-44 rounded-[1.5rem] border border-af-border bg-af-surface-subtle">
        <FactoryGraphEditorToolbar
          activeTool={activeTool}
          addMenuActions={[...ADD_MENU_ACTIONS]}
          canInteract={true}
          canDiscard={true}
          canSave={true}
          hasPendingChanges={true}
          onDiscard={() => {}}
          onAddAction={() => {}}
          onAddMenuOpenChange={setAddMenuOpen}
          onSave={() => {}}
          onSelectTool={setActiveTool}
          openAddMenu={addMenuOpen}
          visible={true}
        />
      </div>
    </div>
  );
}

function AddMenuOpenStory() {
  return (
    <div className="grid gap-4 p-6">
      <div className="relative min-h-52 rounded-[1.5rem] border border-af-border bg-af-surface-subtle">
        <FactoryGraphEditorToolbar
          activeTool="add"
          addMenuActions={[...ADD_MENU_ACTIONS]}
          canInteract={true}
          canDiscard={true}
          canSave={true}
          hasPendingChanges={true}
          onDiscard={() => {}}
          onAddAction={() => {}}
          onAddMenuOpenChange={() => {}}
          onSave={() => {}}
          onSelectTool={() => {}}
          openAddMenu={true}
          visible={true}
        />
      </div>
    </div>
  );
}

function DeleteConfirmationStory() {
  return (
    <div className="grid gap-4 p-6">
      <FactoryGraphEditorConfirmationDialog
        cancelLabel="Cancel removal"
        confirmLabel="Delete review workstation"
        confirmTone="destructive"
        description="Deleting review will remove 3 graph edges and clear its worker assignment."
        isOpen={true}
        onCancel={() => {}}
        onConfirm={() => {}}
        title="Remove review workstation?"
      />
    </div>
  );
}

function PendingDraftActionsStory() {
  return (
    <div className="grid gap-4 p-6">
      <div className="relative min-h-44 rounded-[1.5rem] border border-af-overlay/12 bg-af-overlay/4">
        <FactoryGraphEditorToolbar
          activeTool="connect"
          addMenuActions={[...ADD_MENU_ACTIONS]}
          canInteract={true}
          canDiscard={true}
          canSave={false}
          hasPendingChanges={true}
          onDiscard={() => {}}
          onAddAction={() => {}}
          onAddMenuOpenChange={() => {}}
          onSave={() => {}}
          onSelectTool={() => {}}
          openAddMenu={false}
          saveDisabledReason="Save is unavailable while active work is still running in the current factory."
          visible={true}
        />
      </div>
    </div>
  );
}

function SaveConfirmationStory() {
  return (
    <div className="grid gap-4 p-6">
      <FactoryGraphEditorConfirmationDialog
        cancelLabel="Keep editing"
        confirmLabel="Save topology"
        description="This save will apply 2 created entities, 1 deleted entity and 3 changed edges."
        isOpen={true}
        onCancel={() => {}}
        onConfirm={() => {}}
        title="Save factory graph changes?"
      />
    </div>
  );
}

function ActionPopoverStory() {
  const [open, setOpen] = useState(true);

  return (
    <div className="grid gap-4 p-6">
      <FactoryGraphEditorActionPopover
        description="Keyboard-reachable graph actions can use the same popover shell."
        onOpenChange={setOpen}
        open={open}
        title="Node actions"
        trigger={
          <Button aria-label="Open node actions" tone="outline" type="button">
            Open node actions
          </Button>
        }
      >
        <div className="grid gap-2">
          <Button size="sm" tone="outline" type="button">
            Rename node
          </Button>
          <Button size="sm" tone="outline" type="button">
            Duplicate node
          </Button>
        </div>
      </FactoryGraphEditorActionPopover>
    </div>
  );
}

function NoticeStory({
  children,
  title,
  tone,
}: {
  children: string;
  title: string;
  tone: "danger" | "warning";
}) {
  return (
    <div className="grid gap-4 p-6">
      <FactoryGraphEditorNotice title={title} tone={tone}>
        {children}
      </FactoryGraphEditorNotice>
    </div>
  );
}

export default {
  title: "Agent Factory/Dashboard/Factory Graph Editor Controls",
  tags: ["test"],
};

export const ObserveMode = {
  render: () => <ObserveModeStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const toggle = canvas.getByRole("button", {
      name: "Enter factory graph editor",
    });

    await expect(canvas.getByText("Observe mode")).toBeVisible();
    await userEvent.hover(toggle);
    await expect(
      await within(canvasElement.ownerDocument.body).findByRole("tooltip", {
        name: "Enter factory graph editor",
      }),
    ).toBeVisible();
  },
};

export const EditorMode = {
  render: () => <EditorModeStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const toolbar = canvas.getByRole("region", {
      name: "Factory graph editor tools",
    });
    const addMenuButton = within(toolbar).getByRole("button", {
      name: "Open add entity menu",
    });

    await expect(addMenuButton).toHaveAttribute("aria-expanded", "false");
    await expect(addMenuButton).toBeVisible();
    await expect(
      within(toolbar).getByRole("button", { name: "Connect" }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(
      within(toolbar).getByText("Draft changes pending"),
    ).toBeVisible();
  },
};

export const AddMenuOpen = {
  render: () => <AddMenuOpenStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const page = within(canvasElement.ownerDocument.body);

    await expect(page.getByLabelText("Add graph entity menu")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Workstation" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Worker" })).toBeVisible();
  },
};

export const DeleteConfirmation = {
  render: () => <DeleteConfirmationStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      {
        name: "Remove review workstation?",
      },
    );

    await expect(
      within(dialog).getByRole("button", { name: "Cancel removal" }),
    ).toBeVisible();
    await expect(
      within(dialog).getByRole("button", { name: "Delete review workstation" }),
    ).toBeVisible();
  },
};

export const PendingDraftActions = {
  render: () => <PendingDraftActionsStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const toolbar = canvas.getByRole("region", {
      name: "Factory graph editor tools",
    });

    await expect(
      within(toolbar).getByText("Draft changes pending"),
    ).toBeVisible();
    await expect(
      within(toolbar).getByRole("button", { name: "Discard changes" }),
    ).toBeVisible();
    await expect(
      within(toolbar).getByRole("button", { name: "Save changes" }),
    ).toBeVisible();
  },
};

export const SaveConfirmation = {
  render: () => <SaveConfirmationStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      {
        name: "Save factory graph changes?",
      },
    );

    await expect(
      within(dialog).getByText(
        "This save will apply 2 created entities, 1 deleted entity and 3 changed edges.",
      ),
    ).toBeVisible();
    await expect(
      within(dialog).getByRole("button", { name: "Save topology" }),
    ).toBeVisible();
  },
};

export const ActionPopover = {
  render: () => <ActionPopoverStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const page = within(canvasElement.ownerDocument.body);

    await expect(page.getByText("Node actions")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Rename node" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Duplicate node" }),
    ).toBeVisible();
  },
};

export const ActiveWorkBlocked = {
  render: () => (
    <NoticeStory title="Topology edits are blocked" tone="danger">
      Save is unavailable while 3 work items are still active in the running
      factory.
    </NoticeStory>
  ),
};

export const RemovalBlocked = {
  render: () => (
    <NoticeStory title="Removal blocked" tone="warning">
      This worker is still assigned to 1 workstation. Reassign or remove those
      workstations before deleting writer.
    </NoticeStory>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("Removal blocked")).toBeVisible();
    await expect(
      canvas.getByText(
        "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
      ),
    ).toBeVisible();
  },
};

export const StaleTimestampWarning = {
  render: () => (
    <NoticeStory title="A newer factory definition is available" tone="warning">
      Refresh the editor before saving so you do not overwrite a newer topology
      version.
    </NoticeStory>
  ),
};
