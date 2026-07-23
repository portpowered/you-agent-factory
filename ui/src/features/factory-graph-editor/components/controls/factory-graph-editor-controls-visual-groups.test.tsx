import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";

import type { FactoryGraphNodeKind } from "../../lib/draft/factory-graph-draft-types";
import {
  type FactoryGraphEditorTool,
  FactoryGraphEditorToolbar,
} from "../controls/factory-graph-editor-controls";

function renderToolbar({
  editMode = false,
  onCreateVisualGroup = vi.fn(),
}: {
  editMode?: boolean;
  onCreateVisualGroup?: () => void;
} = {}) {
  function ToolbarHarness() {
    const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
    const [menuOpen, setMenuOpen] = useState(false);
    const [hideShowMenuOpen, setHideShowMenuOpen] = useState(false);

    return (
      <div className="relative min-h-48">
        <FactoryGraphEditorToolbar
          activeTool={activeTool}
          addMenuActions={[]}
          canInteract={true}
          canSave={true}
          canDiscard={true}
          editModeToggle={{
            editorMode: editMode,
            hasChanges: true,
            onToggle: () => {},
          }}
          hiddenNodeClasses={new Set<FactoryGraphNodeKind>()}
          hideShowMenuOpen={hideShowMenuOpen}
          hideShowVisible={true}
          onCreateVisualGroup={onCreateVisualGroup}
          onDiscard={() => {}}
          onAddAction={() => {}}
          onAddMenuOpenChange={setMenuOpen}
          onHideShowMenuOpenChange={setHideShowMenuOpen}
          onSave={() => {}}
          onSelectTool={setActiveTool}
          onToggleHiddenNodeClass={() => {}}
          openAddMenu={menuOpen}
          visible={true}
        />
      </div>
    );
  }

  render(<ToolbarHarness />);
}

describe("factory graph editor toolbar visual groups", () => {
  it("invokes create visual group from the toolbar when edit mode is active", async () => {
    const user = userEvent.setup();
    const onCreateVisualGroup = vi.fn();

    renderToolbar({ editMode: true, onCreateVisualGroup });

    await user.click(screen.getByRole("button", { name: "Create group" }));

    expect(onCreateVisualGroup).toHaveBeenCalledTimes(1);
  });
});
