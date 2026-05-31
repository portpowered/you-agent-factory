import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useFactoryGraphTopologyEditorBridge } from "../../../workflow-activity/state/factory-graph-topology-editor-bridge";
import { WorkTypeDetailCard } from "./work-type-detail-card";

describe("WorkTypeDetailCard", () => {
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

  it("shows the selected work type name and topology delete control", () => {
    render(<WorkTypeDetailCard workTypeName="story" />);

    expect(screen.getByText("story")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Delete story work type" }),
    ).toBeTruthy();
  });
});
