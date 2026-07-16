// biome-ignore lint/style/noExcessiveLinesPerFile: toolbar controls share stateful harnesses and interaction coverage in one focused suite.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";

import type { FactoryGraphNodeKind } from "../../lib/draft/factory-graph-draft-types";
import type { FactoryGraphEditorToolbarSelectionState } from "../../lib/selection/factory-graph-editor-toolbar-selection";
import {
  FactoryGraphEditorActionPopover,
  FactoryGraphEditorConfirmationDialog,
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
  FactoryGraphEditorToolbar,
  FactoryGraphEditorVisibilityPanel,
} from "../controls/factory-graph-editor-controls";

function renderToolbar({
  editMode = false,
  hasPendingChanges = true,
  hiddenNodeClasses = new Set<FactoryGraphNodeKind>(),
  hideShowVisible = true,
  onCreateVisualGroup = vi.fn(),
  onToggleEditMode = vi.fn(),
  onToggleHiddenNodeClass = vi.fn(),
  visible = true,
}: {
  editMode?: boolean;
  hasPendingChanges?: boolean;
  hiddenNodeClasses?: ReadonlySet<FactoryGraphNodeKind>;
  hideShowVisible?: boolean;
  onCreateVisualGroup?: () => void;
  onToggleEditMode?: () => void;
  onToggleHiddenNodeClass?: (kind: FactoryGraphNodeKind) => void;
  visible?: boolean;
} = {}) {
  function ToolbarHarness() {
    const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
    const [menuOpen, setMenuOpen] = useState(false);
    const [hideShowMenuOpen, setHideShowMenuOpen] = useState(false);

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
          canSave={hasPendingChanges}
          canDiscard={hasPendingChanges}
          editModeToggle={{
            editorMode: editMode,
            hasChanges: hasPendingChanges,
            onToggle: onToggleEditMode,
          }}
          hiddenNodeClasses={hiddenNodeClasses}
          hideShowMenuOpen={hideShowMenuOpen}
          hideShowVisible={hideShowVisible}
          onCreateVisualGroup={onCreateVisualGroup}
          onDiscard={() => {}}
          onAddAction={() => {}}
          onAddMenuOpenChange={setMenuOpen}
          onHideShowMenuOpenChange={setHideShowMenuOpen}
          onSave={() => {}}
          onSelectTool={setActiveTool}
          onToggleHiddenNodeClass={onToggleHiddenNodeClass}
          openAddMenu={menuOpen}
          visible={visible}
        />
      </div>
    );
  }

  render(<ToolbarHarness />);
}

function renderSelectionToolbar({
  canDeleteSelection = false,
  graphSelectionToolbarState = {
    mode: "none",
    selectedItemCount: 0,
  } satisfies FactoryGraphEditorToolbarSelectionState,
  onDeleteSelection = vi.fn(),
  onSelectTool = vi.fn(),
}: {
  canDeleteSelection?: boolean;
  graphSelectionToolbarState?: FactoryGraphEditorToolbarSelectionState;
  onDeleteSelection?: () => void;
  onSelectTool?: (tool: FactoryGraphEditorTool) => void;
} = {}) {
  render(
    <div className="relative min-h-48">
      <FactoryGraphEditorToolbar
        activeTool={null}
        canDeleteSelection={canDeleteSelection}
        canDiscard={false}
        canInteract={true}
        canSave={false}
        graphSelectionToolbarState={graphSelectionToolbarState}
        onDeleteSelection={onDeleteSelection}
        onDiscard={() => {}}
        onSelectTool={onSelectTool}
        onSave={() => {}}
        visible={true}
      />
    </div>,
  );
}

