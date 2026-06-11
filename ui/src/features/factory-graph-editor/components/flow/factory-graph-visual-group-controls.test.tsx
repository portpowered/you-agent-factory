import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { FactoryGraphVisualGroupControls } from "./factory-graph-visual-group-controls";

describe("FactoryGraphVisualGroupControls", () => {
  it("renders membership checkboxes and reports toggle changes", async () => {
    const user = userEvent.setup();
    const onToggleNodeMembership = vi.fn();

    render(
      <FactoryGraphVisualGroupControls
        canvasNodeOptions={[
          { id: "workstation:draft", label: "Draft" },
          { id: "worker:writer", label: "Writer" },
        ]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        emptyLabelError="Enter a group label."
        group={{
          bounds: { height: 120, width: 200, x: 0, y: 0 },
          id: "group-1",
          label: "Review",
          nodeIds: ["workstation:draft"],
        }}
        isNodeMember={(nodeId) => nodeId === "workstation:draft"}
        labelFieldLabel="Group label"
        membershipEmptyLabel="No canvas nodes are available to assign."
        membershipLabel="Group members"
        membershipNodeLabel={(label) => `Include ${label} in this group`}
        membershipStaleNodeLabel={(nodeId) =>
          `Saved member ${nodeId} is no longer on the canvas.`
        }
        onRenameGroup={vi.fn()}
        onSetGroupColor={vi.fn()}
        onToggleNodeMembership={onToggleNodeMembership}
        selectedGroupLabel="Selected visual group"
        staleMemberNodeIds={["workstation:missing"]}
      />,
    );

    expect(
      screen.getByRole("checkbox", {
        name: "Include Draft in this group",
      }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", {
        name: "Include Writer in this group",
      }),
    ).not.toBeChecked();
    expect(
      screen.getByText(
        "Saved member workstation:missing is no longer on the canvas.",
      ),
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole("checkbox", {
        name: "Include Writer in this group",
      }),
    );

    expect(onToggleNodeMembership).toHaveBeenCalledWith(
      "worker:writer",
      true,
    );
  });

  it("shows an empty membership state when no canvas nodes are available", () => {
    render(
      <FactoryGraphVisualGroupControls
        canvasNodeOptions={[]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        emptyLabelError="Enter a group label."
        group={{
          bounds: { height: 120, width: 200, x: 0, y: 0 },
          id: "group-1",
          label: "Review",
          nodeIds: [],
        }}
        isNodeMember={() => false}
        labelFieldLabel="Group label"
        membershipEmptyLabel="No canvas nodes are available to assign."
        membershipLabel="Group members"
        membershipNodeLabel={(label) => `Include ${label} in this group`}
        membershipStaleNodeLabel={(nodeId) =>
          `Saved member ${nodeId} is no longer on the canvas.`
        }
        onRenameGroup={vi.fn()}
        onSetGroupColor={vi.fn()}
        onToggleNodeMembership={vi.fn()}
        selectedGroupLabel="Selected visual group"
        staleMemberNodeIds={[]}
      />,
    );

    expect(
      screen.getByText("No canvas nodes are available to assign."),
    ).toBeInTheDocument();
  });
});
