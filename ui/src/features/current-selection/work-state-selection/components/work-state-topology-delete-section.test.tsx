import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useFactoryGraphTopologyEditorBridge } from "../../../workflow-activity/state/factory-graph-topology-editor-bridge";
import { getWorkStateDetailMessages } from "../messages/work-state-detail";
import { WorkStateTopologyDeleteSection } from "./work-state-topology-delete-section";

describe("WorkStateTopologyDeleteSection", () => {
  const messages = getWorkStateDetailMessages("en");
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
      <WorkStateTopologyDeleteSection
        messages={messages}
        placeId="story:queued"
        stateName="queued"
        workTypeName="story"
      />,
    );

    const deleteButton = screen.getByRole("button", {
      name: "Delete story queued work state",
    });
    expect(deleteButton).toBeTruthy();

    await user.click(deleteButton);

    expect(requestNodeRemoval).toHaveBeenCalledWith("work-state:story:queued");
  });

  it("does not render outside factory-graph editor mode", () => {
    useFactoryGraphTopologyEditorBridge.setState({ handlers: null });

    render(
      <WorkStateTopologyDeleteSection
        messages={messages}
        placeId="story:queued"
        stateName="queued"
        workTypeName="story"
      />,
    );

    expect(
      screen.queryByRole("button", { name: "Delete story queued work state" }),
    ).toBeNull();
  });

  it("surfaces blocked removal reasons from the graph editor bridge", () => {
    useFactoryGraphTopologyEditorBridge.setState({
      handlers: {
        blockedRemovalReason:
          "This work state is still referenced by a workstation route.",
        canInteractWithEditor: true,
        editorMode: true,
        requestNodeRemoval,
      },
    });

    render(
      <WorkStateTopologyDeleteSection
        messages={messages}
        placeId="essay:draft"
        stateName="draft"
        workTypeName="essay"
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Work state cannot be deleted:",
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "still referenced by a workstation route",
    );
  });
});
