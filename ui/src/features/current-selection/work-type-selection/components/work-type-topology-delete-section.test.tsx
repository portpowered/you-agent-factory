import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useFactoryGraphTopologyEditorBridge } from "../../../workflow-activity/state/factory-graph-topology-editor-bridge";
import { getWorkTypeDetailMessages } from "../messages/work-type-detail";
import { WorkTypeTopologyDeleteSection } from "./work-type-topology-delete-section";

describe("WorkTypeTopologyDeleteSection", () => {
  const messages = getWorkTypeDetailMessages("en");
  const requestNodeRemoval = vi.fn();

  beforeEach(() => {
    requestNodeRemoval.mockReset();
    useFactoryGraphTopologyEditorBridge.setState({
      handlers: {
        blockedRemovalReason: null,
        canInteractWithEditor: true,
        editorMode: true,
        requestNodeRemoval,
      },
    });
  });

  it("renders a destructive delete action in factory-graph editor mode", async () => {
    const user = userEvent.setup();

    render(
      <WorkTypeTopologyDeleteSection messages={messages} workTypeName="story" />,
    );

    const deleteButton = screen.getByRole("button", {
      name: "Delete story work type",
    });
    expect(deleteButton).toBeTruthy();

    await user.click(deleteButton);

    expect(requestNodeRemoval).toHaveBeenCalledWith("work-type:story");
  });

  it("does not render outside factory-graph editor mode", () => {
    useFactoryGraphTopologyEditorBridge.setState({ handlers: null });

    render(
      <WorkTypeTopologyDeleteSection messages={messages} workTypeName="story" />,
    );

    expect(
      screen.queryByRole("button", { name: "Delete story work type" }),
    ).toBeNull();
  });

  it("surfaces blocked removal reasons from the graph editor bridge", () => {
    useFactoryGraphTopologyEditorBridge.setState({
      handlers: {
        blockedRemovalReason:
          "This work type still has active work items in the factory.",
        canInteractWithEditor: true,
        editorMode: true,
        requestNodeRemoval,
      },
    });

    render(
      <WorkTypeTopologyDeleteSection messages={messages} workTypeName="essay" />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Work type cannot be deleted:",
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "still has active work items",
    );
  });
});
