import { render, screen } from "@testing-library/react";
import { useState } from "react";

import {
  type FactoryGraphEditorTool,
  FactoryGraphEditorToolbar,
} from "../controls/factory-graph-editor-controls";

function ToolbarSizingProbe({
  editMode,
  visible,
}: {
  editMode: boolean;
  visible: boolean;
}) {
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);

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
        canDiscard={true}
        canSave={true}
        editModeToggle={{
          editorMode: editMode,
          hasChanges: false,
          onToggle: () => {},
        }}
        hasPendingChanges={false}
        onDiscard={() => {}}
        onAddAction={() => {}}
        onSave={() => {}}
        onSelectTool={setActiveTool}
        onToggleHiddenNodeClass={() => {}}
        visible={visible}
      />
    </div>
  );
}

describe("factory graph editor toolbar sizing", () => {
  it("expands editor controls horizontally without changing toolbar height", () => {
    const { rerender } = render(
      <ToolbarSizingProbe editMode={false} visible={false} />,
    );

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });
    let controlsLane = toolbar.querySelector(
      "[data-toolbar-editor-controls-lane]",
    ) as HTMLElement;
    let controlsRow = toolbar.querySelector(
      "[data-toolbar-editor-controls-row]",
    ) as HTMLElement;

    expect(toolbar.className).not.toContain("h-14");
    expect(toolbar.className).not.toContain("min-h-14");
    expect(toolbar.className).not.toContain("max-h-14");
    expect(toolbar.className).toContain("flex-nowrap");
    expect(toolbar.className).toContain("overflow-hidden");
    expect(controlsLane.getAttribute("data-toolbar-editor-controls")).toBe(
      "collapsed",
    );
    expect(controlsLane.className).toContain("max-w-0");
    expect(controlsLane.className).toContain("min-h-10");
    expect(controlsLane.className).toContain("max-h-11");
    expect(controlsLane.className).not.toContain("max-h-10");
    expect(controlsRow.className).toContain("min-h-10");
    expect(controlsRow.className).toContain("max-h-11");
    expect(controlsRow.className).not.toContain("max-h-10");
    expect(controlsRow.className).toContain("flex-nowrap");

    rerender(<ToolbarSizingProbe editMode={true} visible={true} />);

    controlsLane = toolbar.querySelector(
      "[data-toolbar-editor-controls-lane]",
    ) as HTMLElement;
    controlsRow = toolbar.querySelector(
      "[data-toolbar-editor-controls-row]",
    ) as HTMLElement;

    expect(controlsLane.getAttribute("data-toolbar-editor-controls")).toBe(
      "expanded",
    );
    expect(controlsLane.className).toContain("max-w-xl");
    expect(controlsLane.className).toContain("min-h-10");
    expect(controlsLane.className).toContain("max-h-11");
    expect(controlsLane.className).not.toContain("max-h-10");
    expect(controlsRow.className).toContain("min-h-10");
    expect(controlsRow.className).toContain("max-h-11");
    expect(controlsRow.className).not.toContain("max-h-10");
    expect(controlsRow.className).toContain("gap-2");
  });
});
