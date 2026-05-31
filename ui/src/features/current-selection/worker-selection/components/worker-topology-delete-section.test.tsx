import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useFactoryGraphTopologyEditorBridge } from "../../../workflow-activity/state/factory-graph-topology-editor-bridge";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import { WorkerTopologyDeleteSection } from "./worker-topology-delete-section";

describe("WorkerTopologyDeleteSection", () => {
  const messages = getWorkerDetailMessages("en");
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

    render(<WorkerTopologyDeleteSection messages={messages} workerName="editor" />);

    const deleteButton = screen.getByRole("button", {
      name: "Delete editor worker",
    });
    expect(deleteButton).toBeTruthy();

    await user.click(deleteButton);

    expect(requestNodeRemoval).toHaveBeenCalledWith("worker:editor");
  });

  it("does not render outside factory-graph editor mode", () => {
    useFactoryGraphTopologyEditorBridge.setState({ handlers: null });

    render(<WorkerTopologyDeleteSection messages={messages} workerName="editor" />);

    expect(
      screen.queryByRole("button", { name: "Delete editor worker" }),
    ).toBeNull();
  });

  it("surfaces blocked removal reasons from the graph editor bridge", () => {
    useFactoryGraphTopologyEditorBridge.setState({
      handlers: {
        blockedRemovalReason:
          "This worker is still assigned to 1 workstation. Reassign or remove those workstations before deleting writer.",
        canInteractWithEditor: true,
        editorMode: true,
        requestNodeRemoval,
      },
    });

    render(<WorkerTopologyDeleteSection messages={messages} workerName="writer" />);

    expect(screen.getByRole("alert").textContent).toContain(
      "Worker cannot be deleted:",
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "still assigned to 1 workstation",
    );
  });
});
