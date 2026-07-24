import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { FactoryGraphVisualGroupControls } from "./factory-graph-visual-group-controls";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: control scenarios share one fixture shape.
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
        boundsError={null}
        deleteGroupLabel="Delete group"
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
        onDeleteGroup={vi.fn()}
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

    expect(onToggleNodeMembership).toHaveBeenCalledWith("worker:writer", true);
  });

  it("invokes color selection when a group color option is activated", async () => {
    const user = userEvent.setup();
    const onSetGroupColor = vi.fn();

    render(
      <FactoryGraphVisualGroupControls
        canvasNodeOptions={[]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        boundsError={null}
        deleteGroupLabel="Delete group"
        emptyLabelError="Enter a group label."
        group={{
          bounds: { height: 120, width: 200, x: 0, y: 0 },
          color: "info",
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
        onDeleteGroup={vi.fn()}
        onRenameGroup={vi.fn()}
        onSetGroupColor={onSetGroupColor}
        onToggleNodeMembership={vi.fn()}
        selectedGroupLabel="Selected visual group"
        staleMemberNodeIds={[]}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Use success group color" }),
    );
    expect(onSetGroupColor).toHaveBeenCalledWith("success");
  });

  it("invokes delete when the delete group action is activated", async () => {
    const user = userEvent.setup();
    const onDeleteGroup = vi.fn();

    render(
      <FactoryGraphVisualGroupControls
        canvasNodeOptions={[]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        boundsError={null}
        deleteGroupLabel="Delete group"
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
        onDeleteGroup={onDeleteGroup}
        onRenameGroup={vi.fn()}
        onSetGroupColor={vi.fn()}
        onToggleNodeMembership={vi.fn()}
        selectedGroupLabel="Selected visual group"
        staleMemberNodeIds={[]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Delete group" }));
    expect(onDeleteGroup).toHaveBeenCalledTimes(1);
  });

  it("shows an empty membership state when no canvas nodes are available", () => {
    render(
      <FactoryGraphVisualGroupControls
        canvasNodeOptions={[]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        boundsError={null}
        deleteGroupLabel="Delete group"
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
        onDeleteGroup={vi.fn()}
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

  it("reports rename changes and empty label validation", async () => {
    const user = userEvent.setup();
    const onRenameGroup = vi.fn();

    const { rerender } = render(
      <FactoryGraphVisualGroupControls
        canvasNodeOptions={[]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        boundsError={null}
        deleteGroupLabel="Delete group"
        emptyLabelError="Enter a group label."
        group={{
          bounds: { height: 120, width: 200, x: 0, y: 0 },
          id: "group-1",
          label: "   ",
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
        onDeleteGroup={vi.fn()}
        onRenameGroup={onRenameGroup}
        onSetGroupColor={vi.fn()}
        onToggleNodeMembership={vi.fn()}
        selectedGroupLabel="Selected visual group"
        staleMemberNodeIds={[]}
      />,
    );

    expect(screen.getByText("Enter a group label.")).toBeInTheDocument();

    rerender(
      <FactoryGraphVisualGroupControls
        canvasNodeOptions={[]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        boundsError={null}
        deleteGroupLabel="Delete group"
        emptyLabelError="Enter a group label."
        group={{
          bounds: { height: 120, width: 200, x: 0, y: 0 },
          id: "group-1",
          label: "Planning",
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
        onDeleteGroup={vi.fn()}
        onRenameGroup={onRenameGroup}
        onSetGroupColor={vi.fn()}
        onToggleNodeMembership={vi.fn()}
        selectedGroupLabel="Selected visual group"
        staleMemberNodeIds={[]}
      />,
    );

    const labelField = screen.getByRole("textbox", { name: "Group label" });
    fireEvent.change(labelField, { target: { value: "Planning lane" } });
    expect(onRenameGroup).toHaveBeenLastCalledWith("Planning lane");
    await user.click(labelField);
  });

  it("unchecks a member and falls back to the primary color token", async () => {
    const user = userEvent.setup();
    const onToggleNodeMembership = vi.fn();

    render(
      <FactoryGraphVisualGroupControls
        canvasNodeOptions={[{ id: "workstation:draft", label: "Draft" }]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        boundsError={null}
        deleteGroupLabel="Delete group"
        emptyLabelError="Enter a group label."
        group={{
          bounds: { height: 120, width: 200, x: 0, y: 0 },
          color: "not-a-token",
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
        onDeleteGroup={vi.fn()}
        onRenameGroup={vi.fn()}
        onSetGroupColor={vi.fn()}
        onToggleNodeMembership={onToggleNodeMembership}
        selectedGroupLabel="Selected visual group"
        staleMemberNodeIds={[]}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Use primary group color" }),
    ).toHaveAttribute("aria-pressed", "true");

    await user.click(
      screen.getByRole("checkbox", {
        name: "Include Draft in this group",
      }),
    );
    expect(onToggleNodeMembership).toHaveBeenCalledWith(
      "workstation:draft",
      false,
    );
  });

  it("shows invalid bounds feedback without blocking other controls", () => {
    render(
      <FactoryGraphVisualGroupControls
        boundsError="Group bounds contain non-finite geometry. Resize the group to correct them before saving."
        canvasNodeOptions={[]}
        colorLabel="Group color"
        colorOptionLabel={(token) => `Use ${token} group color`}
        deleteGroupLabel="Delete group"
        emptyLabelError="Enter a group label."
        group={{
          bounds: {
            height: Number.NaN,
            width: 200,
            x: 0,
            y: 0,
          },
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
        onDeleteGroup={vi.fn()}
        onRenameGroup={vi.fn()}
        onSetGroupColor={vi.fn()}
        onToggleNodeMembership={vi.fn()}
        selectedGroupLabel="Selected visual group"
        staleMemberNodeIds={[]}
      />,
    );

    expect(
      screen.getByText(
        "Group bounds contain non-finite geometry. Resize the group to correct them before saving.",
      ),
    ).toBeInTheDocument();
  });
});