describe("factory graph editor toolbar controls", () => {
  it("opens the add menu from the keyboard and exposes action copy", async () => {
    const user = userEvent.setup();

    renderToolbar();

    const addMenuButton = screen.getByRole("button", {
      name: "Add",
    });
    expect(addMenuButton.textContent).toBe("");
    expect(addMenuButton.getAttribute("aria-expanded")).toBe("false");

    addMenuButton.focus();
    await user.keyboard("{Enter}");

    const menu = await screen.findByLabelText("Add graph entity menu");
    expect(menu).toBeTruthy();
    expect(menu.getAttribute("data-side")).toBe("top");
    expect(menu.className).toContain(
      "max-h-[min(var(--radix-popover-content-available-height),calc(100vh-8rem))]",
    );
    expect(addMenuButton.getAttribute("aria-expanded")).toBe("true");
    expect(
      within(menu).getByRole("button", { name: "Workstation" }),
    ).toBeTruthy();
    expect(
      within(menu).getByRole("button", { name: "Workstation" }).className,
    ).toContain("rounded-lg");
    expect(
      within(menu).queryByRole("button", {
        name: "Add",
      }),
    ).toBeNull();
  });

  it("uses icon toggles with accessible names and pressed state", () => {
    renderToolbar();

    const showButton = screen.getByRole("button", { name: "Show or hide" });
    const modeButton = screen.getByRole("button", { name: "Edit mode" });
    const deleteButton = screen.getByRole("button", {
      name: "Delete, no graph items selected",
    });
    const discardButton = screen.getByRole("button", {
      name: "Discard changes",
    });
    const saveButton = screen.getByRole("button", { name: "Save changes" });
    const addButton = screen.getByRole("button", {
      name: "Add",
    });

    expect(addButton.textContent).toBe("");
    expect(showButton.textContent).toBe("");
    expect(modeButton.textContent).toBe("");
    expect(deleteButton.textContent).toBe("");
    expect(discardButton.textContent).toBe("");
    expect(saveButton.textContent).toBe("");
    expect(showButton.getAttribute("aria-pressed")).toBe("false");
    expect(modeButton.getAttribute("aria-pressed")).toBe("false");
    expect(saveButton).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Discard changes" }),
    ).toBeTruthy();
    expect(addButton.className).toContain("h-10");
    expect(showButton.className).toContain("h-10");
    expect(modeButton.className).toContain("h-10");
    expect(deleteButton.className).toContain("h-10");
    expect(deleteButton.getAttribute("disabled")).not.toBeNull();
  });

  it("shows the add menu tooltip on hover and keyboard focus", async () => {
    const user = userEvent.setup();

    renderToolbar();

    const addButton = screen.getByRole("button", { name: "Add" });

    await user.hover(addButton);
    expect(
      await screen.findByRole("tooltip", {
        name: "Add",
      }),
    ).toBeTruthy();

    await user.unhover(addButton);
    expect(screen.queryByRole("tooltip")).toBeNull();

    addButton.focus();
    expect(
      await screen.findByRole("tooltip", {
        name: "Add",
      }),
    ).toBeTruthy();
  });

  it("renders the confirmation dialog portaled to document.body", async () => {
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
    expect(document.body.contains(dialog)).toBe(true);
    expect(
      within(dialog).getByRole("button", { name: "Cancel removal" }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: "Delete review workstation" }),
    ).toBeTruthy();
  });
});

describe("factory graph editor mode toggle controls", () => {
  it("applies warning styling when there are unsaved changes", () => {
    render(
      <FactoryGraphEditorModeToggle
        editorMode={true}
        hasChanges={true}
        onClick={() => {}}
      />,
    );

    const toggle = screen.getByRole("button", {
      name: "Leave editor",
    });

    expect(toggle.className).toContain("border-af-warning-border");
    expect(toggle.className).toContain("bg-warning-container");
    expect(toggle.className).toContain("text-on-warning-container");
  });

  it("keeps default enter and leave tones when there are no unsaved changes", () => {
    const { rerender } = render(
      <FactoryGraphEditorModeToggle editorMode={false} onClick={() => {}} />,
    );

    const enterToggle = screen.getByRole("button", {
      name: "Edit mode",
    });
    expect(enterToggle.className).toContain("border-outline");
    expect(enterToggle.className).not.toContain("border-af-warning-border");

    rerender(
      <FactoryGraphEditorModeToggle editorMode={true} onClick={() => {}} />,
    );

    const leaveToggle = screen.getByRole("button", {
      name: "Leave editor",
    });
    expect(leaveToggle.className).toContain("border-outline-variant");
    expect(leaveToggle.className).not.toContain("border-af-warning-border");
  });

  it("shows the mode-toggle tooltip on hover", async () => {
    const user = userEvent.setup();

    render(
      <FactoryGraphEditorModeToggle editorMode={false} onClick={() => {}} />,
    );

    await user.hover(screen.getByRole("button", { name: "Edit mode" }));
    const tooltip = await screen.findByRole("tooltip", {
      name: "Edit mode",
    });
    expect(tooltip).toBeTruthy();
    expect(tooltip.className).toContain("border-outline-variant");
    expect(tooltip.className).toContain("bg-surface-container-high");
    expect(tooltip.className).toContain("text-on-surface");
  });

  it("keeps the mode-toggle tooltip below the trigger button", async () => {
    const user = userEvent.setup();

    render(
      <FactoryGraphEditorModeToggle editorMode={false} onClick={() => {}} />,
    );

    await user.hover(screen.getByRole("button", { name: "Edit mode" }));
    const tooltip = await screen.findByRole("tooltip", {
      name: "Edit mode",
    });
    expect(tooltip.className).toContain("top-full");
    expect(tooltip.className).toContain("mt-2");
    expect(tooltip.className).not.toContain("bottom-full");
  });
});

