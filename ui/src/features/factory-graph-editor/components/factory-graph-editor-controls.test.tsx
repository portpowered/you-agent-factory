import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";

import {
  FactoryGraphEditorActionPopover,
  FactoryGraphEditorConfirmationDialog,
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
  FactoryGraphEditorToolbar,
  FactoryGraphEditorVisibilityPanel,
} from "./factory-graph-editor-controls";

function renderToolbar() {
  function ToolbarHarness() {
    const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
    const [menuOpen, setMenuOpen] = useState(false);

    return (
      <div className="relative min-h-48">
        <FactoryGraphEditorToolbar
          activeTool={activeTool}
          addMenuActions={[
            {
              description: "Create a workstation node.",
              id: "workstation",
              label: "Workstation",
            },
          ]}
          canInteract={true}
          canSave={true}
          canDiscard={true}
          hasPendingChanges={true}
          onDiscard={() => {}}
          onAddAction={() => {}}
          onAddMenuOpenChange={setMenuOpen}
          onSave={() => {}}
          onSelectTool={setActiveTool}
          openAddMenu={menuOpen}
          visible={true}
        />
      </div>
    );
  }

  render(<ToolbarHarness />);
}

describe("factory graph editor toolbar controls", () => {
  it("opens the add menu from the keyboard and exposes action copy", async () => {
    const user = userEvent.setup();

    renderToolbar();

    await user.tab();
    await user.keyboard("{Enter}");

    const menu = await screen.findByLabelText("Add graph entity menu");
    expect(menu).toBeTruthy();
    expect(
      within(menu).getByRole("button", { name: "Workstation" }),
    ).toBeTruthy();
    expect(screen.getByText("Draft changes pending")).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Add",
      }),
    ).toBeNull();
  });

  it("uses icon toggles with accessible names and pressed state", async () => {
    const user = userEvent.setup();

    renderToolbar();

    const connectButton = screen.getByRole("button", { name: "Connect" });
    const deleteButton = screen.getByRole("button", { name: "Delete" });

    expect(connectButton.textContent).toBe("");
    expect(deleteButton.textContent).toBe("");
    expect(connectButton.getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByRole("button", { name: "Save changes" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Discard changes" }),
    ).toBeTruthy();

    await user.click(connectButton);
    expect(connectButton.getAttribute("aria-pressed")).toBe("true");

    await user.tab();
    await user.tab();
    await user.hover(deleteButton);
    expect(
      await screen.findByRole("tooltip", {
        name: "Remove nodes or edges from the draft",
      }),
    ).toBeTruthy();
  });

  it("renders the confirmation dialog through the shared dialog pattern", async () => {
    render(
      <FactoryGraphEditorConfirmationDialog
        cancelLabel="Cancel removal"
        confirmLabel="Delete review workstation"
        confirmTone="destructive"
        description="Deleting review will remove 3 graph edges and clear its worker assignment."
        isOpen={true}
        onCancel={() => {}}
        onConfirm={() => {}}
        title="Remove review workstation?"
      />,
    );

    const dialog = screen.getByRole("dialog", {
      name: "Remove review workstation?",
    });
    expect(
      within(dialog).getByRole("button", { name: "Cancel removal" }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: "Delete review workstation" }),
    ).toBeTruthy();
  });

  it("shows the mode-toggle tooltip on hover", async () => {
    const user = userEvent.setup();

    render(
      <FactoryGraphEditorModeToggle editorMode={false} onClick={() => {}} />,
    );

    await user.hover(
      screen.getByRole("button", { name: "Enter factory graph editor" }),
    );
    expect(
      await screen.findByRole("tooltip", {
        name: "Enter factory graph editor",
      }),
    ).toBeTruthy();
  });
});

describe("factory graph editor status and popover controls", () => {
  it("renders the unavailable status reason and disables entering edit mode", () => {
    render(
      <>
        <FactoryGraphEditorStatus
          editorMode={false}
          editorUnavailableReason="Classifier workstation routes are read-only in this editor."
          hasChanges={false}
          isDefinitionLoading={false}
        />
        <FactoryGraphEditorModeToggle
          disabled
          editorMode={false}
          onClick={() => {}}
          tooltipOverride="Classifier workstation routes are read-only in this editor."
        />
      </>,
    );

    expect(
      screen.getByText(
        "Editor unavailable: Classifier workstation routes are read-only in this editor.",
      ),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", {
          name: "Classifier workstation routes are read-only in this editor.",
        })
        .getAttribute("disabled"),
    ).not.toBeNull();
  });

  it("keeps action popovers keyboard reachable without right-click", async () => {
    const user = userEvent.setup();

    function PopoverHarness() {
      const [open, setOpen] = useState(false);

      return (
        <FactoryGraphEditorActionPopover
          description="Reusable graph action shell"
          onOpenChange={setOpen}
          open={open}
          title="Node actions"
          trigger={<button type="button">Open actions</button>}
        >
          <button type="button">Rename node</button>
        </FactoryGraphEditorActionPopover>
      );
    }

    render(<PopoverHarness />);

    await user.tab();
    await user.keyboard("{Enter}");

    const menu = await screen.findByText("Node actions");
    expect(menu).toBeTruthy();
    expect(screen.getByRole("button", { name: "Rename node" })).toBeTruthy();
  });
});

describe("factory graph editor visibility controls", () => {
  it("exposes keyboard-reachable graph visibility presets with pressed state", async () => {
    const user = userEvent.setup();
    const onSelectPreset = vi.fn();

    render(
      <FactoryGraphEditorVisibilityPanel
        onSelectPreset={onSelectPreset}
        options={[
          { key: "all", label: "All", selected: false },
          { key: "workflow", label: "Workflow", selected: true },
          { key: "execution", label: "Execution", selected: false },
          {
            key: "infrastructure",
            label: "Infrastructure",
            selected: false,
          },
        ]}
        visible={true}
      />,
    );

    await user.tab();
    await user.keyboard("{Enter}");

    expect(onSelectPreset).toHaveBeenCalledWith("all");
    expect(
      screen
        .getByRole("button", { name: "Workflow" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      screen.getByRole("button", { name: "All" }).getAttribute("aria-pressed"),
    ).toBe("false");
  });
});