describe("factory graph editor toolbar tooltip placement", () => {
  it("renders toolbar tooltips above the trigger buttons", async () => {
    const user = userEvent.setup();

    renderToolbar();

    const toolbarTooltips = [
      {
        buttonName: "Show or hide",
        tooltipName: "Show",
      },
      {
        buttonName: "Save changes",
        tooltipName: "Save changes",
      },
      {
        buttonName: "Discard changes",
        tooltipName: "Discard changes",
      },
    ] as const;

    for (const { buttonName, tooltipName } of toolbarTooltips) {
      await user.hover(screen.getByRole("button", { name: buttonName }));

      const tooltip = await screen.findByRole("tooltip", { name: tooltipName });
      expect(tooltip.className).toContain("bottom-full");
      expect(tooltip.className).toContain("mb-2");
      expect(tooltip.className).not.toContain("top-full");

      await user.unhover(screen.getByRole("button", { name: buttonName }));
      expect(screen.queryByRole("tooltip")).toBeNull();
    }
  });

  it("renders the batch-delete tooltip above the trigger when selection is deletable", async () => {
    const user = userEvent.setup();

    renderSelectionToolbar({
      canDeleteSelection: true,
      graphSelectionToolbarState: {
        mode: "single",
        selectedItemCount: 1,
      },
    });

    await user.hover(
      screen.getByRole("button", { name: "Delete selected graph item" }),
    );
    const deleteTooltip = await screen.findByRole("tooltip", {
      name: "Delete selected graph item",
    });
    expect(deleteTooltip.className).toContain("bottom-full");
    expect(deleteTooltip.className).toContain("mb-2");
    expect(deleteTooltip.className).not.toContain("top-full");
  });
});

describe("factory graph editor toolbar action-row composition", () => {
  it("renders show before edit in the toolbar frame", () => {
    renderToolbar();

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });
    const buttonNames = within(toolbar)
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label"));

    expect(buttonNames[0]).toBe("Show or hide");
    expect(buttonNames[buttonNames.length - 1]).toBe("Edit mode");
  });

  it("renders discard and save actions when pending changes exist", () => {
    renderToolbar();

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });
    const sections = toolbar.querySelectorAll(
      "[data-action-row-section]",
    );

    expect(sections).toHaveLength(1);
    expect(sections[0]?.getAttribute("data-action-row-section")).toBe(
      "actions",
    );
    expect(within(toolbar).queryByRole("status")).toBeNull();
    expect(
      within(sections[0] as HTMLElement).getByRole("button", {
        name: "Discard changes",
      }),
    ).toBeTruthy();
    expect(
      within(sections[0] as HTMLElement).getByRole("button", {
        name: "Save changes",
      }),
    ).toBeTruthy();
  });

  it("keeps disabled draft actions mounted when no pending changes exist", () => {
    renderToolbar({ hasPendingChanges: false });

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });
    const sections = toolbar.querySelectorAll(
      "[data-action-row-section]",
    );
    const discardButton = within(toolbar).getByRole("button", {
      name: "Discard changes",
    });
    const saveButton = within(toolbar).getByRole("button", {
      name: "Save changes",
    });

    expect(sections).toHaveLength(1);
    expect(sections[0]?.getAttribute("data-action-row-section")).toBe(
      "actions",
    );
    expect(discardButton.getAttribute("disabled")).not.toBeNull();
    expect(saveButton.getAttribute("disabled")).not.toBeNull();
    expect(within(toolbar).queryByRole("status")).toBeNull();
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

  it("treats preferences-only state as active instead of unsaved", () => {
    render(
      <FactoryGraphEditorStatus
        dirtyState={{
          layoutDirty: false,
          preferencesDirty: true,
          topologyDirty: false,
        }}
        editorMode
        hasChanges={false}
        isDefinitionLoading={false}
      />,
    );

    expect(screen.getByText("Editor mode active")).toBeTruthy();
    expect(screen.queryByText("Private view preferences changed")).toBeNull();
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

describe("factory graph editor hide/show controls", () => {
  it("opens the hide/show menu from the keyboard and toggles node class visibility", async () => {
    const user = userEvent.setup();
    const onToggleHiddenNodeClass = vi.fn();

    renderToolbar({ onToggleHiddenNodeClass });

    const hideShowButton = screen.getByRole("button", {
      name: "Show or hide",
    });
    expect(hideShowButton.getAttribute("aria-pressed")).toBe("false");
    expect(hideShowButton.getAttribute("aria-expanded")).toBe("false");

    hideShowButton.focus();
    await user.keyboard("{Enter}");

    const menu = await screen.findByLabelText(
      "Factory graph node class visibility menu",
    );
    expect(menu).toBeTruthy();
    expect(hideShowButton.getAttribute("aria-expanded")).toBe("true");
    expect(hideShowButton.getAttribute("aria-pressed")).toBe("true");

    const workStateToggle = within(menu).getByRole("menuitemcheckbox", {
      name: "Work state",
    });
    expect(workStateToggle.getAttribute("aria-checked")).toBe("true");

    await user.click(workStateToggle);
    expect(onToggleHiddenNodeClass).toHaveBeenCalledWith("work-state");
  });

  it("marks the hide/show button pressed when any node class is hidden", () => {
    renderToolbar({
      hiddenNodeClasses: new Set<FactoryGraphNodeKind>(["worker"]),
    });

    expect(
      screen
        .getByRole("button", { name: "Show or hide" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("renders hide/show in observer mode without editor tools", () => {
    renderToolbar({ hideShowVisible: true, visible: false });

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });

    expect(
      screen.getByRole("button", {
        name: "Show or hide",
      }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit mode" })).toBeTruthy();
    expect(
      toolbar.querySelector("[data-toolbar-editor-controls='collapsed']"),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Connect" })).toBeNull();
  });
});

describe("factory graph editor toolbar selection states", () => {
  it("marks the toolbar row as having no graph selection by default", () => {
    renderSelectionToolbar();

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });

    expect(
      toolbar.querySelector("[data-toolbar-graph-selection='none']"),
    ).toBeTruthy();
    const deleteButton = screen.getByRole("button", {
      name: "Delete, no graph items selected",
    });
    expect(deleteButton.getAttribute("disabled")).not.toBeNull();
    expect(deleteButton.getAttribute("aria-pressed")).toBe("false");
  });

  it("enables batch delete for a single deletable selection", async () => {
    const user = userEvent.setup();
    const onDeleteSelection = vi.fn();

    renderSelectionToolbar({
      canDeleteSelection: true,
      graphSelectionToolbarState: {
        mode: "single",
        selectedItemCount: 1,
      },
      onDeleteSelection,
    });

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });
    const deleteButton = screen.getByRole("button", {
      name: "Delete selected graph item",
    });

    expect(
      toolbar.querySelector("[data-toolbar-graph-selection='single']"),
    ).toBeTruthy();
    expect(deleteButton.getAttribute("disabled")).toBeNull();

    await user.click(deleteButton);
    expect(onDeleteSelection).toHaveBeenCalledTimes(1);
  });

  it("enables batch delete for multi-selection with count-specific copy", async () => {
    const user = userEvent.setup();
    const onDeleteSelection = vi.fn();

    renderSelectionToolbar({
      canDeleteSelection: true,
      graphSelectionToolbarState: {
        mode: "multi",
        selectedItemCount: 3,
      },
      onDeleteSelection,
    });

    const deleteButton = screen.getByRole("button", {
      name: "Delete 3 selected graph items",
    });

    expect(
      screen
        .getByRole("region", { name: "Factory graph editor tools" })
        .querySelector("[data-toolbar-graph-selection='multi']"),
    ).toBeTruthy();

    await user.hover(deleteButton);
    expect(
      await screen.findByRole("tooltip", {
        name: "Delete 3 selected graph items",
      }),
    ).toBeTruthy();

    await user.click(deleteButton);
    expect(onDeleteSelection).toHaveBeenCalledTimes(1);
  });

  it("disables delete with explicit semantics for non-deletable selections", async () => {
    const user = userEvent.setup();
    const onSelectTool = vi.fn();

    renderSelectionToolbar({
      graphSelectionToolbarState: {
        mode: "single",
        selectedItemCount: 1,
      },
      onSelectTool,
    });

    const deleteButton = screen.getByRole("button", {
      name: "Delete, selected items cannot be removed",
    });

    expect(deleteButton.getAttribute("disabled")).not.toBeNull();
    expect(deleteButton.getAttribute("aria-pressed")).toBe("false");

    await user.click(deleteButton);
    expect(onSelectTool).not.toHaveBeenCalled();
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
